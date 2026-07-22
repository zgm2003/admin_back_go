package sms

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
)

const (
	safeProviderFailureMessage  = "短信发送失败"
	verificationFinalizeTimeout = 5 * time.Second
)

var (
	verificationCodePattern = regexp.MustCompile(`^[0-9]{6}$`)
	providerOpaqueIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
)

var providerOpaqueIDMarkers = [...]string{"payload", "templateparamset", "secretid", "secretkey"}

type verificationDeliveryInput struct {
	TemplateScene    string
	LogScene         string
	ToPhone          string
	Code             string
	TTL              time.Duration
	RecordTestResult bool
}

func (s *Service) SendVerifyCode(ctx context.Context, scene, toPhone, code string, ttl time.Duration) *apperror.Error {
	return s.sendVerificationCode(ctx, verificationDeliveryInput{
		TemplateScene: scene,
		LogScene:      scene,
		ToPhone:       toPhone,
		Code:          code,
		TTL:           ttl,
	})
}

func (s *Service) sendVerificationCode(ctx context.Context, input verificationDeliveryInput) *apperror.Error {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	sender, appErr := s.requireSender()
	if appErr != nil {
		return appErr
	}

	input.TemplateScene = strings.TrimSpace(input.TemplateScene)
	input.LogScene = strings.TrimSpace(input.LogScene)
	if !enum.IsSmsTemplateScene(input.TemplateScene) {
		return badRequest("sms.scene.invalid", "无效的短信模板场景")
	}
	if !enum.IsSmsLogScene(input.LogScene) {
		return badRequest("sms.log.scene.invalid", "无效的短信日志场景")
	}
	phone, appErr := normalizePhone(input.ToPhone)
	if appErr != nil {
		return appErr
	}
	if !verificationCodePattern.MatchString(input.Code) {
		return badRequest("sms.verify_code.invalid", "短信验证码必须是六位数字")
	}
	if input.TTL < 0 || (input.TTL == 0 && !input.RecordTestResult) {
		return badRequest("auth.verify_code.ttl.out_of_range", "验证码有效期必须在 1-60 分钟之间")
	}

	cfg, appErr := s.enabledConfig(ctx, repo)
	if appErr != nil {
		s.recordTestFailure(ctx, repo, input.RecordTestResult, appErr)
		return appErr
	}
	tmpl, appErr := enabledTemplate(ctx, repo, input.TemplateScene)
	if appErr != nil {
		s.recordTestFailure(ctx, repo, input.RecordTestResult, appErr)
		return appErr
	}

	ttlMinutes := 0
	if input.TTL == 0 {
		ttlMinutes, appErr = verifyCodeTTLMinutesFromConfig(cfg)
	} else {
		ttlMinutes, appErr = verifyCodeTTLMinutesFromDuration(input.TTL)
	}
	if appErr != nil {
		s.recordTestFailure(ctx, repo, input.RecordTestResult, appErr)
		return appErr
	}
	params, appErr := templateParamsFromRow(*tmpl, map[string]string{
		"code":        input.Code,
		"ttl_minutes": strconv.Itoa(ttlMinutes),
	})
	if appErr != nil {
		s.recordTestFailure(ctx, repo, input.RecordTestResult, appErr)
		return appErr
	}
	secretID, secretKey, appErr := s.decryptCredentials(*cfg)
	if appErr != nil {
		s.recordTestFailure(ctx, repo, input.RecordTestResult, appErr)
		return appErr
	}

	logID, err := repo.CreateLog(ctx, Log{
		Scene:      input.LogScene,
		TemplateID: &tmpl.ID,
		ToPhone:    phone,
		Status:     enum.SmsLogStatusPending,
		IsDel:      enum.CommonNo,
	})
	if err != nil {
		return wrapInternal("sms.log.create_failed", "创建短信发送日志失败", err)
	}

	started := time.Now()
	result, err := sender.Send(ctx, SendInput{
		SecretID: secretID, SecretKey: secretKey, Region: cfg.Region, Endpoint: cfg.Endpoint,
		SmsSdkAppID: cfg.SmsSdkAppID, SignName: cfg.SignName, TemplateID: tmpl.TencentTemplateID,
		PhoneNumber: phone, TemplateParamSet: params,
	})
	result = safeProviderResult(result, input.Code, secretID, secretKey)
	finalizeCtx, cancelFinalize := verificationFinalizeContext(ctx)
	defer cancelFinalize()
	duration := uint64(time.Since(started).Milliseconds())
	finishedAt := time.Now()
	if err != nil {
		errorCode, message := safeProviderFailure(err)
		finish := LogFinish{
			Status: enum.SmsLogStatusFailed, RequestID: result.RequestID, SerialNo: result.SerialNo,
			Fee: result.Fee, ErrorCode: errorCode, ErrorMessage: message, DurationMS: duration,
		}
		if finishErr := repo.FinishLog(finalizeCtx, logID, finish); finishErr != nil {
			return wrapInternal("sms.log.finish_failed", "更新短信发送日志失败", finishErr)
		}
		if input.RecordTestResult {
			_ = repo.UpdateConfigTestResult(finalizeCtx, &finishedAt, message)
		}
		return wrapInternal("sms.send.failed", safeProviderFailureMessage, errors.New(message))
	}

	finish := LogFinish{
		Status: enum.SmsLogStatusSuccess, RequestID: result.RequestID, SerialNo: result.SerialNo,
		Fee: result.Fee, DurationMS: duration, SentAt: &finishedAt,
	}
	if err := repo.FinishLog(finalizeCtx, logID, finish); err != nil {
		return wrapInternal("sms.log.finish_failed", "更新短信发送日志失败", err)
	}
	if input.RecordTestResult {
		if err := repo.UpdateConfigTestResult(finalizeCtx, &finishedAt, ""); err != nil {
			return wrapInternal("sms.config.test_result_failed", "更新短信测试结果失败", err)
		}
	}
	return nil
}

func safeProviderResult(result SendResult, code, secretID, secretKey string) SendResult {
	sensitive := []string{code, secretID, secretKey}
	result.RequestID = safeProviderOpaqueID(result.RequestID, sensitive)
	result.SerialNo = safeProviderOpaqueID(result.SerialNo, sensitive)
	return result
}

func safeProviderOpaqueID(value string, sensitive []string) string {
	if !providerOpaqueIDPattern.MatchString(value) {
		return ""
	}
	canonical := canonicalOpaqueID(value)
	for _, marker := range providerOpaqueIDMarkers {
		if strings.Contains(canonical, marker) {
			return ""
		}
	}
	for _, item := range sensitive {
		item = canonicalOpaqueID(item)
		if item != "" && strings.Contains(canonical, item) {
			return ""
		}
	}
	return value
}

func canonicalOpaqueID(value string) string {
	var canonical strings.Builder
	canonical.Grow(len(value))
	for i := 0; i < len(value); i++ {
		char := value[i]
		switch {
		case char >= 'A' && char <= 'Z':
			canonical.WriteByte(char + ('a' - 'A'))
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9':
			canonical.WriteByte(char)
		}
	}
	return canonical.String()
}

func verificationFinalizeContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(ctx), verificationFinalizeTimeout)
}

func safeProviderFailure(err error) (string, string) {
	if err == nil {
		return "", safeProviderFailureMessage
	}

	errorCode := senderErrorCode(err)
	// Raw provider messages are untrusted; only code-owned catalog entries may be persisted.
	switch errorCode {
	case "FailedOperation.TemplateIncorrect":
		return errorCode, "template incorrect"
	default:
		return "", safeProviderFailureMessage
	}
}

func verifyCodeTTLMinutesFromDuration(ttl time.Duration) (int, *apperror.Error) {
	if ttl < time.Minute || ttl > maxVerifyCodeTTLMinutes*time.Minute {
		return 0, badRequest("auth.verify_code.ttl.out_of_range", "验证码有效期必须在 1-60 分钟之间")
	}
	minutes := int((ttl + time.Minute - 1) / time.Minute)
	return normalizeVerifyCodeTTLMinutes(minutes)
}

func (s *Service) recordTestFailure(ctx context.Context, repo Repository, enabled bool, appErr *apperror.Error) {
	if enabled {
		_ = repo.UpdateConfigTestResult(ctx, timePtr(time.Now()), appErr.Message)
	}
}

package sms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/module/systemsetting"
	"admin_back_go/internal/platform/secretbox"
)

const (
	timeLayout              = "2006-01-02 15:04:05"
	defaultPage             = 1
	defaultPageSize         = 20
	maxTemplateVarLen       = 64
	verifyCodeTTLSettingKey = "auth.verify_code.ttl_minutes"
	defaultVerifyCodeTTLMin = 5
	minVerifyCodeTTLMin     = 1
	maxVerifyCodeTTLMin     = 60
	testCode                = "123456"
)

var phonePattern = regexp.MustCompile(`^1[3-9][0-9]{9}$`)

type Sender interface {
	Send(ctx context.Context, input SendInput) (SendResult, error)
}

type SenderFunc func(ctx context.Context, input SendInput) (SendResult, error)

func (f SenderFunc) Send(ctx context.Context, input SendInput) (SendResult, error) {
	return f(ctx, input)
}

type codedError interface {
	ErrorCode() string
}

type Service struct {
	repository Repository
	secretBox  secretbox.Box
	sender     Sender
}

func NewService(repository Repository, secretBox secretbox.Box, sender Sender) *Service {
	return &Service{repository: repository, secretBox: secretBox, sender: sender}
}

func (s *Service) PageInit(ctx context.Context) (*PageInitResponse, *apperror.Error) {
	return &PageInitResponse{Dict: PageInitDict{
		CommonStatusArr:   dict.CommonStatusOptions(),
		SmsSceneArr:       dict.SmsSceneOptions(),
		SmsLogSceneArr:    dict.SmsLogSceneOptions(),
		SmsLogStatusArr:   dict.SmsLogStatusOptions(),
		SmsRegionArr:      dict.SmsRegionOptions(),
		DefaultRegion:     DefaultRegion,
		DefaultEndpoint:   DefaultEndpoint,
		DefaultTTLMinutes: defaultVerifyCodeTTLMin,
	}}, nil
}

func (s *Service) Config(ctx context.Context) (*ConfigResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, err := repo.DefaultConfig(ctx)
	if err != nil {
		return nil, wrapInternal("sms.config.query_failed", "查询短信配置失败", err)
	}
	ttl, appErr := s.configuredVerifyCodeTTL(ctx, repo)
	if appErr != nil {
		return nil, appErr
	}
	if row == nil {
		return defaultConfigResponse(ttl), nil
	}
	return configResponseFromRow(*row, ttl), nil
}

func (s *Service) SaveConfig(ctx context.Context, input SaveConfigInput) *apperror.Error {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	input, appErr = normalizeConfigInput(input)
	if appErr != nil {
		return appErr
	}
	ttl, appErr := normalizeVerifyCodeTTLMinutes(input.VerifyCodeTTLMinutes)
	if appErr != nil {
		return appErr
	}
	existing, err := repo.DefaultConfig(ctx)
	if err != nil {
		return wrapInternal("sms.config.query_failed", "查询短信配置失败", err)
	}
	row, appErr := s.configRowFromInput(existing, input)
	if appErr != nil {
		return appErr
	}
	if err := repo.SaveDefaultConfig(ctx, row); err != nil {
		return wrapInternal("sms.config.save_failed", "保存短信配置失败", err)
	}
	if err := repo.SaveSetting(ctx, systemsetting.Setting{
		SettingKey:   verifyCodeTTLSettingKey,
		SettingValue: strconv.Itoa(ttl),
		ValueType:    enum.SystemSettingValueNumber,
		Remark:       "验证码有效期分钟数，邮件和短信共用",
		Status:       enum.CommonYes,
		IsDel:        enum.CommonNo,
	}); err != nil {
		return wrapInternal("sms.ttl.save_failed", "保存验证码有效期配置失败", err)
	}
	if err := repo.InvalidateSettingCache(ctx, verifyCodeTTLSettingKey); err != nil {
		return wrapInternal("sms.ttl.cache_clear_failed", "清理验证码有效期配置缓存失败", err)
	}
	return nil
}

func (s *Service) DeleteConfig(ctx context.Context) *apperror.Error {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if err := repo.SoftDeleteDefaultConfig(ctx); err != nil {
		return wrapInternal("sms.config.delete_failed", "删除短信配置失败", err)
	}
	return nil
}

func (s *Service) TestSend(ctx context.Context, input TestInput) *apperror.Error {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	sender, appErr := s.requireSender()
	if appErr != nil {
		return appErr
	}
	input.TemplateScene = strings.TrimSpace(input.TemplateScene)
	if !enum.IsSmsTemplateScene(input.TemplateScene) {
		return badRequest("sms.scene.invalid", "无效的短信模板场景")
	}
	phone, appErr := normalizePhone(input.ToPhone)
	if appErr != nil {
		return appErr
	}
	cfg, appErr := s.enabledConfig(ctx, repo)
	if appErr != nil {
		_ = repo.UpdateConfigTestResult(ctx, timePtr(time.Now()), appErr.Message)
		return appErr
	}
	tmpl, appErr := enabledTemplate(ctx, repo, input.TemplateScene)
	if appErr != nil {
		_ = repo.UpdateConfigTestResult(ctx, timePtr(time.Now()), appErr.Message)
		return appErr
	}
	ttl, appErr := s.configuredVerifyCodeTTL(ctx, repo)
	if appErr != nil {
		_ = repo.UpdateConfigTestResult(ctx, timePtr(time.Now()), appErr.Message)
		return appErr
	}
	params, appErr := templateParamsFromRow(*tmpl, map[string]string{"code": testCode, "ttl_minutes": strconv.Itoa(ttl)})
	if appErr != nil {
		_ = repo.UpdateConfigTestResult(ctx, timePtr(time.Now()), appErr.Message)
		return appErr
	}
	secretID, secretKey, appErr := s.decryptCredentials(*cfg)
	if appErr != nil {
		_ = repo.UpdateConfigTestResult(ctx, timePtr(time.Now()), appErr.Message)
		return appErr
	}
	logID, err := repo.CreateLog(ctx, Log{Scene: enum.SmsSceneTest, TemplateID: &tmpl.ID, ToPhone: phone, Status: enum.SmsLogStatusPending, IsDel: enum.CommonNo})
	if err != nil {
		return wrapInternal("sms.log.create_failed", "创建短信发送日志失败", err)
	}
	started := time.Now()
	result, err := sender.Send(ctx, SendInput{
		SecretID: secretID, SecretKey: secretKey, Region: cfg.Region, Endpoint: cfg.Endpoint,
		SmsSdkAppID: cfg.SmsSdkAppID, SignName: cfg.SignName, TemplateID: tmpl.TencentTemplateID,
		PhoneNumber: phone, TemplateParamSet: params,
	})
	duration := uint64(time.Since(started).Milliseconds())
	finishedAt := time.Now()
	if err != nil {
		message := err.Error()
		finish := LogFinish{Status: enum.SmsLogStatusFailed, RequestID: result.RequestID, SerialNo: result.SerialNo, Fee: result.Fee, ErrorCode: senderErrorCode(err), ErrorMessage: message, DurationMS: duration}
		if finishErr := repo.FinishLog(ctx, logID, finish); finishErr != nil {
			return wrapInternal("sms.log.finish_failed", "更新短信发送日志失败", finishErr)
		}
		_ = repo.UpdateConfigTestResult(ctx, &finishedAt, message)
		return wrapInternal("sms.send.failed", "短信发送失败", err)
	}
	finish := LogFinish{Status: enum.SmsLogStatusSuccess, RequestID: result.RequestID, SerialNo: result.SerialNo, Fee: result.Fee, DurationMS: duration, SentAt: &finishedAt}
	if err := repo.FinishLog(ctx, logID, finish); err != nil {
		return wrapInternal("sms.log.finish_failed", "更新短信发送日志失败", err)
	}
	if err := repo.UpdateConfigTestResult(ctx, &finishedAt, ""); err != nil {
		return wrapInternal("sms.config.test_result_failed", "更新短信测试结果失败", err)
	}
	return nil
}

func (s *Service) Templates(ctx context.Context) ([]TemplateDTO, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	rows, err := repo.ListTemplates(ctx)
	if err != nil {
		return nil, wrapInternal("sms.template.query_failed", "查询短信模板失败", err)
	}
	result := make([]TemplateDTO, 0, len(rows))
	for _, row := range rows {
		dto, err := templateDTOFromRow(row)
		if err != nil {
			return nil, wrapInternal("sms.template.parse_failed", "解析短信模板变量失败", err)
		}
		result = append(result, dto)
	}
	return result, nil
}

func (s *Service) CreateTemplate(ctx context.Context, input SaveTemplateInput) (uint64, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return 0, appErr
	}
	row, appErr := templateRowFromInput(input)
	if appErr != nil {
		return 0, appErr
	}
	id, err := repo.SaveTemplate(ctx, row)
	if err != nil {
		return 0, wrapInternal("sms.template.save_failed", "保存短信模板失败", err)
	}
	return id, nil
}

func (s *Service) UpdateTemplate(ctx context.Context, id uint64, input SaveTemplateInput) *apperror.Error {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if appErr := ensureTemplateExists(ctx, repo, id); appErr != nil {
		return appErr
	}
	row, appErr := templateRowFromInput(input)
	if appErr != nil {
		return appErr
	}
	if err := repo.UpdateTemplate(ctx, id, TemplateUpdate{Scene: row.Scene, Name: row.Name, TencentTemplateID: row.TencentTemplateID, VariablesJSON: row.VariablesJSON, SampleVariablesJSON: row.SampleVariablesJSON, Status: row.Status}); err != nil {
		return wrapInternal("sms.template.update_failed", "更新短信模板失败", err)
	}
	return nil
}

func (s *Service) ChangeTemplateStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if !enum.IsCommonStatus(status) {
		return badRequest("sms.status.invalid", "无效的状态")
	}
	if appErr := ensureTemplateExists(ctx, repo, id); appErr != nil {
		return appErr
	}
	row, err := repo.TemplateByID(ctx, id)
	if err != nil {
		return wrapInternal("sms.template.query_failed", "查询短信模板失败", err)
	}
	if row == nil {
		return notFound("sms.template.not_found", "短信模板不存在")
	}
	if err := repo.UpdateTemplate(ctx, id, TemplateUpdate{Scene: row.Scene, Name: row.Name, TencentTemplateID: row.TencentTemplateID, VariablesJSON: row.VariablesJSON, SampleVariablesJSON: row.SampleVariablesJSON, Status: status}); err != nil {
		return wrapInternal("sms.template.status_failed", "修改短信模板状态失败", err)
	}
	return nil
}

func (s *Service) DeleteTemplate(ctx context.Context, id uint64) *apperror.Error {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if appErr := ensureTemplateExists(ctx, repo, id); appErr != nil {
		return appErr
	}
	if err := repo.SoftDeleteTemplate(ctx, id); err != nil {
		return wrapInternal("sms.template.delete_failed", "删除短信模板失败", err)
	}
	return nil
}

func (s *Service) Logs(ctx context.Context, query LogQuery) (*LogListResponse, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	query, appErr = normalizeLogQuery(query)
	if appErr != nil {
		return nil, appErr
	}
	rows, total, err := repo.ListLogs(ctx, query)
	if err != nil {
		return nil, wrapInternal("sms.log.query_failed", "查询短信日志失败", err)
	}
	list := make([]LogDTO, 0, len(rows))
	for _, row := range rows {
		list = append(list, logDTOFromRow(row))
	}
	return &LogListResponse{List: list, Page: Page{PageSize: query.PageSize, CurrentPage: query.CurrentPage, TotalPage: totalPage(total, query.PageSize), Total: total}}, nil
}

func (s *Service) Log(ctx context.Context, id uint64) (*LogDTO, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return nil, appErr
	}
	row, err := repo.LogByID(ctx, id)
	if err != nil {
		return nil, wrapInternal("sms.log.query_failed", "查询短信日志失败", err)
	}
	if row == nil {
		return nil, notFound("sms.log.not_found", "短信日志不存在")
	}
	dto := logDTOFromRow(*row)
	if row.TemplateID != nil {
		tmpl, err := repo.TemplateByID(ctx, *row.TemplateID)
		if err != nil {
			return nil, wrapInternal("sms.template.query_failed", "查询短信模板失败", err)
		}
		if tmpl != nil {
			logTmpl, appErr := logTemplateDTOFromRow(*tmpl)
			if appErr != nil {
				return nil, appErr
			}
			dto.Template = logTmpl
		}
	}
	return &dto, nil
}

func (s *Service) DeleteLogs(ctx context.Context, ids []uint64) *apperror.Error {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return appErr
	}
	if len(normalizeUint64IDs(ids)) == 0 {
		return badRequest("sms.log.delete.required", "请选择要删除的短信日志")
	}
	if err := repo.SoftDeleteLogs(ctx, ids); err != nil {
		return wrapInternal("sms.log.delete_failed", "删除短信日志失败", err)
	}
	return nil
}

func (s *Service) enabledConfig(ctx context.Context, repo Repository) (*Config, *apperror.Error) {
	cfg, err := repo.DefaultConfig(ctx)
	if err != nil {
		return nil, wrapInternal("sms.config.query_failed", "查询短信配置失败", err)
	}
	if cfg == nil {
		return nil, internalError("sms.service_unconfigured", "短信服务未配置")
	}
	if cfg.Status != enum.CommonYes {
		return nil, badRequest("sms.service_disabled", "短信服务已禁用")
	}
	return cfg, nil
}

func (s *Service) configuredVerifyCodeTTL(ctx context.Context, repo Repository) (int, *apperror.Error) {
	row, err := repo.SettingByKey(ctx, verifyCodeTTLSettingKey)
	if err != nil {
		return 0, wrapInternal("sms.ttl.query_failed", "查询验证码有效期配置失败", err)
	}
	if row == nil || row.IsDel != enum.CommonNo || row.Status != enum.CommonYes || strings.TrimSpace(row.SettingValue) == "" {
		return defaultVerifyCodeTTLMin, nil
	}
	ttl, err := strconv.Atoi(strings.TrimSpace(row.SettingValue))
	if err != nil {
		return 0, badRequest("sms.ttl.integer_required", "验证码有效期必须为整数分钟")
	}
	return normalizeVerifyCodeTTLMinutes(ttl)
}

func enabledTemplate(ctx context.Context, repo Repository, scene string) (*Template, *apperror.Error) {
	tmpl, err := repo.TemplateByScene(ctx, scene)
	if err != nil {
		return nil, wrapInternal("sms.template.query_failed", "查询短信模板失败", err)
	}
	if tmpl == nil {
		return nil, badRequest("sms.template.unconfigured", "短信模板未配置")
	}
	if tmpl.Status != enum.CommonYes {
		return nil, badRequest("sms.template.disabled", "短信模板已禁用")
	}
	return tmpl, nil
}

func (s *Service) decryptCredentials(cfg Config) (string, string, *apperror.Error) {
	secretID, err := s.secretBox.Decrypt(cfg.SecretIDEnc)
	if err != nil {
		return "", "", wrapInternal("sms.secret_id.decrypt_failed", "解密 Tencent SecretId 失败", err)
	}
	secretKey, err := s.secretBox.Decrypt(cfg.SecretKeyEnc)
	if err != nil {
		return "", "", wrapInternal("sms.secret_key.decrypt_failed", "解密 Tencent SecretKey 失败", err)
	}
	if secretID == "" || secretKey == "" {
		return "", "", internalError("sms.tencent_secret_missing", "腾讯云短信密钥未配置")
	}
	return secretID, secretKey, nil
}

func (s *Service) configRowFromInput(existing *Config, input SaveConfigInput) (Config, *apperror.Error) {
	secretIDEnc, secretIDHint, appErr := s.secretValue(existing, input.SecretID, true)
	if appErr != nil {
		return Config{}, appErr
	}
	secretKeyEnc, secretKeyHint, appErr := s.secretValue(existing, input.SecretKey, false)
	if appErr != nil {
		return Config{}, appErr
	}
	return Config{ConfigKey: defaultConfigKey, SecretIDEnc: secretIDEnc, SecretIDHint: secretIDHint, SecretKeyEnc: secretKeyEnc, SecretKeyHint: secretKeyHint, SmsSdkAppID: input.SmsSdkAppID, SignName: input.SignName, Region: input.Region, Endpoint: input.Endpoint, Status: input.Status, IsDel: enum.CommonNo}, nil
}

func (s *Service) secretValue(existing *Config, plain string, secretID bool) (string, string, *apperror.Error) {
	if plain != "" {
		enc, err := s.secretBox.Encrypt(plain)
		if err != nil {
			return "", "", wrapInternal("sms.secret.encrypt_failed", "加密短信密钥失败", err)
		}
		return enc, secretbox.Hint(plain), nil
	}
	if existing == nil {
		return "", "", badRequest("sms.secret.required", "首次配置必须填写腾讯云 SecretId 和 SecretKey")
	}
	if secretID {
		return existing.SecretIDEnc, existing.SecretIDHint, nil
	}
	return existing.SecretKeyEnc, existing.SecretKeyHint, nil
}

func (s *Service) requireRepository() (Repository, *apperror.Error) {
	if s == nil || s.repository == nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, http.StatusInternalServerError, "sms.repository_missing", nil, "短信仓储未配置", ErrRepositoryNotConfigured)
	}
	return s.repository, nil
}

func (s *Service) requireSender() (Sender, *apperror.Error) {
	if s == nil || s.sender == nil {
		return nil, apperror.WrapKey(apperror.CodeInternal, http.StatusInternalServerError, "sms.sender_missing", nil, "短信发送器未配置", ErrSenderNotConfigured)
	}
	return s.sender, nil
}

func normalizeConfigInput(input SaveConfigInput) (SaveConfigInput, *apperror.Error) {
	input.SecretID = strings.TrimSpace(input.SecretID)
	input.SecretKey = strings.TrimSpace(input.SecretKey)
	input.SmsSdkAppID = strings.TrimSpace(input.SmsSdkAppID)
	input.SignName = strings.TrimSpace(input.SignName)
	input.Region = strings.TrimSpace(input.Region)
	input.Endpoint = strings.TrimSpace(input.Endpoint)
	if input.Region == "" {
		input.Region = DefaultRegion
	}
	if input.Endpoint == "" {
		input.Endpoint = DefaultEndpoint
	}
	if input.SmsSdkAppID == "" {
		return input, badRequest("sms.app_id.required", "短信应用 ID 不能为空")
	}
	if input.SignName == "" {
		return input, badRequest("sms.sign_name.required", "短信签名不能为空")
	}
	if !dict.IsSmsRegion(input.Region) {
		return input, badRequest("sms.region.unsupported", "不支持的腾讯云短信地域")
	}
	if !enum.IsCommonStatus(input.Status) {
		return input, badRequest("sms.status.invalid", "无效的状态")
	}
	return input, nil
}

func normalizeVerifyCodeTTLMinutes(value int) (int, *apperror.Error) {
	if value < minVerifyCodeTTLMin || value > maxVerifyCodeTTLMin {
		return 0, badRequest("sms.ttl.out_of_range", "验证码有效期必须在 1-60 分钟之间")
	}
	return value, nil
}

func templateRowFromInput(input SaveTemplateInput) (Template, *apperror.Error) {
	input.Scene = strings.TrimSpace(input.Scene)
	input.Name = strings.TrimSpace(input.Name)
	input.TencentTemplateID = strings.TrimSpace(input.TencentTemplateID)
	if !enum.IsSmsTemplateScene(input.Scene) {
		return Template{}, badRequest("sms.scene.invalid", "无效的短信模板场景")
	}
	if input.Name == "" {
		return Template{}, badRequest("sms.template.name.required", "模板名称不能为空")
	}
	if input.TencentTemplateID == "" {
		return Template{}, badRequest("sms.template_id.required", "腾讯云模板 ID 不能为空")
	}
	if !enum.IsCommonStatus(input.Status) {
		return Template{}, badRequest("sms.status.invalid", "无效的状态")
	}
	if appErr := ensureVerifyCodeTemplateVariables(input.Variables, input.SampleVariables); appErr != nil {
		return Template{}, appErr
	}
	variablesJSON, appErr := encodeVariables(input.Variables)
	if appErr != nil {
		return Template{}, appErr
	}
	sampleJSON, appErr := encodeSampleVariables(input.SampleVariables, input.Variables)
	if appErr != nil {
		return Template{}, appErr
	}
	return Template{Scene: input.Scene, Name: input.Name, TencentTemplateID: input.TencentTemplateID, VariablesJSON: variablesJSON, SampleVariablesJSON: sampleJSON, Status: input.Status, IsDel: enum.CommonNo}, nil
}

func encodeVariables(values []string) (string, *apperror.Error) {
	normalized, appErr := normalizeVariables(values)
	if appErr != nil {
		return "", appErr
	}
	body, err := json.Marshal(normalized)
	if err != nil {
		return "", badRequest("sms.template.variables.invalid", "模板变量格式错误")
	}
	return string(body), nil
}

func decodeVariables(raw string) ([]string, error) {
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	return normalizeVariablesAsError(values)
}

func encodeSampleVariables(values map[string]string, variables []string) (string, *apperror.Error) {
	normalizedVars, appErr := normalizeVariables(variables)
	if appErr != nil {
		return "", appErr
	}
	normalized := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			return "", badRequest("sms.template.sample_key.required", "测试变量名不能为空")
		}
		normalized[key] = strings.TrimSpace(value)
	}
	for _, key := range normalizedVars {
		if _, ok := normalized[key]; !ok {
			return "", badRequestWithField("sms.template.sample_missing", "测试变量缺少 "+key, key)
		}
	}
	body, err := json.Marshal(normalized)
	if err != nil {
		return "", badRequest("sms.template.sample.invalid", "测试变量格式错误")
	}
	return string(body), nil
}

func decodeSampleVariables(raw string) (map[string]string, error) {
	var values map[string]string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	if values == nil {
		return map[string]string{}, nil
	}
	return values, nil
}

func ensureVerifyCodeTemplateVariables(variables []string, sample map[string]string) *apperror.Error {
	normalized, appErr := normalizeVariables(variables)
	if appErr != nil {
		return appErr
	}
	if len(normalized) != 2 || normalized[0] != "code" || normalized[1] != "ttl_minutes" {
		return badRequest("sms.template.variables.must_be_verify_code", "验证码模板变量必须且只能是 code、ttl_minutes")
	}
	if len(sample) != 2 {
		return badRequest("sms.template.sample.must_be_verify_code", "验证码模板测试变量必须且只能是 code、ttl_minutes")
	}
	for _, key := range normalized {
		if _, ok := sample[key]; !ok {
			return badRequestWithField("sms.template.sample_missing", "测试变量缺少 "+key, key)
		}
	}
	for key := range sample {
		if key != "code" && key != "ttl_minutes" {
			return badRequest("sms.template.sample.must_be_verify_code", "验证码模板测试变量必须且只能是 code、ttl_minutes")
		}
	}
	return nil
}

func normalizeVariables(values []string) ([]string, *apperror.Error) {
	result, err := normalizeVariablesAsError(values)
	if err != nil {
		return nil, badRequest("sms.template.variables.invalid", err.Error())
	}
	return result, nil
}

func normalizeVariablesAsError(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("模板变量不能为空")
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, fmt.Errorf("模板变量名不能为空")
		}
		if len([]rune(value)) > maxTemplateVarLen {
			return nil, fmt.Errorf("模板变量名过长")
		}
		if _, ok := seen[value]; ok {
			return nil, fmt.Errorf("模板变量名重复: %s", value)
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func templateParamsFromRow(row Template, data map[string]string) ([]string, *apperror.Error) {
	variables, err := decodeVariables(row.VariablesJSON)
	if err != nil {
		return nil, wrapInternal("sms.template.parse_failed", "解析短信模板变量失败", err)
	}
	allowed := make(map[string]struct{}, len(variables))
	params := make([]string, 0, len(variables))
	for _, key := range variables {
		allowed[key] = struct{}{}
		value, ok := data[key]
		if !ok {
			return nil, internalError("sms.template.param_missing", "短信模板变量缺少 "+key)
		}
		params = append(params, value)
	}
	for key := range data {
		if _, ok := allowed[key]; !ok {
			return nil, internalError("sms.template.param_extra", "短信模板变量多余 "+key)
		}
	}
	return params, nil
}

func normalizeLogQuery(query LogQuery) (LogQuery, *apperror.Error) {
	if query.CurrentPage <= 0 {
		query.CurrentPage = defaultPage
	}
	if query.PageSize <= 0 {
		query.PageSize = defaultPageSize
	}
	if query.PageSize > enum.PageSizeMax {
		query.PageSize = enum.PageSizeMax
	}
	query.Scene = strings.TrimSpace(query.Scene)
	query.ToPhone = strings.TrimSpace(query.ToPhone)
	if query.Scene != "" && !enum.IsSmsLogScene(query.Scene) {
		return query, badRequest("sms.log.scene.invalid", "无效的短信日志场景")
	}
	if query.Status != nil && !enum.IsSmsLogStatus(*query.Status) {
		return query, badRequest("sms.log.status.invalid", "无效的短信日志状态")
	}
	return query, nil
}

func ensureTemplateExists(ctx context.Context, repo Repository, id uint64) *apperror.Error {
	row, err := repo.TemplateByID(ctx, id)
	if err != nil {
		return wrapInternal("sms.template.query_failed", "查询短信模板失败", err)
	}
	if row == nil {
		return notFound("sms.template.not_found", "短信模板不存在")
	}
	return nil
}

func templateDTOFromRow(row Template) (TemplateDTO, error) {
	variables, err := decodeVariables(row.VariablesJSON)
	if err != nil {
		return TemplateDTO{}, err
	}
	sample, err := decodeSampleVariables(row.SampleVariablesJSON)
	if err != nil {
		return TemplateDTO{}, err
	}
	return TemplateDTO{ID: row.ID, Scene: row.Scene, Name: row.Name, TencentTemplateID: row.TencentTemplateID, Variables: variables, SampleVariables: sample, Status: row.Status, CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt)}, nil
}

func logDTOFromRow(row Log) LogDTO {
	return LogDTO{ID: row.ID, Scene: row.Scene, TemplateID: row.TemplateID, ToPhone: row.ToPhone, Status: row.Status, TencentRequestID: row.TencentRequestID, TencentSerialNo: row.TencentSerialNo, TencentFee: row.TencentFee, ErrorCode: row.ErrorCode, ErrorMessage: row.ErrorMessage, DurationMS: row.DurationMS, SentAt: formatOptionalTime(row.SentAt), CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt)}
}

func configResponseFromRow(row Config, ttlMinutes int) *ConfigResponse {
	id := row.ID
	return &ConfigResponse{ID: &id, Configured: true, SecretIDHint: row.SecretIDHint, SecretKeyHint: row.SecretKeyHint, SmsSdkAppID: row.SmsSdkAppID, SignName: row.SignName, Region: row.Region, Endpoint: row.Endpoint, Status: row.Status, VerifyCodeTTLMinutes: ttlMinutes, LastTestAt: formatOptionalTime(row.LastTestAt), LastTestError: row.LastTestError, CreatedAt: optionalTime(row.CreatedAt), UpdatedAt: optionalTime(row.UpdatedAt)}
}

func logTemplateDTOFromRow(row Template) (*LogTemplateDTO, *apperror.Error) {
	variables, err := decodeVariables(row.VariablesJSON)
	if err != nil {
		return nil, wrapInternal("sms.log.template_parse_failed", "解析短信日志模板变量失败", err)
	}
	return &LogTemplateDTO{ID: row.ID, Scene: row.Scene, Name: row.Name, TencentTemplateID: row.TencentTemplateID, Variables: variables, Status: row.Status}, nil
}

func defaultConfigResponse(ttlMinutes int) *ConfigResponse {
	return &ConfigResponse{Configured: false, Region: DefaultRegion, Endpoint: DefaultEndpoint, Status: enum.CommonNo, VerifyCodeTTLMinutes: ttlMinutes}
}

func normalizePhone(value string) (string, *apperror.Error) {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, " ", "")
	value = strings.ReplaceAll(value, "-", "")
	if strings.HasPrefix(value, "+86") {
		value = strings.TrimPrefix(value, "+86")
	} else if strings.HasPrefix(value, "86") && len(value) == 13 {
		value = strings.TrimPrefix(value, "86")
	}
	if !phonePattern.MatchString(value) {
		return "", badRequest("sms.phone.invalid", "手机号格式不正确")
	}
	return "+86" + value, nil
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(timeLayout)
}

func optionalTime(value time.Time) *string {
	if value.IsZero() {
		return nil
	}
	formatted := value.Format(timeLayout)
	return &formatted
}

func formatOptionalTime(value *time.Time) *string {
	if value == nil || value.IsZero() {
		return nil
	}
	formatted := value.Format(timeLayout)
	return &formatted
}

func parseTime(value string) (*time.Time, *apperror.Error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.ParseInLocation(timeLayout, value, time.Local)
	if err != nil {
		return nil, badRequest("sms.time.invalid", "时间格式必须为 YYYY-MM-DD HH:mm:ss")
	}
	return &parsed, nil
}

func totalPage(total int64, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}

func senderErrorCode(err error) string {
	var coded codedError
	if err != nil && errors.As(err, &coded) {
		return coded.ErrorCode()
	}
	return ""
}

func timePtr(value time.Time) *time.Time { return &value }

func badRequest(key string, fallback string) *apperror.Error {
	return apperror.BadRequestKey(key, nil, fallback)
}

func badRequestWithField(key string, fallback string, field string) *apperror.Error {
	return apperror.BadRequestKey(key, map[string]any{"field": field}, fallback)
}

func notFound(key string, fallback string) *apperror.Error {
	return apperror.NotFoundKey(key, nil, fallback)
}

func internalError(key string, fallback string) *apperror.Error {
	return apperror.InternalKey(key, nil, fallback)
}

func wrapInternal(key string, fallback string, err error) *apperror.Error {
	return apperror.WrapKey(apperror.CodeInternal, http.StatusInternalServerError, key, nil, fallback, err)
}

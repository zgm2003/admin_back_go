package setting

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"admin_back_go/internal/module/systemsetting"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
)

const (
	AuthCaptchaTTLKey    = "auth.captcha.ttl_minutes"
	AuthVerifyCodeTTLKey = "auth.verify_code.ttl_minutes"
	UploadTokenTTLKey    = "upload.token.ttl_minutes"

	DefaultAuthVerifyCodeTTLMinutes = 5
	DefaultUploadTokenTTL           = 15 * time.Minute

	minAuthCaptchaTTLMinutes    = 1
	minAuthVerifyCodeTTLMinutes = 1
	maxAuthVerifyCodeTTLMinutes = 60
	minUploadTokenTTLMinutes    = 1
	maxUploadTokenTTLMinutes    = 1440
)

type Reader interface {
	SettingByKey(ctx context.Context, key string) (*systemsetting.Setting, error)
}

type Writer interface {
	SaveSetting(ctx context.Context, row systemsetting.Setting) error
	InvalidateSettingCache(ctx context.Context, key string) error
}

func AuthCaptchaTTLMinutes(ctx context.Context, reader Reader) (int, *apperror.Error) {
	return requiredInt(ctx, reader, AuthCaptchaTTLKey, intRule{
		min: minAuthCaptchaTTLMinutes,
		missing: func(key string) *apperror.Error {
			return apperror.InternalKey("captcha.policy.missing", map[string]any{"key": key}, "验证码策略配置缺失")
		},
		disabled: func(key string) *apperror.Error {
			return apperror.BadRequestKey("captcha.policy.disabled", map[string]any{"key": key}, "验证码策略配置已禁用")
		},
		wrongType: func(key string) *apperror.Error {
			return apperror.InternalKey("captcha.policy.type_invalid", map[string]any{"key": key}, "验证码策略配置类型必须为数字")
		},
		notInteger: func(key string) *apperror.Error {
			return apperror.BadRequestKey("captcha.policy.integer_invalid", map[string]any{"key": key}, "验证码策略配置必须为整数")
		},
		outOfRange: func(key string) *apperror.Error {
			return apperror.BadRequestKey("captcha.policy.value_invalid", map[string]any{"key": key}, "验证码策略配置取值无效")
		},
		queryFailed: func(key string, err error) *apperror.Error {
			return apperror.WrapKey(apperror.CodeInternal, http.StatusInternalServerError, "captcha.policy.query_failed", map[string]any{"key": key}, "查询验证码策略配置失败", err)
		},
	})
}

func AuthVerifyCodeTTLMinutes(ctx context.Context, reader Reader) (int, *apperror.Error) {
	return requiredInt(ctx, reader, AuthVerifyCodeTTLKey, authVerifyCodeTTLRule())
}

func AuthVerifyCodeTTLMinutesOrDefault(ctx context.Context, reader Reader) (int, *apperror.Error) {
	return optionalInt(ctx, reader, AuthVerifyCodeTTLKey, DefaultAuthVerifyCodeTTLMinutes, authVerifyCodeTTLRule())
}

func authVerifyCodeTTLRule() intRule {
	return intRule{
		min: minAuthVerifyCodeTTLMinutes,
		max: maxAuthVerifyCodeTTLMinutes,
		missing: func(key string) *apperror.Error {
			return apperror.InternalKey("auth.verify_code.ttl.missing", map[string]any{"key": key}, "验证码有效期配置缺失")
		},
		disabled: func(key string) *apperror.Error {
			return apperror.BadRequestKey("auth.verify_code.ttl.disabled", map[string]any{"key": key}, "验证码有效期配置已禁用")
		},
		wrongType: func(key string) *apperror.Error {
			return apperror.InternalKey("auth.verify_code.ttl.type_invalid", map[string]any{"key": key}, "验证码有效期配置类型必须为数字")
		},
		notInteger: func(key string) *apperror.Error {
			return apperror.BadRequestKey("auth.verify_code.ttl.integer_required", map[string]any{"key": key}, "验证码有效期必须为整数分钟")
		},
		outOfRange: func(key string) *apperror.Error {
			return apperror.BadRequestKey("auth.verify_code.ttl.out_of_range", map[string]any{"key": key}, "验证码有效期必须在 1-60 分钟之间")
		},
		queryFailed: func(key string, err error) *apperror.Error {
			return apperror.WrapKey(apperror.CodeInternal, http.StatusInternalServerError, "auth.verify_code.ttl.query_failed", map[string]any{"key": key}, "查询验证码有效期配置失败", err)
		},
	}
}

func UploadTokenTTLMinutes(ctx context.Context, reader Reader) int {
	minutes, appErr := optionalInt(ctx, reader, UploadTokenTTLKey, int(DefaultUploadTokenTTL/time.Minute), intRule{
		min: minUploadTokenTTLMinutes,
		max: maxUploadTokenTTLMinutes,
	})
	if appErr != nil {
		return int(DefaultUploadTokenTTL / time.Minute)
	}
	return minutes
}

func SaveAuthVerifyCodeTTLMinutes(ctx context.Context, writer Writer, minutes int) *apperror.Error {
	if writer == nil {
		return apperror.InternalKey("setting.repository_missing", nil, "系统设置仓储未配置")
	}
	normalized, appErr := NormalizeAuthVerifyCodeTTLMinutes(minutes)
	if appErr != nil {
		return appErr
	}
	if err := writer.SaveSetting(ctx, systemsetting.Setting{
		SettingKey:   AuthVerifyCodeTTLKey,
		SettingValue: strconv.Itoa(normalized),
		ValueType:    enum.SystemSettingValueNumber,
		Remark:       "验证码有效期分钟数，邮件和短信共用",
		Status:       enum.CommonYes,
		IsDel:        enum.CommonNo,
	}); err != nil {
		return apperror.WrapKey(apperror.CodeInternal, http.StatusInternalServerError, "setting.save_failed", map[string]any{"key": AuthVerifyCodeTTLKey}, "保存系统设置失败", err)
	}
	if err := writer.InvalidateSettingCache(ctx, AuthVerifyCodeTTLKey); err != nil {
		return apperror.WrapKey(apperror.CodeInternal, http.StatusInternalServerError, "setting.cache_clear_failed", map[string]any{"key": AuthVerifyCodeTTLKey}, "清理系统设置缓存失败", err)
	}
	return nil
}

func NormalizeAuthVerifyCodeTTLMinutes(minutes int) (int, *apperror.Error) {
	if minutes < minAuthVerifyCodeTTLMinutes || minutes > maxAuthVerifyCodeTTLMinutes {
		return 0, apperror.BadRequestKey("auth.verify_code.ttl.out_of_range", map[string]any{"key": AuthVerifyCodeTTLKey}, "验证码有效期必须在 1-60 分钟之间")
	}
	return minutes, nil
}

type intRule struct {
	min         int
	max         int
	missing     func(string) *apperror.Error
	disabled    func(string) *apperror.Error
	wrongType   func(string) *apperror.Error
	notInteger  func(string) *apperror.Error
	outOfRange  func(string) *apperror.Error
	queryFailed func(string, error) *apperror.Error
}

func requiredInt(ctx context.Context, reader Reader, key string, rule intRule) (int, *apperror.Error) {
	row, appErr := readRow(ctx, reader, key, rule)
	if appErr != nil {
		return 0, appErr
	}
	if row == nil || row.IsDel != enum.CommonNo {
		return 0, callOrDefault(rule.missing, key, apperror.InternalKey("setting.missing", map[string]any{"key": key}, "系统设置缺失"))
	}
	if row.Status != enum.CommonYes {
		return 0, callOrDefault(rule.disabled, key, apperror.BadRequestKey("setting.disabled", map[string]any{"key": key}, "系统设置已禁用"))
	}
	return parseActiveInt(row, rule)
}

func optionalInt(ctx context.Context, reader Reader, key string, fallback int, rule intRule) (int, *apperror.Error) {
	row, appErr := readRow(ctx, reader, key, rule)
	if appErr != nil {
		return 0, appErr
	}
	if row == nil || row.IsDel != enum.CommonNo || row.Status != enum.CommonYes || strings.TrimSpace(row.SettingValue) == "" {
		return fallback, nil
	}
	return parseActiveInt(row, rule)
}

func readRow(ctx context.Context, reader Reader, key string, rule intRule) (*systemsetting.Setting, *apperror.Error) {
	if reader == nil {
		return nil, apperror.InternalKey("setting.repository_missing", map[string]any{"key": key}, "系统设置仓储未配置")
	}
	row, err := reader.SettingByKey(ctx, key)
	if err != nil {
		if rule.queryFailed != nil {
			return nil, rule.queryFailed(key, err)
		}
		return nil, apperror.WrapKey(apperror.CodeInternal, http.StatusInternalServerError, "setting.query_failed", map[string]any{"key": key}, "查询系统设置失败", err)
	}
	return row, nil
}

func parseActiveInt(row *systemsetting.Setting, rule intRule) (int, *apperror.Error) {
	key := row.SettingKey
	if row.ValueType != enum.SystemSettingValueNumber {
		return 0, callOrDefault(rule.wrongType, key, apperror.InternalKey("setting.type_invalid", map[string]any{"key": key}, "系统设置类型必须为数字"))
	}
	value, err := strconv.Atoi(strings.TrimSpace(row.SettingValue))
	if err != nil {
		return 0, callOrDefault(rule.notInteger, key, apperror.BadRequestKey("setting.integer_required", map[string]any{"key": key}, "系统设置必须为整数"))
	}
	if value < rule.min || (rule.max > 0 && value > rule.max) {
		return 0, callOrDefault(rule.outOfRange, key, apperror.BadRequestKey("setting.out_of_range", map[string]any{"key": key}, "系统设置取值超出范围"))
	}
	return value, nil
}

func callOrDefault(fn func(string) *apperror.Error, key string, fallback *apperror.Error) *apperror.Error {
	if fn == nil {
		return fallback
	}
	return fn(key)
}

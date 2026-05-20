package captcha

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/module/systemsetting"
)

const (
	// CaptchaTTLSettingKey is the system_settings key for slide CAPTCHA lifetime in minutes.
	CaptchaTTLSettingKey = "auth.captcha.ttl_minutes"
	// CaptchaSlidePaddingSettingKey is the system_settings key for tolerated slide-answer offset.
	CaptchaSlidePaddingSettingKey = "auth.captcha.slide_padding"
)

// CaptchaPolicyProvider reads runtime CAPTCHA policy.
type CaptchaPolicyProvider interface {
	TTL(ctx context.Context) (time.Duration, *apperror.Error)
	SlidePadding(ctx context.Context) (int, *apperror.Error)
}

// CaptchaPolicyRepository is the minimal system-setting read boundary CAPTCHA needs.
type CaptchaPolicyRepository interface {
	SettingByKey(ctx context.Context, key string) (*systemsetting.Setting, error)
}

// SystemSettingCaptchaPolicyProvider reads CAPTCHA policy from system_settings.
type SystemSettingCaptchaPolicyProvider struct {
	repository CaptchaPolicyRepository
}

// NewSystemSettingCaptchaPolicyProvider returns a DB-backed CAPTCHA policy provider.
func NewSystemSettingCaptchaPolicyProvider(repository CaptchaPolicyRepository) *SystemSettingCaptchaPolicyProvider {
	return &SystemSettingCaptchaPolicyProvider{repository: repository}
}

// TTL returns the enabled CAPTCHA lifetime.
func (p *SystemSettingCaptchaPolicyProvider) TTL(ctx context.Context) (time.Duration, *apperror.Error) {
	minutes, appErr := p.positiveIntSetting(ctx, CaptchaTTLSettingKey)
	if appErr != nil {
		return 0, appErr
	}
	return time.Duration(minutes) * time.Minute, nil
}

// SlidePadding returns the enabled tolerated slide-answer offset.
func (p *SystemSettingCaptchaPolicyProvider) SlidePadding(ctx context.Context) (int, *apperror.Error) {
	return p.nonNegativeIntSetting(ctx, CaptchaSlidePaddingSettingKey)
}

func (p *SystemSettingCaptchaPolicyProvider) positiveIntSetting(ctx context.Context, key string) (int, *apperror.Error) {
	value, appErr := p.intSetting(ctx, key)
	if appErr != nil {
		return 0, appErr
	}
	if value <= 0 {
		return 0, apperror.BadRequestKey("captcha.policy.value_invalid", map[string]any{"key": key}, "验证码策略配置取值无效")
	}
	return value, nil
}

func (p *SystemSettingCaptchaPolicyProvider) nonNegativeIntSetting(ctx context.Context, key string) (int, *apperror.Error) {
	value, appErr := p.intSetting(ctx, key)
	if appErr != nil {
		return 0, appErr
	}
	if value < 0 {
		return 0, apperror.BadRequestKey("captcha.policy.value_invalid", map[string]any{"key": key}, "验证码策略配置取值无效")
	}
	return value, nil
}

func (p *SystemSettingCaptchaPolicyProvider) intSetting(ctx context.Context, key string) (int, *apperror.Error) {
	if p == nil || p.repository == nil {
		return 0, apperror.InternalKey("captcha.policy.repository_missing", nil, "验证码策略仓储未配置")
	}
	row, err := p.repository.SettingByKey(ctx, key)
	if err != nil {
		return 0, apperror.WrapKey(apperror.CodeInternal, http.StatusInternalServerError, "captcha.policy.query_failed", map[string]any{"key": key}, "查询验证码策略配置失败", err)
	}
	if row == nil || row.IsDel != enum.CommonNo {
		return 0, apperror.InternalKey("captcha.policy.missing", map[string]any{"key": key}, "验证码策略配置缺失")
	}
	if row.Status != enum.CommonYes {
		return 0, apperror.BadRequestKey("captcha.policy.disabled", map[string]any{"key": key}, "验证码策略配置已禁用")
	}
	if row.ValueType != enum.SystemSettingValueNumber {
		return 0, apperror.InternalKey("captcha.policy.type_invalid", map[string]any{"key": key}, "验证码策略配置类型必须为数字")
	}
	value, err := strconv.Atoi(strings.TrimSpace(row.SettingValue))
	if err != nil {
		return 0, apperror.BadRequestKey("captcha.policy.integer_invalid", map[string]any{"key": key}, "验证码策略配置必须为整数")
	}
	return value, nil
}

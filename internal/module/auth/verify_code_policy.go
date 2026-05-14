package auth

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
	// VerifyCodeTTLSettingKey is the system_settings key for the shared verification-code TTL.
	VerifyCodeTTLSettingKey = "auth.verify_code.ttl_minutes"
	minVerifyCodeTTLMinutes = 1
	maxVerifyCodeTTLMinutes = 60
)

// VerifyCodePolicyProvider reads shared verification-code policy for auth flows.
type VerifyCodePolicyProvider interface {
	VerifyCodeTTL(ctx context.Context) (time.Duration, *apperror.Error)
}

// VerifyCodePolicyRepository is the minimal system-setting read boundary auth needs.
type VerifyCodePolicyRepository interface {
	SettingByKey(ctx context.Context, key string) (*systemsetting.Setting, error)
}

// SystemSettingVerifyCodePolicyProvider reads verification-code policy from system_settings.
type SystemSettingVerifyCodePolicyProvider struct {
	repository VerifyCodePolicyRepository
}

// NewSystemSettingVerifyCodePolicyProvider returns a DB-backed verification-code policy provider.
func NewSystemSettingVerifyCodePolicyProvider(repository VerifyCodePolicyRepository) *SystemSettingVerifyCodePolicyProvider {
	return &SystemSettingVerifyCodePolicyProvider{repository: repository}
}

// VerifyCodeTTL returns the enabled shared verification-code TTL.
func (p *SystemSettingVerifyCodePolicyProvider) VerifyCodeTTL(ctx context.Context) (time.Duration, *apperror.Error) {
	if p == nil || p.repository == nil {
		return 0, apperror.Internal("验证码策略仓储未配置")
	}
	row, err := p.repository.SettingByKey(ctx, VerifyCodeTTLSettingKey)
	if err != nil {
		return 0, apperror.Wrap(apperror.CodeInternal, http.StatusInternalServerError, "查询验证码有效期配置失败", err)
	}
	if row == nil || row.IsDel != enum.CommonNo {
		return 0, apperror.Internal("验证码有效期配置缺失")
	}
	if row.Status != enum.CommonYes {
		return 0, apperror.BadRequest("验证码有效期配置已禁用")
	}
	if row.ValueType != enum.SystemSettingValueNumber {
		return 0, apperror.Internal("验证码有效期配置类型必须为数字")
	}
	minutes, err := strconv.Atoi(strings.TrimSpace(row.SettingValue))
	if err != nil {
		return 0, apperror.BadRequest("验证码有效期必须为整数分钟")
	}
	if minutes < minVerifyCodeTTLMinutes || minutes > maxVerifyCodeTTLMinutes {
		return 0, apperror.BadRequest("验证码有效期必须在 1-60 分钟之间")
	}
	return time.Duration(minutes) * time.Minute, nil
}

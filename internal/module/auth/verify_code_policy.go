package auth

import (
	"context"
	"time"

	"admin_back_go/internal/shared/apperror"
	sharedsetting "admin_back_go/internal/shared/setting"
)

// VerifyCodePolicyProvider reads shared verification-code policy for auth flows.
type VerifyCodePolicyProvider interface {
	VerifyCodeTTL(ctx context.Context) (time.Duration, *apperror.Error)
}

// VerifyCodePolicyRepository is the minimal system-setting read boundary auth needs.
type VerifyCodePolicyRepository interface {
	sharedsetting.Reader
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
		return 0, apperror.InternalKey("setting.repository_missing", nil, "系统设置仓储未配置")
	}
	minutes, appErr := sharedsetting.AuthVerifyCodeTTLMinutes(ctx, p.repository)
	if appErr != nil {
		return 0, appErr
	}
	return time.Duration(minutes) * time.Minute, nil
}

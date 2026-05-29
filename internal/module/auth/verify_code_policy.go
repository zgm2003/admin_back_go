package auth

import (
	"context"
	"time"

	"admin_back_go/internal/shared/apperror"
)

// VerifyCodePolicyProvider chooses the verification-code TTL for an auth account type.
type VerifyCodePolicyProvider interface {
	VerifyCodeTTL(ctx context.Context, accountType string) (time.Duration, *apperror.Error)
}

// VerifyCodeTTLProvider is the narrow channel service contract auth needs.
type VerifyCodeTTLProvider interface {
	VerifyCodeTTL(ctx context.Context) (time.Duration, *apperror.Error)
}

type ChannelVerifyCodePolicyProvider struct {
	email VerifyCodeTTLProvider
	phone VerifyCodeTTLProvider
}

func NewChannelVerifyCodePolicyProvider(email VerifyCodeTTLProvider, phone VerifyCodeTTLProvider) *ChannelVerifyCodePolicyProvider {
	return &ChannelVerifyCodePolicyProvider{email: email, phone: phone}
}

func (p *ChannelVerifyCodePolicyProvider) VerifyCodeTTL(ctx context.Context, accountType string) (time.Duration, *apperror.Error) {
	if p == nil {
		return 0, apperror.InternalKey("auth.verify_code.policy_missing", nil, "验证码有效期策略未配置")
	}
	switch accountType {
	case LoginTypeEmail:
		if p.email == nil {
			return 0, apperror.InternalKey("auth.verify_code.email_policy_missing", nil, "邮箱验证码有效期策略未配置")
		}
		return p.email.VerifyCodeTTL(ctx)
	case LoginTypePhone:
		if p.phone == nil {
			return 0, apperror.InternalKey("auth.verify_code.phone_policy_missing", nil, "短信验证码有效期策略未配置")
		}
		return p.phone.VerifyCodeTTL(ctx)
	default:
		return 0, apperror.BadRequestKey("auth.verify_code.account_type_invalid", map[string]any{"account_type": accountType}, "无效的验证码账号类型")
	}
}

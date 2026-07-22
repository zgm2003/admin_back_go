package auth

import (
	"context"

	"admin_back_go/internal/shared/apperror"
)

type VerifyCodeReadinessProvider interface {
	VerifyCodeReady(ctx context.Context, accountType string, scene string) (bool, *apperror.Error)
}

type VerifyCodeChannelReadinessProvider interface {
	VerifyCodeReady(ctx context.Context, scene string) (bool, *apperror.Error)
}

type ChannelVerifyCodeReadinessProvider struct {
	email VerifyCodeChannelReadinessProvider
	phone VerifyCodeChannelReadinessProvider
}

func NewChannelVerifyCodeReadinessProvider(email, phone VerifyCodeChannelReadinessProvider) *ChannelVerifyCodeReadinessProvider {
	return &ChannelVerifyCodeReadinessProvider{email: email, phone: phone}
}

func (p *ChannelVerifyCodeReadinessProvider) VerifyCodeReady(ctx context.Context, accountType string, scene string) (bool, *apperror.Error) {
	if p == nil {
		return false, apperror.InternalKey("auth.verify_code.readiness_missing", nil, "验证码渠道就绪检查未配置")
	}
	switch accountType {
	case LoginTypeEmail:
		if p.email == nil {
			return false, apperror.InternalKey("auth.verify_code.email_readiness_missing", nil, "邮箱验证码渠道就绪检查未配置")
		}
		return p.email.VerifyCodeReady(ctx, scene)
	case LoginTypePhone:
		if p.phone == nil {
			return false, apperror.InternalKey("auth.verify_code.phone_readiness_missing", nil, "短信验证码渠道就绪检查未配置")
		}
		return p.phone.VerifyCodeReady(ctx, scene)
	default:
		return false, apperror.BadRequestKey("auth.verify_code.account_type_invalid", map[string]any{"account_type": accountType}, "无效的验证码账号类型")
	}
}

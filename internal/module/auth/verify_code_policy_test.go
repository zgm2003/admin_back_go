package auth

import (
	"context"
	"testing"
	"time"

	"admin_back_go/internal/shared/apperror"
)

type fakeChannelTTLProvider struct {
	ttl    time.Duration
	err    *apperror.Error
	called bool
}

func (f *fakeChannelTTLProvider) VerifyCodeTTL(ctx context.Context) (time.Duration, *apperror.Error) {
	f.called = true
	if f.err != nil {
		return 0, f.err
	}
	return f.ttl, nil
}

func TestChannelVerifyCodePolicyProviderUsesEmailProvider(t *testing.T) {
	email := &fakeChannelTTLProvider{ttl: 7 * time.Minute}
	phone := &fakeChannelTTLProvider{ttl: 8 * time.Minute}
	provider := NewChannelVerifyCodePolicyProvider(email, phone)

	got, appErr := provider.VerifyCodeTTL(context.Background(), LoginTypeEmail)

	if appErr != nil || got != 7*time.Minute {
		t.Fatalf("ttl=%s err=%#v", got, appErr)
	}
	if !email.called || phone.called {
		t.Fatalf("email provider only should be called: email=%v phone=%v", email.called, phone.called)
	}
}

func TestChannelVerifyCodePolicyProviderUsesPhoneProvider(t *testing.T) {
	email := &fakeChannelTTLProvider{ttl: 7 * time.Minute}
	phone := &fakeChannelTTLProvider{ttl: 8 * time.Minute}
	provider := NewChannelVerifyCodePolicyProvider(email, phone)

	got, appErr := provider.VerifyCodeTTL(context.Background(), LoginTypePhone)

	if appErr != nil || got != 8*time.Minute {
		t.Fatalf("ttl=%s err=%#v", got, appErr)
	}
	if email.called || !phone.called {
		t.Fatalf("phone provider only should be called: email=%v phone=%v", email.called, phone.called)
	}
}

func TestChannelVerifyCodePolicyProviderRejectsUnknownAccountType(t *testing.T) {
	provider := NewChannelVerifyCodePolicyProvider(&fakeChannelTTLProvider{ttl: time.Minute}, &fakeChannelTTLProvider{ttl: time.Minute})

	got, appErr := provider.VerifyCodeTTL(context.Background(), "password")

	if appErr == nil || appErr.Code != apperror.CodeBadRequest || got != 0 {
		t.Fatalf("ttl=%s err=%#v", got, appErr)
	}
}

func TestChannelVerifyCodePolicyProviderPropagatesProviderError(t *testing.T) {
	wantErr := apperror.BadRequest("短信验证码有效期配置错误")
	provider := NewChannelVerifyCodePolicyProvider(&fakeChannelTTLProvider{ttl: time.Minute}, &fakeChannelTTLProvider{err: wantErr})

	got, appErr := provider.VerifyCodeTTL(context.Background(), LoginTypePhone)

	if got != 0 || appErr != wantErr {
		t.Fatalf("ttl=%s err=%#v", got, appErr)
	}
}

func TestChannelVerifyCodePolicyProviderRequiresConfiguredProviders(t *testing.T) {
	for _, tt := range []struct {
		name        string
		email       VerifyCodeTTLProvider
		phone       VerifyCodeTTLProvider
		accountType string
	}{
		{name: "missing email", phone: &fakeChannelTTLProvider{ttl: time.Minute}, accountType: LoginTypeEmail},
		{name: "missing phone", email: &fakeChannelTTLProvider{ttl: time.Minute}, accountType: LoginTypePhone},
	} {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewChannelVerifyCodePolicyProvider(tt.email, tt.phone)
			got, appErr := provider.VerifyCodeTTL(context.Background(), tt.accountType)
			if appErr == nil || appErr.Code != apperror.CodeInternal || got != 0 {
				t.Fatalf("ttl=%s err=%#v", got, appErr)
			}
		})
	}
}

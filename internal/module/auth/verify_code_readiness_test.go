package auth

import (
	"context"
	"testing"

	"admin_back_go/internal/shared/apperror"
)

type fakeChannelReadinessProvider struct {
	ready  bool
	err    *apperror.Error
	scenes []string
}

func (f *fakeChannelReadinessProvider) VerifyCodeReady(_ context.Context, scene string) (bool, *apperror.Error) {
	f.scenes = append(f.scenes, scene)
	return f.ready, f.err
}

func TestChannelVerifyCodeReadinessProviderRoutesChannelAndScene(t *testing.T) {
	for _, tt := range []struct {
		name        string
		accountType string
		scene       string
		wantEmail   bool
		wantPhone   bool
	}{
		{name: "email bind", accountType: LoginTypeEmail, scene: VerifyCodeSceneBindEmail, wantEmail: true},
		{name: "phone login", accountType: LoginTypePhone, scene: VerifyCodeSceneLogin, wantPhone: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			email := &fakeChannelReadinessProvider{ready: true}
			phone := &fakeChannelReadinessProvider{ready: true}
			provider := NewChannelVerifyCodeReadinessProvider(email, phone)
			ready, appErr := provider.VerifyCodeReady(context.Background(), tt.accountType, tt.scene)
			if appErr != nil || !ready {
				t.Fatalf("ready=%v err=%#v", ready, appErr)
			}
			if (len(email.scenes) == 1) != tt.wantEmail || (len(phone.scenes) == 1) != tt.wantPhone {
				t.Fatalf("email=%#v phone=%#v", email.scenes, phone.scenes)
			}
			selected := email
			if tt.wantPhone {
				selected = phone
			}
			if selected.scenes[0] != tt.scene {
				t.Fatalf("scenes=%#v", selected.scenes)
			}
		})
	}
}

func TestChannelVerifyCodeReadinessProviderPropagatesError(t *testing.T) {
	wantErr := apperror.Internal("query failed")
	provider := NewChannelVerifyCodeReadinessProvider(
		&fakeChannelReadinessProvider{err: wantErr},
		&fakeChannelReadinessProvider{ready: true},
	)
	ready, appErr := provider.VerifyCodeReady(context.Background(), LoginTypeEmail, VerifyCodeSceneLogin)
	if ready || appErr != wantErr {
		t.Fatalf("ready=%v err=%#v", ready, appErr)
	}
}

func TestChannelVerifyCodeReadinessProviderRejectsUnknownOrMissingProvider(t *testing.T) {
	provider := NewChannelVerifyCodeReadinessProvider(nil, &fakeChannelReadinessProvider{ready: true})
	if ready, appErr := provider.VerifyCodeReady(context.Background(), LoginTypeEmail, VerifyCodeSceneLogin); ready || appErr == nil || appErr.LegacyCode != apperror.CodeInternal {
		t.Fatalf("missing email ready=%v err=%#v", ready, appErr)
	}
	if ready, appErr := provider.VerifyCodeReady(context.Background(), LoginTypePassword, VerifyCodeSceneLogin); ready || appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest {
		t.Fatalf("unknown type ready=%v err=%#v", ready, appErr)
	}
}

package sms

import (
	"context"
	"errors"
	"testing"

	"admin_back_go/internal/infra/secretbox"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
)

func readySMSConfig() *Config {
	return &Config{
		SecretIDEnc: "cipher-id", SecretKeyEnc: "cipher-key",
		SmsSdkAppID: "1400000000", SignName: "Admin",
		Region: DefaultRegion, Endpoint: DefaultEndpoint,
		VerifyCodeTTLMinutes: 5, Status: enum.CommonYes, IsDel: enum.CommonNo,
	}
}

func readySMSTemplate(scene string) *Template {
	return &Template{
		ID: 7, Scene: scene, TencentTemplateID: "12345",
		VariablesJSON: `["code","ttl_minutes"]`, Status: enum.CommonYes,
		IsDel: enum.CommonNo,
	}
}

func TestVerifyCodeReadyRequiresCompleteEnabledSMSSetup(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Service, *fakeSmsRepository)
	}{
		{name: "sender missing", mutate: func(s *Service, _ *fakeSmsRepository) { s.sender = nil }},
		{name: "config missing", mutate: func(_ *Service, r *fakeSmsRepository) { r.config = nil }},
		{name: "config disabled", mutate: func(_ *Service, r *fakeSmsRepository) { r.config.Status = enum.CommonNo }},
		{name: "config deleted", mutate: func(_ *Service, r *fakeSmsRepository) { r.config.IsDel = enum.CommonYes }},
		{name: "credential missing", mutate: func(_ *Service, r *fakeSmsRepository) { r.config.SecretIDEnc = "" }},
		{name: "credential key missing", mutate: func(_ *Service, r *fakeSmsRepository) { r.config.SecretKeyEnc = "" }},
		{name: "app id missing", mutate: func(_ *Service, r *fakeSmsRepository) { r.config.SmsSdkAppID = "" }},
		{name: "sign missing", mutate: func(_ *Service, r *fakeSmsRepository) { r.config.SignName = "" }},
		{name: "region invalid", mutate: func(_ *Service, r *fakeSmsRepository) { r.config.Region = "invalid" }},
		{name: "endpoint missing", mutate: func(_ *Service, r *fakeSmsRepository) { r.config.Endpoint = "" }},
		{name: "ttl too low", mutate: func(_ *Service, r *fakeSmsRepository) { r.config.VerifyCodeTTLMinutes = 0 }},
		{name: "ttl invalid", mutate: func(_ *Service, r *fakeSmsRepository) { r.config.VerifyCodeTTLMinutes = 61 }},
		{name: "template missing", mutate: func(_ *Service, r *fakeSmsRepository) { delete(r.templates, enum.VerifyCodeSceneLogin) }},
		{name: "template disabled", mutate: func(_ *Service, r *fakeSmsRepository) { r.templates[enum.VerifyCodeSceneLogin].Status = enum.CommonNo }},
		{name: "template deleted", mutate: func(_ *Service, r *fakeSmsRepository) { r.templates[enum.VerifyCodeSceneLogin].IsDel = enum.CommonYes }},
		{name: "template id missing", mutate: func(_ *Service, r *fakeSmsRepository) { r.templates[enum.VerifyCodeSceneLogin].TencentTemplateID = "" }},
		{name: "variables malformed", mutate: func(_ *Service, r *fakeSmsRepository) { r.templates[enum.VerifyCodeSceneLogin].VariablesJSON = `{}` }},
		{name: "variables extra", mutate: func(_ *Service, r *fakeSmsRepository) {
			r.templates[enum.VerifyCodeSceneLogin].VariablesJSON = `["code","ttl_minutes","extra"]`
		}},
		{name: "variables reversed", mutate: func(_ *Service, r *fakeSmsRepository) {
			r.templates[enum.VerifyCodeSceneLogin].VariablesJSON = `["ttl_minutes","code"]`
		}},
		{name: "variables wrong", mutate: func(_ *Service, r *fakeSmsRepository) {
			r.templates[enum.VerifyCodeSceneLogin].VariablesJSON = `["code","expires"]`
		}},
		{name: "variable value has whitespace", mutate: func(_ *Service, r *fakeSmsRepository) {
			r.templates[enum.VerifyCodeSceneLogin].VariablesJSON = `[" code ","ttl_minutes"]`
		}},
		{name: "variables invalid json", mutate: func(_ *Service, r *fakeSmsRepository) {
			r.templates[enum.VerifyCodeSceneLogin].VariablesJSON = `invalid`
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeSmsRepository()
			repo.config = readySMSConfig()
			repo.templates[enum.VerifyCodeSceneLogin] = readySMSTemplate(enum.VerifyCodeSceneLogin)
			service := NewService(repo, secretbox.Box{}, &fakeSmsSender{})
			tt.mutate(service, repo)
			ready, appErr := service.VerifyCodeReady(context.Background(), enum.VerifyCodeSceneLogin)
			if appErr != nil || ready {
				t.Fatalf("ready=%v err=%#v", ready, appErr)
			}
		})
	}
}

func TestVerifyCodeReadyReturnsTrueForCompleteSMSSetup(t *testing.T) {
	repo := newFakeSmsRepository()
	repo.config = readySMSConfig()
	repo.templates[enum.VerifyCodeSceneLogin] = readySMSTemplate(enum.VerifyCodeSceneLogin)
	senderCalls := 0
	sender := SenderFunc(func(context.Context, SendInput) (SendResult, error) {
		senderCalls++
		return SendResult{}, errors.New("readiness must not call sender")
	})

	ready, appErr := NewService(repo, secretbox.Box{}, sender).
		VerifyCodeReady(context.Background(), enum.VerifyCodeSceneLogin)

	if appErr != nil || !ready {
		t.Fatalf("ready=%v err=%#v", ready, appErr)
	}
	if senderCalls != 0 {
		t.Fatalf("sender calls=%d, want 0", senderCalls)
	}
	if len(repo.createdLogs) != 0 {
		t.Fatalf("created logs=%d, want 0", len(repo.createdLogs))
	}
	if len(repo.finishes) != 0 {
		t.Fatalf("finished logs=%d, want 0", len(repo.finishes))
	}
}

func TestVerifyCodeReadyAcceptsSMSConfigTTLBoundaries(t *testing.T) {
	tests := []struct {
		name string
		ttl  int
	}{
		{name: "minimum", ttl: 1},
		{name: "maximum", ttl: 60},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeSmsRepository()
			repo.config = readySMSConfig()
			repo.config.VerifyCodeTTLMinutes = tt.ttl
			repo.templates[enum.VerifyCodeSceneLogin] = readySMSTemplate(enum.VerifyCodeSceneLogin)

			ready, appErr := NewService(repo, secretbox.Box{}, &fakeSmsSender{}).
				VerifyCodeReady(context.Background(), enum.VerifyCodeSceneLogin)

			if appErr != nil || !ready {
				t.Fatalf("ttl=%d ready=%v err=%#v", tt.ttl, ready, appErr)
			}
		})
	}
}

func TestVerifyCodeReadyRejectsUnconfiguredSMSRepository(t *testing.T) {
	ready, appErr := NewService(nil, secretbox.Box{}, &fakeSmsSender{}).
		VerifyCodeReady(context.Background(), enum.VerifyCodeSceneLogin)
	if ready || appErr == nil || appErr.LegacyCode != apperror.CodeInternal || !errors.Is(appErr, ErrRepositoryNotConfigured) {
		t.Fatalf("ready=%v err=%#v", ready, appErr)
	}
}

func TestVerifyCodeReadyPropagatesSMSRepositoryFailure(t *testing.T) {
	wantErr := errors.New("sms database unavailable")
	repo := newFakeSmsRepository()
	repo.config, repo.configErr = readySMSConfig(), wantErr

	ready, appErr := NewService(repo, secretbox.Box{}, &fakeSmsSender{}).
		VerifyCodeReady(context.Background(), enum.VerifyCodeSceneLogin)

	if ready || appErr == nil || !errors.Is(appErr, wantErr) || appErr.LegacyCode != apperror.CodeInternal || appErr.MessageID != "sms.config.query_failed" {
		t.Fatalf("ready=%v err=%#v", ready, appErr)
	}
}

func TestVerifyCodeReadyPropagatesSMSTemplateQueryFailure(t *testing.T) {
	wantErr := errors.New("sms template database unavailable")
	repo := newFakeSmsRepository()
	repo.config = readySMSConfig()
	repo.templates[enum.VerifyCodeSceneLogin] = readySMSTemplate(enum.VerifyCodeSceneLogin)
	repo.templateErr = wantErr

	ready, appErr := NewService(repo, secretbox.Box{}, &fakeSmsSender{}).
		VerifyCodeReady(context.Background(), enum.VerifyCodeSceneLogin)

	if ready || appErr == nil || !errors.Is(appErr, wantErr) || appErr.LegacyCode != apperror.CodeInternal || appErr.MessageID != "sms.template.query_failed" {
		t.Fatalf("ready=%v err=%#v", ready, appErr)
	}
}

func TestVerifyCodeReadyRejectsNonSMSScene(t *testing.T) {
	repo := newFakeSmsRepository()
	repo.config = readySMSConfig()
	ready, appErr := NewService(repo, secretbox.Box{}, &fakeSmsSender{}).
		VerifyCodeReady(context.Background(), enum.VerifyCodeSceneBindEmail)
	if ready || appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest || appErr.MessageID != "sms.scene.invalid" {
		t.Fatalf("ready=%v err=%#v", ready, appErr)
	}
}

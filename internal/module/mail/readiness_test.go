package mail

import (
	"context"
	"errors"
	"testing"

	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
)

func readyMailConfig() *Config {
	return &Config{
		SecretIDEnc: "cipher-id", SecretKeyEnc: "cipher-key",
		Region: DefaultRegion, Endpoint: DefaultEndpoint,
		FromEmail: "noreply@example.com", VerifyCodeTTLMinutes: 5,
		Status: enum.CommonYes, IsDel: enum.CommonNo,
	}
}

func readyMailTemplate(scene string) *Template {
	return &Template{
		ID: 7, Scene: scene, Subject: "Verification code", TencentTemplateID: 12345,
		VariablesJSON: `["code","ttl_minutes"]`, Status: enum.CommonYes,
		IsDel: enum.CommonNo,
	}
}

type templateQueryErrorMailRepository struct {
	*fakeMailRepository
	err error
}

func (r *templateQueryErrorMailRepository) TemplateByScene(context.Context, string) (*Template, error) {
	return nil, r.err
}

func TestVerifyCodeReadyRequiresCompleteEnabledMailSetup(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Service, *fakeMailRepository)
	}{
		{name: "sender missing", mutate: func(s *Service, _ *fakeMailRepository) { s.sender = nil }},
		{name: "config missing", mutate: func(_ *Service, r *fakeMailRepository) { r.config = nil }},
		{name: "config disabled", mutate: func(_ *Service, r *fakeMailRepository) { r.config.Status = enum.CommonNo }},
		{name: "config deleted", mutate: func(_ *Service, r *fakeMailRepository) { r.config.IsDel = enum.CommonYes }},
		{name: "credential id missing", mutate: func(_ *Service, r *fakeMailRepository) { r.config.SecretIDEnc = "" }},
		{name: "credential key missing", mutate: func(_ *Service, r *fakeMailRepository) { r.config.SecretKeyEnc = "" }},
		{name: "region unsupported", mutate: func(_ *Service, r *fakeMailRepository) { r.config.Region = "ap-shanghai" }},
		{name: "endpoint missing", mutate: func(_ *Service, r *fakeMailRepository) { r.config.Endpoint = "" }},
		{name: "sender address invalid", mutate: func(_ *Service, r *fakeMailRepository) { r.config.FromEmail = "bad" }},
		{name: "reply-to address invalid", mutate: func(_ *Service, r *fakeMailRepository) { r.config.ReplyTo = "bad" }},
		{name: "ttl too low", mutate: func(_ *Service, r *fakeMailRepository) { r.config.VerifyCodeTTLMinutes = 0 }},
		{name: "ttl too high", mutate: func(_ *Service, r *fakeMailRepository) { r.config.VerifyCodeTTLMinutes = 61 }},
		{name: "template missing", mutate: func(_ *Service, r *fakeMailRepository) { delete(r.templates, enum.VerifyCodeSceneLogin) }},
		{name: "template disabled", mutate: func(_ *Service, r *fakeMailRepository) { r.templates[enum.VerifyCodeSceneLogin].Status = enum.CommonNo }},
		{name: "template deleted", mutate: func(_ *Service, r *fakeMailRepository) { r.templates[enum.VerifyCodeSceneLogin].IsDel = enum.CommonYes }},
		{name: "provider template missing", mutate: func(_ *Service, r *fakeMailRepository) { r.templates[enum.VerifyCodeSceneLogin].TencentTemplateID = 0 }},
		{name: "subject missing", mutate: func(_ *Service, r *fakeMailRepository) { r.templates[enum.VerifyCodeSceneLogin].Subject = "" }},
		{name: "variables malformed", mutate: func(_ *Service, r *fakeMailRepository) {
			r.templates[enum.VerifyCodeSceneLogin].VariablesJSON = `["code"]`
		}},
		{name: "variables reversed", mutate: func(_ *Service, r *fakeMailRepository) {
			r.templates[enum.VerifyCodeSceneLogin].VariablesJSON = `["ttl_minutes","code"]`
		}},
		{name: "variables wrong", mutate: func(_ *Service, r *fakeMailRepository) {
			r.templates[enum.VerifyCodeSceneLogin].VariablesJSON = `["code","expires"]`
		}},
		{name: "variable value has whitespace", mutate: func(_ *Service, r *fakeMailRepository) {
			r.templates[enum.VerifyCodeSceneLogin].VariablesJSON = `[" code ","ttl_minutes"]`
		}},
		{name: "variables invalid json", mutate: func(_ *Service, r *fakeMailRepository) {
			r.templates[enum.VerifyCodeSceneLogin].VariablesJSON = `invalid`
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeMailRepository{
				config:    readyMailConfig(),
				templates: map[string]*Template{enum.VerifyCodeSceneLogin: readyMailTemplate(enum.VerifyCodeSceneLogin)},
			}
			service := NewServiceWithDependencies(ServiceDependencies{
				Repository: repo, CredentialBox: testSecretBox(), DiagnosticBox: testDiagnosticBox(), Sender: &fakeMailSender{},
			})
			tt.mutate(service, repo)
			ready, appErr := service.VerifyCodeReady(context.Background(), enum.VerifyCodeSceneLogin)
			if appErr != nil || ready {
				t.Fatalf("ready=%v err=%#v", ready, appErr)
			}
		})
	}
}

func TestVerifyCodeReadyReturnsTrueForCompleteMailSetup(t *testing.T) {
	repo := &fakeMailRepository{
		config: readyMailConfig(),
		templates: map[string]*Template{
			enum.VerifyCodeSceneLogin: readyMailTemplate(enum.VerifyCodeSceneLogin),
		},
	}
	senderCalls := 0
	sender := SenderFunc(func(context.Context, SendInput) (SendResult, error) {
		senderCalls++
		return SendResult{}, errors.New("readiness must not call sender")
	})
	ready, appErr := NewServiceWithDependencies(ServiceDependencies{
		Repository: repo, CredentialBox: testSecretBox(), DiagnosticBox: testDiagnosticBox(), Sender: sender,
	}).
		VerifyCodeReady(context.Background(), enum.VerifyCodeSceneLogin)
	if appErr != nil || !ready {
		t.Fatalf("ready=%v err=%#v", ready, appErr)
	}
	if senderCalls != 0 {
		t.Fatalf("sender calls=%d, want 0", senderCalls)
	}
}

func TestVerifyCodeReadyRejectsUnconfiguredMailRepository(t *testing.T) {
	ready, appErr := NewServiceWithDependencies(ServiceDependencies{
		Repository: nil, CredentialBox: testSecretBox(), DiagnosticBox: testDiagnosticBox(), Sender: &fakeMailSender{},
	}).
		VerifyCodeReady(context.Background(), enum.VerifyCodeSceneLogin)
	if ready || appErr == nil || appErr.LegacyCode != apperror.CodeInternal || !errors.Is(appErr, ErrRepositoryNotConfigured) {
		t.Fatalf("ready=%v err=%#v", ready, appErr)
	}
}

func TestVerifyCodeReadyPropagatesMailConfigQueryFailure(t *testing.T) {
	wantErr := errors.New("mail database unavailable")
	repo := &fakeMailRepository{config: readyMailConfig(), err: wantErr}
	ready, appErr := NewServiceWithDependencies(ServiceDependencies{
		Repository: repo, CredentialBox: testSecretBox(), DiagnosticBox: testDiagnosticBox(), Sender: &fakeMailSender{},
	}).
		VerifyCodeReady(context.Background(), enum.VerifyCodeSceneLogin)
	if ready || appErr == nil || !errors.Is(appErr, wantErr) || appErr.LegacyCode != apperror.CodeInternal {
		t.Fatalf("ready=%v err=%#v", ready, appErr)
	}
}

func TestVerifyCodeReadyPropagatesMailTemplateQueryFailure(t *testing.T) {
	wantErr := errors.New("mail template database unavailable")
	repo := &templateQueryErrorMailRepository{
		fakeMailRepository: &fakeMailRepository{config: readyMailConfig()},
		err:                wantErr,
	}
	ready, appErr := NewServiceWithDependencies(ServiceDependencies{
		Repository: repo, CredentialBox: testSecretBox(), DiagnosticBox: testDiagnosticBox(), Sender: &fakeMailSender{},
	}).
		VerifyCodeReady(context.Background(), enum.VerifyCodeSceneLogin)
	if ready || appErr == nil || !errors.Is(appErr, wantErr) || appErr.LegacyCode != apperror.CodeInternal {
		t.Fatalf("ready=%v err=%#v", ready, appErr)
	}
}

func TestVerifyCodeReadyRejectsNonMailScene(t *testing.T) {
	repo := &fakeMailRepository{config: readyMailConfig()}
	ready, appErr := NewServiceWithDependencies(ServiceDependencies{
		Repository: repo, CredentialBox: testSecretBox(), DiagnosticBox: testDiagnosticBox(), Sender: &fakeMailSender{},
	}).
		VerifyCodeReady(context.Background(), enum.VerifyCodeSceneBindPhone)
	if ready || appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest {
		t.Fatalf("ready=%v err=%#v", ready, appErr)
	}
}

package jobs

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"admin_back_go/internal/module/auth"
	"admin_back_go/internal/platform/taskqueue"
)

func TestNewNoopTaskUsesVersionedType(t *testing.T) {
	task, err := NewNoopTask(NoopPayload{Message: "hello"})
	if err != nil {
		t.Fatalf("NewNoopTask returned error: %v", err)
	}

	if task.Type != TypeSystemNoopV1 {
		t.Fatalf("expected type %s, got %s", TypeSystemNoopV1, task.Type)
	}
	var payload NoopPayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.Message != "hello" {
		t.Fatalf("expected message hello, got %q", payload.Message)
	}
}

func TestRegisterHandlesNoopTask(t *testing.T) {
	mux := taskqueue.NewMux()
	Register(mux, Dependencies{Logger: slog.Default()})

	task, err := NewNoopTask(NoopPayload{Message: "ok"})
	if err != nil {
		t.Fatalf("NewNoopTask returned error: %v", err)
	}

	if err := mux.ProcessProjectTask(context.Background(), task); err != nil {
		t.Fatalf("ProcessProjectTask returned error: %v", err)
	}
}

func TestRegisterHandlesAuthLoginLogTask(t *testing.T) {
	repo := &fakeAuthRepository{}
	mux := taskqueue.NewMux()
	Register(mux, Dependencies{
		Logger:         slog.Default(),
		AuthRepository: repo,
	})

	task, err := auth.NewLoginLogTask(auth.LoginAttempt{
		LoginAccount: "15671628271",
		LoginType:    auth.LoginTypePhone,
		Platform:     "admin",
		IsSuccess:    2,
		Reason:       "invalid_code",
	})
	if err != nil {
		t.Fatalf("NewLoginLogTask returned error: %v", err)
	}

	if err := mux.ProcessProjectTask(context.Background(), task); err != nil {
		t.Fatalf("ProcessProjectTask returned error: %v", err)
	}
	if len(repo.attempts) != 1 || repo.attempts[0].Reason != "invalid_code" {
		t.Fatalf("expected auth login log handler to write repository, got %#v", repo.attempts)
	}
}

type fakeAuthRepository struct {
	attempts []auth.LoginAttempt
}

func (f *fakeAuthRepository) WithTx(ctx context.Context, fn func(auth.Repository) error) error {
	return fn(f)
}

func (f *fakeAuthRepository) FindCredentialByEmail(ctx context.Context, email string) (*auth.UserCredential, error) {
	return nil, nil
}

func (f *fakeAuthRepository) FindCredentialByPhone(ctx context.Context, phone string) (*auth.UserCredential, error) {
	return nil, nil
}

func (f *fakeAuthRepository) FindCredentialByID(ctx context.Context, id int64) (*auth.UserCredential, error) {
	return nil, nil
}

func (f *fakeAuthRepository) FindDefaultRole(ctx context.Context) (*auth.DefaultRole, error) {
	return nil, nil
}

func (f *fakeAuthRepository) CreateUser(ctx context.Context, input auth.CreateUserInput) (int64, error) {
	return 0, nil
}

func (f *fakeAuthRepository) CreateProfile(ctx context.Context, input auth.CreateProfileInput) error {
	return nil
}

func (f *fakeAuthRepository) RecordLoginAttempt(ctx context.Context, attempt auth.LoginAttempt) error {
	f.attempts = append(f.attempts, attempt)
	return nil
}

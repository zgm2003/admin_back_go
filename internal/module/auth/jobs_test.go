package auth

import (
	"context"
	"testing"

	"admin_back_go/internal/platform/taskqueue"
)

func TestNewLoginLogTaskUsesVersionedCriticalQueue(t *testing.T) {
	userID := int64(9)

	task, err := NewLoginLogTask(LoginAttempt{
		UserID:       &userID,
		LoginAccount: "15671628271",
		LoginType:    LoginTypePassword,
		Platform:     "admin",
		IP:           "127.0.0.1",
		UserAgent:    "test-agent",
		IsSuccess:    commonYes,
	})

	if err != nil {
		t.Fatalf("NewLoginLogTask returned error: %v", err)
	}
	if task.Type != TypeAuthLoginLogV1 {
		t.Fatalf("expected task type %s, got %s", TypeAuthLoginLogV1, task.Type)
	}
	if task.Queue != taskqueue.QueueCritical {
		t.Fatalf("expected critical queue, got %q", task.Queue)
	}

	attempt, err := DecodeLoginLogPayload(task.Payload)
	if err != nil {
		t.Fatalf("DecodeLoginLogPayload returned error: %v", err)
	}
	if attempt.UserID == nil || *attempt.UserID != userID || attempt.LoginAccount != "15671628271" || attempt.IsSuccess != commonYes {
		t.Fatalf("unexpected decoded attempt: %#v", attempt)
	}
}

func TestRegisterLoginLogHandlerWritesRepository(t *testing.T) {
	repo := &fakeAuthRepository{}
	mux := taskqueue.NewMux()
	RegisterLoginLogHandler(mux, repo, nil)

	task, err := NewLoginLogTask(LoginAttempt{
		LoginAccount: "15671628271",
		LoginType:    LoginTypePhone,
		Platform:     "admin",
		IsSuccess:    commonNo,
		Reason:       "invalid_code",
	})
	if err != nil {
		t.Fatalf("NewLoginLogTask returned error: %v", err)
	}

	if err := mux.ProcessProjectTask(context.Background(), task); err != nil {
		t.Fatalf("ProcessProjectTask returned error: %v", err)
	}
	if len(repo.attempts) != 1 || repo.attempts[0].Reason != "invalid_code" || repo.attempts[0].IsSuccess != commonNo {
		t.Fatalf("expected repository login log write, got %#v", repo.attempts)
	}
}

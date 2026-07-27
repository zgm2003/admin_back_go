package admin

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin_back_go/internal/middleware"
	aimessage "admin_back_go/internal/module/ai/message"
	"admin_back_go/internal/module/ai/replycommand"
	"admin_back_go/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

type acceptedMessageService struct{}

func (acceptedMessageService) List(context.Context, int64, aimessage.ListQuery) (*aimessage.ListResponse, *apperror.Error) {
	return &aimessage.ListResponse{}, nil
}

func TestSendAndCancelRequestIDContractIs128Characters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(acceptedMessageService{})
	identity := func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 7, Platform: "admin"})
	}
	router.POST("/api/admin/v1/ai-conversations/:id/messages", func(c *gin.Context) { identity(c); handler.Send(c) })
	router.POST("/api/admin/v1/ai-conversations/:id/messages/cancel", func(c *gin.Context) { identity(c); handler.Cancel(c) })

	for _, test := range []struct {
		name       string
		path       string
		body       func(string) string
		wantStatus int
	}{
		{name: "send 128", path: "/api/admin/v1/ai-conversations/3/messages", body: func(id string) string { return fmt.Sprintf(`{"content":"hello","request_id":"%s"}`, id) }, wantStatus: http.StatusAccepted},
		{name: "cancel 128", path: "/api/admin/v1/ai-conversations/3/messages/cancel", body: func(id string) string { return fmt.Sprintf(`{"request_id":"%s"}`, id) }, wantStatus: http.StatusOK},
		{name: "send 129", path: "/api/admin/v1/ai-conversations/3/messages", body: func(id string) string { return fmt.Sprintf(`{"content":"hello","request_id":"%s"}`, id) }, wantStatus: http.StatusBadRequest},
		{name: "cancel 129", path: "/api/admin/v1/ai-conversations/3/messages/cancel", body: func(id string) string { return fmt.Sprintf(`{"request_id":"%s"}`, id) }, wantStatus: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			length := 128
			if strings.Contains(test.name, "129") {
				length = 129
			}
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body(strings.Repeat("界", length))))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, test.wantStatus, recorder.Body.String())
			}
		})
	}
}

func (acceptedMessageService) Send(_ context.Context, _ int64, input aimessage.SendInput) (*aimessage.SendResponse, *apperror.Error) {
	return &aimessage.SendResponse{
		ConversationID: input.ConversationID,
		UserMessageID:  12,
		CommandID:      99,
		RequestID:      input.RequestID,
		State:          replycommand.StatePending,
	}, nil
}

func (acceptedMessageService) Cancel(context.Context, int64, aimessage.CancelInput) (*aimessage.CancelResponse, *apperror.Error) {
	return &aimessage.CancelResponse{ConversationID: 3, RequestID: "request-1", Status: "stopping"}, nil
}

func TestSendReturnsAcceptedDurableCommand(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(acceptedMessageService{})
	router.POST("/api/admin/v1/ai-conversations/:id/messages", func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 7, Platform: "admin"})
		handler.Send(c)
	})

	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/ai-conversations/3/messages", strings.NewReader(`{"content":"hello","request_id":"request-1"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	for _, field := range []string{`"command_id":99`, `"user_message_id":12`, `"request_id":"request-1"`, `"state":"pending"`} {
		if !strings.Contains(recorder.Body.String(), field) {
			t.Fatalf("missing %s in %s", field, recorder.Body.String())
		}
	}
}

func TestCancelReturnsStoppingIntentInsteadOfTerminalState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(acceptedMessageService{})
	router.POST("/api/admin/v1/ai-conversations/:id/messages/cancel", func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 7, Platform: "admin"})
		handler.Cancel(c)
	})

	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/ai-conversations/3/messages/cancel", strings.NewReader(`{"request_id":"request-1"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	for _, field := range []string{`"conversation_id":3`, `"request_id":"request-1"`, `"status":"stopping"`} {
		if !strings.Contains(recorder.Body.String(), field) {
			t.Fatalf("missing %s in %s", field, recorder.Body.String())
		}
	}
	if strings.Contains(recorder.Body.String(), `"status":"canceled"`) {
		t.Fatalf("cancel intent response reported terminal state: %s", recorder.Body.String())
	}
}

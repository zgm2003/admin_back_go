package admin

import (
	"context"
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
	return &aimessage.CancelResponse{}, nil
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

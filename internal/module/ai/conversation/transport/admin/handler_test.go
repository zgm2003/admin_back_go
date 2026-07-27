package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"admin_back_go/internal/middleware"
	aiconversationmodule "admin_back_go/internal/module/ai/conversation"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

func TestReadCursorHandlerUsesAuthenticatedUserAndReturnsExactState(t *testing.T) {
	router, service := newConversationCursorTestRouter()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/admin/v1/ai-conversations/3/read-cursor", bytes.NewBufferString(`{"message_id":11}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.cursorUserID != 7 || service.cursorConversationID != 3 || service.cursorMessageID != 11 {
		t.Fatalf("cursor call=%d/%d/%d", service.cursorUserID, service.cursorConversationID, service.cursorMessageID)
	}
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data) != 3 || body.Data["conversation_id"] != float64(3) || body.Data["last_read_message_id"] != float64(11) || body.Data["unread_count"] != float64(2) {
		t.Fatalf("cursor response must contain exact state: %#v", body.Data)
	}
}

func TestReadCursorHandlerRequiresPositiveMessageID(t *testing.T) {
	router, service := newConversationCursorTestRouter()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/admin/v1/ai-conversations/3/read-cursor", bytes.NewBufferString(`{"message_id":0}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || service.cursorCalls != 0 {
		t.Fatalf("status=%d calls=%d body=%s", recorder.Code, service.cursorCalls, recorder.Body.String())
	}
}

func TestReadCursorRouteIsAuthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := adminroute.NewRegistry()
	Register(gin.New(), &fakeConversationHTTPService{}, registry)
	for _, definition := range registry.Definitions() {
		if definition.Method == http.MethodPut && definition.Path == "/api/admin/v1/ai-conversations/:id/read-cursor" {
			if definition.Access.Kind != adminroute.AccessAuthenticated {
				t.Fatalf("read cursor access=%+v", definition.Access)
			}
			return
		}
	}
	t.Fatal("read cursor route definition missing")
}

func newConversationCursorTestRouter() (*gin.Engine, *fakeConversationHTTPService) {
	gin.SetMode(gin.TestMode)
	service := &fakeConversationHTTPService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 7, Platform: "admin"})
	})
	Register(router, service)
	return router, service
}

type fakeConversationHTTPService struct {
	managementConversationHTTPService
	cursorCalls          int
	cursorUserID         int64
	cursorConversationID int64
	cursorMessageID      int64
}

type managementConversationHTTPService struct{}

var _ aiconversationmodule.HTTPService = managementConversationHTTPService{}

func (managementConversationHTTPService) List(context.Context, int64, aiconversationmodule.ListQuery) (*aiconversationmodule.ListResponse, *apperror.Error) {
	return &aiconversationmodule.ListResponse{}, nil
}
func (managementConversationHTTPService) Detail(context.Context, int64, int64) (*aiconversationmodule.ConversationDetail, *apperror.Error) {
	return &aiconversationmodule.ConversationDetail{}, nil
}
func (managementConversationHTTPService) Create(context.Context, int64, aiconversationmodule.CreateInput) (int64, *apperror.Error) {
	return 1, nil
}
func (managementConversationHTTPService) Update(context.Context, int64, int64, aiconversationmodule.UpdateInput) *apperror.Error {
	return nil
}
func (managementConversationHTTPService) Delete(context.Context, int64, int64) *apperror.Error {
	return nil
}
func (service *fakeConversationHTTPService) AdvanceReadCursor(_ context.Context, userID int64, conversationID int64, messageID int64) (*aiconversationmodule.ReadCursorResponse, *apperror.Error) {
	service.cursorCalls++
	service.cursorUserID = userID
	service.cursorConversationID = conversationID
	service.cursorMessageID = messageID
	return &aiconversationmodule.ReadCursorResponse{ConversationID: conversationID, LastReadMessageID: messageID, UnreadCount: 2}, nil
}

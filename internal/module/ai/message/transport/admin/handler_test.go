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
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

type acceptedMessageService struct {
	revisionUserID     int64
	revisionInput      aimessage.EditInput
	regenerationUserID int64
	regenerationInput  aimessage.RegenerateInput
	deleteUserID       int64
	deleteInput        aimessage.DeleteInput
}

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
		{name: "cancel 128", path: "/api/admin/v1/ai-conversations/3/messages/cancel", body: func(id string) string { return fmt.Sprintf(`{"request_id":"%s","delivered_seq":0}`, id) }, wantStatus: http.StatusOK},
		{name: "send 129", path: "/api/admin/v1/ai-conversations/3/messages", body: func(id string) string { return fmt.Sprintf(`{"content":"hello","request_id":"%s"}`, id) }, wantStatus: http.StatusBadRequest},
		{name: "cancel 129", path: "/api/admin/v1/ai-conversations/3/messages/cancel", body: func(id string) string { return fmt.Sprintf(`{"request_id":"%s","delivered_seq":0}`, id) }, wantStatus: http.StatusBadRequest},
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

func (acceptedMessageService) Cancel(_ context.Context, _ int64, input aimessage.CancelInput) (*aimessage.CancelResponse, *apperror.Error) {
	assistantID := int64(input.DeliveredSeq)
	return &aimessage.CancelResponse{
		ConversationID: 3, RequestID: input.RequestID, Status: string(replycommand.CancelStatusStopped),
		AssistantMessageID: &assistantID, SettlementPending: true,
	}, nil
}

func (s *acceptedMessageService) Revise(_ context.Context, userID int64, input aimessage.EditInput) (*aimessage.SendResponse, *apperror.Error) {
	s.revisionUserID, s.revisionInput = userID, input
	return &aimessage.SendResponse{ConversationID: input.ConversationID, UserMessageID: 71, CommandID: 81, RequestID: input.RequestID, State: replycommand.StatePending}, nil
}

func (s *acceptedMessageService) Regenerate(_ context.Context, userID int64, input aimessage.RegenerateInput) (*aimessage.SendResponse, *apperror.Error) {
	s.regenerationUserID, s.regenerationInput = userID, input
	return &aimessage.SendResponse{ConversationID: input.ConversationID, UserMessageID: 72, CommandID: 82, RequestID: input.RequestID, State: replycommand.StatePending}, nil
}

func (s *acceptedMessageService) DeleteMessages(_ context.Context, userID int64, input aimessage.DeleteInput) (*aimessage.DeleteResponse, *apperror.Error) {
	s.deleteUserID, s.deleteInput = userID, input
	return &aimessage.DeleteResponse{DeletedIDs: []int64{41, 63, 97}}, nil
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

func TestSendRejectsMaxTokensEvenWhenJSONIsForged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(acceptedMessageService{})
	router.POST("/api/admin/v1/ai-conversations/:id/messages", func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 7, Platform: "admin"})
		handler.Send(c)
	})

	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/ai-conversations/3/messages", strings.NewReader(`{"content":"hello","request_id":"request-1","max_tokens":4096}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestCancelReturnsStoppedMessageAndRejectsClientContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(acceptedMessageService{})
	router.POST("/api/admin/v1/ai-conversations/:id/messages/cancel", func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 7, Platform: "admin"})
		handler.Cancel(c)
	})

	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/ai-conversations/3/messages/cancel", strings.NewReader(`{"request_id":"request-1","delivered_seq":4}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	for _, field := range []string{`"conversation_id":3`, `"request_id":"request-1"`, `"status":"stopped"`, `"assistant_message_id":4`, `"settlement_pending":true`} {
		if !strings.Contains(recorder.Body.String(), field) {
			t.Fatalf("missing %s in %s", field, recorder.Body.String())
		}
	}

	for _, body := range []string{
		`{"request_id":"request-1"}`,
		`{"request_id":"request-1","delivered_seq":4,"content":"forged"}`,
	} {
		invalid := httptest.NewRequest(http.MethodPost, "/api/admin/v1/ai-conversations/3/messages/cancel", strings.NewReader(body))
		invalid.Header.Set("Content-Type", "application/json")
		invalidRecorder := httptest.NewRecorder()
		router.ServeHTTP(invalidRecorder, invalid)
		if invalidRecorder.Code != http.StatusBadRequest {
			t.Fatalf("body=%s status=%d response=%s", body, invalidRecorder.Code, invalidRecorder.Body.String())
		}
	}
}

func TestRevisionAndRegenerationHandlersUseAuthenticatedOwnerAndPathSource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &acceptedMessageService{}
	router := gin.New()
	handler := NewHandler(service)
	identity := func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 7, Platform: "admin"})
	}
	router.POST("/api/admin/v1/ai-conversations/:id/messages/:message_id/revisions", func(c *gin.Context) { identity(c); handler.Revise(c) })
	router.POST("/api/admin/v1/ai-conversations/:id/messages/:message_id/regenerations", func(c *gin.Context) { identity(c); handler.Regenerate(c) })

	revision := httptest.NewRequest(http.MethodPost, "/api/admin/v1/ai-conversations/3/messages/41/revisions", strings.NewReader(`{"content":"changed","request_id":"revision-1","user_id":999,"paired_message_id":999,"run_id":999}`))
	revision.Header.Set("Content-Type", "application/json")
	revisionRecorder := httptest.NewRecorder()
	router.ServeHTTP(revisionRecorder, revision)
	if revisionRecorder.Code != http.StatusAccepted {
		t.Fatalf("revision status=%d body=%s", revisionRecorder.Code, revisionRecorder.Body.String())
	}
	if service.revisionUserID != 7 || service.revisionInput.ConversationID != 3 || service.revisionInput.MessageID != 41 || service.revisionInput.Content != "changed" || service.revisionInput.RequestID != "revision-1" {
		t.Fatalf("revision input=%+v owner=%d", service.revisionInput, service.revisionUserID)
	}
	if service.revisionInput.Attachments != nil {
		t.Fatalf("omitted attachments must remain nil: %+v", service.revisionInput.Attachments)
	}

	regeneration := httptest.NewRequest(http.MethodPost, "/api/admin/v1/ai-conversations/3/messages/97/regenerations", strings.NewReader(`{"request_id":"regen-1","user_id":999,"paired_message_id":999,"run_id":999}`))
	regeneration.Header.Set("Content-Type", "application/json")
	regenerationRecorder := httptest.NewRecorder()
	router.ServeHTTP(regenerationRecorder, regeneration)
	if regenerationRecorder.Code != http.StatusAccepted {
		t.Fatalf("regeneration status=%d body=%s", regenerationRecorder.Code, regenerationRecorder.Body.String())
	}
	if service.regenerationUserID != 7 || service.regenerationInput.ConversationID != 3 || service.regenerationInput.AssistantMessageID != 97 || service.regenerationInput.RequestID != "regen-1" {
		t.Fatalf("regeneration input=%+v owner=%d", service.regenerationInput, service.regenerationUserID)
	}
}

func TestRevisionHandlerDistinguishesEmptyAndExplicitAttachmentsWithoutAcceptingETag(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name    string
		body    string
		wantLen int
	}{
		{name: "empty", body: `{"content":"changed","request_id":"revision-empty","attachments":[]}`, wantLen: 0},
		{name: "explicit", body: `{"content":"changed","request_id":"revision-file","attachments":[{"type":"file","object_key":"ai_chat_attachments/2026/07/report.pdf","mime_type":"application/pdf","url":"https://browser.test/report.pdf","name":"report.pdf","size":4096}]}`, wantLen: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &acceptedMessageService{}
			router := gin.New()
			handler := NewHandler(service)
			router.POST("/api/admin/v1/ai-conversations/:id/messages/:message_id/revisions", func(c *gin.Context) {
				c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 7, Platform: "admin"})
				handler.Revise(c)
			})
			request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/ai-conversations/3/messages/41/revisions", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusAccepted || service.revisionInput.Attachments == nil || len(*service.revisionInput.Attachments) != test.wantLen {
				t.Fatalf("status=%d body=%s input=%+v", recorder.Code, recorder.Body.String(), service.revisionInput)
			}
			if test.wantLen == 1 && (*service.revisionInput.Attachments)[0].ETag != "" {
				t.Fatalf("browser supplied an ETag: %+v", (*service.revisionInput.Attachments)[0])
			}
		})
	}
}

func TestDeleteMessagesHandlerReturnsExactSortedDeletedIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &acceptedMessageService{}
	router := gin.New()
	handler := NewHandler(service)
	router.DELETE("/api/admin/v1/ai-conversations/:id/messages", func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 7, Platform: "admin"})
		handler.DeleteMessages(c)
	})
	request := httptest.NewRequest(http.MethodDelete, "/api/admin/v1/ai-conversations/3/messages", strings.NewReader(`{"ids":[97,41,63]}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"deleted_ids":[41,63,97]`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.deleteUserID != 7 || service.deleteInput.ConversationID != 3 || fmt.Sprint(service.deleteInput.IDs) != "[97 41 63]" {
		t.Fatalf("delete input=%+v owner=%d", service.deleteInput, service.deleteUserID)
	}
}

func TestHistoryRoutesPublishAuthenticatedExplicitOperationMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := adminroute.NewRegistry()
	Register(gin.New(), &acceptedMessageService{}, registry)
	want := map[string]struct {
		method, operation, action string
		status                    int
	}{
		"/api/admin/v1/ai-conversations/:id/messages/:message_id/revisions":     {http.MethodPost, "post_api_admin_v1_ai_conversations_id_messages_message_id_revisions", "revise", http.StatusAccepted},
		"/api/admin/v1/ai-conversations/:id/messages/:message_id/regenerations": {http.MethodPost, "post_api_admin_v1_ai_conversations_id_messages_message_id_regenerations", "regenerate", http.StatusAccepted},
		"/api/admin/v1/ai-conversations/:id/messages":                           {http.MethodDelete, "delete_api_admin_v1_ai_conversations_id_messages", "delete", http.StatusOK},
	}
	for _, definition := range registry.Definitions() {
		expected, ok := want[definition.Path]
		if !ok || definition.Method != expected.method {
			continue
		}
		if definition.OperationID != expected.operation || definition.Access.Kind != adminroute.AccessAuthenticated || !definition.Audit.Enabled || definition.Audit.Module != "ai_message" || definition.Audit.Action != expected.action || definition.SuccessStatus != expected.status {
			t.Fatalf("route metadata=%+v", definition)
		}
		delete(want, definition.Path)
	}
	if len(want) != 0 {
		t.Fatalf("missing history routes: %v", want)
	}
}

package userquickentry

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/middleware"

	"github.com/gin-gonic/gin"
)

type fakeHTTPService struct {
	userID int64
	input  SaveInput
	result *SaveResponse
	err    *apperror.Error
}

func (f *fakeHTTPService) Save(ctx context.Context, userID int64, input SaveInput) (*SaveResponse, *apperror.Error) {
	f.userID = userID
	f.input = input
	return f.result, f.err
}

func TestHandlerSaveRequiresAuthIdentity(t *testing.T) {
	router := newQuickEntryTestRouter(&fakeHTTPService{}, nil)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/admin/v1/users/me/quick-entries", bytes.NewBufferString(`{"permission_ids":[1]}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := decodeQuickEntryBody(t, recorder)
	if body["msg"] != "Token无效或已过期" {
		t.Fatalf("unexpected message: %#v", body["msg"])
	}
}

func TestHandlerSaveUsesAuthUserAndReturnsQuickEntryField(t *testing.T) {
	service := &fakeHTTPService{result: &SaveResponse{QuickEntry: []QuickEntry{{ID: 7, PermissionID: 3, Sort: 1}}}}
	router := newQuickEntryTestRouter(service, &middleware.AuthIdentity{UserID: 44, SessionID: 9, Platform: "admin"})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/admin/v1/users/me/quick-entries", bytes.NewBufferString(`{"permission_ids":[3,1,3]}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.userID != 44 || !reflect.DeepEqual(service.input.PermissionIDs, []int64{3, 1, 3}) {
		t.Fatalf("service input mismatch: userID=%d input=%#v", service.userID, service.input)
	}
	body := decodeQuickEntryBody(t, recorder)
	data := body["data"].(map[string]any)
	if _, ok := data["quick_entry"]; !ok {
		t.Fatalf("missing quick_entry response field: %#v", data)
	}
}

func newQuickEntryTestRouter(service HTTPService, identity *middleware.AuthIdentity) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	if identity != nil {
		router.Use(func(c *gin.Context) {
			c.Set(middleware.ContextAuthIdentity, identity)
			c.Next()
		})
	}
	RegisterRoutes(router, service)
	return router
}

func decodeQuickEntryBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	return body
}

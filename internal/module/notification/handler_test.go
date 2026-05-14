package notification

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"admin_back_go/internal/apperror"
	projecti18n "admin_back_go/internal/i18n"
	"admin_back_go/internal/middleware"

	"github.com/gin-gonic/gin"
)

type fakeHTTPService struct {
	err *apperror.Error
}

func (f *fakeHTTPService) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return &InitResponse{}, f.err
}

func (f *fakeHTTPService) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	return &ListResponse{Page: Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize}}, f.err
}

func (f *fakeHTTPService) UnreadCount(ctx context.Context, identity Identity) (*UnreadCountResponse, *apperror.Error) {
	return &UnreadCountResponse{}, f.err
}

func (f *fakeHTTPService) MarkRead(ctx context.Context, identity Identity, ids []int64) *apperror.Error {
	return f.err
}

func (f *fakeHTTPService) Delete(ctx context.Context, identity Identity, ids []int64) *apperror.Error {
	return f.err
}

func TestHandlerListLocalizesInvalidQuery(t *testing.T) {
	router := newNotificationLocalizedTestRouter(&fakeHTTPService{}, &middleware.AuthIdentity{UserID: 12, Platform: "admin"})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/notifications?current_page=abc", nil)
	request.Header.Set("Accept-Language", "en-US")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := decodeNotificationBody(t, recorder)
	if body["msg"] != "Invalid notification list request" {
		t.Fatalf("expected localized list query error, got %#v", body["msg"])
	}
}

func TestHandlerListLocalizesMissingIdentity(t *testing.T) {
	router := newNotificationLocalizedTestRouter(&fakeHTTPService{}, nil)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/notifications?current_page=1&page_size=20", nil)
	request.Header.Set("Accept-Language", "en-US")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := decodeNotificationBody(t, recorder)
	if body["msg"] != "Token is invalid or expired" {
		t.Fatalf("expected localized token error, got %#v", body["msg"])
	}
}

func newNotificationLocalizedTestRouter(service HTTPService, identity *middleware.AuthIdentity) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(projecti18n.Localize())
	if identity != nil {
		router.Use(func(c *gin.Context) {
			c.Set(middleware.ContextAuthIdentity, identity)
			c.Next()
		})
	}
	RegisterRoutes(router, service)
	return router
}

func decodeNotificationBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	return body
}

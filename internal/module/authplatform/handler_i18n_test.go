package authplatform

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	projecti18n "admin_back_go/internal/i18n"

	"github.com/gin-gonic/gin"
)

func TestHandlerLocalizesCreateRequestError(t *testing.T) {
	router, service := newLocalizedAuthPlatformHandlerRouter()

	body := `{"code":"mini","name":"小程序","login_types":["password"],"captcha_type":"click","access_ttl":3600,"refresh_ttl":86400,"bind_platform":1,"bind_device":2,"bind_ip":2,"single_session":1,"max_sessions":1,"allow_register":1}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/auth-platforms", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept-Language", "en-US")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, recorder.Code, recorder.Body.String())
	}
	if service.createInput.Code != "" {
		t.Fatalf("service should not be called for invalid request: %#v", service.createInput)
	}

	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["msg"] != "Invalid create request" {
		t.Fatalf("expected localized create request error, got %#v", payload["msg"])
	}
}

func newLocalizedAuthPlatformHandlerRouter() (*gin.Engine, *fakeHTTPService) {
	gin.SetMode(gin.TestMode)
	service := &fakeHTTPService{}
	router := gin.New()
	router.Use(projecti18n.Localize())
	RegisterRoutes(router, service)
	return router, service
}

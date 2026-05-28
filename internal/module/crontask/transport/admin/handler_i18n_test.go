package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	projecti18n "admin_back_go/internal/shared/i18n"

	"github.com/gin-gonic/gin"
)

func TestCronTaskHandlerLocalizesListRequestError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(projecti18n.Localize())
	RegisterRoutes(router, &fakeCronHTTPService{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/cron-tasks?current_page=1&page_size=20&status=bad", nil)
	request.Header.Set("Accept-Language", "en-US")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, recorder.Code, recorder.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["msg"] != "Invalid cron task list request" {
		t.Fatalf("expected localized list request error, got %#v", payload["msg"])
	}
}

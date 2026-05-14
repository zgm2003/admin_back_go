package system

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	projecti18n "admin_back_go/internal/i18n"
	"admin_back_go/internal/readiness"

	"github.com/gin-gonic/gin"
)

func TestReadyLocalizesNotReadyMessage(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(projecti18n.Localize())
	RegisterRoutes(router, fakeReadinessChecker{report: readiness.NewReport(map[string]readiness.Check{
		"database": {Status: readiness.StatusDown, Message: "db down"},
	})})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ready", nil)
	request.Header.Set("Accept-Language", "en-US")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if body["msg"] != "Service is not ready" {
		t.Fatalf("expected localized readiness message, got %#v", body["msg"])
	}
}

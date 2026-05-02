package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"admin_back_go/internal/apperror"

	"github.com/gin-gonic/gin"
)

func TestErrorWritesUnifiedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	Error(ctx, apperror.Forbidden("无权限访问"))

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected http status %d, got %d", http.StatusForbidden, recorder.Code)
	}

	body := decodeBody(t, recorder)
	if body["code"] != float64(403) {
		t.Fatalf("expected code 403, got %#v", body["code"])
	}
	if body["msg"] != "无权限访问" {
		t.Fatalf("expected msg 无权限访问, got %#v", body["msg"])
	}
	if data, ok := body["data"].(map[string]any); !ok || len(data) != 0 {
		t.Fatalf("expected empty data object, got %#v", body["data"])
	}
}

func TestErrorWithDataWritesProvidedData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	ErrorWithData(ctx, apperror.Internal("service not ready"), gin.H{"status": "not_ready"})

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected http status %d, got %d", http.StatusInternalServerError, recorder.Code)
	}

	body := decodeBody(t, recorder)
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %#v", body["data"])
	}
	if data["status"] != "not_ready" {
		t.Fatalf("expected data.status not_ready, got %#v", data["status"])
	}
}

func TestAbortWritesErrorAndStopsChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		Abort(c, apperror.Unauthorized("缺少Token"))
	})
	router.GET("/", func(c *gin.Context) {
		t.Fatalf("handler should not run after abort")
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected http status %d, got %d", http.StatusUnauthorized, recorder.Code)
	}

	body := decodeBody(t, recorder)
	if body["code"] != float64(401) {
		t.Fatalf("expected code 401, got %#v", body["code"])
	}
}

func decodeBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	return body
}

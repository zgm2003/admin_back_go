package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/response"
	"admin_back_go/internal/telemetry"

	"github.com/gin-gonic/gin"
)

func TestAccessLogWritesStructuredRequestFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))
	router := gin.New()
	router.Use(RequestID())
	router.Use(AccessLog(logger))
	router.GET("/logged", func(c *gin.Context) {
		c.String(http.StatusCreated, "created")
	})

	request := httptest.NewRequest(http.MethodPost, "/logged", nil)
	request.Header.Set(HeaderRequestID, "rid-test")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected gin to reject wrong method before handler, got %d", recorder.Code)
	}

	entry := decodeLogEntry(t, buffer.Bytes())
	if entry["msg"] != "http request" {
		t.Fatalf("expected http request log message, got %#v", entry["msg"])
	}
	if entry["request_id"] != "rid-test" {
		t.Fatalf("expected request_id rid-test, got %#v", entry["request_id"])
	}
	if entry["method"] != http.MethodPost {
		t.Fatalf("expected method POST, got %#v", entry["method"])
	}
	if entry["path"] != "/logged" {
		t.Fatalf("expected path /logged, got %#v", entry["path"])
	}
	if entry["status"] != float64(http.StatusNotFound) {
		t.Fatalf("expected status 404, got %#v", entry["status"])
	}
}

func TestAccessLogHandlesNilLogger(t *testing.T) {
	if AccessLog(nil) == nil {
		t.Fatalf("expected middleware when logger is nil")
	}
}

func TestAccessLogRecordsBoundedRouteTelemetry(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := telemetry.NewMemoryRecorder()
	router := gin.New()
	router.Use(AccessLog(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), recorder))
	router.GET("/users/:id", func(c *gin.Context) {
		response.Error(c, apperror.Wrap(
			"dependency.ai_provider",
			apperror.CategoryDependency,
			http.StatusServiceUnavailable,
			apperror.Retryable,
			"common.dependency_unavailable",
			nil,
			"服务暂不可用",
			errors.New("private provider payload"),
		))
	})

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/users/42?token=private", nil))

	events := recorder.Events()
	if len(events) != 2 {
		t.Fatalf("expected request count and duration, got %+v", events)
	}
	for _, event := range events {
		if event.Attributes["http.route"] != "/users/:id" || event.Attributes["http.method"] != http.MethodGet {
			t.Fatalf("unbounded HTTP attributes: %+v", event)
		}
		if event.Attributes["http.status"] != "503" || event.Attributes["error.code"] != "dependency.ai_provider" {
			t.Fatalf("missing HTTP outcome: %+v", event)
		}
	}
	if text := strings.ToLower(fmtEvents(events)); strings.Contains(text, "private") || strings.Contains(text, "token") || strings.Contains(text, "/users/42") {
		t.Fatalf("request secret or concrete path leaked: %s", text)
	}
}

func fmtEvents(events []telemetry.Event) string {
	data, _ := json.Marshal(events)
	return string(data)
}

func decodeLogEntry(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(data), &entry); err != nil {
		t.Fatalf("invalid json log entry: %v\n%s", err, data)
	}
	return entry
}

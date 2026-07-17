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

	"github.com/gin-gonic/gin"
)

func TestErrorReporterLogsServerCauseOnceWithCorrelation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))
	router := gin.New()
	router.Use(ErrorReporter(logger))
	router.GET("/probe", func(c *gin.Context) {
		c.Set(ContextRequestID, "req-1")
		c.Set("trace_id", "trace-1")
		c.Set("task_id", "task-1")
		c.Set("run_id", "run-1")
		response.Error(c, apperror.Wrap(
			"dependency.mysql",
			apperror.CategoryDependency,
			http.StatusServiceUnavailable,
			apperror.Retryable,
			"common.dependency_unavailable",
			nil,
			"服务暂不可用",
			errors.New("dial mysql user:secret@tcp(private:3306)"),
		))
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/probe", nil))

	lines := strings.Split(strings.TrimSpace(buffer.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected one failure log, got %d: %s", len(lines), buffer.String())
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("invalid reporter log: %v\n%s", err, lines[0])
	}
	if entry["request_id"] != "req-1" || entry["trace_id"] != "trace-1" || entry["task_id"] != "task-1" || entry["run_id"] != "run-1" {
		t.Fatalf("missing correlation fields: %#v", entry)
	}
	if entry["error_code"] != "dependency.mysql" || entry["category"] != "dependency" {
		t.Fatalf("missing classification: %#v", entry)
	}
	if strings.Contains(strings.ToLower(buffer.String()), "secret") {
		t.Fatalf("secret leaked in reporter log: %s", buffer.String())
	}
}

func TestErrorReporterDoesNotLogExpectedClientFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))
	router := gin.New()
	router.Use(ErrorReporter(logger))
	router.GET("/probe", func(c *gin.Context) {
		response.Error(c, apperror.BadRequest("参数错误"))
	})

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/probe", nil))
	if buffer.Len() != 0 {
		t.Fatalf("4xx should not be reported as a server failure: %s", buffer.String())
	}
}

func TestRedactReporterAttributesDropsSensitiveValues(t *testing.T) {
	got := redactReporterAttributes(map[string]any{
		"request_id":      "req-1",
		"authorization":   "Bearer token-value",
		"session_token":   "token-value",
		"cookie":          "access_token=token-value",
		"password":        "password-value",
		"client_secret":   "secret-value",
		"certificate_pem": "certificate-value",
		"prompt":          "private prompt",
		"payload":         map[string]any{"safe": false},
	})

	if got["request_id"] != "req-1" {
		t.Fatalf("safe value missing: %#v", got)
	}
	for key, value := range got {
		if key == "request_id" {
			continue
		}
		if value != redactedValue {
			t.Fatalf("%s was not redacted: %#v", key, value)
		}
	}
}

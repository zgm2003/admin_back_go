package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

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

func decodeLogEntry(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(data), &entry); err != nil {
		t.Fatalf("invalid json log entry: %v\n%s", err, data)
	}
	return entry
}

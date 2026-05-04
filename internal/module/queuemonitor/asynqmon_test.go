package queuemonitor

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin_back_go/internal/config"
	"admin_back_go/internal/platform/taskqueue"
)

func TestNewMonitorUIRejectsMissingRedisAddr(t *testing.T) {
	monitor, err := NewMonitorUI(config.RedisConfig{}, config.QueueConfig{RedisDB: 3})
	if err == nil {
		t.Fatalf("expected missing redis addr error")
	}
	if monitor != nil {
		t.Fatalf("expected nil monitor on error")
	}
	if !errors.Is(err, taskqueue.ErrRedisAddrRequired) {
		t.Fatalf("expected ErrRedisAddrRequired, got %v", err)
	}
	if !IsUIConfigError(err) {
		t.Fatalf("expected config error predicate to match")
	}
}

func TestMonitorUIUsesOfficialReadOnlyAsynqmonRoute(t *testing.T) {
	monitor, err := NewMonitorUI(
		config.RedisConfig{Addr: "127.0.0.1:6379"},
		config.QueueConfig{RedisDB: 3},
	)
	if err != nil {
		t.Fatalf("NewMonitorUI returned error: %v", err)
	}
	defer monitor.Close()

	if monitor.RootPath() != UIPath {
		t.Fatalf("expected root path %q, got %q", UIPath, monitor.RootPath())
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, UIPath+"/api/queues/critical:pause", strings.NewReader("{}"))
	monitor.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected read-only asynqmon to reject POST, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "read-only mode") {
		t.Fatalf("expected read-only response, got %q", recorder.Body.String())
	}
}

func TestMonitorUIServesRootPageOnWindowsPath(t *testing.T) {
	monitor, err := NewMonitorUI(
		config.RedisConfig{Addr: "127.0.0.1:6379"},
		config.QueueConfig{RedisDB: 3},
	)
	if err != nil {
		t.Fatalf("NewMonitorUI returned error: %v", err)
	}
	defer monitor.Close()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, UIPath, nil)
	monitor.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected UI root status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "Asynq - Monitoring") {
		t.Fatalf("expected asynqmon index html, got %q", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `window.FLAG_ROOT_PATH="\/api\/admin\/v1\/queue-monitor-ui"`) {
		t.Fatalf("expected rendered root path in index html")
	}
}

func TestMonitorUIServesEmbeddedStaticAssetOnWindows(t *testing.T) {
	monitor, err := NewMonitorUI(
		config.RedisConfig{Addr: "127.0.0.1:6379"},
		config.QueueConfig{RedisDB: 3},
	)
	if err != nil {
		t.Fatalf("NewMonitorUI returned error: %v", err)
	}
	defer monitor.Close()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, UIPath+"/static/js/runtime-main.9fea6c1a.js", nil)
	monitor.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected asset status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.Contains(contentType, "javascript") {
		t.Fatalf("expected javascript content type, got %q", contentType)
	}
	if !strings.Contains(recorder.Body.String(), "webpackJsonpui") {
		t.Fatalf("expected runtime js asset, got %q", recorder.Body.String())
	}
}

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/config"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/session"
	"admin_back_go/internal/readiness"
)

type fakeReadinessChecker struct {
	report readiness.Report
}

func (f fakeReadinessChecker) Readiness(ctx context.Context) readiness.Report {
	return f.report
}

type fakeAuthService struct{}

func (fakeAuthService) Refresh(ctx context.Context, input session.RefreshInput) (*session.TokenResult, *apperror.Error) {
	return &session.TokenResult{
		AccessToken:      "new-access",
		RefreshToken:     "new-refresh",
		ExpiresIn:        14400,
		RefreshExpiresIn: 1209600,
	}, nil
}

func (fakeAuthService) Logout(ctx context.Context, accessToken string) *apperror.Error {
	return nil
}

func TestHealthEndpointReturnsOK(t *testing.T) {
	router := newTestRouter(t, Dependencies{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	router.ServeHTTP(recorder, request)

	assertRequestID(t, recorder)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	body := decodeRouterBody(t, recorder)
	if body["code"] != float64(0) {
		t.Fatalf("expected code 0, got %#v", body["code"])
	}
	if body["msg"] != "ok" {
		t.Fatalf("expected msg ok, got %#v", body["msg"])
	}

	data := mustRouterData(t, body)
	if data["status"] != "ok" {
		t.Fatalf("expected data.status ok, got %#v", data["status"])
	}
}

func TestPingEndpointReturnsPong(t *testing.T) {
	router := newTestRouter(t, Dependencies{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	router.ServeHTTP(recorder, request)

	assertRequestID(t, recorder)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	body := decodeRouterBody(t, recorder)
	data := mustRouterData(t, body)
	if data["message"] != "pong" {
		t.Fatalf("expected data.message pong, got %#v", data["message"])
	}
}

func TestReadyEndpointReturnsReadyWhenResourcesAreDisabled(t *testing.T) {
	router := newTestRouter(t, Dependencies{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ready", nil)
	router.ServeHTTP(recorder, request)

	assertRequestID(t, recorder)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	body := decodeRouterBody(t, recorder)
	if body["code"] != float64(0) {
		t.Fatalf("expected code 0, got %#v", body["code"])
	}

	data := mustRouterData(t, body)
	if data["status"] != readiness.StatusReady {
		t.Fatalf("expected ready status, got %#v", data["status"])
	}
	checks, ok := data["checks"].(map[string]any)
	if !ok {
		t.Fatalf("expected checks object, got %#v", data["checks"])
	}
	database, ok := checks["database"].(map[string]any)
	if !ok || database["status"] != readiness.StatusDisabled {
		t.Fatalf("expected disabled database check, got %#v", checks["database"])
	}
}

func TestReadyEndpointReturnsErrorWithDetailsWhenResourceIsDown(t *testing.T) {
	router := newTestRouter(t, Dependencies{Readiness: fakeReadinessChecker{report: readiness.NewReport(map[string]readiness.Check{
		"database": {Status: readiness.StatusDown, Message: "connection refused"},
		"redis":    {Status: readiness.StatusDisabled},
	})}})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ready", nil)
	router.ServeHTTP(recorder, request)

	assertRequestID(t, recorder)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, recorder.Code)
	}

	body := decodeRouterBody(t, recorder)
	if body["code"] != float64(500) {
		t.Fatalf("expected code 500, got %#v", body["code"])
	}
	if body["msg"] != "service not ready" {
		t.Fatalf("expected service not ready message, got %#v", body["msg"])
	}

	data := mustRouterData(t, body)
	if data["status"] != readiness.StatusNotReady {
		t.Fatalf("expected not_ready status, got %#v", data["status"])
	}
}

func TestRouterInstallsAccessLogAfterRequestID(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))
	router := NewRouter(Dependencies{Logger: logger})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.Header.Set(middleware.HeaderRequestID, "rid-router")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	entry := decodeRouterLogEntry(t, buffer.Bytes())
	if entry["msg"] != "http request" {
		t.Fatalf("expected http request log message, got %#v", entry["msg"])
	}
	if entry["request_id"] != "rid-router" {
		t.Fatalf("expected request_id rid-router, got %#v", entry["request_id"])
	}
	if entry["method"] != http.MethodGet {
		t.Fatalf("expected method GET, got %#v", entry["method"])
	}
	if entry["path"] != "/health" {
		t.Fatalf("expected path /health, got %#v", entry["path"])
	}
	if entry["status"] != float64(http.StatusOK) {
		t.Fatalf("expected status 200, got %#v", entry["status"])
	}
}

func TestRouterInstallsCORSAfterAccessLog(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))
	router := NewRouter(Dependencies{
		Logger: logger,
		CORS: config.CORSConfig{
			AllowOrigins:  []string{"http://localhost:5173"},
			AllowMethods:  []string{http.MethodGet, http.MethodOptions},
			AllowHeaders:  []string{"Content-Type", "Authorization", "platform", "device-id", "X-Trace-Id", middleware.HeaderRequestID},
			ExposeHeaders: []string{middleware.HeaderRequestID},
			MaxAge:        12 * time.Hour,
		},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/health", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	request.Header.Set("Access-Control-Request-Method", http.MethodGet)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("expected allowed origin, got %q", got)
	}

	entry := decodeRouterLogEntry(t, buffer.Bytes())
	if entry["msg"] != "http request" {
		t.Fatalf("expected http request log message, got %#v", entry["msg"])
	}
	if entry["status"] != float64(http.StatusNoContent) {
		t.Fatalf("expected access log status 204, got %#v", entry["status"])
	}
}

func TestRouterInstallsAuthTokenForNonPublicPaths(t *testing.T) {
	router := newTestRouter(t, Dependencies{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/private", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusUnauthorized, recorder.Code, recorder.Body.String())
	}
	body := decodeRouterBody(t, recorder)
	if body["code"] != float64(401) {
		t.Fatalf("expected code 401, got %#v", body["code"])
	}
	if body["msg"] != "缺少Token" {
		t.Fatalf("expected missing token message, got %#v", body["msg"])
	}
}

func TestRouterInstallsRefreshEndpointAsPublicPath(t *testing.T) {
	router := newTestRouter(t, Dependencies{AuthService: fakeAuthService{}})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", strings.NewReader(`{"refresh_token":"refresh-token"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	body := decodeRouterBody(t, recorder)
	data := mustRouterData(t, body)
	if data["access_token"] != "new-access" {
		t.Fatalf("expected refresh endpoint response, got %#v", data)
	}
}

func newTestRouter(t *testing.T, deps Dependencies) http.Handler {
	t.Helper()
	if deps.Logger == nil {
		deps.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return NewRouter(deps)
}

func decodeRouterBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	return body
}

func decodeRouterLogEntry(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(data), &entry); err != nil {
		t.Fatalf("invalid json log entry: %v\n%s", err, data)
	}
	return entry
}

func mustRouterData(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %#v", body["data"])
	}
	return data
}

func assertRequestID(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder.Header().Get("X-Request-Id") == "" {
		t.Fatalf("expected X-Request-Id header")
	}
}

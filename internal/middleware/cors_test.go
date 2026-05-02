package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/config"

	"github.com/gin-gonic/gin"
)

func TestCORSAllowsConfiguredFrontendPreflight(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handlerRan := false
	router := gin.New()
	router.Use(CORS(config.CORSConfig{
		AllowOrigins:     []string{"http://localhost:5173"},
		AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowHeaders:     []string{"Content-Type", "Authorization", "platform", "device-id", "X-Trace-Id", HeaderRequestID},
		ExposeHeaders:    []string{HeaderRequestID},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))
	router.POST("/api/v1/ping", func(c *gin.Context) {
		handlerRan = true
		c.String(http.StatusOK, "pong")
	})

	request := httptest.NewRequest(http.MethodOptions, "/api/v1/ping", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "Authorization, platform, device-id, X-Trace-Id")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if handlerRan {
		t.Fatalf("preflight should not reach route handler")
	}
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("expected allowed origin, got %q", got)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("expected credentials true, got %q", got)
	}
	allowHeaders := recorder.Header().Get("Access-Control-Allow-Headers")
	for _, header := range []string{"Authorization", "platform", "device-id", "X-Trace-Id"} {
		if !strings.Contains(strings.ToLower(allowHeaders), strings.ToLower(header)) {
			t.Fatalf("expected allow headers to contain %s, got %q", header, allowHeaders)
		}
	}
}

func TestCORSExposesRequestIDOnActualRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(RequestID())
	router.Use(CORS(config.CORSConfig{
		AllowOrigins:  []string{"http://localhost:5173"},
		AllowMethods:  []string{http.MethodGet, http.MethodOptions},
		AllowHeaders:  []string{"Content-Type", HeaderRequestID},
		ExposeHeaders: []string{HeaderRequestID},
		MaxAge:        time.Hour,
	}))
	router.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("expected allowed origin, got %q", got)
	}
	if got := recorder.Header().Get("Access-Control-Expose-Headers"); !strings.Contains(got, HeaderRequestID) {
		t.Fatalf("expected exposed X-Request-Id, got %q", got)
	}
}

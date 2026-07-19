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
	router.Use(CORS(config.DefaultCORSConfig()))
	router.POST("/api/admin/v1/ping", func(c *gin.Context) {
		handlerRan = true
		c.String(http.StatusOK, "pong")
	})

	request := httptest.NewRequest(http.MethodOptions, "/api/admin/v1/ping", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "Authorization, platform, device-id, X-Trace-Id, X-Request-Id, Accept-Language")
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
	if got := recorder.Header().Get("Access-Control-Max-Age"); got != "43200" {
		t.Fatalf("expected max age 43200, got %q", got)
	}
	allowHeaders := recorder.Header().Get("Access-Control-Allow-Headers")
	for _, header := range []string{"Authorization", "platform", "device-id", "X-Trace-Id", "X-Request-Id", "Accept-Language"} {
		if !strings.Contains(strings.ToLower(allowHeaders), strings.ToLower(header)) {
			t.Fatalf("expected allow headers to contain %s, got %q", header, allowHeaders)
		}
	}
}

func TestDefaultCORSRejects5174Preflight(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(CORS(config.DefaultCORSConfig()))
	router.POST("/api/admin/v1/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	request := httptest.NewRequest(http.MethodOptions, "/api/admin/v1/ping", nil)
	request.Header.Set("Origin", "http://localhost:5174")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status %d for non-default 5174 origin, got %d", http.StatusForbidden, recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no allow-origin for 5174, got %q", got)
	}
}

func TestBrowserOnlyCORSDoesNotAdvertiseRetiredClientVariantHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORS(config.DefaultCORSConfig()))
	router.POST("/api/admin/v1/auth/login", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	request := httptest.NewRequest(http.MethodOptions, "/api/admin/v1/auth/login", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "X-Admin-Client-Variant")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected preflight status %d, got %d", http.StatusNoContent, recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Headers"); strings.Contains(strings.ToLower(got), strings.ToLower("X-Admin-Client-Variant")) {
		t.Fatalf("retired client variant header is still advertised: %q", got)
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

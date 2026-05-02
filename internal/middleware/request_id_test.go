package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestIDGeneratesHeaderAndContextValue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID())
	router.GET("/", func(c *gin.Context) {
		requestID := GetRequestID(c)
		if requestID == "" {
			t.Fatalf("expected request id in gin context")
		}
		c.String(http.StatusOK, requestID)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if recorder.Header().Get(HeaderRequestID) == "" {
		t.Fatalf("expected %s response header", HeaderRequestID)
	}
}

func TestRequestIDKeepsIncomingHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID())
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, GetRequestID(c))
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(HeaderRequestID, "client-request-id")
	router.ServeHTTP(recorder, request)

	if got := recorder.Header().Get(HeaderRequestID); got != "client-request-id" {
		t.Fatalf("expected incoming request id to be preserved, got %q", got)
	}
	if got := recorder.Body.String(); got != "client-request-id" {
		t.Fatalf("expected context request id to match incoming header, got %q", got)
	}
}

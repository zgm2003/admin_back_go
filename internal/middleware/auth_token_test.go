package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"admin_back_go/internal/apperror"

	"github.com/gin-gonic/gin"
)

func TestAuthTokenSkipsPublicPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(AuthToken(AuthTokenConfig{
		SkipPaths: map[string]struct{}{"/health": {}},
	}))
	router.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
}

func TestAuthTokenRejectsMissingBearer(t *testing.T) {
	router := newAuthTokenTestRouter(AuthTokenConfig{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	router.ServeHTTP(recorder, request)

	assertJSONError(t, recorder, http.StatusUnauthorized, apperror.CodeUnauthorized, "缺少Token")
}

func TestAuthTokenRejectsMalformedBearer(t *testing.T) {
	router := newAuthTokenTestRouter(AuthTokenConfig{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Token abc")
	router.ServeHTTP(recorder, request)

	assertJSONError(t, recorder, http.StatusUnauthorized, apperror.CodeUnauthorized, "Token格式错误")
}

func TestAuthTokenRejectsMissingAuthenticator(t *testing.T) {
	router := newAuthTokenTestRouter(AuthTokenConfig{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer abc")
	router.ServeHTTP(recorder, request)

	assertJSONError(t, recorder, http.StatusUnauthorized, apperror.CodeUnauthorized, "Token认证未配置")
}

func TestAuthTokenStoresIdentityReturnedByAuthenticator(t *testing.T) {
	var gotInput TokenInput
	router := newAuthTokenTestRouter(AuthTokenConfig{
		Authenticator: func(ctx context.Context, input TokenInput) (*AuthIdentity, *apperror.Error) {
			gotInput = input
			return &AuthIdentity{
				UserID:    12,
				SessionID: 34,
				Platform:  "admin-from-session",
			}, nil
		},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer valid-token")
	request.Header.Set("platform", "app-from-header")
	request.Header.Set("device-id", "device-1")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if gotInput.AccessToken != "valid-token" {
		t.Fatalf("expected token valid-token, got %q", gotInput.AccessToken)
	}
	if gotInput.Platform != "app-from-header" {
		t.Fatalf("expected platform header to be passed to authenticator, got %q", gotInput.Platform)
	}
	if gotInput.DeviceID != "device-1" {
		t.Fatalf("expected device id device-1, got %q", gotInput.DeviceID)
	}
	if recorder.Body.String() != "12|34|admin-from-session" {
		t.Fatalf("expected identity from authenticator, got %q", recorder.Body.String())
	}
}

func newAuthTokenTestRouter(cfg AuthTokenConfig) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(AuthToken(cfg))
	router.GET("/protected", func(c *gin.Context) {
		identity := GetAuthIdentity(c)
		if identity == nil {
			c.String(http.StatusInternalServerError, "missing identity")
			return
		}
		c.String(http.StatusOK, "%d|%d|%s", identity.UserID, identity.SessionID, identity.Platform)
	})
	return router
}

func assertJSONError(t *testing.T, recorder *httptest.ResponseRecorder, httpStatus int, code int, msg string) {
	t.Helper()
	if recorder.Code != httpStatus {
		t.Fatalf("expected http status %d, got %d body=%s", httpStatus, recorder.Code, recorder.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if body["code"] != float64(code) {
		t.Fatalf("expected code %d, got %#v", code, body["code"])
	}
	if body["msg"] != msg {
		t.Fatalf("expected msg %q, got %#v", msg, body["msg"])
	}
}

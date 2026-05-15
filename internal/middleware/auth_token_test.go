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

func TestDefaultAuthSkipPathsDoNotExposePaymentNotifyInConfigOnlySlice(t *testing.T) {
	paths := DefaultAuthSkipPaths()
	if _, ok := paths["/api/payment/notify/alipay"]; ok {
		t.Fatalf("payment notify is not public in the payment order Alipay pay v1 slice")
	}
	if _, ok := paths["/api/pay/notify/alipay"]; ok {
		t.Fatalf("legacy pay notify path must not remain public by default")
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

func TestAuthTokenAllowsCookieOnlyForConfiguredReadOnlyPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var gotInput TokenInput
	router := gin.New()
	router.Use(AuthToken(AuthTokenConfig{
		Authenticator: func(ctx context.Context, input TokenInput) (*AuthIdentity, *apperror.Error) {
			gotInput = input
			return &AuthIdentity{UserID: 12, SessionID: 34, Platform: "admin"}, nil
		},
		CookieTokenPath: CookieTokenPathConfig{
			PathPrefixes: []string{"/api/admin/v1/queue-monitor-ui"},
			Platform:     "admin",
		},
	}))
	router.GET("/api/admin/v1/queue-monitor-ui", func(c *gin.Context) {
		c.String(http.StatusOK, "monitor")
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/queue-monitor-ui", nil)
	request.AddCookie(&http.Cookie{Name: DefaultAccessTokenCookie, Value: "cookie-token"})
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if gotInput.AccessToken != "cookie-token" {
		t.Fatalf("expected cookie token, got %q", gotInput.AccessToken)
	}
	if gotInput.Platform != "admin" {
		t.Fatalf("expected configured cookie platform admin, got %q", gotInput.Platform)
	}
}

func TestAuthTokenDoesNotDefaultPlatformForBearerRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var gotInput TokenInput
	router := gin.New()
	router.Use(AuthToken(AuthTokenConfig{
		Authenticator: func(ctx context.Context, input TokenInput) (*AuthIdentity, *apperror.Error) {
			gotInput = input
			return &AuthIdentity{UserID: 12, SessionID: 34, Platform: "admin"}, nil
		},
		CookieTokenPath: CookieTokenPathConfig{
			PathPrefixes: []string{"/api/admin/v1/queue-monitor-ui"},
			Platform:     "admin",
		},
	}))
	router.GET("/api/admin/v1/queue-monitor-ui", func(c *gin.Context) {
		c.String(http.StatusOK, "monitor")
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/queue-monitor-ui", nil)
	request.Header.Set("Authorization", "Bearer bearer-token")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if gotInput.AccessToken != "bearer-token" {
		t.Fatalf("expected bearer token, got %q", gotInput.AccessToken)
	}
	if gotInput.Platform != "" {
		t.Fatalf("expected bearer request without platform to stay empty, got %q", gotInput.Platform)
	}
}

func TestAuthTokenDoesNotUseCookieForNormalAPIPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(AuthToken(AuthTokenConfig{
		Authenticator: func(ctx context.Context, input TokenInput) (*AuthIdentity, *apperror.Error) {
			t.Fatalf("authenticator should not be called")
			return nil, nil
		},
		CookieTokenPath: CookieTokenPathConfig{
			PathPrefixes: []string{"/api/admin/v1/queue-monitor-ui"},
		},
	}))
	router.GET("/api/admin/v1/users/me", func(c *gin.Context) {
		c.String(http.StatusOK, "me")
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/users/me", nil)
	request.AddCookie(&http.Cookie{Name: DefaultAccessTokenCookie, Value: "cookie-token"})
	router.ServeHTTP(recorder, request)

	assertJSONError(t, recorder, http.StatusUnauthorized, apperror.CodeUnauthorized, "缺少Token")
}

func TestAuthTokenDoesNotUseCookieForMutatingRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(AuthToken(AuthTokenConfig{
		Authenticator: func(ctx context.Context, input TokenInput) (*AuthIdentity, *apperror.Error) {
			t.Fatalf("authenticator should not be called")
			return nil, nil
		},
		CookieTokenPath: CookieTokenPathConfig{
			PathPrefixes: []string{"/api/admin/v1/queue-monitor-ui"},
		},
	}))
	router.POST("/api/admin/v1/queue-monitor-ui/api/queues/critical:pause", func(c *gin.Context) {
		c.String(http.StatusOK, "pause")
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/queue-monitor-ui/api/queues/critical:pause", nil)
	request.AddCookie(&http.Cookie{Name: DefaultAccessTokenCookie, Value: "cookie-token"})
	router.ServeHTTP(recorder, request)

	assertJSONError(t, recorder, http.StatusUnauthorized, apperror.CodeUnauthorized, "缺少Token")
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

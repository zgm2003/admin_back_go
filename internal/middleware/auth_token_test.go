package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"

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

func TestDefaultAuthSkipPathsExposeOnlyCanonicalPaymentCallback(t *testing.T) {
	paths := DefaultAuthSkipPaths()
	if _, ok := paths["/api/payment/callbacks/alipay"]; !ok {
		t.Fatalf("canonical payment callback must be public")
	}
	if _, ok := paths["/api/payment/notify/alipay"]; ok {
		t.Fatalf("old payment notify path must not remain public")
	}
	if _, ok := paths["/api/pay/notify/alipay"]; ok {
		t.Fatalf("legacy pay notify path must not remain public by default")
	}
}

func TestAuthTokenDefaultSkipPathsExcludeLegacyUsersRoutes(t *testing.T) {
	paths := DefaultAuthSkipPaths()
	legacyUsersPrefix := "/api/" + "Users"
	for _, path := range []string{
		legacyUsersPrefix + "/getLoginConfig",
		legacyUsersPrefix + "/sendCode",
		legacyUsersPrefix + "/login",
		legacyUsersPrefix + "/refresh",
		legacyUsersPrefix + "/logout",
		legacyUsersPrefix + "/init",
	} {
		if _, ok := paths[path]; ok {
			t.Fatalf("legacy Users path %s must not be public by default", path)
		}
	}
}

func TestDefaultAuthSkipPathsExcludeRetiredProductEndpoints(t *testing.T) {
	paths := DefaultAuthSkipPaths()
	for _, path := range []string{
		"/api/app/v1/auth/captcha",
		"/api/app/v1/auth/login-config",
		"/api/app/v1/auth/send-code",
		"/api/app/v1/auth/login",
		"/api/canvas/v1/auth/captcha",
		"/api/canvas/v1/auth/login-config",
		"/api/canvas/v1/auth/send-code",
		"/api/canvas/v1/auth/login",
		"/api/canvas/v1/auth/refresh",
	} {
		if _, ok := paths[path]; ok {
			t.Fatalf("retired product endpoint %s must not remain public", path)
		}
	}
}

func TestAuthTokenRejectsMissingTrustedPlatformConfiguration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	called := false
	router := gin.New()
	router.Use(AuthToken(AuthTokenConfig{
		Authenticator: func(ctx context.Context, input TokenInput) (*AuthIdentity, *apperror.Error) {
			called = true
			return &AuthIdentity{UserID: 12, SessionID: 34, Platform: "admin"}, nil
		},
	}))
	router.GET("/retired-product-shaped-path", func(c *gin.Context) {
		c.String(http.StatusOK, "me")
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/retired-product-shaped-path", nil)
	request.Header.Set("Authorization", "Bearer bearer-token")
	router.ServeHTTP(recorder, request)

	assertJSONError(t, recorder, http.StatusInternalServerError, apperror.CodeInternal, "认证平台未配置")
	if called {
		t.Fatal("authenticator must not receive empty trusted provenance")
	}
}

func TestAuthTokenRejectsUnregisteredTrustedPlatformConfiguration(t *testing.T) {
	called := false
	router := newAuthTokenTestRouter(AuthTokenConfig{
		Platform: "partner_portal",
		Authenticator: func(context.Context, TokenInput) (*AuthIdentity, *apperror.Error) {
			called = true
			return &AuthIdentity{UserID: 12, SessionID: 34, Platform: "partner_portal"}, nil
		},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer bearer-token")
	router.ServeHTTP(recorder, request)

	assertJSONError(t, recorder, http.StatusInternalServerError, apperror.CodeInternal, "认证平台未注册")
	if called {
		t.Fatal("authenticator must not receive unregistered provenance")
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
		Platform: enum.PlatformAdmin,
		Authenticator: func(ctx context.Context, input TokenInput) (*AuthIdentity, *apperror.Error) {
			gotInput = input
			return &AuthIdentity{
				UserID:    12,
				SessionID: 34,
				Platform:  enum.PlatformAdmin,
			}, nil
		},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer valid-token")
	request.Header.Set("platform", "partner_portal")
	request.Header.Set("device-id", "device-1")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if gotInput.AccessToken != "valid-token" {
		t.Fatalf("expected token valid-token, got %q", gotInput.AccessToken)
	}
	if gotInput.Platform != enum.PlatformAdmin {
		t.Fatalf("auth middleware trusted client platform header: %q", gotInput.Platform)
	}
	if gotInput.DeviceID != "device-1" {
		t.Fatalf("expected device id device-1, got %q", gotInput.DeviceID)
	}
	if recorder.Body.String() != "12|34|admin" {
		t.Fatalf("expected identity from authenticator, got %q", recorder.Body.String())
	}
}

func TestAuthTokenRejectsIdentityFromDifferentPlatform(t *testing.T) {
	router := newAuthTokenTestRouter(AuthTokenConfig{
		Platform: enum.PlatformAdmin,
		Authenticator: func(context.Context, TokenInput) (*AuthIdentity, *apperror.Error) {
			return &AuthIdentity{UserID: 12, SessionID: 34, Platform: "partner_portal"}, nil
		},
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer valid-token")
	router.ServeHTTP(recorder, request)

	assertJSONError(t, recorder, http.StatusUnauthorized, apperror.CodeUnauthorized, "Token平台与当前客户端不一致")
}

func TestAuthTokenConsumesRealtimeTicketInsteadOfBearerOrAccessCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	consumed := ""
	router := gin.New()
	router.Use(AuthToken(AuthTokenConfig{
		Platform: enum.PlatformAdmin,
		BrowserGrants: BrowserGrantAuthConfig{
			RealtimePath: "/api/admin/v1/realtime/ws",
			ConsumeRealtimeTicket: func(_ context.Context, credential string) (*AuthIdentity, *apperror.Error) {
				consumed = credential
				return &AuthIdentity{UserID: 12, SessionID: 34, Platform: "admin"}, nil
			},
		},
	}))
	router.GET("/api/admin/v1/realtime/ws", func(c *gin.Context) {
		identity := GetAuthIdentity(c)
		c.String(http.StatusOK, "%d", identity.SessionID)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/realtime/ws?ticket=one-time-ticket", nil)
	request.Header.Set("Authorization", "Bearer must-not-win")
	request.AddCookie(&http.Cookie{Name: "access_token", Value: "must-not-win"})
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if consumed != "one-time-ticket" || recorder.Body.String() != "34" {
		t.Fatalf("ticket=%q body=%q", consumed, recorder.Body.String())
	}
}

func TestAuthTokenValidatesQueueMonitorGrantInsteadOfBearer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	validated := ""
	router := gin.New()
	router.Use(AuthToken(AuthTokenConfig{
		Platform: enum.PlatformAdmin,
		BrowserGrants: BrowserGrantAuthConfig{
			QueueMonitorPathPrefixes: []string{"/api/admin/v1/queue-monitor-ui"},
			QueueMonitorCookieName:   "__Secure-admin_queue_monitor",
			ValidateQueueMonitorGrant: func(_ context.Context, credential string) (*AuthIdentity, *apperror.Error) {
				validated = credential
				return &AuthIdentity{UserID: 12, SessionID: 34, Platform: "admin"}, nil
			},
		},
	}))
	router.GET("/api/admin/v1/queue-monitor-ui", func(c *gin.Context) {
		c.String(http.StatusOK, "monitor")
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/queue-monitor-ui", nil)
	request.Header.Set("Authorization", "Bearer must-not-win")
	request.AddCookie(&http.Cookie{Name: "__Secure-admin_queue_monitor", Value: "queue-grant"})
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if validated != "queue-grant" {
		t.Fatalf("validated credential=%q", validated)
	}
}

func TestAuthTokenRejectsBrowserGrantIdentityFromDifferentPlatform(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(AuthToken(AuthTokenConfig{
		Platform: enum.PlatformAdmin,
		BrowserGrants: BrowserGrantAuthConfig{
			RealtimePath: "/api/admin/v1/realtime/ws",
			ConsumeRealtimeTicket: func(context.Context, string) (*AuthIdentity, *apperror.Error) {
				return &AuthIdentity{UserID: 12, SessionID: 34, Platform: "partner_portal"}, nil
			},
		},
	}))
	router.GET("/api/admin/v1/realtime/ws", func(c *gin.Context) { c.Status(http.StatusOK) })
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/realtime/ws?ticket=grant", nil)
	router.ServeHTTP(recorder, request)

	assertJSONError(t, recorder, http.StatusUnauthorized, apperror.CodeUnauthorized, "浏览器授权平台与当前客户端不一致")
}

func TestAuthTokenDoesNotUseCookieForNormalAPIPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(AuthToken(AuthTokenConfig{
		Authenticator: func(ctx context.Context, input TokenInput) (*AuthIdentity, *apperror.Error) {
			t.Fatalf("authenticator should not be called")
			return nil, nil
		},
	}))
	router.GET("/api/admin/v1/users/me", func(c *gin.Context) {
		c.String(http.StatusOK, "me")
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/users/me", nil)
	request.AddCookie(&http.Cookie{Name: "access_token", Value: "cookie-token"})
	router.ServeHTTP(recorder, request)

	assertJSONError(t, recorder, http.StatusUnauthorized, apperror.CodeUnauthorized, "缺少Token")
}

func TestAuthTokenDoesNotUseCookieForMutatingRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(AuthToken(AuthTokenConfig{
		BrowserGrants: BrowserGrantAuthConfig{
			QueueMonitorPathPrefixes: []string{"/api/admin/v1/queue-monitor-ui"},
			QueueMonitorCookieName:   "__Secure-admin_queue_monitor",
			ValidateQueueMonitorGrant: func(context.Context, string) (*AuthIdentity, *apperror.Error) {
				t.Fatal("queue grant validator should not receive access_token cookie")
				return nil, nil
			},
		},
	}))
	router.POST("/api/admin/v1/queue-monitor-ui/api/queues/critical:pause", func(c *gin.Context) {
		c.String(http.StatusOK, "pause")
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/queue-monitor-ui/api/queues/critical:pause", nil)
	request.AddCookie(&http.Cookie{Name: "access_token", Value: "cookie-token"})
	router.ServeHTTP(recorder, request)

	assertJSONError(t, recorder, http.StatusUnauthorized, apperror.CodeUnauthorized, "缺少队列监控授权")
}

func TestAuthTokenRejectsRealtimeAccessTokenQueryFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(AuthToken(AuthTokenConfig{
		BrowserGrants: BrowserGrantAuthConfig{
			RealtimePath: "/api/admin/v1/realtime/ws",
			ConsumeRealtimeTicket: func(context.Context, string) (*AuthIdentity, *apperror.Error) {
				t.Fatal("ticket consumer must not receive access_token query")
				return nil, nil
			},
		},
	}))
	router.GET("/api/admin/v1/realtime/ws", func(c *gin.Context) { c.Status(http.StatusOK) })
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/realtime/ws?access_token=legacy", nil)
	router.ServeHTTP(recorder, request)
	assertJSONError(t, recorder, http.StatusUnauthorized, apperror.CodeUnauthorized, "缺少实时授权票据")
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

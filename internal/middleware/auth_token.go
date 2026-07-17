package middleware

import (
	"context"
	"net/http"
	"strings"

	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

const (
	ContextAuthIdentity = "auth_identity"
)

type TokenAuthenticator func(ctx context.Context, input TokenInput) (*AuthIdentity, *apperror.Error)

type TokenInput struct {
	AccessToken string
	Platform    string
	DeviceID    string
	ClientIP    string
}

type AuthIdentity struct {
	UserID    int64
	SessionID int64
	Platform  string
}

type AuthTokenConfig struct {
	Authenticator TokenAuthenticator
	SkipPaths     map[string]struct{}
	BrowserGrants BrowserGrantAuthConfig
}

type BrowserGrantAuthenticator func(context.Context, string) (*AuthIdentity, *apperror.Error)

type BrowserGrantAuthConfig struct {
	RealtimePath              string
	ConsumeRealtimeTicket     BrowserGrantAuthenticator
	QueueMonitorPathPrefixes  []string
	QueueMonitorCookieName    string
	ValidateQueueMonitorGrant BrowserGrantAuthenticator
}

type requestToken struct {
	value string
}

func AuthToken(cfg AuthTokenConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if shouldSkipAuth(c.Request, cfg.SkipPaths) {
			c.Next()
			return
		}

		if handled := authenticateBrowserGrant(c, cfg.BrowserGrants); handled {
			return
		}

		token, tokenErr := tokenFromRequest(c.Request)
		if tokenErr != nil {
			response.Abort(c, tokenErr)
			return
		}
		if cfg.Authenticator == nil {
			response.Abort(c, apperror.UnauthorizedKey("auth.token.authenticator_missing", nil, "Token认证未配置"))
			return
		}

		platform := c.GetHeader("platform")
		if strings.TrimSpace(platform) == "" {
			platform = defaultPlatformForPath(c.Request.URL.Path)
		}

		identity, err := cfg.Authenticator(c.Request.Context(), TokenInput{
			AccessToken: token.value,
			Platform:    platform,
			DeviceID:    c.GetHeader("device-id"),
			ClientIP:    c.ClientIP(),
		})
		if err != nil {
			response.Abort(c, err)
			return
		}
		if identity == nil {
			response.Abort(c, apperror.UnauthorizedKey("auth.token.invalid_or_expired", nil, "Token无效或已过期"))
			return
		}

		c.Set(ContextAuthIdentity, identity)
		c.Next()
	}
}

func TokenFromRequest(request *http.Request) (string, *apperror.Error) {
	token, err := tokenFromRequest(request)
	if err != nil {
		return "", err
	}
	return token.value, nil
}

func tokenFromRequest(request *http.Request) (requestToken, *apperror.Error) {
	if request == nil {
		return requestToken{}, apperror.UnauthorizedKey("auth.token.missing", nil, "缺少Token")
	}

	if value := strings.TrimSpace(request.Header.Get("Authorization")); value != "" {
		token, err := ParseBearerToken(value)
		if err != nil {
			return requestToken{}, err
		}
		return requestToken{value: token}, nil
	}
	return requestToken{}, apperror.UnauthorizedKey("auth.token.missing", nil, "缺少Token")
}

func GetAuthIdentity(c *gin.Context) *AuthIdentity {
	value, ok := c.Get(ContextAuthIdentity)
	if !ok {
		return nil
	}
	identity, ok := value.(*AuthIdentity)
	if !ok {
		return nil
	}
	return identity
}

func DefaultAuthSkipPaths() map[string]struct{} {
	return map[string]struct{}{
		"/health":                                     {},
		"/ready":                                      {},
		"/api/admin/v1/ping":                          {},
		"/api/admin/v1/auth/captcha":                  {},
		"/api/admin/v1/auth/login-config":             {},
		"/api/admin/v1/auth/send-code":                {},
		"/api/admin/v1/auth/forgot-password":          {},
		"/api/admin/v1/auth/login":                    {},
		"/api/admin/v1/auth/refresh":                  {},
		"/api/app/v1/auth/captcha":                    {},
		"/api/app/v1/auth/login-config":               {},
		"/api/app/v1/auth/send-code":                  {},
		"/api/app/v1/auth/login":                      {},
		"/api/canvas/v1/auth/captcha":                 {},
		"/api/canvas/v1/auth/login-config":            {},
		"/api/canvas/v1/auth/send-code":               {},
		"/api/canvas/v1/auth/login":                   {},
		"/api/canvas/v1/auth/refresh":                 {},
		"/api/admin/v1/client-versions/current-check": {},
		"/api/payment/callbacks/alipay":               {},
		"/favicon.ico":                                {},
		"/robots.txt":                                 {},
		"/openapi.json":                               {},
	}
}

func shouldSkipAuth(request *http.Request, skipPaths map[string]struct{}) bool {
	if request.Method == http.MethodOptions {
		return true
	}
	if len(skipPaths) == 0 {
		return false
	}
	_, ok := skipPaths[request.URL.Path]
	return ok
}

func ParseBearerToken(value string) (string, *apperror.Error) {
	if strings.TrimSpace(value) == "" {
		return "", apperror.UnauthorizedKey("auth.token.missing", nil, "缺少Token")
	}
	prefix, token, ok := strings.Cut(value, " ")
	if !ok || !strings.EqualFold(prefix, "Bearer") {
		return "", apperror.UnauthorizedKey("auth.token.invalid_format", nil, "Token格式错误")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", apperror.UnauthorizedKey("auth.token.invalid_format", nil, "Token格式错误")
	}
	return token, nil
}

func matchesPathPrefix(path string, prefixes []string) bool {
	for _, prefix := range prefixes {
		prefix = strings.TrimRight(strings.TrimSpace(prefix), "/")
		if prefix == "" {
			continue
		}
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func authenticateBrowserGrant(c *gin.Context, cfg BrowserGrantAuthConfig) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}
	path := c.Request.URL.Path
	if realtimePath := strings.TrimSpace(cfg.RealtimePath); realtimePath != "" && path == realtimePath {
		credential := strings.TrimSpace(c.Query("ticket"))
		if credential == "" {
			response.Abort(c, apperror.UnauthorizedKey("auth.realtime_ticket_missing", nil, "缺少实时授权票据"))
			return true
		}
		authenticateGrant(c, credential, cfg.ConsumeRealtimeTicket)
		return true
	}
	if matchesPathPrefix(path, cfg.QueueMonitorPathPrefixes) {
		cookieName := strings.TrimSpace(cfg.QueueMonitorCookieName)
		cookie, err := c.Request.Cookie(cookieName)
		if err != nil || strings.TrimSpace(cookie.Value) == "" {
			response.Abort(c, apperror.UnauthorizedKey("auth.queue_monitor_grant_missing", nil, "缺少队列监控授权"))
			return true
		}
		authenticateGrant(c, strings.TrimSpace(cookie.Value), cfg.ValidateQueueMonitorGrant)
		return true
	}
	return false
}

func authenticateGrant(c *gin.Context, credential string, authenticator BrowserGrantAuthenticator) {
	if authenticator == nil {
		response.Abort(c, apperror.UnauthorizedKey("auth.browser_grant_authenticator_missing", nil, "浏览器授权服务未配置"))
		return
	}
	identity, appErr := authenticator(c.Request.Context(), credential)
	if appErr != nil {
		response.Abort(c, appErr)
		return
	}
	if identity == nil {
		response.Abort(c, apperror.UnauthorizedKey("auth.browser_grant_invalid", nil, "浏览器授权已失效"))
		return
	}
	c.Set(ContextAuthIdentity, identity)
	c.Next()
}

func defaultPlatformForPath(path string) string {
	if strings.HasPrefix(path, "/api/app/v1/") || path == "/api/app/v1" {
		return enum.PlatformApp
	}
	if strings.HasPrefix(path, "/api/canvas/v1/") || path == "/api/canvas/v1" {
		return enum.PlatformCanvas
	}
	return ""
}

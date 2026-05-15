package middleware

import (
	"context"
	"net/http"
	"strings"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

const (
	ContextAuthIdentity = "auth_identity"

	DefaultAccessTokenCookie = "access_token"
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
	Authenticator   TokenAuthenticator
	SkipPaths       map[string]struct{}
	CookieTokenPath CookieTokenPathConfig
}

// CookieTokenPathConfig allows browser document requests to authenticate with
// the existing access_token cookie for explicitly configured read-only path
// prefixes. This is intentionally narrow: normal API calls still use the
// Authorization header so cookie auth does not become a hidden global fallback.
type CookieTokenPathConfig struct {
	CookieName   string
	PathPrefixes []string
	Platform     string
}

type requestToken struct {
	value      string
	fromCookie bool
}

func AuthToken(cfg AuthTokenConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if shouldSkipAuth(c.Request, cfg.SkipPaths) {
			c.Next()
			return
		}

		token, tokenErr := tokenFromRequest(c.Request, cfg.CookieTokenPath)
		if tokenErr != nil {
			response.Abort(c, tokenErr)
			return
		}
		if cfg.Authenticator == nil {
			response.Abort(c, apperror.UnauthorizedKey("auth.token.authenticator_missing", nil, "Token认证未配置"))
			return
		}

		platform := c.GetHeader("platform")
		if token.fromCookie && strings.TrimSpace(platform) == "" {
			platform = strings.TrimSpace(cfg.CookieTokenPath.Platform)
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

func TokenFromRequest(request *http.Request, cookieCfg CookieTokenPathConfig) (string, *apperror.Error) {
	token, err := tokenFromRequest(request, cookieCfg)
	if err != nil {
		return "", err
	}
	return token.value, nil
}

func tokenFromRequest(request *http.Request, cookieCfg CookieTokenPathConfig) (requestToken, *apperror.Error) {
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
	if token, ok := cookieTokenForPath(request, cookieCfg); ok {
		return requestToken{value: token, fromCookie: true}, nil
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
		"/api/admin/v1/client-versions/current-check": {},
		"/api/Users/getLoginConfig":                   {},
		"/api/Users/sendCode":                         {},
		"/api/Users/login":                            {},
		"/api/Users/refresh":                          {},
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

func cookieTokenForPath(request *http.Request, cfg CookieTokenPathConfig) (string, bool) {
	if request == nil || request.URL == nil {
		return "", false
	}
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		return "", false
	}
	if !matchesCookieTokenPath(request.URL.Path, cfg.PathPrefixes) {
		return "", false
	}

	cookieName := strings.TrimSpace(cfg.CookieName)
	if cookieName == "" {
		cookieName = DefaultAccessTokenCookie
	}
	cookie, err := request.Cookie(cookieName)
	if err != nil {
		return "", false
	}
	token := strings.TrimSpace(cookie.Value)
	return token, token != ""
}

func matchesCookieTokenPath(path string, prefixes []string) bool {
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

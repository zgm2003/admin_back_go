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
}

func AuthToken(cfg AuthTokenConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if shouldSkipAuth(c.Request, cfg.SkipPaths) {
			c.Next()
			return
		}

		token, tokenErr := ParseBearerToken(c.GetHeader("Authorization"))
		if tokenErr != nil {
			response.Abort(c, tokenErr)
			return
		}
		if cfg.Authenticator == nil {
			response.Abort(c, apperror.Unauthorized("Token认证未配置"))
			return
		}

		identity, err := cfg.Authenticator(c.Request.Context(), TokenInput{
			AccessToken: token,
			Platform:    c.GetHeader("platform"),
			DeviceID:    c.GetHeader("device-id"),
			ClientIP:    c.ClientIP(),
		})
		if err != nil {
			response.Abort(c, err)
			return
		}
		if identity == nil {
			response.Abort(c, apperror.Unauthorized("Token无效或已过期"))
			return
		}

		c.Set(ContextAuthIdentity, identity)
		c.Next()
	}
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
		"/health":                         {},
		"/ready":                          {},
		"/api/admin/v1/ping":              {},
		"/api/admin/v1/auth/captcha":      {},
		"/api/admin/v1/auth/login-config": {},
		"/api/admin/v1/auth/send-code":    {},
		"/api/admin/v1/auth/login":        {},
		"/api/admin/v1/auth/refresh":      {},
		"/api/Users/getLoginConfig":       {},
		"/api/Users/sendCode":             {},
		"/api/Users/login":                {},
		"/api/Users/refresh":              {},
		"/favicon.ico":                    {},
		"/robots.txt":                     {},
		"/openapi.json":                   {},
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
		return "", apperror.Unauthorized("缺少Token")
	}
	prefix, token, ok := strings.Cut(value, " ")
	if !ok || !strings.EqualFold(prefix, "Bearer") {
		return "", apperror.Unauthorized("Token格式错误")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", apperror.Unauthorized("Token格式错误")
	}
	return token, nil
}

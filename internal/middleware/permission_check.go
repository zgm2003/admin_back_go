package middleware

import (
	"context"
	"net/http"
	"strings"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/response"

	"github.com/gin-gonic/gin"
)

type RouteKey struct {
	Method string
	Path   string
}

func NewRouteKey(method string, path string) RouteKey {
	return RouteKey{Method: strings.ToUpper(strings.TrimSpace(method)), Path: strings.TrimSpace(path)}
}

type PermissionInput struct {
	UserID    int64
	SessionID int64
	Platform  string
	Method    string
	Path      string
	Code      string
}

type PermissionChecker func(ctx context.Context, input PermissionInput) *apperror.Error

type PermissionCheckConfig struct {
	Rules   map[RouteKey]string
	Checker PermissionChecker
}

func PermissionCheck(cfg PermissionCheckConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := matchedRoutePath(c)
		code := strings.TrimSpace(cfg.Rules[NewRouteKey(c.Request.Method, path)])
		if code == "" {
			code = strings.TrimSpace(cfg.Rules[NewRouteKey(c.Request.Method, normalizedEscapedRoutePath(path))])
		}
		if code == "" || c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		identity := GetAuthIdentity(c)
		if identity == nil || identity.UserID <= 0 {
			response.Abort(c, apperror.Unauthorized("Token无效或已过期"))
			return
		}
		if cfg.Checker == nil {
			response.Abort(c, apperror.Forbidden("权限检查未配置"))
			return
		}

		if err := cfg.Checker(c.Request.Context(), PermissionInput{
			UserID:    identity.UserID,
			SessionID: identity.SessionID,
			Platform:  identity.Platform,
			Method:    c.Request.Method,
			Path:      path,
			Code:      code,
		}); err != nil {
			response.Abort(c, err)
			return
		}

		c.Next()
	}
}

func matchedRoutePath(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	if path := c.FullPath(); path != "" {
		return path
	}
	return c.Request.URL.Path
}

func normalizedEscapedRoutePath(path string) string {
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if strings.Contains(part, "%2F") || strings.Contains(part, "%2f") {
			parts[i] = ":name"
		}
	}
	return strings.Join(parts, "/")
}

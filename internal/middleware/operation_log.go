package middleware

import (
	"context"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

type OperationRule struct {
	Module string
	Action string
	Title  string
}

type OperationInput struct {
	UserID    int64
	SessionID int64
	Platform  string
	Method    string
	Path      string
	Module    string
	Action    string
	Title     string
	RequestID string
	ClientIP  string
	Status    int
	Success   bool
	LatencyMs int64
}

type OperationRecorder func(ctx context.Context, input OperationInput) error

type OperationLogConfig struct {
	Rules    map[RouteKey]OperationRule
	Recorder OperationRecorder
	Logger   *slog.Logger
}

func OperationLog(cfg OperationLogConfig) gin.HandlerFunc {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return func(c *gin.Context) {
		path := matchedRoutePath(c)
		rule, ok := cfg.Rules[NewRouteKey(c.Request.Method, path)]
		if !ok || cfg.Recorder == nil {
			c.Next()
			return
		}

		startedAt := time.Now()
		c.Next()

		identity := GetAuthIdentity(c)
		input := OperationInput{
			Method:    c.Request.Method,
			Path:      path,
			Module:    rule.Module,
			Action:    rule.Action,
			Title:     rule.Title,
			RequestID: GetRequestID(c),
			ClientIP:  c.ClientIP(),
			Status:    c.Writer.Status(),
			Success:   c.Writer.Status() < 400,
			LatencyMs: time.Since(startedAt).Milliseconds(),
		}
		if identity != nil {
			input.UserID = identity.UserID
			input.SessionID = identity.SessionID
			input.Platform = identity.Platform
		}

		if err := cfg.Recorder(c.Request.Context(), input); err != nil {
			logger.WarnContext(c.Request.Context(), "operation log record failed",
				"request_id", input.RequestID,
				"method", input.Method,
				"path", input.Path,
				"module", input.Module,
				"action", input.Action,
				"error", err,
			)
		}
	}
}

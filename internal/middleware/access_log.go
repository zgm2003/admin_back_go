package middleware

import (
	"log/slog"
	"strings"
	"time"

	"admin_back_go/internal/shared/response"
	"admin_back_go/internal/telemetry"

	"github.com/gin-gonic/gin"
)

func AccessLog(logger *slog.Logger, recorders ...telemetry.Recorder) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	recorder := telemetry.Noop()
	if len(recorders) > 0 && recorders[0] != nil {
		recorder = recorders[0]
	}

	return func(c *gin.Context) {
		startedAt := time.Now()
		c.Next()
		duration := time.Since(startedAt)

		route := strings.TrimSpace(c.FullPath())
		if route == "" {
			route = "unmatched"
		}
		attributes := telemetry.Attributes{
			"http.method": c.Request.Method,
			"http.route":  route,
			"http.status": c.Writer.Status(),
		}
		if appErr := response.GetError(c); appErr != nil {
			attributes["error.code"] = appErr.Code
		}
		recorder.Count("http.requests", 1, attributes)
		recorder.Observe("http.duration_seconds", duration.Seconds(), attributes)

		logger.InfoContext(c.Request.Context(), "http request",
			"request_id", GetRequestID(c),
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency_ms", duration.Milliseconds(),
			"client_ip", c.ClientIP(),
		)
	}
}

package middleware

import (
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"admin_back_go/internal/shared/response"

	"github.com/gin-gonic/gin"
)

const redactedValue = "[REDACTED]"

var (
	reporterSensitiveKeys = []string{
		"token",
		"authorization",
		"cookie",
		"password",
		"secret",
		"certificate",
		"prompt",
		"payload",
	}
	reporterCredentialPattern = regexp.MustCompile(`(?i)([a-z0-9._-]+):([^@\s]+)@`)
	reporterKeyValuePattern   = regexp.MustCompile(`(?i)(token|authorization|cookie|password|secret|certificate|prompt|payload)(\s*[:=]\s*)([^\s,;]+)`)
)

func ErrorReporter(logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}

	return func(c *gin.Context) {
		c.Next()

		err := response.GetError(c)
		if err == nil || c.Writer.Status() < 500 {
			return
		}

		cause := err.Cause
		if cause == nil {
			cause = err
		}
		attributes := redactReporterAttributes(map[string]any{
			"request_id": GetRequestID(c),
			"trace_id":   contextOrHeader(c, "trace_id", "X-Trace-Id"),
			"task_id":    contextString(c, "task_id"),
			"run_id":     contextString(c, "run_id"),
			"error_code": err.Code,
			"category":   string(err.Category),
			"operation":  err.Operation,
			"cause_type": fmt.Sprintf("%T", cause),
			"cause":      sanitizeReporterText(cause.Error()),
		})

		logger.ErrorContext(c.Request.Context(), "application request failed",
			"request_id", attributes["request_id"],
			"trace_id", attributes["trace_id"],
			"task_id", attributes["task_id"],
			"run_id", attributes["run_id"],
			"error_code", attributes["error_code"],
			"category", attributes["category"],
			"operation", attributes["operation"],
			"cause_type", attributes["cause_type"],
			"cause", attributes["cause"],
		)
	}
}

func redactReporterAttributes(attributes map[string]any) map[string]any {
	redacted := make(map[string]any, len(attributes))
	for key, value := range attributes {
		if reporterSensitiveKey(key) {
			redacted[key] = redactedValue
			continue
		}
		if text, ok := value.(string); ok {
			redacted[key] = sanitizeReporterText(text)
			continue
		}
		redacted[key] = value
	}
	return redacted
}

func reporterSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	for _, sensitive := range reporterSensitiveKeys {
		if strings.Contains(normalized, sensitive) {
			return true
		}
	}
	return false
}

func sanitizeReporterText(value string) string {
	value = reporterCredentialPattern.ReplaceAllString(value, `${1}:`+redactedValue+`@`)
	return reporterKeyValuePattern.ReplaceAllString(value, `${1}${2}`+redactedValue)
}

func contextString(c *gin.Context, key string) string {
	if c == nil {
		return ""
	}
	value, exists := c.Get(key)
	if !exists {
		return ""
	}
	text, _ := value.(string)
	return text
}

func contextOrHeader(c *gin.Context, key string, header string) string {
	if value := contextString(c, key); value != "" {
		return value
	}
	if c == nil || c.Request == nil {
		return ""
	}
	return c.GetHeader(header)
}

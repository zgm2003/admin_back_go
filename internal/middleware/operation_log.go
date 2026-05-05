package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const maxOperationLogPayloadBytes = 64 * 1024

type OperationRule struct {
	Module string
	Action string
	Title  string
}

type OperationInput struct {
	UserID          int64
	SessionID       int64
	Platform        string
	Method          string
	Path            string
	Module          string
	Action          string
	Title           string
	RequestID       string
	ClientIP        string
	Status          int
	Success         bool
	LatencyMs       int64
	RequestPayload  any
	ResponsePayload any
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

		requestPayload := readRequestPayload(c, logger)
		bodyWriter := &operationBodyWriter{
			ResponseWriter: c.Writer,
			body:           bytes.NewBuffer(nil),
		}
		c.Writer = bodyWriter

		startedAt := time.Now()
		c.Next()

		identity := GetAuthIdentity(c)
		input := OperationInput{
			Method:          c.Request.Method,
			Path:            path,
			Module:          rule.Module,
			Action:          rule.Action,
			Title:           rule.Title,
			RequestID:       GetRequestID(c),
			ClientIP:        c.ClientIP(),
			Status:          c.Writer.Status(),
			Success:         c.Writer.Status() < 400,
			LatencyMs:       time.Since(startedAt).Milliseconds(),
			RequestPayload:  requestPayload,
			ResponsePayload: readResponsePayload(bodyWriter.BodyBytes(), logger),
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

type operationBodyWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *operationBodyWriter) Write(data []byte) (int, error) {
	if w.body != nil && w.body.Len() < maxOperationLogPayloadBytes {
		remain := maxOperationLogPayloadBytes - w.body.Len()
		if len(data) <= remain {
			_, _ = w.body.Write(data)
		} else {
			_, _ = w.body.Write(data[:remain])
		}
	}
	return w.ResponseWriter.Write(data)
}

func (w *operationBodyWriter) WriteString(data string) (int, error) {
	if w.body != nil && w.body.Len() < maxOperationLogPayloadBytes {
		remain := maxOperationLogPayloadBytes - w.body.Len()
		if len(data) <= remain {
			_, _ = w.body.WriteString(data)
		} else {
			_, _ = w.body.WriteString(data[:remain])
		}
	}
	return w.ResponseWriter.WriteString(data)
}

func (w *operationBodyWriter) BodyBytes() []byte {
	if w == nil || w.body == nil || w.body.Len() == 0 {
		return nil
	}
	return append([]byte(nil), w.body.Bytes()...)
}

func readRequestPayload(c *gin.Context, logger *slog.Logger) any {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return nil
	}
	if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead || c.Request.Method == http.MethodOptions {
		return nil
	}

	limited := io.LimitReader(c.Request.Body, maxOperationLogPayloadBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		if logger != nil {
			logger.WarnContext(c.Request.Context(), "operation log read request body failed", "error", err)
		}
		return nil
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	if len(body) == 0 {
		return nil
	}
	if len(body) > maxOperationLogPayloadBytes {
		body = body[:maxOperationLogPayloadBytes]
	}
	return decodeJSONPayload(body)
}

func readResponsePayload(body []byte, logger *slog.Logger) any {
	if len(body) == 0 {
		return nil
	}
	if len(body) > maxOperationLogPayloadBytes {
		body = body[:maxOperationLogPayloadBytes]
	}
	return decodeJSONPayload(body)
}

func decodeJSONPayload(body []byte) any {
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return string(body)
	}
	return payload
}

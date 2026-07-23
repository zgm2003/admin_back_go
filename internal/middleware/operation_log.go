package middleware

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/response"
	"admin_back_go/internal/telemetry"

	"github.com/gin-gonic/gin"
)

const (
	maxOperationLogPayloadBytes     = 64 * 1024
	maxRequiredAuditResponseBytes   = 1 << 20
	requiredAuditFailureMetric      = "operation.audit.required_failure"
	requiredAuditRecorderMissing    = "recorder_missing"
	requiredAuditRecorderFailed     = "recorder_failed"
	requiredAuditResponseOverflowed = "response_overflow"
)

var (
	errRequiredAuditHijackUnsupported = errors.New("required audit response staging does not support hijacking")
	errRequiredAuditResponseSealed    = errors.New("required audit response staging is sealed")
)

type OperationRule struct {
	Module string
	Action string
	Title  string

	SkipRequestPayload  bool
	SkipResponsePayload bool
	Required            bool
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
	Rules     map[RouteKey]OperationRule
	Recorder  OperationRecorder
	Logger    *slog.Logger
	Telemetry telemetry.Recorder
}

func OperationLog(cfg OperationLogConfig) gin.HandlerFunc {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	metrics := cfg.Telemetry
	if metrics == nil {
		metrics = telemetry.Noop()
	}

	return func(c *gin.Context) {
		path := matchedRoutePath(c)
		rule, ok := cfg.Rules[NewRouteKey(c.Request.Method, path)]
		if !ok {
			c.Next()
			return
		}
		if rule.Required {
			runRequiredOperationLog(c, path, rule, cfg.Recorder, logger, metrics)
			return
		}
		if cfg.Recorder == nil {
			c.Next()
			return
		}

		var requestPayload any
		if !rule.SkipRequestPayload {
			requestPayload = readRequestPayload(c, logger)
		}
		var bodyWriter *operationBodyWriter
		if !rule.SkipResponsePayload {
			bodyWriter = &operationBodyWriter{
				ResponseWriter: c.Writer,
				body:           bytes.NewBuffer(nil),
			}
			c.Writer = bodyWriter
		}

		startedAt := time.Now()
		c.Next()

		identity := GetAuthIdentity(c)
		input := OperationInput{
			Method:         c.Request.Method,
			Path:           path,
			Module:         rule.Module,
			Action:         rule.Action,
			Title:          rule.Title,
			RequestID:      GetRequestID(c),
			ClientIP:       c.ClientIP(),
			Status:         c.Writer.Status(),
			Success:        c.Writer.Status() < 400,
			LatencyMs:      time.Since(startedAt).Milliseconds(),
			RequestPayload: requestPayload,
		}
		if bodyWriter != nil {
			input.ResponsePayload = readResponsePayload(bodyWriter.BodyBytes(), logger)
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

func runRequiredOperationLog(
	c *gin.Context,
	path string,
	rule OperationRule,
	recorder OperationRecorder,
	logger *slog.Logger,
	metrics telemetry.Recorder,
) {
	startedAt := time.Now()
	if recorder == nil {
		input := requiredOperationInput(c, path, rule, http.StatusInternalServerError, false, startedAt, nil, nil)
		writeRequiredAuditFailure(c, input, requiredAuditRecorderMissing, logger, metrics)
		return
	}

	var requestPayload any
	if !rule.SkipRequestPayload {
		requestPayload = readRequestPayload(c, logger)
	}

	destination := c.Writer
	staged := newRequiredAuditWriter(destination)
	restored := false
	restore := func() {
		if restored {
			return
		}
		c.Writer = destination
		restored = true
	}
	c.Writer = staged
	defer restore()
	downstreamPanicked := false
	func() {
		defer func() {
			if recover() != nil {
				downstreamPanicked = true
			}
		}()
		c.Next()
	}()
	staged.seal()
	if downstreamPanicked {
		input := requiredOperationInput(c, path, rule, http.StatusInternalServerError, false, startedAt, nil, nil)
		logger.WarnContext(c.Request.Context(), "required operation handler panicked",
			"request_id", input.RequestID,
			"user_id", input.UserID,
			"session_id", input.SessionID,
			"method", input.Method,
			"route", input.Path,
			"module", input.Module,
			"action", input.Action,
		)
		recordErr := recorder(c.Request.Context(), input)
		restore()
		if recordErr != nil {
			writeRequiredAuditFailure(c, input, requiredAuditRecorderFailed, logger, metrics)
			return
		}
		writeRequiredInternalFailure(c)
		return
	}

	var responsePayload any
	if !rule.SkipResponsePayload && !staged.Overflowed() {
		responsePayload = readResponsePayload(staged.BodyBytes(), logger)
	}
	input := requiredOperationInput(
		c,
		path,
		rule,
		staged.Status(),
		staged.Status() < http.StatusBadRequest,
		startedAt,
		requestPayload,
		responsePayload,
	)
	if staged.Overflowed() {
		input.Status = http.StatusInternalServerError
		input.Success = false
		input.ResponsePayload = nil
		_ = recorder(c.Request.Context(), input)
		restore()
		writeRequiredAuditFailure(c, input, requiredAuditResponseOverflowed, logger, metrics)
		return
	}
	recordErr := recorder(c.Request.Context(), input)
	restore()
	if recordErr != nil {
		writeRequiredAuditFailure(c, input, requiredAuditRecorderFailed, logger, metrics)
		return
	}
	if err := staged.release(); err != nil {
		logger.WarnContext(c.Request.Context(), "required operation response release failed",
			"request_id", input.RequestID,
			"user_id", input.UserID,
			"session_id", input.SessionID,
			"method", input.Method,
			"route", input.Path,
			"module", input.Module,
			"action", input.Action,
		)
	}
}

func requiredOperationInput(
	c *gin.Context,
	path string,
	rule OperationRule,
	status int,
	success bool,
	startedAt time.Time,
	requestPayload any,
	responsePayload any,
) OperationInput {
	input := OperationInput{
		Method:          c.Request.Method,
		Path:            path,
		Module:          rule.Module,
		Action:          rule.Action,
		Title:           rule.Title,
		RequestID:       GetRequestID(c),
		ClientIP:        c.ClientIP(),
		Status:          status,
		Success:         success,
		LatencyMs:       time.Since(startedAt).Milliseconds(),
		RequestPayload:  requestPayload,
		ResponsePayload: responsePayload,
	}
	if identity := GetAuthIdentity(c); identity != nil {
		input.UserID = identity.UserID
		input.SessionID = identity.SessionID
		input.Platform = identity.Platform
	}
	return input
}

func writeRequiredAuditFailure(
	c *gin.Context,
	input OperationInput,
	reason string,
	logger *slog.Logger,
	metrics telemetry.Recorder,
) {
	logger.WarnContext(c.Request.Context(), "required operation audit failed",
		"request_id", input.RequestID,
		"user_id", input.UserID,
		"session_id", input.SessionID,
		"method", input.Method,
		"route", input.Path,
		"module", input.Module,
		"action", input.Action,
		"reason", reason,
	)
	metrics.Count(requiredAuditFailureMetric, 1, telemetry.Attributes{
		"user_id":     input.UserID,
		"session_id":  input.SessionID,
		"http.method": input.Method,
		"http.route":  input.Path,
		"http.status": http.StatusInternalServerError,
		"request_id":  input.RequestID,
		"error.code":  reason,
		"outcome":     "error",
	})
	writeRequiredInternalFailure(c)
}

func writeRequiredInternalFailure(c *gin.Context) {
	header := c.Writer.Header()
	for _, name := range []string{"Content-Encoding", "Content-Length", "Content-Type", "Etag", "Last-Modified", "Location"} {
		header.Del(name)
	}
	header.Set("Cache-Control", "no-store, private")
	header.Set("Pragma", "no-cache")
	response.Abort(c, apperror.InternalKey("common.internal_error", nil, "系统错误"))
}

type requiredAuditWriter struct {
	destination   gin.ResponseWriter
	header        http.Header
	sealedHeader  http.Header
	discardHeader http.Header
	body          bytes.Buffer
	closeNotify   <-chan bool
	status        int
	size          int
	written       bool
	flushed       bool
	overflowed    bool
	sealed        bool
}

func newRequiredAuditWriter(destination gin.ResponseWriter) *requiredAuditWriter {
	status := http.StatusOK
	if destination != nil && destination.Status() > 0 {
		status = destination.Status()
	}
	header := make(http.Header)
	if destination != nil {
		header = destination.Header().Clone()
	}
	return &requiredAuditWriter{
		destination: destination,
		header:      header,
		closeNotify: requiredAuditCloseNotify(destination),
		status:      status,
		size:        -1,
	}
}

func (w *requiredAuditWriter) Header() http.Header {
	if w.sealed {
		return w.discardHeader
	}
	return w.header
}

func (w *requiredAuditWriter) WriteHeader(status int) {
	if w.sealed || w.written || status <= 0 {
		return
	}
	w.status = status
}

func (w *requiredAuditWriter) WriteHeaderNow() {
	if w.sealed || w.written {
		return
	}
	w.written = true
	w.size = 0
}

func (w *requiredAuditWriter) Write(data []byte) (int, error) {
	if w.sealed {
		return 0, errRequiredAuditResponseSealed
	}
	w.WriteHeaderNow()
	w.size += len(data)
	if w.overflowed || len(data) == 0 {
		return len(data), nil
	}
	remaining := maxRequiredAuditResponseBytes - w.body.Len()
	if len(data) > remaining {
		if remaining > 0 {
			_, _ = w.body.Write(data[:remaining])
		}
		w.overflowed = true
		return len(data), nil
	}
	_, _ = w.body.Write(data)
	return len(data), nil
}

func (w *requiredAuditWriter) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}

func (w *requiredAuditWriter) Status() int {
	return w.status
}

func (w *requiredAuditWriter) Size() int {
	return w.size
}

func (w *requiredAuditWriter) Written() bool {
	return w.written
}

func (w *requiredAuditWriter) Flush() {
	if w.sealed {
		return
	}
	w.WriteHeaderNow()
	w.flushed = true
}

func (w *requiredAuditWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, errRequiredAuditHijackUnsupported
}

func (w *requiredAuditWriter) CloseNotify() <-chan bool {
	return w.closeNotify
}

func (*requiredAuditWriter) Pusher() http.Pusher {
	return nil
}

func (w *requiredAuditWriter) BodyBytes() []byte {
	if w == nil || w.body.Len() == 0 {
		return nil
	}
	return append([]byte(nil), w.body.Bytes()...)
}

func (w *requiredAuditWriter) Overflowed() bool {
	return w != nil && w.overflowed
}

func (w *requiredAuditWriter) seal() {
	if w == nil || w.sealed {
		return
	}
	w.sealedHeader = w.header.Clone()
	w.discardHeader = make(http.Header)
	w.sealed = true
}

func (w *requiredAuditWriter) release() error {
	if w == nil || w.destination == nil {
		return errors.New("required audit response destination is unavailable")
	}
	releaseHeader := w.header
	if w.sealedHeader != nil {
		releaseHeader = w.sealedHeader
	}
	replaceOperationHeader(w.destination.Header(), releaseHeader)
	w.destination.WriteHeader(w.status)
	if w.body.Len() > 0 {
		n, err := w.destination.Write(w.body.Bytes())
		if err != nil {
			return err
		}
		if n != w.body.Len() {
			return io.ErrShortWrite
		}
	} else if w.written {
		w.destination.WriteHeaderNow()
	}
	if w.flushed {
		w.destination.Flush()
	}
	return nil
}

func replaceOperationHeader(destination http.Header, staged http.Header) {
	clear(destination)
	for key, values := range staged {
		destination[key] = append([]string(nil), values...)
	}
}

func requiredAuditCloseNotify(destination gin.ResponseWriter) (channel <-chan bool) {
	fallback := make(chan bool)
	channel = fallback
	if destination == nil {
		return channel
	}
	defer func() {
		if recover() != nil || channel == nil {
			channel = fallback
		}
	}()
	channel = destination.CloseNotify()
	return channel
}

var _ gin.ResponseWriter = (*requiredAuditWriter)(nil)

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

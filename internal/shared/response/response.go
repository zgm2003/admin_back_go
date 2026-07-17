package response

import (
	"net/http"

	"admin_back_go/internal/shared/apperror"
	projecti18n "admin_back_go/internal/shared/i18n"

	"github.com/gin-gonic/gin"
)

type Body struct {
	Code  int        `json:"code"`
	Data  any        `json:"data"`
	Msg   string     `json:"msg"`
	Error *ErrorMeta `json:"error,omitempty"`
}

type ErrorMeta struct {
	Code      string            `json:"code"`
	Category  apperror.Category `json:"category"`
	Retryable bool              `json:"retryable"`
	RequestID string            `json:"request_id,omitempty"`
	TraceID   string            `json:"trace_id,omitempty"`
}

const contextApplicationError = "response.application_error"

func OK(c *gin.Context, data any) {
	OKWithMessage(c, data, "ok")
}

func Accepted(c *gin.Context, data any) {
	if data == nil {
		data = gin.H{}
	}
	writeOKStatus(c, http.StatusAccepted, data, "", nil, "ok")
}

func OKNull(c *gin.Context) {
	writeOK(c, nil, "", nil, "ok")
}

func OKWithMessage(c *gin.Context, data any, message string) {
	OKWithMessageKey(c, data, "", nil, message)
}

func OKWithMessageKey(c *gin.Context, data any, messageID string, templateData map[string]any, fallback string) {
	if data == nil {
		data = gin.H{}
	}
	writeOK(c, data, messageID, templateData, fallback)
}

func writeOK(c *gin.Context, data any, messageID string, templateData map[string]any, fallback string) {
	writeOKStatus(c, http.StatusOK, data, messageID, templateData, fallback)
}

func writeOKStatus(c *gin.Context, status int, data any, messageID string, templateData map[string]any, fallback string) {
	message := fallback
	if localized, localizeErr := projecti18n.Message(c, messageID, templateData, fallback); localizeErr == nil && localized != "" {
		message = localized
	}
	c.JSON(status, Body{
		Code: apperror.CodeOK,
		Data: data,
		Msg:  message,
	})
}

func Error(c *gin.Context, err *apperror.Error) {
	ErrorWithData(c, err, gin.H{})
}

func ErrorWithData(c *gin.Context, err *apperror.Error, data any) {
	if err == nil {
		err = apperror.InternalKey("common.internal_error", nil, "系统错误")
	}
	if data == nil {
		data = gin.H{}
	}
	c.Set(contextApplicationError, err)

	message := err.Message
	if localized, localizeErr := projecti18n.Message(c, err.MessageID, err.TemplateData, err.Message); localizeErr == nil && localized != "" {
		message = localized
	}

	c.JSON(err.HTTPStatus, Body{
		Code: err.LegacyCode,
		Data: data,
		Msg:  message,
		Error: &ErrorMeta{
			Code:      err.Code,
			Category:  err.Category,
			Retryable: err.Retryable(),
			RequestID: correlationValue(c, "request_id", "X-Request-Id"),
			TraceID:   correlationValue(c, "trace_id", "X-Trace-Id"),
		},
	})
}

func GetError(c *gin.Context) *apperror.Error {
	if c == nil {
		return nil
	}
	value, exists := c.Get(contextApplicationError)
	if !exists {
		return nil
	}
	err, _ := value.(*apperror.Error)
	return err
}

func correlationValue(c *gin.Context, contextKey string, header string) string {
	if c == nil {
		return ""
	}
	if value, exists := c.Get(contextKey); exists {
		if text, ok := value.(string); ok && text != "" {
			return text
		}
	}
	if c.Request == nil {
		return ""
	}
	return c.GetHeader(header)
}

func Abort(c *gin.Context, err *apperror.Error) {
	Error(c, err)
	c.Abort()
}

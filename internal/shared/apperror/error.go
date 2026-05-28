package apperror

import "net/http"

const (
	CodeOK           = 0
	CodeBadRequest   = 100
	CodeUnauthorized = 401
	CodeForbidden    = 403
	CodeNotFound     = 404
	CodeInternal     = 500
)

type Error struct {
	Code         int
	HTTPStatus   int
	Message      string
	MessageID    string
	TemplateData map[string]any
	Cause        error
}

func New(code int, httpStatus int, message string) *Error {
	return &Error{Code: code, HTTPStatus: httpStatus, Message: message}
}

func Wrap(code int, httpStatus int, message string, cause error) *Error {
	return &Error{Code: code, HTTPStatus: httpStatus, Message: message, Cause: cause}
}

func NewKey(code int, httpStatus int, messageID string, templateData map[string]any, fallback string) *Error {
	return &Error{Code: code, HTTPStatus: httpStatus, Message: fallback, MessageID: messageID, TemplateData: templateData}
}

func WrapKey(code int, httpStatus int, messageID string, templateData map[string]any, fallback string, cause error) *Error {
	return &Error{Code: code, HTTPStatus: httpStatus, Message: fallback, MessageID: messageID, TemplateData: templateData, Cause: cause}
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func BadRequest(message string) *Error {
	return New(CodeBadRequest, http.StatusBadRequest, message)
}

func BadRequestKey(messageID string, templateData map[string]any, fallback string) *Error {
	return NewKey(CodeBadRequest, http.StatusBadRequest, messageID, templateData, fallback)
}

func Unauthorized(message string) *Error {
	return New(CodeUnauthorized, http.StatusUnauthorized, message)
}

func UnauthorizedKey(messageID string, templateData map[string]any, fallback string) *Error {
	return NewKey(CodeUnauthorized, http.StatusUnauthorized, messageID, templateData, fallback)
}

func Forbidden(message string) *Error {
	return New(CodeForbidden, http.StatusForbidden, message)
}

func ForbiddenKey(messageID string, templateData map[string]any, fallback string) *Error {
	return NewKey(CodeForbidden, http.StatusForbidden, messageID, templateData, fallback)
}

func NotFound(message string) *Error {
	return New(CodeNotFound, http.StatusNotFound, message)
}

func NotFoundKey(messageID string, templateData map[string]any, fallback string) *Error {
	return NewKey(CodeNotFound, http.StatusNotFound, messageID, templateData, fallback)
}

func Internal(message string) *Error {
	return New(CodeInternal, http.StatusInternalServerError, message)
}

func InternalKey(messageID string, templateData map[string]any, fallback string) *Error {
	return NewKey(CodeInternal, http.StatusInternalServerError, messageID, templateData, fallback)
}

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
	Code       int
	HTTPStatus int
	Message    string
	Cause      error
}

func New(code int, httpStatus int, message string) *Error {
	return &Error{Code: code, HTTPStatus: httpStatus, Message: message}
}

func Wrap(code int, httpStatus int, message string, cause error) *Error {
	return &Error{Code: code, HTTPStatus: httpStatus, Message: message, Cause: cause}
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

func Unauthorized(message string) *Error {
	return New(CodeUnauthorized, http.StatusUnauthorized, message)
}

func Forbidden(message string) *Error {
	return New(CodeForbidden, http.StatusForbidden, message)
}

func NotFound(message string) *Error {
	return New(CodeNotFound, http.StatusNotFound, message)
}

func Internal(message string) *Error {
	return New(CodeInternal, http.StatusInternalServerError, message)
}

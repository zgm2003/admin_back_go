package apperror

import (
	"net/http"
	"strings"
)

const (
	CodeOK           = 0
	CodeBadRequest   = 100
	CodeUnauthorized = 401
	CodeForbidden    = 403
	CodeNotFound     = 404
	CodeInternal     = 500
)

type Category string

const (
	CategoryValidation     Category = "validation"
	CategoryAuthentication Category = "authentication"
	CategoryAuthorization  Category = "authorization"
	CategoryNotFound       Category = "not_found"
	CategoryConflict       Category = "conflict"
	CategoryRateLimit      Category = "rate_limit"
	CategoryDependency     Category = "dependency"
	CategoryTimeout        Category = "timeout"
	CategoryInternal       Category = "internal"
	CategoryCanceled       Category = "canceled"
)

type RetryClass string

const (
	Permanent RetryClass = "permanent"
	Retryable RetryClass = "retryable"
)

type Error struct {
	Code         string
	LegacyCode   int
	Category     Category
	HTTPStatus   int
	Retry        RetryClass
	MessageID    string         `json:"-"`
	TemplateData map[string]any `json:"-"`
	Message      string
	Cause        error  `json:"-"`
	Operation    string `json:"-"`
}

func New(
	code string,
	category Category,
	httpStatus int,
	retry RetryClass,
	messageID string,
	templateData map[string]any,
	message string,
) *Error {
	return newError(code, category, httpStatus, retry, messageID, templateData, message, nil)
}

func Wrap(
	code string,
	category Category,
	httpStatus int,
	retry RetryClass,
	messageID string,
	templateData map[string]any,
	message string,
	cause error,
) *Error {
	return newError(code, category, httpStatus, retry, messageID, templateData, message, cause)
}

// LegacyNew keeps the historical numeric constructor available while callers
// migrate to stable machine codes.
func LegacyNew(code int, httpStatus int, message string) *Error {
	return newLegacyError(code, httpStatus, "", nil, message, nil)
}

// LegacyWrap keeps the historical numeric constructor available while callers
// migrate to stable machine codes.
func LegacyWrap(code int, httpStatus int, message string, cause error) *Error {
	return newLegacyError(code, httpStatus, "", nil, message, cause)
}

func NewKey(code int, httpStatus int, messageID string, templateData map[string]any, fallback string) *Error {
	return newLegacyError(code, httpStatus, messageID, templateData, fallback, nil)
}

func WrapKey(code int, httpStatus int, messageID string, templateData map[string]any, fallback string, cause error) *Error {
	return newLegacyError(code, httpStatus, messageID, templateData, fallback, cause)
}

func newLegacyError(code int, httpStatus int, messageID string, templateData map[string]any, message string, cause error) *Error {
	stableCode, category := legacyClassification(code, httpStatus)
	err := newError(stableCode, category, httpStatus, Permanent, messageID, templateData, message, cause)
	err.LegacyCode = code
	return err
}

func newError(
	code string,
	category Category,
	httpStatus int,
	retry RetryClass,
	messageID string,
	templateData map[string]any,
	message string,
	cause error,
) *Error {
	if category == "" {
		category = CategoryInternal
	}
	if strings.TrimSpace(code) == "" {
		code = defaultCode(category)
	}
	if httpStatus == 0 {
		httpStatus = defaultHTTPStatus(category)
	}
	if retry == "" {
		retry = Permanent
	}
	return &Error{
		Code:         code,
		LegacyCode:   legacyCode(category),
		Category:     category,
		HTTPStatus:   httpStatus,
		Retry:        retry,
		MessageID:    messageID,
		TemplateData: cloneTemplateData(templateData),
		Message:      message,
		Cause:        cause,
	}
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

func (e *Error) Retryable() bool {
	return e != nil && e.Retry == Retryable
}

func (e *Error) WithCode(code string) *Error {
	clone := e.clone()
	if clone == nil {
		return nil
	}
	if strings.TrimSpace(code) != "" {
		clone.Code = code
	}
	return clone
}

func (e *Error) WithOperation(operation string) *Error {
	clone := e.clone()
	if clone == nil {
		return nil
	}
	clone.Operation = strings.TrimSpace(operation)
	return clone
}

func (e *Error) clone() *Error {
	if e == nil {
		return nil
	}
	clone := *e
	clone.TemplateData = cloneTemplateData(e.TemplateData)
	return &clone
}

func cloneTemplateData(data map[string]any) map[string]any {
	if data == nil {
		return nil
	}
	clone := make(map[string]any, len(data))
	for key, value := range data {
		clone[key] = value
	}
	return clone
}

func legacyClassification(code int, httpStatus int) (string, Category) {
	switch code {
	case CodeBadRequest:
		return "request.invalid", CategoryValidation
	case CodeUnauthorized:
		return "auth.unauthenticated", CategoryAuthentication
	case CodeForbidden:
		return "auth.forbidden", CategoryAuthorization
	case CodeNotFound:
		return "resource.not_found", CategoryNotFound
	case CodeInternal:
		return "internal.unknown", CategoryInternal
	}

	switch httpStatus {
	case http.StatusUnauthorized:
		return "auth.unauthenticated", CategoryAuthentication
	case http.StatusForbidden:
		return "auth.forbidden", CategoryAuthorization
	case http.StatusNotFound:
		return "resource.not_found", CategoryNotFound
	case http.StatusConflict:
		return "resource.conflict", CategoryConflict
	case http.StatusTooManyRequests:
		return "request.rate_limited", CategoryRateLimit
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return "request.timeout", CategoryTimeout
	default:
		if httpStatus >= http.StatusInternalServerError {
			return "internal.unknown", CategoryInternal
		}
		return "request.invalid", CategoryValidation
	}
}

func defaultCode(category Category) string {
	switch category {
	case CategoryValidation:
		return "request.invalid"
	case CategoryAuthentication:
		return "auth.unauthenticated"
	case CategoryAuthorization:
		return "auth.forbidden"
	case CategoryNotFound:
		return "resource.not_found"
	case CategoryConflict:
		return "resource.conflict"
	case CategoryRateLimit:
		return "request.rate_limited"
	case CategoryDependency:
		return "dependency.unavailable"
	case CategoryTimeout:
		return "request.timeout"
	case CategoryCanceled:
		return "request.canceled"
	default:
		return "internal.unknown"
	}
}

func defaultHTTPStatus(category Category) int {
	switch category {
	case CategoryValidation:
		return http.StatusBadRequest
	case CategoryAuthentication:
		return http.StatusUnauthorized
	case CategoryAuthorization:
		return http.StatusForbidden
	case CategoryNotFound:
		return http.StatusNotFound
	case CategoryConflict:
		return http.StatusConflict
	case CategoryRateLimit:
		return http.StatusTooManyRequests
	case CategoryDependency:
		return http.StatusServiceUnavailable
	case CategoryTimeout:
		return http.StatusGatewayTimeout
	case CategoryCanceled:
		return http.StatusRequestTimeout
	default:
		return http.StatusInternalServerError
	}
}

func legacyCode(category Category) int {
	switch category {
	case CategoryValidation, CategoryConflict, CategoryRateLimit, CategoryCanceled:
		return CodeBadRequest
	case CategoryAuthentication:
		return CodeUnauthorized
	case CategoryAuthorization:
		return CodeForbidden
	case CategoryNotFound:
		return CodeNotFound
	default:
		return CodeInternal
	}
}

func BadRequest(message string) *Error {
	return LegacyNew(CodeBadRequest, http.StatusBadRequest, message)
}

func BadRequestKey(messageID string, templateData map[string]any, fallback string) *Error {
	return NewKey(CodeBadRequest, http.StatusBadRequest, messageID, templateData, fallback)
}

func Unauthorized(message string) *Error {
	return LegacyNew(CodeUnauthorized, http.StatusUnauthorized, message)
}

func UnauthorizedKey(messageID string, templateData map[string]any, fallback string) *Error {
	return NewKey(CodeUnauthorized, http.StatusUnauthorized, messageID, templateData, fallback)
}

func Forbidden(message string) *Error {
	return LegacyNew(CodeForbidden, http.StatusForbidden, message)
}

func ForbiddenKey(messageID string, templateData map[string]any, fallback string) *Error {
	return NewKey(CodeForbidden, http.StatusForbidden, messageID, templateData, fallback)
}

func NotFound(message string) *Error {
	return LegacyNew(CodeNotFound, http.StatusNotFound, message)
}

func NotFoundKey(messageID string, templateData map[string]any, fallback string) *Error {
	return NewKey(CodeNotFound, http.StatusNotFound, messageID, templateData, fallback)
}

func Internal(message string) *Error {
	return LegacyNew(CodeInternal, http.StatusInternalServerError, message)
}

func InternalKey(messageID string, templateData map[string]any, fallback string) *Error {
	return NewKey(CodeInternal, http.StatusInternalServerError, messageID, templateData, fallback)
}

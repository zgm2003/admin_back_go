package apperror

import (
	"errors"
	"net/http"
	"testing"
)

func TestPredefinedErrorsUseLegacyCompatibleCodes(t *testing.T) {
	cases := []struct {
		name       string
		err        *Error
		code       int
		httpStatus int
	}{
		{name: "bad request", err: BadRequest("参数错误"), code: 100, httpStatus: http.StatusBadRequest},
		{name: "unauthorized", err: Unauthorized("未登录"), code: 401, httpStatus: http.StatusUnauthorized},
		{name: "forbidden", err: Forbidden("无权限访问"), code: 403, httpStatus: http.StatusForbidden},
		{name: "not found", err: NotFound("资源不存在"), code: 404, httpStatus: http.StatusNotFound},
		{name: "internal", err: Internal("系统错误"), code: 500, httpStatus: http.StatusInternalServerError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err.Code != tc.code {
				t.Fatalf("expected code %d, got %d", tc.code, tc.err.Code)
			}
			if tc.err.HTTPStatus != tc.httpStatus {
				t.Fatalf("expected http status %d, got %d", tc.httpStatus, tc.err.HTTPStatus)
			}
		})
	}
}

func TestErrorWrapsCause(t *testing.T) {
	cause := errors.New("db failed")
	err := Wrap(500, http.StatusInternalServerError, "系统错误", cause)

	if !errors.Is(err, cause) {
		t.Fatalf("expected wrapped cause")
	}
	if err.Error() != "系统错误" {
		t.Fatalf("expected public message, got %q", err.Error())
	}
}

func TestKeyedErrorPreservesFallbackMessage(t *testing.T) {
	err := UnauthorizedKey("auth.token.missing", nil, "缺少Token")
	if err.Code != CodeUnauthorized || err.HTTPStatus != http.StatusUnauthorized {
		t.Fatalf("unexpected error codes: %#v", err)
	}
	if err.MessageID != "auth.token.missing" {
		t.Fatalf("expected message id, got %q", err.MessageID)
	}
	if err.Message != "缺少Token" || err.Error() != "缺少Token" {
		t.Fatalf("fallback message broken: %#v", err)
	}
}

func TestKeyedErrorTemplateDataIsStored(t *testing.T) {
	data := map[string]any{"field": "email"}
	err := BadRequestKey("common.request.invalid", data, "参数错误")
	if err.TemplateData["field"] != "email" {
		t.Fatalf("expected template data to be stored, got %#v", err.TemplateData)
	}
}

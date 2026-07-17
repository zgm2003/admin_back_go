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
		legacyCode int
		code       string
		category   Category
		httpStatus int
	}{
		{name: "bad request", err: BadRequest("参数错误"), legacyCode: 100, code: "request.invalid", category: CategoryValidation, httpStatus: http.StatusBadRequest},
		{name: "unauthorized", err: Unauthorized("未登录"), legacyCode: 401, code: "auth.unauthenticated", category: CategoryAuthentication, httpStatus: http.StatusUnauthorized},
		{name: "forbidden", err: Forbidden("无权限访问"), legacyCode: 403, code: "auth.forbidden", category: CategoryAuthorization, httpStatus: http.StatusForbidden},
		{name: "not found", err: NotFound("资源不存在"), legacyCode: 404, code: "resource.not_found", category: CategoryNotFound, httpStatus: http.StatusNotFound},
		{name: "internal", err: Internal("系统错误"), legacyCode: 500, code: "internal.unknown", category: CategoryInternal, httpStatus: http.StatusInternalServerError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err.LegacyCode != tc.legacyCode {
				t.Fatalf("expected legacy code %d, got %d", tc.legacyCode, tc.err.LegacyCode)
			}
			if tc.err.Code != tc.code || tc.err.Category != tc.category {
				t.Fatalf("expected code/category %q/%q, got %q/%q", tc.code, tc.category, tc.err.Code, tc.err.Category)
			}
			if tc.err.HTTPStatus != tc.httpStatus {
				t.Fatalf("expected http status %d, got %d", tc.httpStatus, tc.err.HTTPStatus)
			}
			if tc.err.Retryable() {
				t.Fatalf("legacy helper should default to permanent: %+v", tc.err)
			}
		})
	}
}

func TestErrorPreservesCauseAndExposesSafeMetadata(t *testing.T) {
	cause := errors.New("dial mysql user:secret@tcp(private:3306)")
	err := Wrap(
		"dependency.mysql",
		CategoryDependency,
		http.StatusServiceUnavailable,
		Retryable,
		"common.dependency_unavailable",
		nil,
		"服务暂不可用",
		cause,
	)

	if !errors.Is(err, cause) {
		t.Fatalf("expected wrapped cause")
	}
	if err.Code != "dependency.mysql" || err.Category != CategoryDependency || !err.Retryable() {
		t.Fatalf("unexpected classification: %+v", err)
	}
	if err.LegacyCode != CodeInternal || err.HTTPStatus != http.StatusServiceUnavailable {
		t.Fatalf("unexpected transport compatibility: %+v", err)
	}
	if err.Error() != "服务暂不可用" {
		t.Fatalf("expected public message, got %q", err.Error())
	}
}

func TestKeyedErrorPreservesFallbackMessage(t *testing.T) {
	err := UnauthorizedKey("auth.token.missing", nil, "缺少Token")
	if err.LegacyCode != CodeUnauthorized || err.Code != "auth.unauthenticated" || err.HTTPStatus != http.StatusUnauthorized {
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

func TestLegacyConstructorPreservesCustomNumericCode(t *testing.T) {
	err := NewKey(http.StatusServiceUnavailable, http.StatusServiceUnavailable, "realtime.disabled", nil, "Realtime未启用")
	if err.LegacyCode != http.StatusServiceUnavailable {
		t.Fatalf("expected legacy code %d, got %d", http.StatusServiceUnavailable, err.LegacyCode)
	}
	if err.Code != "internal.unknown" || err.Category != CategoryInternal {
		t.Fatalf("unexpected stable classification: %+v", err)
	}
}

func TestWithCodeAndOperationCloneWithoutMutatingOriginal(t *testing.T) {
	original := BadRequestKey("common.request.invalid", map[string]any{"field": "email"}, "参数错误")

	coded := original.WithCode("user.email.invalid")
	operated := coded.WithOperation("user.validate_email")
	operated.TemplateData["field"] = "phone"

	if original.Code != "request.invalid" || original.Operation != "" {
		t.Fatalf("original mutated: %+v", original)
	}
	if original.TemplateData["field"] != "email" {
		t.Fatalf("original template data mutated: %#v", original.TemplateData)
	}
	if coded.Code != "user.email.invalid" || coded.Operation != "" {
		t.Fatalf("coded clone mismatch: %+v", coded)
	}
	if operated.Code != "user.email.invalid" || operated.Operation != "user.validate_email" {
		t.Fatalf("operated clone mismatch: %+v", operated)
	}
}

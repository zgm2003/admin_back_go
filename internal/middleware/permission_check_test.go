package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"admin_back_go/internal/apperror"

	"github.com/gin-gonic/gin"
)

func TestPermissionCheckSkipsRoutesWithoutPermissionRule(t *testing.T) {
	router := newPermissionCheckTestRouter(PermissionCheckConfig{}, nil)
	router.GET("/open", func(c *gin.Context) { c.String(http.StatusCreated, "open") })

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/open", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated || recorder.Body.String() != "open" {
		t.Fatalf("expected open route to continue, got %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestPermissionCheckCallsCheckerWithRoutePermissionCode(t *testing.T) {
	var got PermissionInput
	router := newPermissionCheckTestRouter(PermissionCheckConfig{
		Rules: map[RouteKey]string{NewRouteKey(http.MethodPost, "/api/v1/permissions"): "permission:create"},
		Checker: func(ctx context.Context, input PermissionInput) *apperror.Error {
			got = input
			return nil
		},
	}, &AuthIdentity{UserID: 12, SessionID: 34, Platform: "admin"})
	router.POST("/api/v1/permissions", func(c *gin.Context) { c.String(http.StatusOK, "created") })

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/permissions", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || recorder.Body.String() != "created" {
		t.Fatalf("expected protected route to continue, got %d %s", recorder.Code, recorder.Body.String())
	}
	if got.UserID != 12 || got.SessionID != 34 || got.Platform != "admin" || got.Code != "permission:create" {
		t.Fatalf("unexpected permission input: %#v", got)
	}
	if got.Method != http.MethodPost || got.Path != "/api/v1/permissions" {
		t.Fatalf("unexpected route input: %#v", got)
	}
}

func TestPermissionCheckMatchesGinFullPathForRouteParams(t *testing.T) {
	var got PermissionInput
	router := newPermissionCheckTestRouter(PermissionCheckConfig{
		Rules: map[RouteKey]string{NewRouteKey(http.MethodPut, "/api/v1/permissions/:id"): "permission_permission_edit"},
		Checker: func(ctx context.Context, input PermissionInput) *apperror.Error {
			got = input
			return nil
		},
	}, &AuthIdentity{UserID: 12, SessionID: 34, Platform: "admin"})
	router.PUT("/api/v1/permissions/:id", func(c *gin.Context) { c.String(http.StatusOK, "updated") })

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/v1/permissions/9", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected route to continue, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if got.Code != "permission_permission_edit" || got.Path != "/api/v1/permissions/:id" {
		t.Fatalf("unexpected permission input: %#v", got)
	}
}

func TestPermissionCheckRejectsWhenCheckerDenies(t *testing.T) {
	router := newPermissionCheckTestRouter(PermissionCheckConfig{
		Rules: map[RouteKey]string{NewRouteKey(http.MethodDelete, "/api/v1/permissions/1"): "permission:delete"},
		Checker: func(ctx context.Context, input PermissionInput) *apperror.Error {
			return apperror.Forbidden("无接口权限")
		},
	}, &AuthIdentity{UserID: 12, SessionID: 34, Platform: "admin"})
	router.DELETE("/api/v1/permissions/1", func(c *gin.Context) { c.String(http.StatusOK, "deleted") })

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/permissions/1", nil)
	router.ServeHTTP(recorder, request)

	assertMiddlewareJSONError(t, recorder, http.StatusForbidden, apperror.CodeForbidden, "无接口权限")
}

func TestPermissionCheckFailsClosedWithoutAuthIdentity(t *testing.T) {
	router := newPermissionCheckTestRouter(PermissionCheckConfig{
		Rules:   map[RouteKey]string{NewRouteKey(http.MethodPost, "/api/v1/permissions"): "permission:create"},
		Checker: func(ctx context.Context, input PermissionInput) *apperror.Error { return nil },
	}, nil)
	router.POST("/api/v1/permissions", func(c *gin.Context) { c.String(http.StatusOK, "created") })

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/permissions", nil)
	router.ServeHTTP(recorder, request)

	assertMiddlewareJSONError(t, recorder, http.StatusUnauthorized, apperror.CodeUnauthorized, "Token无效或已过期")
}

func newPermissionCheckTestRouter(cfg PermissionCheckConfig, identity *AuthIdentity) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	if identity != nil {
		router.Use(func(c *gin.Context) {
			c.Set(ContextAuthIdentity, identity)
			c.Next()
		})
	}
	router.Use(PermissionCheck(cfg))
	return router
}

func assertMiddlewareJSONError(t *testing.T, recorder *httptest.ResponseRecorder, httpStatus int, code int, msg string) {
	t.Helper()
	if recorder.Code != httpStatus {
		t.Fatalf("expected http status %d, got %d body=%s", httpStatus, recorder.Code, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if body["code"] != float64(code) || body["msg"] != msg {
		t.Fatalf("unexpected error body: %#v", body)
	}
}

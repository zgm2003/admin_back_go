package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestOperationLogSkipsRoutesWithoutOperationRule(t *testing.T) {
	called := false
	router := newOperationLogTestRouter(OperationLogConfig{
		Recorder: func(ctx context.Context, input OperationInput) error {
			called = true
			return nil
		},
	}, nil)
	router.POST("/api/admin/v1/no-log", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/no-log", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || recorder.Body.String() != "ok" {
		t.Fatalf("expected route to continue, got %d %s", recorder.Code, recorder.Body.String())
	}
	if called {
		t.Fatalf("expected recorder not to be called for route without operation rule")
	}
}

func TestOperationLogRecordsMatchedRouteAfterHandler(t *testing.T) {
	var got OperationInput
	router := newOperationLogTestRouter(OperationLogConfig{
		Rules: map[RouteKey]OperationRule{
			NewRouteKey(http.MethodPost, "/api/admin/v1/permissions"): {Module: "permission", Action: "create", Title: "新增菜单"},
		},
		Recorder: func(ctx context.Context, input OperationInput) error {
			got = input
			return nil
		},
	}, &AuthIdentity{UserID: 12, SessionID: 34, Platform: "admin"})
	router.POST("/api/admin/v1/permissions", func(c *gin.Context) { c.String(http.StatusCreated, "created") })

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/permissions", nil)
	request.Header.Set(HeaderRequestID, "rid-operation")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated || recorder.Body.String() != "created" {
		t.Fatalf("expected route response to stay intact, got %d %s", recorder.Code, recorder.Body.String())
	}
	if got.UserID != 12 || got.SessionID != 34 || got.Platform != "admin" {
		t.Fatalf("unexpected identity fields: %#v", got)
	}
	if got.Module != "permission" || got.Action != "create" || got.Title != "新增菜单" {
		t.Fatalf("unexpected operation rule fields: %#v", got)
	}
	if got.Method != http.MethodPost || got.Path != "/api/admin/v1/permissions" || got.Status != http.StatusCreated || !got.Success {
		t.Fatalf("unexpected request/status fields: %#v", got)
	}
	if got.RequestID != "rid-operation" {
		t.Fatalf("expected request id rid-operation, got %q", got.RequestID)
	}
}

func TestOperationLogMatchesGinFullPathForRouteParams(t *testing.T) {
	var got OperationInput
	router := newOperationLogTestRouter(OperationLogConfig{
		Rules: map[RouteKey]OperationRule{
			NewRouteKey(http.MethodDelete, "/api/admin/v1/permissions/:id"): {Module: "permission", Action: "delete", Title: "删除菜单"},
		},
		Recorder: func(ctx context.Context, input OperationInput) error {
			got = input
			return nil
		},
	}, &AuthIdentity{UserID: 12, SessionID: 34, Platform: "admin"})
	router.DELETE("/api/admin/v1/permissions/:id", func(c *gin.Context) { c.String(http.StatusOK, "deleted") })

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/api/admin/v1/permissions/9", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected route to continue, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if got.Action != "delete" || got.Path != "/api/admin/v1/permissions/:id" {
		t.Fatalf("unexpected operation input: %#v", got)
	}
}

func TestOperationLogDoesNotBreakResponseWhenRecorderFails(t *testing.T) {
	router := newOperationLogTestRouter(OperationLogConfig{
		Rules: map[RouteKey]OperationRule{
			NewRouteKey(http.MethodDelete, "/api/admin/v1/permissions/1"): {Module: "permission", Action: "delete", Title: "删除菜单"},
		},
		Recorder: func(ctx context.Context, input OperationInput) error {
			return errors.New("insert log failed")
		},
	}, &AuthIdentity{UserID: 12, SessionID: 34, Platform: "admin"})
	router.DELETE("/api/admin/v1/permissions/1", func(c *gin.Context) { c.String(http.StatusOK, "deleted") })

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/api/admin/v1/permissions/1", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || recorder.Body.String() != "deleted" {
		t.Fatalf("expected operation log failure not to alter response, got %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestOperationLogRecordsFailedConfiguredRouteWithStatusAndSuccess(t *testing.T) {
	var got OperationInput
	router := newOperationLogTestRouter(OperationLogConfig{
		Rules: map[RouteKey]OperationRule{
			NewRouteKey(http.MethodPut, "/api/admin/v1/users/:id"): {Module: "user", Action: "update", Title: "编辑用户"},
		},
		Recorder: func(ctx context.Context, input OperationInput) error {
			got = input
			return nil
		},
	}, &AuthIdentity{UserID: 12, SessionID: 34, Platform: "admin"})
	router.PUT("/api/admin/v1/users/:id", func(c *gin.Context) { c.JSON(http.StatusBadRequest, gin.H{"code": 100, "msg": "参数错误"}) })

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/admin/v1/users/9", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request response, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if got.Status != http.StatusBadRequest || got.Success {
		t.Fatalf("failed configured route should be logged with status/success=false: %#v", got)
	}
	if got.Path != "/api/admin/v1/users/:id" || got.Module != "user" || got.Action != "update" {
		t.Fatalf("route metadata mismatch: %#v", got)
	}
}

func newOperationLogTestRouter(cfg OperationLogConfig, identity *AuthIdentity) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID())
	if identity != nil {
		router.Use(func(c *gin.Context) {
			c.Set(ContextAuthIdentity, identity)
			c.Next()
		})
	}
	router.Use(OperationLog(cfg))
	return router
}

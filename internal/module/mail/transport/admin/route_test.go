package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"admin_back_go/internal/middleware"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

func TestMailLogReadsRequirePermissionAndAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := adminroute.NewRegistry()
	permissionRules := registry.PermissionRules()
	service := &fakeMailHTTPService{}
	principalPermissions := map[string]struct{}{}
	checkedCode := ""

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 41, SessionID: 51, Platform: "admin"})
		c.Next()
	})
	router.Use(middleware.PermissionCheck(middleware.PermissionCheckConfig{
		Rules: permissionRules,
		Checker: func(ctx context.Context, input middleware.PermissionInput) *apperror.Error {
			checkedCode = input.Code
			if _, allowed := principalPermissions[input.Code]; !allowed {
				return apperror.Forbidden("permission denied")
			}
			return nil
		},
	}))
	RegisterRoutes(router, service, registry)

	routes := router.Routes()
	actual := make([]adminroute.Route, 0, len(routes))
	for _, route := range routes {
		actual = append(actual, adminroute.Route{Method: route.Method, Path: route.Path})
	}
	if err := registry.CompileRoutes(actual); err != nil {
		t.Fatalf("CompileRoutes: %v", err)
	}

	expected := map[string]struct {
		action string
		title  string
	}{
		"/api/admin/v1/mail/logs":     {action: "list_logs", title: "查看邮件日志及验证码"},
		"/api/admin/v1/mail/logs/:id": {action: "view_log", title: "查看单条邮件日志及验证码"},
	}
	definitions := registry.Definitions()
	for path, want := range expected {
		var definition *adminroute.Definition
		for index := range definitions {
			if definitions[index].Method == http.MethodGet && definitions[index].Path == path {
				definition = &definitions[index]
				break
			}
		}
		if definition == nil {
			t.Fatalf("missing GET route definition for %s", path)
		}
		if definition.Access.Kind != adminroute.AccessPermission || definition.Access.PermissionCode != "system_mail_logView" {
			t.Fatalf("%s access=%+v", path, definition.Access)
		}
		if !definition.Audit.Enabled || !definition.Audit.Required || definition.Audit.Module != "mail" ||
			definition.Audit.Action != want.action || definition.Audit.Title != want.title ||
			!definition.Audit.SkipRequestPayload || !definition.Audit.SkipResponsePayload {
			t.Fatalf("%s audit=%+v", path, definition.Audit)
		}
		key := middleware.NewRouteKey(http.MethodGet, path)
		if permissionRules[key] != "system_mail_logView" {
			t.Fatalf("%s permission runtime rule=%q", path, permissionRules[key])
		}
		operation := registry.OperationRules()[key]
		if !operation.Required || !operation.SkipRequestPayload || !operation.SkipResponsePayload ||
			operation.Module != "mail" || operation.Action != want.action || operation.Title != want.title {
			t.Fatalf("%s operation runtime rule=%+v", path, operation)
		}
	}

	for _, principalCode := range []string{"system_mail", "system_mail_logDel"} {
		for _, target := range []string{"/api/admin/v1/mail/logs", "/api/admin/v1/mail/logs/7"} {
			t.Run(principalCode+" "+target, func(t *testing.T) {
				principalPermissions = map[string]struct{}{principalCode: {}}
				checkedCode = ""
				service.logsCalls = 0
				service.logCalls = 0
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
				if recorder.Code != http.StatusForbidden {
					t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
				}
				if checkedCode != "system_mail_logView" {
					t.Fatalf("permission code=%q", checkedCode)
				}
				if service.logsCalls != 0 || service.logCalls != 0 {
					t.Fatalf("permission denial reached handler: logs=%d log=%d", service.logsCalls, service.logCalls)
				}
			})
		}
	}
}

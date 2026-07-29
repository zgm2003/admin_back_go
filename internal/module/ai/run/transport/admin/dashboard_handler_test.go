package admin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin_back_go/internal/middleware"
	airunmodule "admin_back_go/internal/module/ai/run"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

type dashboardHTTPService struct {
	nilHTTPService
	dashboardFilter airunmodule.DashboardFilter
	dashboardCalls  int
	dashboardResult *airunmodule.DashboardResponse
	dashboardErr    *apperror.Error
	pageInitFilter  airunmodule.PageInitFilter
}

func (service *dashboardHTTPService) Dashboard(_ context.Context, filter airunmodule.DashboardFilter) (*airunmodule.DashboardResponse, *apperror.Error) {
	service.dashboardCalls++
	service.dashboardFilter = filter
	return service.dashboardResult, service.dashboardErr
}

func (service *dashboardHTTPService) PageInit(_ context.Context, filter airunmodule.PageInitFilter) (*airunmodule.InitResponse, *apperror.Error) {
	service.pageInitFilter = filter
	return &airunmodule.InitResponse{}, nil
}

func TestDashboardHandlerBindsEveryFilterAndReturnsCompleteResponse(t *testing.T) {
	service := &dashboardHTTPService{dashboardResult: completeDashboardHandlerResponse()}
	router := dashboardHandlerRouter(service)
	request := httptest.NewRequest(http.MethodGet,
		"/api/admin/v1/ai-runs/dashboard?date_start=2026-07-23&date_end=2026-07-29&platform=admin&model_id=gpt-5.5&agent_id=2&provider_id=3&user_id=4", nil)
	request.Header.Set(middleware.HeaderRequestID, "dashboard-request-7")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	filter := service.dashboardFilter
	if service.dashboardCalls != 1 || filter.RequestID != "dashboard-request-7" || filter.DateStart != "2026-07-23" ||
		filter.DateEnd != "2026-07-29" || filter.Platform != "admin" || filter.ModelID != "gpt-5.5" ||
		filter.AgentID == nil || *filter.AgentID != 2 || filter.ProviderID == nil || *filter.ProviderID != 3 ||
		filter.UserID == nil || *filter.UserID != 4 {
		t.Fatalf("dashboard filter=%+v calls=%d", filter, service.dashboardCalls)
	}
	for _, fragment := range []string{`"generated_at":"2026-07-29T15:42:18+08:00"`, `"trend":[]`, `"models":[]`, `"errors":[]`, `"tools":[]`} {
		if !strings.Contains(recorder.Body.String(), fragment) {
			t.Fatalf("complete dashboard response missing %s: %s", fragment, recorder.Body.String())
		}
	}
}

func TestPageInitHandlerBindsDashboardDateRange(t *testing.T) {
	service := &dashboardHTTPService{}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/admin/v1/ai-runs/page-init", NewHandler(service).PageInit)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
		"/api/admin/v1/ai-runs/page-init?date_start=2026-07-23&date_end=2026-07-29", nil))

	if recorder.Code != http.StatusOK || service.pageInitFilter.DateStart != "2026-07-23" || service.pageInitFilter.DateEnd != "2026-07-29" {
		t.Fatalf("status=%d body=%s filter=%+v", recorder.Code, recorder.Body.String(), service.pageInitFilter)
	}
}

func TestDashboardHandlerRejectsInvalidPositiveIDs(t *testing.T) {
	for _, query := range []string{"agent_id=0", "provider_id=-1", "user_id=0"} {
		t.Run(query, func(t *testing.T) {
			service := &dashboardHTTPService{dashboardResult: completeDashboardHandlerResponse()}
			router := dashboardHandlerRouter(service)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/v1/ai-runs/dashboard?"+query, nil))
			if recorder.Code != http.StatusBadRequest || service.dashboardCalls != 0 {
				t.Fatalf("query=%s status=%d body=%s calls=%d", query, recorder.Code, recorder.Body.String(), service.dashboardCalls)
			}
		})
	}
}

func TestDashboardHandlerDoesNotLeakRepositorySQL(t *testing.T) {
	service := &dashboardHTTPService{dashboardErr: apperror.WrapKey(
		apperror.CodeInternal, http.StatusInternalServerError, "airun.dashboard.query_failed", nil,
		"查询AI运行驾驶舱失败", errors.New("SELECT secret FROM ai_runs WHERE password='raw-secret'"),
	)}
	router := dashboardHandlerRouter(service)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/v1/ai-runs/dashboard", nil))

	body := strings.ToLower(recorder.Body.String())
	if recorder.Code != http.StatusInternalServerError || strings.Contains(body, "select secret") || strings.Contains(body, "raw-secret") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestDashboardRouteUsesAIRunListPermissionAndNoAudit(t *testing.T) {
	registry := adminroute.NewRegistry()
	Register(gin.New(), &dashboardHTTPService{}, registry)
	for _, definition := range registry.Definitions() {
		if definition.Method == http.MethodGet && definition.Path == "/api/admin/v1/ai-runs/dashboard" {
			if definition.Access.Kind != adminroute.AccessPermission || definition.Access.PermissionCode != "ai_run_list" ||
				definition.Audit.Enabled || definition.Audit.Reason != "read-only" {
				t.Fatalf("dashboard route policy=%+v", definition)
			}
			return
		}
	}
	t.Fatal("dashboard route is not registered")
}

func TestLegacyAIRunStatsRoutesAreNotRegistered(t *testing.T) {
	registry := adminroute.NewRegistry()
	Register(gin.New(), &dashboardHTTPService{}, registry)
	registered := make(map[string]struct{})
	for _, definition := range registry.Definitions() {
		registered[definition.Method+" "+definition.Path] = struct{}{}
	}
	for _, path := range []string{
		"/api/admin/v1/ai-runs/stats",
		"/api/admin/v1/ai-runs/stats/latency",
		"/api/admin/v1/ai-runs/stats/by-date",
		"/api/admin/v1/ai-runs/stats/by-agent",
		"/api/admin/v1/ai-runs/stats/by-user",
	} {
		if _, exists := registered[http.MethodGet+" "+path]; exists {
			t.Fatalf("legacy route is still registered: %s", path)
		}
	}
}

func dashboardHandlerRouter(service *dashboardHTTPService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestID())
	router.GET("/api/admin/v1/ai-runs/dashboard", NewHandler(service).Dashboard)
	return router
}

func completeDashboardHandlerResponse() *airunmodule.DashboardResponse {
	return &airunmodule.DashboardResponse{
		GeneratedAt: "2026-07-29T15:42:18+08:00",
		Timezone:    "Asia/Shanghai",
		DateRange: airunmodule.DashboardDateRange{
			StartAt: "2026-07-23T00:00:00+08:00", EndExclusive: "2026-07-30T00:00:00+08:00",
		},
		Trend: []airunmodule.DashboardTrendItem{},
		Breakdowns: airunmodule.DashboardBreakdowns{
			Models: []airunmodule.DashboardModelBreakdown{}, Providers: []airunmodule.DashboardProviderBreakdown{},
			Agents: []airunmodule.DashboardAgentBreakdown{}, Users: []airunmodule.DashboardUserBreakdown{},
			Errors: []airunmodule.DashboardErrorBreakdown{}, Tools: []airunmodule.DashboardToolBreakdown{},
		},
		Anomalies: airunmodule.DashboardAnomalies{RunItems: []airunmodule.DashboardAnomalyItem{}, BillingItems: []airunmodule.DashboardAnomalyItem{}},
	}
}

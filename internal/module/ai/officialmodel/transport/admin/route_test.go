package admin

import (
	"context"
	"net/http"
	"testing"

	"admin_back_go/internal/module/ai/officialmodel"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

func TestOfficialModelRoutesUseFinalPermissionsAndAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registry := adminroute.NewRegistry()
	Register(router, &fakeHTTPService{}, registry)

	definitions := make(map[string]adminroute.Definition)
	for _, definition := range registry.Definitions() {
		definitions[definition.Method+" "+definition.Path] = definition
	}
	if len(definitions) != 5 {
		t.Fatalf("route count = %d", len(definitions))
	}
	for _, path := range []string{
		"/api/admin/v1/ai-official-models/page-init",
		"/api/admin/v1/ai-official-models",
		"/api/admin/v1/ai-official-models/:model_id",
	} {
		definition := definitions[http.MethodGet+" "+path]
		if definition.Access.PermissionCode != "ai_official_model_list" || definition.Audit.Enabled || definition.Audit.Reason != "read-only" {
			t.Fatalf("GET %s policy = %#v", path, definition)
		}
	}
	for _, expected := range []struct {
		method string
		path   string
		action string
	}{
		{http.MethodPut, "/api/admin/v1/ai-official-models/:model_id/price", "sync_price"},
		{http.MethodDelete, "/api/admin/v1/ai-official-models/:model_id/price-override", "restore_official_price"},
	} {
		definition := definitions[expected.method+" "+expected.path]
		if definition.Access.PermissionCode != "ai_official_model_price_sync" || !definition.Audit.Enabled || definition.Audit.Module != "ai_official_model" || definition.Audit.Action != expected.action {
			t.Fatalf("%s %s policy = %#v", expected.method, expected.path, definition)
		}
	}
}

type fakeHTTPService struct {
	setInput   officialmodel.SetPriceOverrideInput
	setCalls   int
	restoreID  uint64
	listCalls  int
	serviceErr *apperror.Error
}

func (fake *fakeHTTPService) PageInit(context.Context) (*officialmodel.PageInitResponse, *apperror.Error) {
	return &officialmodel.PageInitResponse{}, fake.serviceErr
}

func (fake *fakeHTTPService) List(context.Context, officialmodel.ListQuery) (*officialmodel.ListResponse, *apperror.Error) {
	fake.listCalls++
	return &officialmodel.ListResponse{}, fake.serviceErr
}

func (fake *fakeHTTPService) Detail(context.Context, string) (*officialmodel.OfficialModelDTO, *apperror.Error) {
	return &officialmodel.OfficialModelDTO{}, fake.serviceErr
}

func (fake *fakeHTTPService) SetPriceOverride(_ context.Context, _ string, input officialmodel.SetPriceOverrideInput) (*officialmodel.MutationSummary, *apperror.Error) {
	fake.setCalls++
	fake.setInput = input
	return testMutationSummary(), fake.serviceErr
}

func (fake *fakeHTTPService) RestoreOfficialPrice(_ context.Context, _ string, _ int64, administratorID uint64) (*officialmodel.MutationSummary, *apperror.Error) {
	fake.restoreID = administratorID
	return testMutationSummary(), fake.serviceErr
}

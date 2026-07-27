package admin

import (
	"context"
	"net/http"
	"testing"

	"admin_back_go/internal/module/ai/modelpricing"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

func TestRegisterDefinesModelPricingAccessAndAudit(t *testing.T) {
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
	for _, path := range []string{"/api/admin/v1/ai-model-prices/page-init", "/api/admin/v1/ai-model-prices", "/api/admin/v1/ai-model-prices/:model_id"} {
		definition := definitions[http.MethodGet+" "+path]
		if definition.Access.PermissionCode != "ai_model_pricing_list" || definition.Audit.Enabled || definition.Audit.Reason != "read-only" {
			t.Fatalf("GET %s policy = %#v", path, definition)
		}
	}
	for _, expected := range []struct {
		method string
		path   string
		action string
	}{
		{http.MethodPut, "/api/admin/v1/ai-model-prices/:model_id", "update"},
		{http.MethodDelete, "/api/admin/v1/ai-model-prices/:model_id/override", "restore_official"},
	} {
		definition := definitions[expected.method+" "+expected.path]
		if definition.Access.PermissionCode != "ai_model_pricing_edit" || !definition.Audit.Enabled || definition.Audit.Module != "ai_model_pricing" || definition.Audit.Action != expected.action {
			t.Fatalf("%s %s policy = %#v", expected.method, expected.path, definition)
		}
	}
}

type fakeHTTPService struct {
	setInput   modelpricing.SetOverrideInput
	setCalls   int
	restoreID  uint64
	listCalls  int
	serviceErr *apperror.Error
}

func (f *fakeHTTPService) PageInit(context.Context) (*modelpricing.PageInitResponse, *apperror.Error) {
	return &modelpricing.PageInitResponse{}, f.serviceErr
}
func (f *fakeHTTPService) List(context.Context, modelpricing.ListQuery) (*modelpricing.ListResponse, *apperror.Error) {
	f.listCalls++
	return &modelpricing.ListResponse{}, f.serviceErr
}
func (f *fakeHTTPService) Detail(context.Context, string) (*modelpricing.ModelPriceDTO, *apperror.Error) {
	return &modelpricing.ModelPriceDTO{}, f.serviceErr
}
func (f *fakeHTTPService) SetOverride(_ context.Context, _ string, input modelpricing.SetOverrideInput) (*modelpricing.MutationSummary, *apperror.Error) {
	f.setCalls++
	f.setInput = input
	return testMutationSummary(), f.serviceErr
}
func (f *fakeHTTPService) RestoreOfficial(_ context.Context, _ string, _ int64, administratorID uint64) (*modelpricing.MutationSummary, *apperror.Error) {
	f.restoreID = administratorID
	return testMutationSummary(), f.serviceErr
}

package admin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/ai/officialmodel"
	"admin_back_go/internal/module/ai/pricing"

	"github.com/gin-gonic/gin"
)

func TestOfficialModelPriceMutationCannotChangeCatalogFacts(t *testing.T) {
	router, service := newOfficialModelTestRouter()
	body := `{"expected_version":0,"rates":[{"category":"input","unit":"token","tier_key":"","price":"1.25","unit_scale":1000000}],"source_url":"https://openai.com/pricing","verified_at":"2026-07-27"}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/admin/v1/ai-official-models/gpt-reviewed/price", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.setCalls != 1 || service.setInput.AdministratorID != 7 || service.setInput.Prices[0].Price != "1.25" {
		t.Fatalf("status=%d calls=%d input=%#v body=%s", recorder.Code, service.setCalls, service.setInput, recorder.Body.String())
	}

	for _, field := range []string{`"model_id":"other"`, `"max_output_tokens":1`, `"supports_tools":true`} {
		recorder = httptest.NewRecorder()
		forged := body[:len(body)-1] + "," + field + "}"
		request = httptest.NewRequest(http.MethodPut, "/api/admin/v1/ai-official-models/gpt-reviewed/price", bytes.NewBufferString(forged))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest || service.setCalls != 1 {
			t.Fatalf("forged field %s status=%d calls=%d body=%s", field, recorder.Code, service.setCalls, recorder.Body.String())
		}
	}
}

func newOfficialModelTestRouter() (*gin.Engine, *fakeHTTPService) {
	gin.SetMode(gin.TestMode)
	service := &fakeHTTPService{}
	router := gin.New()
	router.Use(func(context *gin.Context) {
		context.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 7, Platform: "admin"})
	})
	Register(router, service)
	return router, service
}

func testMutationSummary() *officialmodel.MutationSummary {
	rates := []pricing.Rate{{Category: pricing.InputTokens, Unit: "token", PriceUnits: 100_000_000, UnitScale: 1_000_000}}
	model := officialmodel.Model{CatalogVersion: "catalog-v3", CatalogVendor: "openai", ModelID: "gpt-reviewed", MaxOutputTokens: 100}
	before := officialmodel.ResolvedModel{
		Model: model, EffectivePrice: pricing.PriceBook{ModelID: model.ModelID, Rates: rates}, PriceSource: officialmodel.PriceSourceOfficial,
		PriceSourceURL: "https://openai.com/pricing", PriceVerifiedAt: time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
	}
	after := before
	after.PriceSource, after.OverrideVersion = officialmodel.PriceSourceOverride, 1
	return &officialmodel.MutationSummary{Before: before, After: after}
}

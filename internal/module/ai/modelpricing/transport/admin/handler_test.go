package admin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/ai/modelpricing"
	"admin_back_go/internal/module/ai/pricing"

	"github.com/gin-gonic/gin"
)

func TestHandlerUpdateUsesAuthenticatedAdministratorAndRejectsUnknownInput(t *testing.T) {
	router, service := newModelPricingTestRouter()
	body := `{"expected_version":0,"rates":[{"category":"input","unit":"token","tier_key":"","price":"1.25","unit_scale":1000000}],"source_url":"https://openai.com/pricing","verified_at":"2026-07-27"}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/admin/v1/ai-model-prices/gpt-reviewed", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.setCalls != 1 || service.setInput.AdministratorID != 7 || service.setInput.Prices[0].Price != "1.25" {
		t.Fatalf("status=%d calls=%d input=%#v body=%s", recorder.Code, service.setCalls, service.setInput, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPut, "/api/admin/v1/ai-model-prices/gpt-reviewed", bytes.NewBufferString(body[:len(body)-1]+`,"administrator_id":99}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || service.setCalls != 1 {
		t.Fatalf("unknown field status=%d calls=%d body=%s", recorder.Code, service.setCalls, recorder.Body.String())
	}

	for _, invalidBody := range []string{
		`{"rates":[{"category":"input","unit":"token","tier_key":"","price":"1.25","unit_scale":1000000}],"source_url":"https://openai.com/pricing","verified_at":"2026-07-27"}`,
		`{"expected_version":0,"rates":[{"category":"input","unit":"token","price":"1.25","unit_scale":1000000}],"source_url":"https://openai.com/pricing","verified_at":"2026-07-27"}`,
	} {
		recorder = httptest.NewRecorder()
		request = httptest.NewRequest(http.MethodPut, "/api/admin/v1/ai-model-prices/gpt-reviewed", bytes.NewBufferString(invalidBody))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest || service.setCalls != 1 {
			t.Fatalf("missing required field status=%d calls=%d body=%s", recorder.Code, service.setCalls, recorder.Body.String())
		}
	}
}

func TestHandlerRejectsUnknownOrDuplicateQueriesBeforeService(t *testing.T) {
	for _, target := range []string{
		"/api/admin/v1/ai-model-prices/page-init?unexpected=1",
		"/api/admin/v1/ai-model-prices?unexpected=1",
		"/api/admin/v1/ai-model-prices?family=gpt&family=claude",
		"/api/admin/v1/ai-model-prices/gpt-reviewed?unexpected=1",
		"/api/admin/v1/ai-model-prices/gpt-reviewed/override?expected_version=2&unexpected=1",
	} {
		router, service := newModelPricingTestRouter()
		recorder := httptest.NewRecorder()
		method := http.MethodGet
		if bytes.Contains([]byte(target), []byte("/override")) {
			method = http.MethodDelete
		}
		router.ServeHTTP(recorder, httptest.NewRequest(method, target, nil))
		if recorder.Code != http.StatusBadRequest || service.listCalls != 0 || service.restoreID != 0 {
			t.Fatalf("%s status=%d list=%d restore=%d body=%s", target, recorder.Code, service.listCalls, service.restoreID, recorder.Body.String())
		}
	}
}

func newModelPricingTestRouter() (*gin.Engine, *fakeHTTPService) {
	gin.SetMode(gin.TestMode)
	service := &fakeHTTPService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 7, Platform: "admin"})
	})
	Register(router, service)
	return router, service
}

func testMutationSummary() *modelpricing.MutationSummary {
	before := pricing.ModelPrice{Version: "catalog-v3", PriceSource: "official", SourceURL: "https://openai.com/pricing", RetrievedAt: "2026-07-27", Rates: []pricing.Rate{{Category: pricing.InputTokens, Unit: "token", PriceUnits: 100_000_000, UnitScale: 1_000_000}}}
	after := before
	after.Version, after.PriceSource, after.OverrideVersion = "catalog-v3:override:1", "override", 1
	return &modelpricing.MutationSummary{Before: modelpricing.PriceSummary{ModelPrice: before}, After: modelpricing.PriceSummary{ModelPrice: after}}
}

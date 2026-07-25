package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"admin_back_go/internal/middleware"
	redeemcode "admin_back_go/internal/module/payment/redeemcode"
	"admin_back_go/internal/module/payment/wallet"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

func TestHandlerGenerateUsesMiddlewareIdentityAndDisablesCaching(t *testing.T) {
	router, service := newRedeemCodeTestRouter()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/payment/redeem-code-batches", bytes.NewBufferString(`{"request_id":"request-1","amount":"10.00","quantity":1}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.generatedBy != 7 || service.generateInput.RequestID != "request-1" {
		t.Fatalf("generate identity/input = %d/%+v", service.generatedBy, service.generateInput)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store, private" {
		t.Fatalf("Cache-Control=%q", got)
	}
	if got := recorder.Header().Get("Pragma"); got != "no-cache" {
		t.Fatalf("Pragma=%q", got)
	}
}

func TestHandlerRedeemUsesMiddlewareIdentityAndWritesLimiterRetryAfter(t *testing.T) {
	router, service := newRedeemCodeTestRouter()
	service.redeemErr = apperror.New("wallet.redeem.unavailable", apperror.CategoryRateLimit, http.StatusTooManyRequests, apperror.Retryable, "wallet.redeem.unavailable", map[string]any{"retry_after": 17}, "limited")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/wallet/redemptions", bytes.NewBufferString(`{"code":"ZHR-2345-6789-ABCD-EFGH-JKMN"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.redeemedBy != 7 || service.redeemedPlatform != "admin" || service.redeemCode != "ZHR-2345-6789-ABCD-EFGH-JKMN" {
		t.Fatalf("redeem identity/input = %d/%q/%q", service.redeemedBy, service.redeemedPlatform, service.redeemCode)
	}
	if got := recorder.Header().Get("Retry-After"); got != "17" {
		t.Fatalf("Retry-After=%q", got)
	}
	assertErrorMeta(t, recorder, "wallet.redeem.rate_limited", "rate_limit", true)
}

func TestHandlerRedemptionResponseExcludesBatchAndIdentityFields(t *testing.T) {
	router, service := newRedeemCodeTestRouter()
	service.redeemResponse = &redeemcode.RedemptionResponse{
		Amount: "2.50", Replayed: true,
		Transaction: wallet.TransactionItem{ID: 9, TransactionNo: "WLT1", UserID: 7, Username: "admin", Account: "secret", Direction: wallet.DirectionIn, AmountCents: 250, AmountText: "2.50", BalanceBeforeCents: 100, BalanceBeforeText: "1.00", BalanceAfterCents: 350, BalanceAfterText: "3.50", SourceType: wallet.SourceRedeemCode, SourceID: 88, Remark: "RCB-SECRET", CreatedAt: "2026-07-25T00:00:00.000000Z"},
		Wallet:      wallet.SummaryResponse{BalanceCents: 350, BalanceText: "3.50"},
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/wallet/redemptions", bytes.NewBufferString(`{"code":"ZHR-2345-6789-ABCD-EFGH-JKMN"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, forbidden := range []string{"RCB-SECRET", `"user_id"`, `"username"`, `"account"`, `"source_id"`, `"remark"`} {
		if bytes.Contains([]byte(body), []byte(forbidden)) {
			t.Fatalf("sensitive redemption field %q leaked in %s", forbidden, body)
		}
	}
	if !bytes.Contains([]byte(body), []byte(`"transaction_no":"WLT1"`)) || !bytes.Contains([]byte(body), []byte(`"wallet"`)) {
		t.Fatalf("redemption facts missing from %s", body)
	}
}

func TestRedeemCanonicalWalletErrorsHaveExactCodesAndMessageIDs(t *testing.T) {
	tests := []struct {
		code     string
		category apperror.Category
		want     string
	}{
		{redeemcode.ErrorWalletCodeRequired, apperror.CategoryValidation, "wallet.redeem.code_required"},
		{redeemcode.ErrorWalletUnavailable, apperror.CategoryValidation, "wallet.redeem.unavailable"},
		{redeemcode.ErrorWalletUnavailable, apperror.CategoryRateLimit, "wallet.redeem.rate_limited"},
		{redeemcode.ErrorWalletRateLimitUnavailable, apperror.CategoryDependency, "wallet.redeem.rate_limit_unavailable"},
		{redeemcode.ErrorWalletDependencyUnavailable, apperror.CategoryDependency, "wallet.redeem.dependency_unavailable"},
		{redeemcode.ErrorWalletIntegrityViolation, apperror.CategoryInternal, "wallet.redeem.integrity_violation"},
	}
	for _, test := range tests {
		got := canonicalWalletError(apperror.New(test.code, test.category, 0, "", test.code, nil, "ignored"))
		if got.Code != test.want || got.MessageID != test.want {
			t.Fatalf("input=%s/%s got=%+v", test.code, test.category, got)
		}
	}
}

func TestGenerateThousandCodeResponseStaysBelowAuditLimit(t *testing.T) {
	router, service := newRedeemCodeTestRouter()
	codes := make([]redeemcode.GeneratedCodeItem, 1000)
	for index := range codes {
		codes[index] = redeemcode.GeneratedCodeItem{ID: int64(index + 1), Code: "ZHR-2345-6789-ABCD-EFGH-JKMN"}
	}
	service.generateResponse = &redeemcode.GenerateBatchResponse{Codes: codes}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/payment/redeem-code-batches", bytes.NewBufferString(`{"request_id":"request-1000","amount":"1.00","quantity":1000}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.Len() >= 1<<20 {
		t.Fatalf("status=%d response bytes=%d", recorder.Code, recorder.Body.Len())
	}
}

func TestHandlerRejectsOversizedOrMalformedBodiesBeforeService(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		code   string
	}{
		{name: "management oversized", method: http.MethodPost, path: "/api/admin/v1/payment/redeem-code-lookups", body: `{"code":"` + string(bytes.Repeat([]byte("a"), 1100)) + `"}`, code: "payment.redeem_code.request_invalid"},
		{name: "management malformed", method: http.MethodPost, path: "/api/admin/v1/payment/redeem-code-lookups", body: `{`, code: "payment.redeem_code.request_invalid"},
		{name: "generate oversized", method: http.MethodPost, path: "/api/admin/v1/payment/redeem-code-batches", body: `{"note":"` + string(bytes.Repeat([]byte("a"), managementBodyLimit)) + `"}`, code: "payment.redeem_code.request_invalid"},
		{name: "export oversized", method: http.MethodPost, path: "/api/admin/v1/payment/redeem-code-exports", body: `{"note":"` + string(bytes.Repeat([]byte("a"), managementBodyLimit)) + `"}`, code: "payment.redeem_code.request_invalid"},
		{name: "void oversized", method: http.MethodPatch, path: "/api/admin/v1/payment/redeem-codes", body: `{"padding":"` + string(bytes.Repeat([]byte("a"), managementBodyLimit)) + `"}`, code: "payment.redeem_code.request_invalid"},
		{name: "wallet oversized", method: http.MethodPost, path: "/api/admin/v1/wallet/redemptions", body: `{"code":"` + string(bytes.Repeat([]byte("a"), 1100)) + `"}`, code: "wallet.redeem.unavailable"},
		{name: "wallet malformed", method: http.MethodPost, path: "/api/admin/v1/wallet/redemptions", body: `{`, code: "wallet.redeem.unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router, service := newRedeemCodeTestRouter()
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(test.method, test.path, bytes.NewBufferString(test.body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if service.calls != 0 {
				t.Fatalf("service calls=%d", service.calls)
			}
			assertErrorMeta(t, recorder, test.code, "validation", false)
		})
	}
}

func TestRedeemHandlersRejectTrailingJSONAndOversizedTrailingContentBeforeService(t *testing.T) {
	tests := []struct {
		name string
		path string
		body string
		code string
	}{
		{name: "lookup trailing JSON", path: "/api/admin/v1/payment/redeem-code-lookups", body: `{"code":"ZHR-2345-6789-ABCD-EFGH-JKMN"}{"code":"other"}`, code: "payment.redeem_code.request_invalid"},
		{name: "lookup trailing garbage", path: "/api/admin/v1/payment/redeem-code-lookups", body: `{"code":"ZHR-2345-6789-ABCD-EFGH-JKMN"} trailing`, code: "payment.redeem_code.request_invalid"},
		{name: "redemption trailing JSON", path: "/api/admin/v1/wallet/redemptions", body: `{"code":"ZHR-2345-6789-ABCD-EFGH-JKMN"}{"code":"other"}`, code: "wallet.redeem.unavailable"},
		{name: "lookup oversized trailing whitespace", path: "/api/admin/v1/payment/redeem-code-lookups", body: `{"code":"ZHR-2345-6789-ABCD-EFGH-JKMN"}` + string(bytes.Repeat([]byte(" "), sensitiveBodyLimit)), code: "payment.redeem_code.request_invalid"},
		{name: "redemption oversized trailing whitespace", path: "/api/admin/v1/wallet/redemptions", body: `{"code":"ZHR-2345-6789-ABCD-EFGH-JKMN"}` + string(bytes.Repeat([]byte(" "), sensitiveBodyLimit)), code: "wallet.redeem.unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router, service := newRedeemCodeTestRouter()
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, test.path, bytes.NewBufferString(test.body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest || service.calls != 0 {
				t.Fatalf("status=%d calls=%d body=%s", recorder.Code, service.calls, recorder.Body.String())
			}
			assertErrorMeta(t, recorder, test.code, "validation", false)
		})
	}
}

func TestRedeemHandlersRejectUnknownJSONFieldsBeforeService(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		code   string
	}{
		{name: "lookup", method: http.MethodPost, path: "/api/admin/v1/payment/redeem-code-lookups", body: `{"code":"ZHR-2345-6789-ABCD-EFGH-JKMN","unexpected":true}`, code: "payment.redeem_code.request_invalid"},
		{name: "export code", method: http.MethodPost, path: "/api/admin/v1/payment/redeem-code-exports", body: `{"code":"ZHR-2345-6789-ABCD-EFGH-JKMN"}`, code: "payment.redeem_code.request_invalid"},
		{name: "generate code", method: http.MethodPost, path: "/api/admin/v1/payment/redeem-code-batches", body: `{"request_id":"request-unknown","amount":"1.00","quantity":1,"code":"ZHR-2345-6789-ABCD-EFGH-JKMN"}`, code: "payment.redeem_code.request_invalid"},
		{name: "void code", method: http.MethodPatch, path: "/api/admin/v1/payment/redeem-codes", body: `{"ids":[1],"code":"ZHR-2345-6789-ABCD-EFGH-JKMN"}`, code: "payment.redeem_code.request_invalid"},
		{name: "redemption identity", method: http.MethodPost, path: "/api/admin/v1/wallet/redemptions", body: `{"code":"ZHR-2345-6789-ABCD-EFGH-JKMN","user_id":99,"platform":"untrusted"}`, code: "wallet.redeem.unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router, service := newRedeemCodeTestRouter()
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(test.method, test.path, bytes.NewBufferString(test.body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest || service.calls != 0 {
				t.Fatalf("status=%d calls=%d body=%s", recorder.Code, service.calls, recorder.Body.String())
			}
			assertErrorMeta(t, recorder, test.code, "validation", false)
		})
	}
}

func TestRedeemCodeResponsesDisableCaching(t *testing.T) {
	tests := []struct{ method, path, body string }{
		{http.MethodGet, "/api/admin/v1/payment/redeem-codes", ""},
		{http.MethodPost, "/api/admin/v1/payment/redeem-code-lookups", `{"code":"ZHR-2345-6789-ABCD-EFGH-JKMN"}`},
		{http.MethodPost, "/api/admin/v1/payment/redeem-code-exports", `{}`},
		{http.MethodPost, "/api/admin/v1/payment/redeem-code-batches", `{"request_id":"request-2","amount":"1.00","quantity":1}`},
	}
	for _, test := range tests {
		router, _ := newRedeemCodeTestRouter()
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(test.method, test.path, bytes.NewBufferString(test.body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-store, private" || recorder.Header().Get("Pragma") != "no-cache" {
			t.Fatalf("%s %s status=%d cache=%q pragma=%q", test.method, test.path, recorder.Code, recorder.Header().Get("Cache-Control"), recorder.Header().Get("Pragma"))
		}
	}
}

func TestRedeemCodeListRejectsUnknownOrAmbiguousQueryKeysBeforeService(t *testing.T) {
	fullCode := "ZHR-2345-6789-ABCD-EFGH-JKMN"
	tests := []struct {
		name  string
		query string
	}{
		{name: "full code", query: "?code=" + fullCode},
		{name: "code fragment", query: "?code_fragment=ZHR-2345"},
		{name: "unknown key", query: "?unexpected=1"},
		{name: "duplicate key", query: "?state=used&state=voided"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router, service := newRedeemCodeTestRouter()
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/payment/redeem-codes"+test.query, nil)
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest || service.calls != 0 {
				t.Fatalf("status=%d calls=%d body=%s", recorder.Code, service.calls, recorder.Body.String())
			}
			if bytes.Contains(recorder.Body.Bytes(), []byte(fullCode)) {
				t.Fatalf("full code leaked in error body=%s", recorder.Body.String())
			}
			assertErrorMeta(t, recorder, "payment.redeem_code.request_invalid", "validation", false)
		})
	}
}

func TestRegisterRoutesDefinesRedeemAccessAndAuditContracts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registry := adminroute.NewRegistry()
	RegisterRoutes(router, &fakeHTTPService{}, registry)

	definitions := make(map[string]adminroute.Definition)
	for _, definition := range registry.Definitions() {
		definitions[definition.Method+" "+definition.Path] = definition
	}
	if len(definitions) != 7 {
		t.Fatalf("route count=%d", len(definitions))
	}
	assertRoutePolicy(t, definitions, http.MethodGet, "/api/admin/v1/payment/redeem-codes/page-init", "payment_redeem_code_list", false, false, false, false, "read-only")
	assertRoutePolicy(t, definitions, http.MethodGet, "/api/admin/v1/payment/redeem-codes", "payment_redeem_code_list", false, false, false, false, "read-only")
	assertRoutePolicy(t, definitions, http.MethodPost, "/api/admin/v1/payment/redeem-code-lookups", "payment_redeem_code_list", false, false, false, false, "read-only exact lookup")
	assertRoutePolicy(t, definitions, http.MethodPost, "/api/admin/v1/payment/redeem-code-exports", "payment_redeem_code_list", true, false, true, true, "")
	assertRoutePolicy(t, definitions, http.MethodPost, "/api/admin/v1/payment/redeem-code-batches", "payment_redeem_code_generate", true, true, true, true, "")
	assertRoutePolicy(t, definitions, http.MethodPatch, "/api/admin/v1/payment/redeem-codes", "payment_redeem_code_void", true, true, true, false, "")
	redemption := definitions[http.MethodPost+" /api/admin/v1/wallet/redemptions"]
	if redemption.Access.Kind != adminroute.AccessAuthenticated || !redemption.Audit.Enabled || !redemption.Audit.Required || !redemption.Audit.SkipRequestPayload || !redemption.Audit.SkipResponsePayload {
		t.Fatalf("redemption policy=%+v", redemption)
	}
}

func assertRoutePolicy(t *testing.T, definitions map[string]adminroute.Definition, method, path, permission string, auditEnabled, required, skipRequest, skipResponse bool, reason string) {
	t.Helper()
	definition, ok := definitions[method+" "+path]
	if !ok {
		t.Fatalf("missing route %s %s", method, path)
	}
	if definition.Access.Kind != adminroute.AccessPermission || definition.Access.PermissionCode != permission || definition.Audit.Enabled != auditEnabled || definition.Audit.Required != required || definition.Audit.SkipRequestPayload != skipRequest || definition.Audit.SkipResponsePayload != skipResponse || definition.Audit.Reason != reason {
		t.Fatalf("policy %s %s = %+v", method, path, definition)
	}
}

func assertErrorMeta(t *testing.T, recorder *httptest.ResponseRecorder, wantCode, wantCategory string, wantRetryable bool) {
	t.Helper()
	var body struct {
		Error struct {
			Code      string `json:"code"`
			Category  string `json:"category"`
			Retryable bool   `json:"retryable"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.Error.Code != wantCode || body.Error.Category != wantCategory || body.Error.Retryable != wantRetryable {
		t.Fatalf("error meta=%+v", body.Error)
	}
}

func newRedeemCodeTestRouter() (*gin.Engine, *fakeHTTPService) {
	gin.SetMode(gin.TestMode)
	service := &fakeHTTPService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 7, Platform: "admin"})
	})
	RegisterRoutes(router, service)
	return router, service
}

type fakeHTTPService struct {
	calls            int
	generatedBy      int64
	generateInput    redeemcode.GenerateBatchInput
	redeemedBy       int64
	redeemedPlatform string
	redeemCode       string
	redeemErr        *apperror.Error
	redeemResponse   *redeemcode.RedemptionResponse
	generateResponse *redeemcode.GenerateBatchResponse
}

func (service *fakeHTTPService) PageInit(context.Context) (*redeemcode.PageInitResponse, *apperror.Error) {
	service.calls++
	return &redeemcode.PageInitResponse{}, nil
}
func (service *fakeHTTPService) List(context.Context, redeemcode.ListQuery) (*redeemcode.ListResponse, *apperror.Error) {
	service.calls++
	return &redeemcode.ListResponse{}, nil
}
func (service *fakeHTTPService) Lookup(_ context.Context, input redeemcode.LookupInput) (*redeemcode.LookupResponse, *apperror.Error) {
	service.calls++
	return &redeemcode.LookupResponse{}, nil
}
func (service *fakeHTTPService) Export(context.Context, redeemcode.ExportInput) (*redeemcode.ExportResponse, *apperror.Error) {
	service.calls++
	return &redeemcode.ExportResponse{}, nil
}
func (service *fakeHTTPService) GenerateBatch(_ context.Context, userID int64, input redeemcode.GenerateBatchInput) (*redeemcode.GenerateBatchResponse, *apperror.Error) {
	service.calls++
	service.generatedBy, service.generateInput = userID, input
	if service.generateResponse != nil {
		return service.generateResponse, nil
	}
	return &redeemcode.GenerateBatchResponse{}, nil
}
func (service *fakeHTTPService) Void(context.Context, redeemcode.VoidInput) (*redeemcode.VoidResponse, *apperror.Error) {
	service.calls++
	return &redeemcode.VoidResponse{}, nil
}
func (service *fakeHTTPService) Redeem(_ context.Context, userID int64, platform, code string) (*redeemcode.RedemptionResponse, *apperror.Error) {
	service.calls++
	service.redeemedBy, service.redeemCode = userID, code
	service.redeemedPlatform = platform
	return service.redeemResponse, service.redeemErr
}

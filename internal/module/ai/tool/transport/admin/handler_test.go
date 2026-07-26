package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"admin_back_go/internal/middleware"
	aitext "admin_back_go/internal/module/ai/text"
	aitool "admin_back_go/internal/module/ai/tool"
	"admin_back_go/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

type fakeGenerateDraftService struct {
	nilHTTPService
	input  aitool.GenerateDraftInput
	result *aitool.GenerateDraftResponse
	appErr *apperror.Error
}

func (f *fakeGenerateDraftService) GenerateDraft(_ context.Context, input aitool.GenerateDraftInput) (*aitool.GenerateDraftResponse, *apperror.Error) {
	f.input = input
	return f.result, f.appErr
}

func generateDraftRecorder(t *testing.T, service aitool.HTTPService, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/admin/v1/ai-tools/generate-draft", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 7, Platform: "admin"})
	NewHandler(service).GenerateDraft(c)
	return recorder
}

func TestGenerateDraftRequiresRequestIDAtHTTPBoundary(t *testing.T) {
	service := &fakeGenerateDraftService{result: &aitool.GenerateDraftResponse{OK: false}}
	recorder := generateDraftRecorder(t, service, `{"agent_id":5,"requirement":"生成工具"}`)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if service.input.AgentID != 0 {
		t.Fatalf("service called without request_id: %#v", service.input)
	}
}

func TestGenerateDraftPassesRequestIDToService(t *testing.T) {
	service := &fakeGenerateDraftService{result: &aitool.GenerateDraftResponse{OK: false, Warnings: []string{}, ClarifyingQuestions: []string{"补充参数"}}}
	recorder := generateDraftRecorder(t, service, `{"request_id":"request-1","agent_id":5,"requirement":"生成工具","code_hint":"future_tool"}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if service.input.RequestID != "request-1" || service.input.AgentID != 5 || service.input.UserID != 7 {
		t.Fatalf("service input = %#v", service.input)
	}
}

func TestGenerateDraftInsufficientBalanceReturnsExactNavigationData(t *testing.T) {
	service := &fakeGenerateDraftService{appErr: apperror.New(
		aitext.ErrorCodeInsufficientBalance, apperror.CategoryConflict, http.StatusConflict,
		apperror.Permanent, "", nil, "余额不足，请充值后重试",
	)}
	recorder := generateDraftRecorder(t, service, `{"request_id":"request-1","agent_id":5,"requirement":"生成工具"}`)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Data  map[string]string `json:"data"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error.Code != aitext.ErrorCodeInsufficientBalance || len(body.Data) != 2 || body.Data["wallet_path"] != "/profile/wallet" || body.Data["recharge_path"] != "/payment/recharge" {
		t.Fatalf("response = %#v", body)
	}
}

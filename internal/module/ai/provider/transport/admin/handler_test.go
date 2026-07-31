package admin

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	aiprovider "admin_back_go/internal/module/ai/provider"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/validate"

	"github.com/gin-gonic/gin"
)

type fileModeHTTPService struct {
	nilHTTPService
	createCalls int
	createInput aiprovider.CreateInput
	pageInit    *aiprovider.InitResponse
	list        *aiprovider.ListResponse
}

func (service *fileModeHTTPService) Create(_ context.Context, input aiprovider.CreateInput) (uint64, *apperror.Error) {
	service.createCalls++
	service.createInput = input
	return 9, nil
}

func (service *fileModeHTTPService) PageInit(context.Context) (*aiprovider.InitResponse, *apperror.Error) {
	return service.pageInit, nil
}

func (service *fileModeHTTPService) List(context.Context, aiprovider.ListQuery) (*aiprovider.ListResponse, *apperror.Error) {
	return service.list, nil
}

func TestProviderMutationRequiresExplicitFileInputMode(t *testing.T) {
	service := &fileModeHTTPService{}
	recorder := providerHandlerRecorder(t, service, http.MethodPost, "/api/admin/v1/ai-providers", `{"name":"OpenAI","engine_type":"openai","api_key":"sk-test","model_ids":["gpt-5.6"],"status":1}`)

	if recorder.Code != http.StatusBadRequest || service.createCalls != 0 {
		t.Fatalf("status=%d calls=%d body=%s", recorder.Code, service.createCalls, recorder.Body.String())
	}
}

func TestProviderMutationPassesClosedFileInputMode(t *testing.T) {
	service := &fileModeHTTPService{}
	recorder := providerHandlerRecorder(t, service, http.MethodPost, "/api/admin/v1/ai-providers", `{"name":"OpenAI","engine_type":"openai","api_key":"sk-test","file_input_mode":"chat_completions","model_ids":["gpt-5.6"],"status":1}`)

	if recorder.Code != http.StatusOK || service.createCalls != 1 || service.createInput.FileInputMode != aiprovider.FileInputModeChatCompletions {
		t.Fatalf("status=%d calls=%d input=%#v body=%s", recorder.Code, service.createCalls, service.createInput, recorder.Body.String())
	}
}

func TestProviderResponsesProjectClosedFileInputMode(t *testing.T) {
	service := &fileModeHTTPService{
		pageInit: &aiprovider.InitResponse{Dict: aiprovider.InitDict{FileInputModeArr: []aiprovider.FileInputModeOption{
			{Label: "关闭", Value: aiprovider.FileInputModeDisabled},
			{Label: "Chat Completions", Value: aiprovider.FileInputModeChatCompletions},
		}}},
		list: &aiprovider.ListResponse{List: []aiprovider.ProviderDTO{{ID: 1, FileInputMode: aiprovider.FileInputModeDisabled}}},
	}

	pageInit := providerHandlerRecorder(t, service, http.MethodGet, "/api/admin/v1/ai-providers/page-init", "")
	if pageInit.Code != http.StatusOK || !strings.Contains(pageInit.Body.String(), `"file_input_mode_arr":[{"label":"关闭","value":"disabled"},{"label":"Chat Completions","value":"chat_completions"}]`) {
		t.Fatalf("page-init status=%d body=%s", pageInit.Code, pageInit.Body.String())
	}
	list := providerHandlerRecorder(t, service, http.MethodGet, "/api/admin/v1/ai-providers", "")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"file_input_mode":"disabled"`) {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
}

func providerHandlerRecorder(t *testing.T, service *fileModeHTTPService, method string, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	validate.MustRegister()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(method, path, bytes.NewBufferString(body))
	context.Request.Header.Set("Content-Type", "application/json")
	handler := NewHandler(service)
	switch path {
	case "/api/admin/v1/ai-providers/page-init":
		handler.PageInit(context)
	case "/api/admin/v1/ai-providers":
		if method == http.MethodGet {
			handler.List(context)
		} else {
			handler.Create(context)
		}
	default:
		t.Fatalf("unsupported test path %s", path)
	}
	return recorder
}

package paychannel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"

	"github.com/gin-gonic/gin"
)

type fakeHTTPService struct {
	listQuery   ListQuery
	createInput CreateInput
	updateID    int64
	updateInput UpdateInput
	statusID    int64
	status      int
	deleteID    int64
	called      string
}

func (f *fakeHTTPService) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	f.called = "init"
	return &InitResponse{}, nil
}

func (f *fakeHTTPService) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	f.called = "list"
	f.listQuery = query
	return &ListResponse{List: []ListItem{}, Page: Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize}}, nil
}

func (f *fakeHTTPService) Create(ctx context.Context, input CreateInput) (int64, *apperror.Error) {
	f.called = "create"
	f.createInput = input
	return 1, nil
}

func (f *fakeHTTPService) Update(ctx context.Context, id int64, input UpdateInput) *apperror.Error {
	f.called = "update"
	f.updateID = id
	f.updateInput = input
	return nil
}

func (f *fakeHTTPService) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	f.called = "status"
	f.statusID = id
	f.status = status
	return nil
}

func (f *fakeHTTPService) Delete(ctx context.Context, id int64) *apperror.Error {
	f.called = "delete"
	f.deleteID = id
	return nil
}

func TestHandlerBindsListQuery(t *testing.T) {
	router, service := newPayChannelHandlerRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/pay-channels?current_page=1&page_size=20&name=ali&channel=2&status=1", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if service.called != "list" || service.listQuery.CurrentPage != 1 || service.listQuery.PageSize != 20 || service.listQuery.Name != "ali" {
		t.Fatalf("unexpected list query: %#v", service.listQuery)
	}
	if service.listQuery.Channel == nil || *service.listQuery.Channel != enum.PayChannelAlipay || service.listQuery.Status == nil || *service.listQuery.Status != enum.CommonYes {
		t.Fatalf("unexpected optional list filters: %#v", service.listQuery)
	}
}

func TestHandlerRejectsInvalidPayChannelBeforeService(t *testing.T) {
	router, service := newPayChannelHandlerRouter()
	body := `{"name":"bad","channel":9,"supported_methods":["scan"],"mch_id":"mch","status":1,"is_sandbox":2}`

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/pay-channels", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, recorder.Code, recorder.Body.String())
	}
	if service.called != "" {
		t.Fatalf("service should not be called for invalid channel")
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["code"] != float64(apperror.CodeBadRequest) {
		t.Fatalf("unexpected response payload: %#v", payload)
	}
}

func TestHandlerBindsCreateUpdateStatusDelete(t *testing.T) {
	router, service := newPayChannelHandlerRouter()
	body := `{"name":"支付宝","channel":2,"supported_methods":["web","h5"],"mch_id":"mch","app_id":"app","notify_url":"https://example.test/notify","app_private_key":"plain","public_cert_path":"/pub","platform_cert_path":"/platform","root_cert_path":"/root","sort":1,"is_sandbox":1,"status":1,"remark":"demo"}`

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/pay-channels", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("create expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if service.createInput.Channel != enum.PayChannelAlipay || service.createInput.SupportedMethods[0] != enum.PayMethodWeb || service.createInput.AppPrivateKey != "plain" {
		t.Fatalf("unexpected create input: %#v", service.createInput)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPut, "/api/admin/v1/pay-channels/7", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.updateID != 7 || service.updateInput.MchID != "mch" {
		t.Fatalf("unexpected update result status=%d id=%d input=%#v body=%s", recorder.Code, service.updateID, service.updateInput, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPatch, "/api/admin/v1/pay-channels/7/status", strings.NewReader(`{"status":2}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.statusID != 7 || service.status != enum.CommonNo {
		t.Fatalf("unexpected status result status=%d id=%d value=%d body=%s", recorder.Code, service.statusID, service.status, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/api/admin/v1/pay-channels/7", strings.NewReader(`{"id":99}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.deleteID != 7 {
		t.Fatalf("delete must use route id only, status=%d id=%d body=%s", recorder.Code, service.deleteID, recorder.Body.String())
	}
}

func newPayChannelHandlerRouter() (*gin.Engine, *fakeHTTPService) {
	gin.SetMode(gin.TestMode)
	service := &fakeHTTPService{}
	router := gin.New()
	RegisterRoutes(router, service)
	return router, service
}

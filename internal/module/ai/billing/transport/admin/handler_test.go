package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	aibilling "admin_back_go/internal/module/ai/billing"
	"admin_back_go/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

func TestHandlerRoutesAndPassesCreateInput(t *testing.T) {
	router, service := newAdminTestRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/ai-billing-rules", strings.NewReader(`{"scene":"admin_image_generate","unit":"request","unit_price_cents":25,"status":1}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.createInput.Scene != aibilling.SceneAdminImageGenerate || service.createInput.Unit != aibilling.UnitRequest || service.createInput.UnitPriceCents != 25 {
		t.Fatalf("unexpected create input=%#v", service.createInput)
	}
}

func TestHandlerUpdateDoesNotRequireScene(t *testing.T) {
	router, service := newAdminTestRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/admin/v1/ai-billing-rules/8", strings.NewReader(`{"unit":"image","unit_price_cents":250,"status":2}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.updateID != 8 || service.updateInput.Scene != "" || service.updateInput.Unit != aibilling.UnitImage || service.updateInput.Status != aibilling.RuleStatusDisabled {
		t.Fatalf("unexpected update id/input: %d %#v", service.updateID, service.updateInput)
	}
}

func TestHandlerPageInitAndList(t *testing.T) {
	router, service := newAdminTestRouter()

	for _, path := range []string{"/api/admin/v1/ai-billing-rules/page-init", "/api/admin/v1/ai-billing-rules?current_page=2&page_size=10&scene=admin_image_generate&status=1"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s expected 200, got %d body=%s", path, recorder.Code, recorder.Body.String())
		}
	}
	if !service.pageInitCalled {
		t.Fatalf("page init was not called")
	}
	if service.listQuery.CurrentPage != 2 || service.listQuery.PageSize != 10 || service.listQuery.Scene != aibilling.SceneAdminImageGenerate || service.listQuery.Status == nil || *service.listQuery.Status != aibilling.RuleStatusEnabled {
		t.Fatalf("unexpected list query=%#v", service.listQuery)
	}
}

func TestHandlerStatusAndDeleteOne(t *testing.T) {
	router, service := newAdminTestRouter()

	statusRecorder := httptest.NewRecorder()
	statusRequest := httptest.NewRequest(http.MethodPatch, "/api/admin/v1/ai-billing-rules/9/status", strings.NewReader(`{"status":2}`))
	statusRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(statusRecorder, statusRequest)
	if statusRecorder.Code != http.StatusOK {
		t.Fatalf("status expected 200, got %d body=%s", statusRecorder.Code, statusRecorder.Body.String())
	}

	deleteRecorder := httptest.NewRecorder()
	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/admin/v1/ai-billing-rules/9", nil)
	router.ServeHTTP(deleteRecorder, deleteRequest)
	if deleteRecorder.Code != http.StatusOK {
		t.Fatalf("delete expected 200, got %d body=%s", deleteRecorder.Code, deleteRecorder.Body.String())
	}
	if service.statusID != 9 || service.status != aibilling.RuleStatusDisabled || service.deleteID != 9 {
		t.Fatalf("unexpected status/delete calls: statusID=%d status=%d deleteID=%d", service.statusID, service.status, service.deleteID)
	}
}

func newAdminTestRouter() (*gin.Engine, *fakeHTTPService) {
	gin.SetMode(gin.TestMode)
	service := &fakeHTTPService{}
	router := gin.New()
	Register(router, service)
	return router, service
}

type fakeHTTPService struct {
	pageInitCalled bool
	listQuery      aibilling.ListQuery
	createInput    aibilling.CreateRuleInput
	updateID       uint64
	updateInput    aibilling.UpdateRuleInput
	statusID       uint64
	status         int
	deleteID       uint64
}

func (f *fakeHTTPService) PageInit(ctx context.Context) (*aibilling.PageInitResponse, *apperror.Error) {
	f.pageInitCalled = true
	return &aibilling.PageInitResponse{}, nil
}
func (f *fakeHTTPService) List(ctx context.Context, query aibilling.ListQuery) (*aibilling.ListResponse, *apperror.Error) {
	f.listQuery = query
	return &aibilling.ListResponse{}, nil
}
func (f *fakeHTTPService) CreateRule(ctx context.Context, input aibilling.CreateRuleInput) (uint64, *apperror.Error) {
	f.createInput = input
	return 77, nil
}
func (f *fakeHTTPService) UpdateRule(ctx context.Context, id uint64, input aibilling.UpdateRuleInput) *apperror.Error {
	f.updateID = id
	f.updateInput = input
	return nil
}
func (f *fakeHTTPService) ChangeStatus(ctx context.Context, id uint64, status int) *apperror.Error {
	f.statusID = id
	f.status = status
	return nil
}
func (f *fakeHTTPService) DeleteRule(ctx context.Context, id uint64) *apperror.Error {
	f.deleteID = id
	return nil
}
func (f *fakeHTTPService) EnabledRule(ctx context.Context, scene string) (*aibilling.RuleDTO, *apperror.Error) {
	return nil, nil
}
func (f *fakeHTTPService) BillingRecord(ctx context.Context, id int64) (*aibilling.BillingRecord, *apperror.Error) {
	return &aibilling.BillingRecord{ID: id}, nil
}

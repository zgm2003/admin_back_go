package systemsetting

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
	projecti18n "admin_back_go/internal/shared/i18n"
	"admin_back_go/internal/shared/pagination"

	"github.com/gin-gonic/gin"
)

type fakeHTTPService struct {
	gotList   ListRequest
	gotCreate CreateRequest
	statusID  int64
	status    int
}

func (f *fakeHTTPService) PageInit(context.Context) (*PageInitResponse, *apperror.Error) {
	return &PageInitResponse{Dict: PageInitDict{}}, nil
}

func (f *fakeHTTPService) List(_ context.Context, request ListRequest) (*ListResponse, *apperror.Error) {
	f.gotList = request
	return &ListResponse{
		List: []ListItem{{ID: 1, SettingKey: "user.default_avatar"}},
		Page: pagination.Page{
			CurrentPage: request.CurrentPage,
			PageSize:    request.PageSize,
			Total:       1,
			TotalPage:   1,
		},
	}, nil
}

func (f *fakeHTTPService) Create(_ context.Context, request CreateRequest) (int64, *apperror.Error) {
	f.gotCreate = request
	return 1, nil
}

func (*fakeHTTPService) Update(context.Context, int64, UpdateRequest) *apperror.Error { return nil }
func (*fakeHTTPService) Delete(context.Context, []int64) *apperror.Error              { return nil }

func (f *fakeHTTPService) ChangeStatus(_ context.Context, id int64, status int) *apperror.Error {
	f.statusID, f.status = id, status
	return nil
}

func TestHandlerBindsListQueryWithValidator(t *testing.T) {
	router, service := newSystemSettingHandlerRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/system-settings?current_page=1&page_size=50&status=1&key=user.", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if service.gotList.CurrentPage != 1 || service.gotList.PageSize != 50 || service.gotList.Status == nil || *service.gotList.Status != enum.CommonYes || service.gotList.Key != "user." {
		t.Fatalf("unexpected list request: %#v", service.gotList)
	}
}

func TestHandlerRejectsUnsupportedValueTypeBeforeService(t *testing.T) {
	router, service := newSystemSettingHandlerRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/system-settings", strings.NewReader(`{"key":"feature.switch","value":"true","type":9,"remark":"bad"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, recorder.Code, recorder.Body.String())
	}
	if service.gotCreate.Key != "" {
		t.Fatalf("service should not be called for invalid value type: %#v", service.gotCreate)
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["code"] != float64(apperror.CodeBadRequest) {
		t.Fatalf("unexpected response payload: %#v", payload)
	}
}

func TestHandlerBindsStatusPatchBody(t *testing.T) {
	router, service := newSystemSettingHandlerRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/admin/v1/system-settings/2/status", strings.NewReader(`{"status":2}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if service.statusID != 2 || service.status != enum.CommonNo {
		t.Fatalf("unexpected status input: id=%d status=%d", service.statusID, service.status)
	}
}

func TestHandlerListLocalizesInvalidQuery(t *testing.T) {
	router, _ := newSystemSettingLocalizedHandlerRouter()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/system-settings?current_page=abc", nil)
	request.Header.Set("Accept-Language", "en-US")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, recorder.Code, recorder.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["msg"] != "Invalid system setting list request" {
		t.Fatalf("expected localized list error, got %#v", payload["msg"])
	}
}

func TestHandlerListReturnsListObjectInsteadOfBareArray(t *testing.T) {
	router, _ := newSystemSettingHandlerRouter()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/system-settings?current_page=1&page_size=20", nil)
	router.ServeHTTP(recorder, request)

	var payload struct {
		Code int `json:"code"`
		Data struct {
			List []ListItem      `json:"list"`
			Page pagination.Page `json:"page"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if recorder.Code != http.StatusOK || payload.Code != 0 || len(payload.Data.List) != 1 {
		t.Fatalf("unexpected list response: status=%d payload=%#v", recorder.Code, payload)
	}
}

func TestHandlerReportsMissingService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router, nil)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/system-settings?current_page=1&page_size=20", nil)
	router.ServeHTTP(recorder, request)

	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if recorder.Code != http.StatusInternalServerError || payload["msg"] != "系统设置服务未配置" {
		t.Fatalf("unexpected response: status=%d payload=%#v", recorder.Code, payload)
	}
}

func newSystemSettingHandlerRouter() (*gin.Engine, *fakeHTTPService) {
	gin.SetMode(gin.TestMode)
	service := &fakeHTTPService{}
	router := gin.New()
	RegisterRoutes(router, service)
	return router, service
}

func newSystemSettingLocalizedHandlerRouter() (*gin.Engine, *fakeHTTPService) {
	gin.SetMode(gin.TestMode)
	service := &fakeHTTPService{}
	router := gin.New()
	router.Use(projecti18n.Localize())
	RegisterRoutes(router, service)
	return router, service
}

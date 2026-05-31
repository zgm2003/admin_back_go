package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	systemsettingmodule "admin_back_go/internal/module/systemsetting"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
	projecti18n "admin_back_go/internal/shared/i18n"

	"github.com/gin-gonic/gin"
)

type fakeHTTPService struct {
	listQuery   systemsettingmodule.ListQuery
	createInput systemsettingmodule.CreateInput
	statusID    int64
	status      int
}

func (f *fakeHTTPService) PageInit(ctx context.Context) (*systemsettingmodule.InitResponse, *apperror.Error) {
	return &systemsettingmodule.InitResponse{Dict: systemsettingmodule.InitDict{}}, nil
}

func (f *fakeHTTPService) List(ctx context.Context, query systemsettingmodule.ListQuery) (*systemsettingmodule.ListResponse, *apperror.Error) {
	f.listQuery = query
	return &systemsettingmodule.ListResponse{List: []systemsettingmodule.ListItem{}, Page: systemsettingmodule.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize}}, nil
}

func (f *fakeHTTPService) Create(ctx context.Context, input systemsettingmodule.CreateInput) (int64, *apperror.Error) {
	f.createInput = input
	return 1, nil
}

func (f *fakeHTTPService) Update(ctx context.Context, id int64, input systemsettingmodule.UpdateInput) *apperror.Error {
	return nil
}

func (f *fakeHTTPService) Delete(ctx context.Context, ids []int64) *apperror.Error {
	return nil
}

func (f *fakeHTTPService) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	f.statusID = id
	f.status = status
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
	if service.listQuery.CurrentPage != 1 || service.listQuery.PageSize != 50 || service.listQuery.Status == nil || *service.listQuery.Status != enum.CommonYes || service.listQuery.Key != "user." {
		t.Fatalf("unexpected list query: %#v", service.listQuery)
	}
}

func TestHandlerRejectsUnsupportedValueTypeBeforeService(t *testing.T) {
	router, service := newSystemSettingHandlerRouter()

	body := `{"key":"feature.switch","value":"true","type":9,"remark":"bad"}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/system-settings", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, recorder.Code, recorder.Body.String())
	}
	if service.createInput.Key != "" {
		t.Fatalf("service should not be called for invalid value type: %#v", service.createInput)
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

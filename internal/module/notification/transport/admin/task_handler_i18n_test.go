package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	notificationtaskmodule "admin_back_go/internal/module/notification/task"
	"admin_back_go/internal/shared/apperror"
	projecti18n "admin_back_go/internal/shared/i18n"

	"github.com/gin-gonic/gin"
)

type fakeTaskHTTPService struct{}

func (f fakeTaskHTTPService) Init(ctx context.Context) (*notificationtaskmodule.InitResponse, *apperror.Error) {
	return &notificationtaskmodule.InitResponse{}, nil
}

func (f fakeTaskHTTPService) StatusCount(ctx context.Context, query notificationtaskmodule.StatusCountQuery) ([]notificationtaskmodule.StatusCountItem, *apperror.Error) {
	return []notificationtaskmodule.StatusCountItem{}, nil
}

func (f fakeTaskHTTPService) List(ctx context.Context, query notificationtaskmodule.ListQuery) (*notificationtaskmodule.ListResponse, *apperror.Error) {
	return &notificationtaskmodule.ListResponse{List: []notificationtaskmodule.ListItem{}, Page: notificationtaskmodule.Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize}}, nil
}

func (f fakeTaskHTTPService) Create(ctx context.Context, input notificationtaskmodule.CreateInput) (*notificationtaskmodule.CreateResponse, *apperror.Error) {
	return &notificationtaskmodule.CreateResponse{ID: 1, Queued: false}, nil
}

func (f fakeTaskHTTPService) Cancel(ctx context.Context, id int64) *apperror.Error {
	return nil
}

func (f fakeTaskHTTPService) Delete(ctx context.Context, id int64) *apperror.Error {
	return nil
}

func TestNotificationTaskHandlerLocalizesListRequestError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(projecti18n.Localize())
	RegisterTaskRoutes(router, fakeTaskHTTPService{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/notification-tasks?current_page=1&page_size=20&status=99", nil)
	request.Header.Set("Accept-Language", "en-US")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusBadRequest, recorder.Code, recorder.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["msg"] != "Invalid list request" {
		t.Fatalf("expected localized list request error, got %#v", payload["msg"])
	}
}

var _ notificationtaskmodule.HTTPService = fakeTaskHTTPService{}

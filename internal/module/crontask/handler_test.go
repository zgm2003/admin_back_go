package crontask

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"

	"github.com/gin-gonic/gin"
)

type fakeCronHTTPService struct {
	listQuery   ListQuery
	createInput SaveInput
	updateID    int64
	updateInput SaveInput
	statusID    int64
	status      int
	deleteIDs   []int64
	logsQuery   LogsQuery
}

func (f *fakeCronHTTPService) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return NewService(&fakeRepository{}, NewDefaultRegistry()).Init(ctx)
}

func (f *fakeCronHTTPService) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	f.listQuery = query
	return &ListResponse{List: []ListItem{}, Page: Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize}}, nil
}

func (f *fakeCronHTTPService) Create(ctx context.Context, input SaveInput) (*ListItem, *apperror.Error) {
	f.createInput = input
	return &ListItem{ID: 1, Name: input.Name, Title: input.Title}, nil
}

func (f *fakeCronHTTPService) Update(ctx context.Context, id int64, input SaveInput) *apperror.Error {
	f.updateID = id
	f.updateInput = input
	return nil
}

func (f *fakeCronHTTPService) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	f.statusID = id
	f.status = status
	return nil
}

func (f *fakeCronHTTPService) Delete(ctx context.Context, ids []int64) *apperror.Error {
	f.deleteIDs = append([]int64(nil), ids...)
	return nil
}

func (f *fakeCronHTTPService) Logs(ctx context.Context, query LogsQuery) (*LogsResponse, *apperror.Error) {
	f.logsQuery = query
	return &LogsResponse{List: []LogItem{}, Page: Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize}}, nil
}

func TestCronTaskHandlerInitAndList(t *testing.T) {
	router, service := newCronTaskHandlerRouter()

	initRecorder := httptest.NewRecorder()
	router.ServeHTTP(initRecorder, httptest.NewRequest(http.MethodGet, "/api/admin/v1/cron-tasks/init", nil))
	if initRecorder.Code != http.StatusOK {
		t.Fatalf("expected init status %d, got %d body=%s", http.StatusOK, initRecorder.Code, initRecorder.Body.String())
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/cron-tasks?current_page=1&page_size=20&status=1&registry_status=registered&title=通知&name=notification", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected list status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if service.listQuery.CurrentPage != 1 || service.listQuery.PageSize != 20 || service.listQuery.Status == nil || *service.listQuery.Status != enum.CommonYes || service.listQuery.RegistryStatus != RegistryStatusRegistered || service.listQuery.Title != "通知" || service.listQuery.Name != "notification" {
		t.Fatalf("unexpected list query: %#v", service.listQuery)
	}
}

func TestCronTaskHandlerCreateUpdateStatusDeleteAndLogs(t *testing.T) {
	router, service := newCronTaskHandlerRouter()
	body := `{"name":"demo_task","title":"Demo","description":"desc","cron":"0 * * * * *","cron_readable":"每分钟","handler":"app\\process\\Demo","status":1}`

	createRecorder := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/api/admin/v1/cron-tasks", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(createRecorder, createReq)
	if createRecorder.Code != http.StatusOK || service.createInput.Name != "demo_task" || service.createInput.Cron != "0 * * * * *" {
		t.Fatalf("unexpected create status/body/input: code=%d body=%s input=%#v", createRecorder.Code, createRecorder.Body.String(), service.createInput)
	}

	updateRecorder := httptest.NewRecorder()
	updateReq := httptest.NewRequest(http.MethodPut, "/api/admin/v1/cron-tasks/9", strings.NewReader(body))
	updateReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(updateRecorder, updateReq)
	if updateRecorder.Code != http.StatusOK || service.updateID != 9 || service.updateInput.Title != "Demo" {
		t.Fatalf("unexpected update: code=%d body=%s id=%d input=%#v", updateRecorder.Code, updateRecorder.Body.String(), service.updateID, service.updateInput)
	}

	statusRecorder := httptest.NewRecorder()
	statusReq := httptest.NewRequest(http.MethodPatch, "/api/admin/v1/cron-tasks/9/status", strings.NewReader(`{"status":2}`))
	statusReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(statusRecorder, statusReq)
	if statusRecorder.Code != http.StatusOK || service.statusID != 9 || service.status != enum.CommonNo {
		t.Fatalf("unexpected status patch: code=%d body=%s id=%d status=%d", statusRecorder.Code, statusRecorder.Body.String(), service.statusID, service.status)
	}

	deleteRecorder := httptest.NewRecorder()
	router.ServeHTTP(deleteRecorder, httptest.NewRequest(http.MethodDelete, "/api/admin/v1/cron-tasks/9", nil))
	if deleteRecorder.Code != http.StatusOK || !reflect.DeepEqual(service.deleteIDs, []int64{9}) {
		t.Fatalf("unexpected single delete: code=%d body=%s ids=%#v", deleteRecorder.Code, deleteRecorder.Body.String(), service.deleteIDs)
	}

	batchRecorder := httptest.NewRecorder()
	batchReq := httptest.NewRequest(http.MethodDelete, "/api/admin/v1/cron-tasks", strings.NewReader(`{"ids":[9,10]}`))
	batchReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(batchRecorder, batchReq)
	if batchRecorder.Code != http.StatusOK || !reflect.DeepEqual(service.deleteIDs, []int64{9, 10}) {
		t.Fatalf("unexpected batch delete: code=%d body=%s ids=%#v", batchRecorder.Code, batchRecorder.Body.String(), service.deleteIDs)
	}

	logsRecorder := httptest.NewRecorder()
	router.ServeHTTP(logsRecorder, httptest.NewRequest(http.MethodGet, "/api/admin/v1/cron-tasks/9/logs?current_page=1&page_size=20&status=1", nil))
	if logsRecorder.Code != http.StatusOK || service.logsQuery.TaskID != 9 || service.logsQuery.Status == nil || *service.logsQuery.Status != LogStatusSuccess {
		t.Fatalf("unexpected logs query: code=%d body=%s query=%#v", logsRecorder.Code, logsRecorder.Body.String(), service.logsQuery)
	}
}

func newCronTaskHandlerRouter() (*gin.Engine, *fakeCronHTTPService) {
	gin.SetMode(gin.TestMode)
	service := &fakeCronHTTPService{}
	router := gin.New()
	RegisterRoutes(router, service)
	return router, service
}

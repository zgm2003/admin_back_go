package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	promptmodule "admin_back_go/internal/module/ai/prompt"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/dict"

	"github.com/gin-gonic/gin"
)

type fakePromptService struct {
	pageInitCalls   int
	listQuery       promptmodule.ListQuery
	detailID        int64
	created         promptmodule.Input
	updatedID       int64
	updated         promptmodule.Input
	statusID        int64
	status          int
	deletedID       int64
	batchDeletedIDs []int64
}

func (f *fakePromptService) PageInit(ctx context.Context) (*promptmodule.PageInitResponse, *apperror.Error) {
	f.pageInitCalls++
	return &promptmodule.PageInitResponse{CommonStatusArr: []dict.Option[int]{{Label: "启用", Value: promptmodule.StatusEnabled}}}, nil
}

func (f *fakePromptService) List(ctx context.Context, query promptmodule.ListQuery) (*promptmodule.ListResponse, *apperror.Error) {
	f.listQuery = query
	return &promptmodule.ListResponse{List: []promptmodule.Item{{ID: 1, Slug: "cat", Title: "Cat", Prompt: "draw a cat"}}}, nil
}

func (f *fakePromptService) Detail(ctx context.Context, id int64) (*promptmodule.Item, *apperror.Error) {
	f.detailID = id
	return &promptmodule.Item{ID: id, Slug: "cat", Title: "Cat", Prompt: "draw a cat"}, nil
}

func (f *fakePromptService) Create(ctx context.Context, input promptmodule.Input) (int64, *apperror.Error) {
	f.created = input
	return 11, nil
}

func (f *fakePromptService) Update(ctx context.Context, id int64, input promptmodule.Input) *apperror.Error {
	f.updatedID = id
	f.updated = input
	return nil
}

func (f *fakePromptService) ChangeStatus(ctx context.Context, id int64, status int) *apperror.Error {
	f.statusID = id
	f.status = status
	return nil
}

func (f *fakePromptService) DeleteOne(ctx context.Context, id int64) *apperror.Error {
	f.deletedID = id
	return nil
}

func (f *fakePromptService) DeleteBatch(ctx context.Context, ids []int64) *apperror.Error {
	f.batchDeletedIDs = append([]int64(nil), ids...)
	return nil
}

func TestAdminPromptRoutesCallService(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakePromptService{}
	router := gin.New()
	RegisterRoutes(router, service)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/v1/ai-prompts/page-init", nil))
	if recorder.Code != http.StatusOK || service.pageInitCalls != 1 {
		t.Fatalf("page-init code=%d body=%s calls=%d", recorder.Code, recorder.Body.String(), service.pageInitCalls)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/v1/ai-prompts?keyword=cat&category=style&status=2&current_page=2&page_size=5", nil))
	if recorder.Code != http.StatusOK || service.listQuery.Keyword != "cat" || service.listQuery.Category != "style" || service.listQuery.Status != promptmodule.StatusDisabled || service.listQuery.CurrentPage != 2 || service.listQuery.PageSize != 5 {
		t.Fatalf("list code=%d body=%s query=%#v", recorder.Code, recorder.Body.String(), service.listQuery)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/v1/ai-prompts/9", nil))
	if recorder.Code != http.StatusOK || service.detailID != 9 {
		t.Fatalf("detail code=%d body=%s id=%d", recorder.Code, recorder.Body.String(), service.detailID)
	}

	body := `{"slug":"cat","category":"style","title":"Cat","cover_url":"https://example.test/cat.png","prompt":"draw a cat","preview":"preview","tags_json":"[\"poster\"]","source_url":"https://example.test/src","status":2}`
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/admin/v1/ai-prompts", strings.NewReader(body)))
	if recorder.Code != http.StatusOK || service.created.Slug != "cat" || service.created.Status != promptmodule.StatusDisabled || service.created.TagsJSON != `["poster"]` {
		t.Fatalf("create code=%d body=%s input=%#v", recorder.Code, recorder.Body.String(), service.created)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/admin/v1/ai-prompts/7", strings.NewReader(body)))
	if recorder.Code != http.StatusOK || service.updatedID != 7 || service.updated.Prompt != "draw a cat" {
		t.Fatalf("update code=%d body=%s id=%d input=%#v", recorder.Code, recorder.Body.String(), service.updatedID, service.updated)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPatch, "/api/admin/v1/ai-prompts/7/status", strings.NewReader(`{"status":2}`)))
	if recorder.Code != http.StatusOK || service.statusID != 7 || service.status != promptmodule.StatusDisabled {
		t.Fatalf("status code=%d body=%s id=%d status=%d", recorder.Code, recorder.Body.String(), service.statusID, service.status)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/admin/v1/ai-prompts/7", nil))
	if recorder.Code != http.StatusOK || service.deletedID != 7 {
		t.Fatalf("delete one code=%d body=%s id=%d", recorder.Code, recorder.Body.String(), service.deletedID)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/admin/v1/ai-prompts", strings.NewReader(`{"ids":[3,4]}`)))
	if recorder.Code != http.StatusOK || !reflect.DeepEqual(service.batchDeletedIDs, []int64{3, 4}) {
		t.Fatalf("delete batch code=%d body=%s ids=%#v", recorder.Code, recorder.Body.String(), service.batchDeletedIDs)
	}
}

func TestAdminPromptRejectsInvalidRouteIDAndStatus(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakePromptService{}
	router := gin.New()
	RegisterRoutes(router, service)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPatch, "/api/admin/v1/ai-prompts/0/status", strings.NewReader(`{"status":1}`)))
	if recorder.Code != http.StatusBadRequest || service.statusID != 0 {
		t.Fatalf("expected invalid id before service, code=%d body=%s statusID=%d", recorder.Code, recorder.Body.String(), service.statusID)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPatch, "/api/admin/v1/ai-prompts/7/status", strings.NewReader(`{"status":999}`)))
	if recorder.Code != http.StatusBadRequest || service.statusID != 0 {
		t.Fatalf("expected invalid status before service, code=%d body=%s statusID=%d", recorder.Code, recorder.Body.String(), service.statusID)
	}
}

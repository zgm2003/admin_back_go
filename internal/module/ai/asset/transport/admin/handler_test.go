package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	assetmodule "admin_back_go/internal/module/ai/asset"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/dict"

	"github.com/gin-gonic/gin"
)

type fakeAssetService struct {
	pageInitCalls   int
	listQuery       assetmodule.ListQuery
	detailID        int64
	created         assetmodule.Input
	updatedID       int64
	updated         assetmodule.Input
	deletedID       int64
	batchDeletedIDs []int64
}

func (f *fakeAssetService) PageInit(ctx context.Context) (*assetmodule.PageInitResponse, *apperror.Error) {
	f.pageInitCalls++
	return &assetmodule.PageInitResponse{CommonStatusArr: []dict.Option[int]{{Label: "启用", Value: assetmodule.StatusEnabled}}, AIAssetTypeArr: []dict.Option[string]{{Label: "图片", Value: assetmodule.AssetTypeImage}}}, nil
}

func (f *fakeAssetService) List(ctx context.Context, query assetmodule.ListQuery) (*assetmodule.ListResponse, *apperror.Error) {
	f.listQuery = query
	return &assetmodule.ListResponse{List: []assetmodule.Item{{ID: 1, Slug: "hero", Type: assetmodule.AssetTypeImage, Title: "Hero"}}}, nil
}

func (f *fakeAssetService) Detail(ctx context.Context, id int64) (*assetmodule.Item, *apperror.Error) {
	f.detailID = id
	return &assetmodule.Item{ID: id, Slug: "hero", Type: assetmodule.AssetTypeImage, Title: "Hero"}, nil
}

func (f *fakeAssetService) Create(ctx context.Context, input assetmodule.Input) (int64, *apperror.Error) {
	f.created = input
	return 12, nil
}

func (f *fakeAssetService) Update(ctx context.Context, id int64, input assetmodule.Input) *apperror.Error {
	f.updatedID = id
	f.updated = input
	return nil
}

func (f *fakeAssetService) DeleteOne(ctx context.Context, id int64) *apperror.Error {
	f.deletedID = id
	return nil
}

func (f *fakeAssetService) DeleteBatch(ctx context.Context, ids []int64) *apperror.Error {
	f.batchDeletedIDs = append([]int64(nil), ids...)
	return nil
}

func TestAdminAssetRoutesCallService(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeAssetService{}
	router := gin.New()
	RegisterRoutes(router, service)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/v1/ai-assets/page-init", nil))
	if recorder.Code != http.StatusOK || service.pageInitCalls != 1 {
		t.Fatalf("page-init code=%d body=%s calls=%d", recorder.Code, recorder.Body.String(), service.pageInitCalls)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/v1/ai-assets?keyword=sky&type=image&status=2&current_page=2&page_size=5", nil))
	if recorder.Code != http.StatusOK || service.listQuery.Keyword != "sky" || service.listQuery.Type != assetmodule.AssetTypeImage || service.listQuery.Status != assetmodule.StatusDisabled || service.listQuery.CurrentPage != 2 || service.listQuery.PageSize != 5 {
		t.Fatalf("list code=%d body=%s query=%#v", recorder.Code, recorder.Body.String(), service.listQuery)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/v1/ai-assets/9", nil))
	if recorder.Code != http.StatusOK || service.detailID != 9 {
		t.Fatalf("detail code=%d body=%s id=%d", recorder.Code, recorder.Body.String(), service.detailID)
	}

	body := `{"slug":"hero","type":"image","category":"banner","title":"Hero","cover_url":"https://example.test/hero.png","description":"desc","content":"{}","url":"https://example.test/hero.png","tags_json":"[\"poster\"]","status":2}`
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/admin/v1/ai-assets", strings.NewReader(body)))
	if recorder.Code != http.StatusOK || service.created.Slug != "hero" || service.created.Type != assetmodule.AssetTypeImage || service.created.Status != assetmodule.StatusDisabled || service.created.TagsJSON != `["poster"]` {
		t.Fatalf("create code=%d body=%s input=%#v", recorder.Code, recorder.Body.String(), service.created)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/admin/v1/ai-assets/7", strings.NewReader(body)))
	if recorder.Code != http.StatusOK || service.updatedID != 7 || service.updated.URL != "https://example.test/hero.png" {
		t.Fatalf("update code=%d body=%s id=%d input=%#v", recorder.Code, recorder.Body.String(), service.updatedID, service.updated)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/admin/v1/ai-assets/7", nil))
	if recorder.Code != http.StatusOK || service.deletedID != 7 {
		t.Fatalf("delete one code=%d body=%s id=%d", recorder.Code, recorder.Body.String(), service.deletedID)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/admin/v1/ai-assets", strings.NewReader(`{"ids":[3,4]}`)))
	if recorder.Code != http.StatusOK || !reflect.DeepEqual(service.batchDeletedIDs, []int64{3, 4}) {
		t.Fatalf("delete batch code=%d body=%s ids=%#v", recorder.Code, recorder.Body.String(), service.batchDeletedIDs)
	}
}

func TestAdminAssetRejectsInvalidRouteIDAndStatus(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeAssetService{}
	router := gin.New()
	RegisterRoutes(router, service)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/admin/v1/ai-assets/0", strings.NewReader(`{"slug":"hero","type":"image","title":"Hero"}`)))
	if recorder.Code != http.StatusBadRequest || service.updatedID != 0 {
		t.Fatalf("expected invalid id before service, code=%d body=%s updatedID=%d", recorder.Code, recorder.Body.String(), service.updatedID)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/admin/v1/ai-assets", strings.NewReader(`{"slug":"hero","type":"image","title":"Hero","status":999}`)))
	if recorder.Code != http.StatusBadRequest || service.created.Slug != "" {
		t.Fatalf("expected invalid status before service, code=%d body=%s input=%#v", recorder.Code, recorder.Body.String(), service.created)
	}
}

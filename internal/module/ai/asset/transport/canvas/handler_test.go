package canvas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	assetmodule "admin_back_go/internal/module/ai/asset"
	"admin_back_go/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

type fakeAssetService struct {
	query       assetmodule.ListQuery
	created     assetmodule.Input
	updatedID   int64
	updated     assetmodule.Input
	deletedID   int64
	createErr   *apperror.Error
	createCalls int
	updateCalls int
	deleteCalls int
}

func (f *fakeAssetService) PublicList(ctx context.Context, query assetmodule.ListQuery) (*assetmodule.ListResponse, *apperror.Error) {
	f.query = query
	return &assetmodule.ListResponse{List: []assetmodule.Item{{ID: 1, Slug: "hero", Type: assetmodule.AssetTypeImage, Title: "Hero"}}}, nil
}

func (f *fakeAssetService) Create(ctx context.Context, input assetmodule.Input) (int64, *apperror.Error) {
	f.created = input
	f.createCalls++
	if f.createErr != nil {
		return 0, f.createErr
	}
	return 12, nil
}

func (f *fakeAssetService) Update(ctx context.Context, id int64, input assetmodule.Input) *apperror.Error {
	f.updatedID = id
	f.updated = input
	f.updateCalls++
	return nil
}

func (f *fakeAssetService) Delete(ctx context.Context, id int64) *apperror.Error {
	f.deletedID = id
	f.deleteCalls++
	return nil
}

func TestCanvasAssetRoutePassesListQueryToAIAssetService(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeAssetService{}
	router := gin.New()
	RegisterRoutes(router, service)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/canvas/v1/assets?keyword=sky&type=image&current_page=2&page_size=5", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.query.Keyword != "sky" || service.query.Type != assetmodule.AssetTypeImage || service.query.CurrentPage != 2 || service.query.PageSize != 5 {
		t.Fatalf("query mismatch: %#v", service.query)
	}
}

func TestCanvasAssetRouteSupportsCreateUpdateAndDelete(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeAssetService{}
	router := gin.New()
	RegisterRoutes(router, service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/assets", strings.NewReader(`{"slug":"clip","type":"video","title":"Clip","url":"https://example.test/clip.mp4","status":2}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.createCalls != 1 || service.created.Slug != "clip" || service.created.Type != assetmodule.AssetTypeVideo || service.created.Title != "Clip" || service.created.Status != assetmodule.StatusDisabled {
		t.Fatalf("create mismatch code=%d body=%s calls=%d input=%#v", recorder.Code, recorder.Body.String(), service.createCalls, service.created)
	}
	if !strings.Contains(recorder.Body.String(), `"id":12`) {
		t.Fatalf("create response must return id, body=%s", recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPut, "/api/canvas/v1/assets/7", strings.NewReader(`{"slug":"hero","type":"image","title":"Hero"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.updateCalls != 1 || service.updatedID != 7 || service.updated.Type != assetmodule.AssetTypeImage {
		t.Fatalf("update mismatch code=%d body=%s calls=%d id=%d input=%#v", recorder.Code, recorder.Body.String(), service.updateCalls, service.updatedID, service.updated)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/canvas/v1/assets/7", nil))
	if recorder.Code != http.StatusOK || service.deleteCalls != 1 || service.deletedID != 7 {
		t.Fatalf("delete mismatch code=%d body=%s calls=%d id=%d", recorder.Code, recorder.Body.String(), service.deleteCalls, service.deletedID)
	}
}

func TestCanvasAssetRouteSurfacesInvalidStatusAsBadRequest(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeAssetService{
		createErr: apperror.BadRequestKey("ai.asset.request.invalid", nil, "素材参数错误"),
	}
	router := gin.New()
	RegisterRoutes(router, service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/assets", strings.NewReader(`{"slug":"hero","type":"image","title":"Hero","status":999}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest || service.createCalls != 1 || service.created.Status != 999 {
		t.Fatalf("expected bad request passthrough for invalid status, code=%d body=%s calls=%d input=%#v", recorder.Code, recorder.Body.String(), service.createCalls, service.created)
	}
}

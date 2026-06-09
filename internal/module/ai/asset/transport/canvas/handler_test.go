package canvas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin_back_go/internal/middleware"
	assetmodule "admin_back_go/internal/module/ai/asset"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"

	"github.com/gin-gonic/gin"
)

type fakeAssetService struct {
	userID      uint64
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

func (f *fakeAssetService) UserList(ctx context.Context, userID uint64, query assetmodule.ListQuery) (*assetmodule.ListResponse, *apperror.Error) {
	f.userID = userID
	f.query = query
	return &assetmodule.ListResponse{List: []assetmodule.Item{{ID: 1, UserID: userID, Slug: "hero", Type: assetmodule.AssetTypeImage, Title: "Hero"}}}, nil
}

func (f *fakeAssetService) UserCreate(ctx context.Context, userID uint64, input assetmodule.Input) (int64, *apperror.Error) {
	f.userID = userID
	f.created = input
	f.createCalls++
	if f.createErr != nil {
		return 0, f.createErr
	}
	return 12, nil
}

func (f *fakeAssetService) UserUpdate(ctx context.Context, userID uint64, id int64, input assetmodule.Input) *apperror.Error {
	f.userID = userID
	f.updatedID = id
	f.updated = input
	f.updateCalls++
	return nil
}

func (f *fakeAssetService) UserDelete(ctx context.Context, userID uint64, id int64) *apperror.Error {
	f.userID = userID
	f.deletedID = id
	f.deleteCalls++
	return nil
}

func TestCanvasAssetRoutePassesTokenUserAndListQueryToAIAssetService(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeAssetService{}
	router := newCanvasAssetTestRouter(service, &middleware.AuthIdentity{UserID: 9, Platform: enum.PlatformCanvas})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/canvas/v1/assets?keyword=sky&type=image&current_page=2&page_size=5", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.userID != 9 || service.query.Keyword != "sky" || service.query.Type != assetmodule.AssetTypeImage || service.query.CurrentPage != 2 || service.query.PageSize != 5 {
		t.Fatalf("query mismatch user=%d query=%#v", service.userID, service.query)
	}
}

func TestCanvasAssetRouteSupportsUserOwnedCreateUpdateAndDelete(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeAssetService{}
	router := newCanvasAssetTestRouter(service, &middleware.AuthIdentity{UserID: 9, Platform: enum.PlatformCanvas})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/assets", strings.NewReader(`{"slug":"clip","type":"video","title":"Clip","url":"https://storage.example.test/clip.mp4","content":"{\"storageKey\":\"video:task/clip.mp4\",\"width\":1280,\"height\":720,\"bytes\":456789,\"mimeType\":\"video/mp4\",\"duration\":12.5}","status":2}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.userID != 9 || service.createCalls != 1 || service.created.Slug != "clip" || service.created.Type != assetmodule.AssetTypeVideo || service.created.Title != "Clip" || service.created.Status != assetmodule.StatusDisabled {
		t.Fatalf("create mismatch code=%d body=%s user=%d calls=%d input=%#v", recorder.Code, recorder.Body.String(), service.userID, service.createCalls, service.created)
	}
	if !strings.Contains(recorder.Body.String(), `"id":12`) {
		t.Fatalf("create response must return id, body=%s", recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPut, "/api/canvas/v1/assets/7", strings.NewReader(`{"slug":"hero","type":"image","title":"Hero","url":"https://storage.example.test/hero.png","content":"{\"storageKey\":\"image:task/hero.png\",\"width\":1024,\"height\":768,\"bytes\":123456,\"mimeType\":\"image/png\"}"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.userID != 9 || service.updateCalls != 1 || service.updatedID != 7 || service.updated.Type != assetmodule.AssetTypeImage {
		t.Fatalf("update mismatch code=%d body=%s user=%d calls=%d id=%d input=%#v", recorder.Code, recorder.Body.String(), service.userID, service.updateCalls, service.updatedID, service.updated)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/canvas/v1/assets/7", nil))
	if recorder.Code != http.StatusOK || service.userID != 9 || service.deleteCalls != 1 || service.deletedID != 7 {
		t.Fatalf("delete mismatch code=%d body=%s user=%d calls=%d id=%d", recorder.Code, recorder.Body.String(), service.userID, service.deleteCalls, service.deletedID)
	}
}

func TestCanvasAssetRouteRejectsNonCanvasIdentity(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeAssetService{}
	router := newCanvasAssetTestRouter(service, &middleware.AuthIdentity{UserID: 9, Platform: enum.PlatformAdmin})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/canvas/v1/assets", nil))

	if recorder.Code != http.StatusUnauthorized || service.userID != 0 {
		t.Fatalf("expected unauthorized and no service call, code=%d body=%s service=%#v", recorder.Code, recorder.Body.String(), service)
	}
}

func TestCanvasAssetRouteSurfacesInvalidStatusAsBadRequest(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeAssetService{
		createErr: apperror.BadRequestKey("ai.asset.request.invalid", nil, "素材参数错误"),
	}
	router := newCanvasAssetTestRouter(service, &middleware.AuthIdentity{UserID: 9, Platform: enum.PlatformCanvas})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/assets", strings.NewReader(`{"slug":"hero","type":"image","title":"Hero","status":999}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest || service.userID != 9 || service.createCalls != 1 || service.created.Status != 999 {
		t.Fatalf("expected bad request passthrough for invalid status, code=%d body=%s user=%d calls=%d input=%#v", recorder.Code, recorder.Body.String(), service.userID, service.createCalls, service.created)
	}
}

func newCanvasAssetTestRouter(service *fakeAssetService, identity *middleware.AuthIdentity) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if identity != nil {
			c.Set(middleware.ContextAuthIdentity, identity)
		}
		c.Next()
	})
	RegisterRoutes(router, service)
	return router
}

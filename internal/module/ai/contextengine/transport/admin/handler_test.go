package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"admin_back_go/internal/middleware"
	contextengine "admin_back_go/internal/module/ai/contextengine"
	"admin_back_go/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

func TestContextRoutesUseTrustedPlatformAndIgnoreOverrides(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeContextHTTPService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 7, Platform: "admin"})
		c.Next()
	})
	RegisterRoutes(router, "admin", service)
	body, _ := json.Marshal(map[string]any{"profile_id": 3, "name": "docs", "description": "", "status": "enabled", "platform": "canvas"})
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/ai/context-spaces?platform=canvas", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Platform", "canvas")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.platform != "admin" {
		t.Fatalf("status=%d platform=%q body=%s", recorder.Code, service.platform, recorder.Body.String())
	}
}

type fakeContextHTTPService struct{ platform string }

func (service *fakeContextHTTPService) CreateProfile(context.Context, uint32, contextengine.CreateProfileInput) (*contextengine.ProfileDTO, *apperror.Error) {
	return &contextengine.ProfileDTO{ID: 1}, nil
}
func (service *fakeContextHTTPService) UpdateProfile(context.Context, uint64, contextengine.UpdateProfileInput) (*contextengine.ProfileDTO, *apperror.Error) {
	return &contextengine.ProfileDTO{ID: 1}, nil
}
func (service *fakeContextHTTPService) CreateSpace(_ context.Context, platform string, _ uint32, _ contextengine.CreateSpaceInput) (*contextengine.SpaceDTO, *apperror.Error) {
	service.platform = platform
	return &contextengine.SpaceDTO{ID: 2}, nil
}
func (service *fakeContextHTTPService) UpdateSpace(_ context.Context, platform string, _ uint64, _ contextengine.UpdateSpaceInput) (*contextengine.SpaceDTO, *apperror.Error) {
	service.platform = platform
	return &contextengine.SpaceDTO{ID: 2}, nil
}
func (service *fakeContextHTTPService) DeleteSpace(_ context.Context, platform string, _ uint64) *apperror.Error {
	service.platform = platform
	return nil
}
func (service *fakeContextHTTPService) CreateDocument(_ context.Context, platform string, _ uint32, _ contextengine.CreateDocumentInput) (*contextengine.DocumentAdminDTO, *apperror.Error) {
	service.platform = platform
	return &contextengine.DocumentAdminDTO{ID: 3}, nil
}
func (service *fakeContextHTTPService) ReindexDocument(_ context.Context, platform string, _ uint64) (*contextengine.DocumentAdminDTO, *apperror.Error) {
	service.platform = platform
	return &contextengine.DocumentAdminDTO{ID: 3}, nil
}

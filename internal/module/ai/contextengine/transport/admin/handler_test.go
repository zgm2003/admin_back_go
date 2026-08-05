package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"admin_back_go/internal/middleware"
	contextengine "admin_back_go/internal/module/ai/contextengine"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

func TestContextAdminContractMutationRequestsDoNotRepeatPathOrStatusFacts(t *testing.T) {
	profile := reflect.TypeOf(profileUpdateRequest{})
	if profile.NumField() != 1 || profile.Field(0).Name != "Name" {
		t.Fatalf("profile update fields=%v", profile)
	}
	document := reflect.TypeOf(spaceDocumentRequest{})
	for _, forbidden := range []string{"SpaceID", "ConversationID", "SourceMessageID", "SourceAttachmentIndex"} {
		if _, exists := document.FieldByName(forbidden); exists {
			t.Fatalf("space document request repeats %s", forbidden)
		}
	}
}

func TestContextAdminContractSensitiveAuditsDoNotCapturePayloads(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := adminroute.NewRegistry()
	RegisterRoutes(gin.New(), "admin", &fakeContextHTTPService{}, registry)
	for _, definition := range registry.Definitions() {
		if definition.Path == "/api/admin/v1/ai/context-evaluations" {
			if !definition.Audit.SkipRequestPayload || !definition.Audit.SkipResponsePayload {
				t.Fatalf("evaluation audit captures query or context response: %+v", definition.Audit)
			}
			return
		}
	}
	t.Fatal("evaluation route definition missing")
}

func TestContextPageInitRouteIsReadOnlyAndTyped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := adminroute.NewRegistry()
	RegisterRoutes(gin.New(), "admin", &fakeContextHTTPService{}, registry)

	definitions := registry.Definitions()
	for _, definition := range definitions {
		if definition.Path != "/api/admin/v1/ai/context/page-init" {
			continue
		}
		if definition.Method != http.MethodGet || definition.OperationID != "ai_context_page_init" {
			t.Fatalf("page-init route = %#v", definition)
		}
		if definition.Access.Kind != adminroute.AccessPermission || definition.Access.PermissionCode != "ai_context_view" {
			t.Fatalf("page-init access = %#v", definition.Access)
		}
		if definition.Audit.Enabled || definition.Audit.Reason != "read-only" {
			t.Fatalf("page-init audit = %#v", definition.Audit)
		}
		if definition.Contract == nil || reflect.TypeOf(definition.Contract.Response) != reflect.TypeOf(contextengine.ContextPageInitResponse{}) {
			t.Fatalf("page-init contract = %#v", definition.Contract)
		}
		return
	}
	t.Fatal("context page-init route definition missing")
}

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

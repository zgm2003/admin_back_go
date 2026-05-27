package uploadtoken

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/middleware"

	"github.com/gin-gonic/gin"
)

type fakeAppUploadTokenService struct {
	input CreateInput
}

func (f *fakeAppUploadTokenService) Create(ctx context.Context, input CreateInput) (*CreateResponse, *apperror.Error) {
	f.input = input
	return &CreateResponse{Provider: ProviderCOS, Bucket: "bucket-a", Region: "ap-nanjing", Key: "avatars/avatar.png"}, nil
}

func TestUploadTokenModuleRegistersAppRoute(t *testing.T) {
	service := &fakeAppUploadTokenService{}
	router := newAppUploadTokenTestRouter(service, &middleware.AuthIdentity{UserID: 7, SessionID: 20, Platform: enum.PlatformApp})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/app/v1/upload-tokens", strings.NewReader(`{"folder":"avatars","file_name":"avatar.png","file_size":1024,"file_kind":"image"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.input.Folder != "avatars" || service.input.FileName != "avatar.png" || service.input.FileSize != 1024 || service.input.FileKind != "image" {
		t.Fatalf("unexpected create input: %#v", service.input)
	}
	body := decodeAppUploadTokenBody(t, recorder)
	data := body["data"].(map[string]any)
	if data["provider"] != ProviderCOS || data["bucket"] != "bucket-a" {
		t.Fatalf("unexpected response data: %#v", data)
	}
}

func TestUploadTokenAppRouteRequiresIdentity(t *testing.T) {
	router := newAppUploadTokenTestRouter(&fakeAppUploadTokenService{}, nil)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/app/v1/upload-tokens", strings.NewReader(`{"folder":"avatars","file_name":"avatar.png","file_size":1024,"file_kind":"image"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func newAppUploadTokenTestRouter(service HTTPService, identity *middleware.AuthIdentity) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	if identity != nil {
		router.Use(func(c *gin.Context) {
			c.Set(middleware.ContextAuthIdentity, identity)
			c.Next()
		})
	}
	RegisterRoutes(router, service)
	return router
}

func decodeAppUploadTokenBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	return body
}

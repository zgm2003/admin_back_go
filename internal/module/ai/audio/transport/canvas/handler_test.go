package canvas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin_back_go/internal/middleware"
	aiaudiomodule "admin_back_go/internal/module/ai/audio"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"

	"github.com/gin-gonic/gin"
)

type fakeCanvasAudioService struct {
	input       aiaudiomodule.GenerateInput
	body        []byte
	contentType string
}

func (f *fakeCanvasAudioService) Generate(ctx context.Context, input aiaudiomodule.GenerateInput) (*aiaudiomodule.GenerateResponse, *apperror.Error) {
	f.input = input
	if f.body == nil {
		f.body = []byte("audio")
	}
	if f.contentType == "" {
		f.contentType = "audio/wav"
	}
	return &aiaudiomodule.GenerateResponse{Body: f.body, ContentType: f.contentType}, nil
}

func TestCanvasAudioRouteUsesCanvasIdentityAndService(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeCanvasAudioService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 9, Platform: enum.PlatformCanvas})
	})
	RegisterRoutes(router, service)

	speed := "1.25"
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/ai/audios", strings.NewReader(`{"agent_id":10,"prompt":"hello","voice":"nova","response_format":"wav","speed":`+speed+`,"instructions":"warm"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || recorder.Body.String() != "audio" || recorder.Header().Get("Content-Type") != "audio/wav" {
		t.Fatalf("expected audio blob response, got code=%d type=%s body=%s", recorder.Code, recorder.Header().Get("Content-Type"), recorder.Body.String())
	}
	if service.input.UserID != 9 || service.input.AgentID != 10 || service.input.Prompt != "hello" || service.input.ModelID != "" || service.input.Voice != "nova" || service.input.ResponseFormat != "wav" || service.input.Speed == nil || *service.input.Speed != 1.25 || service.input.Instructions != "warm" {
		t.Fatalf("unexpected service input: %#v", service.input)
	}
}

func TestCanvasAudioRouteRejectsClientModelConfigOverride(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	tests := []string{"model", "provider", "api_key", "base_url"}
	for _, field := range tests {
		t.Run(field, func(t *testing.T) {
			service := &fakeCanvasAudioService{}
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 9, Platform: enum.PlatformCanvas})
			})
			RegisterRoutes(router, service)

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/ai/audios", strings.NewReader(`{"agent_id":10,"prompt":"hello","`+field+`":"client-owned"}`))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d body=%s", recorder.Code, recorder.Body.String())
			}
			if service.input.UserID != 0 || service.input.AgentID != 0 || service.input.Prompt != "" || service.input.ModelID != "" {
				t.Fatalf("service must not be called when client overrides provider config: %#v", service.input)
			}
		})
	}
}

func TestCanvasAudioRouteRejectsWrongPlatformIdentity(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeCanvasAudioService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 9, Platform: enum.PlatformAdmin})
	})
	RegisterRoutes(router, service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/ai/audios", strings.NewReader(`{"agent_id":10,"prompt":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.input.UserID != 0 {
		t.Fatalf("service must not be called for wrong platform: %#v", service.input)
	}
}

func TestCanvasAudioRouteNilServiceReturnsServiceMissing(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 9, Platform: enum.PlatformCanvas})
	})
	RegisterRoutes(router, nil)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/ai/audios", strings.NewReader(`{"agent_id":10,"prompt":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), "Canvas音频生成服务未配置") {
		t.Fatalf("expected service missing response, got code=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

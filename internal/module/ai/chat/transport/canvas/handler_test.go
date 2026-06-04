package canvas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin_back_go/internal/middleware"
	aichatmodule "admin_back_go/internal/module/ai/chat"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"

	"github.com/gin-gonic/gin"
)

type fakeCanvasChatService struct {
	input       aichatmodule.CanvasCompletionInput
	returnNil   bool
	returnEmpty bool
}

func (f *fakeCanvasChatService) CanvasCompletion(ctx context.Context, input aichatmodule.CanvasCompletionInput) (*aichatmodule.CanvasCompletionResponse, *apperror.Error) {
	f.input = input
	if f.returnNil {
		return nil, nil
	}
	if f.returnEmpty {
		return &aichatmodule.CanvasCompletionResponse{}, nil
	}
	return &aichatmodule.CanvasCompletionResponse{ID: "chat-1", Object: "chat.completion", Content: "ok"}, nil
}

func TestCanvasChatCompletionUsesCanvasIdentityAndService(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeCanvasChatService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 9, Platform: enum.PlatformCanvas})
	})
	RegisterRoutes(router, service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/ai/chat/completions", strings.NewReader(`{"agent_id":7,"message":"hello","model":"client-model"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.input.UserID != 9 || service.input.AgentID != 7 || service.input.Message != "hello" || service.input.ModelID != "client-model" {
		t.Fatalf("unexpected service input: %#v", service.input)
	}
	for _, want := range []string{`"id":"chat-1"`, `"object":"chat.completion"`, `"content":"ok"`} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Fatalf("response missing %s: %s", want, recorder.Body.String())
		}
	}
}

func TestCanvasChatCompletionRejectsWrongPlatformIdentity(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeCanvasChatService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 9, Platform: enum.PlatformAdmin})
	})
	RegisterRoutes(router, service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/ai/chat/completions", strings.NewReader(`{"agent_id":7,"message":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.input.UserID != 0 {
		t.Fatalf("service must not be called for wrong platform: %#v", service.input)
	}
}

func TestCanvasChatCompletionRejectsInvalidServiceResult(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	for _, tt := range []struct {
		name    string
		service *fakeCanvasChatService
	}{
		{name: "nil result", service: &fakeCanvasChatService{returnNil: true}},
		{name: "empty result", service: &fakeCanvasChatService{returnEmpty: true}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 9, Platform: enum.PlatformCanvas})
			})
			RegisterRoutes(router, tt.service)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/ai/chat/completions", strings.NewReader(`{"agent_id":7,"message":"hello"}`))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)
			if recorder.Code == http.StatusOK {
				t.Fatalf("expected non-200 status, got %d body=%s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), "Canvas文本生成结果无效") {
				t.Fatalf("response missing invalid result message: %s", recorder.Body.String())
			}
		})
	}
}

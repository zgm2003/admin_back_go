package canvas

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin_back_go/internal/middleware"
	canvasmodule "admin_back_go/internal/module/canvas"
	"admin_back_go/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

type fakeCanvasService struct {
	assetQuery     canvasmodule.AssetListQuery
	settingsUserID int64
}

func (f *fakeCanvasService) PublicAssets(ctx context.Context, query canvasmodule.AssetListQuery) (*canvasmodule.AssetListResponse, *apperror.Error) {
	f.assetQuery = query
	return &canvasmodule.AssetListResponse{List: []canvasmodule.AssetItem{{ID: 2, Slug: "a", Type: canvasmodule.AssetTypeImage, Title: "Asset"}}}, nil
}
func (f *fakeCanvasService) PublicSettings(ctx context.Context, input canvasmodule.SettingsInput) (*canvasmodule.SettingsResponse, *apperror.Error) {
	f.settingsUserID = input.UserID
	return &canvasmodule.SettingsResponse{
		AllowRegister: true,
		Scenes:        []string{"canvas_text_generate", "canvas_image_generate", "canvas_video_generate"},
		Agents: canvasmodule.CanvasAgentGroups{
			Text:  []canvasmodule.CanvasAgentOption{{ID: 7, Name: "文本助手", ModelID: "gpt-4.1-mini", ModelDisplayName: "GPT 4.1 Mini", Scene: "canvas_text_generate"}},
			Image: []canvasmodule.CanvasAgentOption{{ID: 8, Name: "绘图助手", ModelID: "gpt-image-2", ModelDisplayName: "GPT Image", Scene: "canvas_image_generate"}},
			Video: []canvasmodule.CanvasAgentOption{{ID: 9, Name: "视频助手", ModelID: "video-model", ModelDisplayName: "Video Model", Scene: "canvas_video_generate"}},
		},
	}, nil
}
func TestCanvasPublicAssetRoute(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeCanvasService{}
	router := gin.New()
	RegisterRoutes(router, service)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/canvas/v1/assets?keyword=sky&type=image", nil))
	if recorder.Code != http.StatusOK || service.assetQuery.Keyword != "sky" || service.assetQuery.Type != canvasmodule.AssetTypeImage {
		t.Fatalf("asset route mismatch code=%d body=%s query=%#v", recorder.Code, recorder.Body.String(), service.assetQuery)
	}
	assertNoProviderSecrets(t, recorder.Body.Bytes())
}

func TestCanvasTransportDoesNotOwnPromptRoute(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeCanvasService{}
	router := gin.New()
	RegisterRoutes(router, service)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/canvas/v1/prompts?keyword=cat", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("canvas transport must not own prompts route, got code=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestCanvasSettingsReturnsOnlyPublicFacade(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeCanvasService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if c.Request.URL.Path == "/api/canvas/v1/settings" {
			c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 9, Platform: "canvas"})
		}
	})
	RegisterRoutes(router, service)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/canvas/v1/settings", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected settings status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.settingsUserID != 9 {
		t.Fatalf("expected settings to include authenticated user id 9, got %d", service.settingsUserID)
	}
	body := recorder.Body.String()
	for _, want := range []string{
		`"allow_register":true`,
		`"canvas_text_generate"`,
		`"canvas_image_generate"`,
		`"canvas_video_generate"`,
		`"agents"`,
		`"text"`,
		`"image"`,
		`"video"`,
		`"model_id":"gpt-image-2"`,
		`"scene":"canvas_image_generate"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("settings response missing %s: %s", want, body)
		}
	}
	assertNoProviderSecrets(t, recorder.Body.Bytes())
	for _, forbidden := range []string{"provider_config", "raw_config", "billing", "wallet", "unit_price_cents"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("settings response leaks %s: %s", forbidden, body)
		}
	}
}

func TestCanvasTransportDoesNotOwnAIVideoRoutes(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeCanvasService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 9, Platform: "canvas"})
	})
	RegisterRoutes(router, service)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/canvas/v1/ai/videos"},
		{http.MethodGet, "/api/canvas/v1/ai/videos/99"},
		{http.MethodGet, "/api/canvas/v1/ai/videos/99/content"},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{"agent_id":10,"prompt":"clip","duration_seconds":4}`))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("canvas transport must not own %s %s, got code=%d body=%s", tc.method, tc.path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestCanvasTransportDoesNotOwnAIChatRoute(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeCanvasService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 9, Platform: "canvas"})
	})
	RegisterRoutes(router, service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/ai/chat/completions", strings.NewReader(`{"agent_id":7,"message":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("canvas transport must not own AI chat completion route, got code=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestCanvasTransportDoesNotOwnAIImageRoutes(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeCanvasService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 9, Platform: "canvas"})
	})
	RegisterRoutes(router, service)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/canvas/v1/ai/images/generations"},
		{http.MethodPost, "/api/canvas/v1/ai/images/edits"},
		{http.MethodGet, "/api/canvas/v1/ai/images/88"},
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(tc.method, tc.path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("canvas transport must not own %s %s, got code=%d body=%s", tc.method, tc.path, recorder.Code, recorder.Body.String())
		}
	}
}

func assertNoProviderSecrets(t *testing.T, body []byte) {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	text := string(body)
	for _, forbidden := range []string{"api_key", "api_key_enc", "base_url", "system_prompt"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("response leaks %s: %s", forbidden, text)
		}
	}
}

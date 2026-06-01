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
	promptQuery    canvasmodule.PromptListQuery
	assetQuery     canvasmodule.AssetListQuery
	settingsUserID int64
	chatInput      canvasmodule.ChatCompletionInput
	imageInput     canvasmodule.ImageGenerationInput
	imageStatusID  uint64
	videoInput     canvasmodule.VideoGenerationInput
	videoStatusID  int64
	videoContentID int64
}

func (f *fakeCanvasService) PublicPrompts(ctx context.Context, query canvasmodule.PromptListQuery) (*canvasmodule.PromptListResponse, *apperror.Error) {
	f.promptQuery = query
	return &canvasmodule.PromptListResponse{List: []canvasmodule.PromptItem{{ID: 1, Slug: "p", Title: "Prompt"}}}, nil
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
func (f *fakeCanvasService) ChatCompletion(ctx context.Context, input canvasmodule.ChatCompletionInput) (*canvasmodule.ChatCompletionResponse, *apperror.Error) {
	f.chatInput = input
	return &canvasmodule.ChatCompletionResponse{ID: "chat-1", Object: "chat.completion", Content: "ok"}, nil
}
func (f *fakeCanvasService) GenerateImage(ctx context.Context, input canvasmodule.ImageGenerationInput) (*canvasmodule.ImageGenerationResponse, *apperror.Error) {
	f.imageInput = input
	return &canvasmodule.ImageGenerationResponse{TaskID: 88, Status: "pending"}, nil
}
func (f *fakeCanvasService) ImageStatus(ctx context.Context, userID int64, id uint64) (*canvasmodule.ImageStatusResponse, *apperror.Error) {
	f.imageStatusID = id
	return &canvasmodule.ImageStatusResponse{}, nil
}
func (f *fakeCanvasService) GenerateVideo(ctx context.Context, input canvasmodule.VideoGenerationInput) (*canvasmodule.VideoGenerationResponse, *apperror.Error) {
	f.videoInput = input
	return &canvasmodule.VideoGenerationResponse{ID: 99, Status: "pending"}, nil
}
func (f *fakeCanvasService) VideoStatus(ctx context.Context, userID int64, id int64) (*canvasmodule.VideoStatusResponse, *apperror.Error) {
	f.videoStatusID = id
	return &canvasmodule.VideoStatusResponse{ID: id, Status: "pending"}, nil
}
func (f *fakeCanvasService) VideoContent(ctx context.Context, userID int64, id int64) ([]byte, string, *apperror.Error) {
	f.videoContentID = id
	return []byte("video"), "video/mp4", nil
}

func TestCanvasPublicRoutes(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeCanvasService{}
	router := gin.New()
	RegisterRoutes(router, service)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/canvas/v1/prompts?keyword=cat&category=style", nil))
	if recorder.Code != http.StatusOK || service.promptQuery.Keyword != "cat" || service.promptQuery.Category != "style" {
		t.Fatalf("prompt route mismatch code=%d body=%s query=%#v", recorder.Code, recorder.Body.String(), service.promptQuery)
	}
	assertNoProviderSecrets(t, recorder.Body.Bytes())

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/canvas/v1/assets?keyword=sky&type=image", nil))
	if recorder.Code != http.StatusOK || service.assetQuery.Keyword != "sky" || service.assetQuery.Type != canvasmodule.AssetTypeImage {
		t.Fatalf("asset route mismatch code=%d body=%s query=%#v", recorder.Code, recorder.Body.String(), service.assetQuery)
	}
	assertNoProviderSecrets(t, recorder.Body.Bytes())
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

func TestCanvasAIRoutesUseAuthenticatedUserAndDoNotLeakProviderConfig(t *testing.T) {
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
	if recorder.Code != http.StatusOK || service.chatInput.UserID != 9 || service.chatInput.AgentID != 7 || service.chatInput.Message != "hello" {
		t.Fatalf("chat route mismatch code=%d body=%s input=%#v", recorder.Code, recorder.Body.String(), service.chatInput)
	}
	assertNoProviderSecrets(t, recorder.Body.Bytes())

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/canvas/v1/ai/images/generations", strings.NewReader(`{"agent_id":8,"prompt":"cat","n":2}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.imageInput.UserID != 9 || service.imageInput.AgentID != 8 || service.imageInput.N != 2 {
		t.Fatalf("image route mismatch code=%d body=%s input=%#v", recorder.Code, recorder.Body.String(), service.imageInput)
	}
	assertNoProviderSecrets(t, recorder.Body.Bytes())

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/canvas/v1/ai/images/88", nil))
	if recorder.Code != http.StatusOK || service.imageStatusID != 88 {
		t.Fatalf("image status route mismatch code=%d body=%s id=%d", recorder.Code, recorder.Body.String(), service.imageStatusID)
	}
	assertNoProviderSecrets(t, recorder.Body.Bytes())

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/canvas/v1/ai/videos", strings.NewReader(`{"agent_id":10,"prompt":"clip","duration_seconds":4}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || service.videoInput.UserID != 9 || service.videoInput.AgentID != 10 || service.videoInput.DurationSeconds != 4 {
		t.Fatalf("video route mismatch code=%d body=%s input=%#v", recorder.Code, recorder.Body.String(), service.videoInput)
	}
	assertNoProviderSecrets(t, recorder.Body.Bytes())

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/canvas/v1/ai/videos/99", nil))
	if recorder.Code != http.StatusOK || service.videoStatusID != 99 {
		t.Fatalf("video status route mismatch code=%d body=%s id=%d", recorder.Code, recorder.Body.String(), service.videoStatusID)
	}
	assertNoProviderSecrets(t, recorder.Body.Bytes())

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/canvas/v1/ai/videos/99/content", nil))
	if recorder.Code != http.StatusOK || service.videoContentID != 99 || recorder.Body.String() != "video" {
		t.Fatalf("video content route mismatch code=%d body=%s id=%d", recorder.Code, recorder.Body.String(), service.videoContentID)
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

package canvas

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"admin_back_go/internal/middleware"
	aivideomodule "admin_back_go/internal/module/ai/video"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"

	"github.com/gin-gonic/gin"
)

type fakeCanvasVideoService struct {
	createInput          aivideomodule.CreateInput
	referenceUploadInput aivideomodule.ReferenceMediaUploadInput
	statusUserID         int64
	statusID             int64
	contentUserID        int64
	contentID            int64
	contentType          string
	contentBody          []byte
}

func (f *fakeCanvasVideoService) Create(ctx context.Context, input aivideomodule.CreateInput) (*aivideomodule.CreateResponse, *apperror.Error) {
	f.createInput = input
	return &aivideomodule.CreateResponse{ID: 99, Status: aivideomodule.StatusPending}, nil
}

func (f *fakeCanvasVideoService) Status(ctx context.Context, userID int64, id int64) (*aivideomodule.StatusResponse, *apperror.Error) {
	f.statusUserID = userID
	f.statusID = id
	return &aivideomodule.StatusResponse{ID: id, Status: aivideomodule.StatusRunning}, nil
}

func (f *fakeCanvasVideoService) Content(ctx context.Context, userID int64, id int64) ([]byte, string, *apperror.Error) {
	f.contentUserID = userID
	f.contentID = id
	if f.contentBody != nil {
		return f.contentBody, f.contentType, nil
	}
	return []byte("video"), "video/mp4", nil
}

func (f *fakeCanvasVideoService) UploadReferenceMedia(ctx context.Context, input aivideomodule.ReferenceMediaUploadInput) (*aivideomodule.ReferenceMediaUploadResponse, *apperror.Error) {
	f.referenceUploadInput = input
	return &aivideomodule.ReferenceMediaUploadResponse{
		ID:              "ref-video-1",
		URL:             "https://cos.test/ai-video-references/video/ref.mp4",
		StorageProvider: "cos",
		StorageKey:      "ai-video-references/video/ref.mp4",
		MimeType:        "video/mp4",
		MediaKind:       "video",
		Bytes:           int64(len(input.Body)),
	}, nil
}

func TestCanvasVideoRoutesUseCanvasIdentityAndService(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeCanvasVideoService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 9, Platform: enum.PlatformCanvas})
	})
	RegisterRoutes(router, service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/ai/videos", strings.NewReader(`{"agent_id":10,"prompt":"clip","duration_seconds":4,"size":"1280x720","resolution_name":"720p","generate_audio":false,"watermark":true}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.createInput.UserID != 9 || service.createInput.AgentID != 10 || service.createInput.Prompt != "clip" || service.createInput.DurationSeconds != 4 || service.createInput.Size != "1280x720" || service.createInput.ResolutionName != "720p" || service.createInput.ModelID != "" || service.createInput.GenerateAudio == nil || *service.createInput.GenerateAudio || service.createInput.Watermark == nil || !*service.createInput.Watermark {
		t.Fatalf("unexpected create input: %#v", service.createInput)
	}
	for _, want := range []string{`"id":99`, `"status":"pending"`} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Fatalf("create response missing %s: %s", want, recorder.Body.String())
		}
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/canvas/v1/ai/videos/99", nil))
	if recorder.Code != http.StatusOK || service.statusUserID != 9 || service.statusID != 99 {
		t.Fatalf("status route mismatch code=%d body=%s service=%#v", recorder.Code, recorder.Body.String(), service)
	}
	if !strings.Contains(recorder.Body.String(), `"status":"running"`) {
		t.Fatalf("status response missing running status: %s", recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/canvas/v1/ai/videos/99/content", nil))
	if recorder.Code != http.StatusOK || service.contentUserID != 9 || service.contentID != 99 || recorder.Body.String() != "video" || recorder.Header().Get("Content-Type") != "video/mp4" {
		t.Fatalf("content route mismatch code=%d body=%s type=%s service=%#v", recorder.Code, recorder.Body.String(), recorder.Header().Get("Content-Type"), service)
	}
}

func TestCanvasVideoRoutesAcceptActiveClientMultipartRequest(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeCanvasVideoService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 9, Platform: enum.PlatformCanvas})
	})
	RegisterRoutes(router, service)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for key, value := range map[string]string{
		"agent_id":         "10",
		"prompt":           "clip",
		"duration_seconds": "4",
		"size":             "1280x720",
		"resolution_name":  "720p",
		"generate_audio":   "true",
		"watermark":        "false",
	} {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write multipart field %s: %v", key, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/ai/videos", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.createInput.UserID != 9 || service.createInput.AgentID != 10 || service.createInput.Prompt != "clip" || service.createInput.DurationSeconds != 4 || service.createInput.Size != "1280x720" || service.createInput.ResolutionName != "720p" || service.createInput.ModelID != "" || service.createInput.GenerateAudio == nil || !*service.createInput.GenerateAudio || service.createInput.Watermark == nil || *service.createInput.Watermark {
		t.Fatalf("unexpected create input: %#v", service.createInput)
	}
}

func TestCanvasVideoReferenceMediaUploadUsesCanvasIdentityAndService(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeCanvasVideoService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 9, Platform: enum.PlatformCanvas})
	})
	RegisterRoutes(router, service)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("media_kind", "video"); err != nil {
		t.Fatalf("write media_kind: %v", err)
	}
	part, err := writer.CreateFormFile("file", "reference.mp4")
	if err != nil {
		t.Fatalf("create file part: %v", err)
	}
	if _, err := part.Write([]byte("video-bytes")); err != nil {
		t.Fatalf("write file part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/ai/videos/reference-media", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.referenceUploadInput.UserID != 9 || service.referenceUploadInput.MediaKind != "video" || service.referenceUploadInput.FileName != "reference.mp4" || service.referenceUploadInput.MimeType != "video/mp4" || string(service.referenceUploadInput.Body) != "video-bytes" {
		t.Fatalf("unexpected reference upload input: %#v", service.referenceUploadInput)
	}
	for _, forbidden := range []string{`"provider":`, `"api_key":`, `"base_url":`, `"model":`} {
		if strings.Contains(recorder.Body.String(), forbidden) {
			t.Fatalf("reference upload response leaked %s: %s", forbidden, recorder.Body.String())
		}
	}
	for _, want := range []string{`"url":"https://cos.test/ai-video-references/video/ref.mp4"`, `"storage_provider":"cos"`, `"media_kind":"video"`, `"bytes":11`} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Fatalf("reference upload response missing %s: %s", want, recorder.Body.String())
		}
	}
}

func TestCanvasVideoReferenceMediaUploadRejectsClientModelConfigOverride(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	for _, field := range []string{"model", "provider", "api_key", "base_url"} {
		t.Run(field, func(t *testing.T) {
			service := &fakeCanvasVideoService{}
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 9, Platform: enum.PlatformCanvas})
			})
			RegisterRoutes(router, service)

			body := &bytes.Buffer{}
			writer := multipart.NewWriter(body)
			_ = writer.WriteField("media_kind", "audio")
			_ = writer.WriteField(field, "client-owned")
			part, err := writer.CreateFormFile("file", "reference.mp3")
			if err != nil {
				t.Fatalf("create file part: %v", err)
			}
			_, _ = part.Write([]byte("audio-bytes"))
			if err := writer.Close(); err != nil {
				t.Fatalf("close multipart writer: %v", err)
			}

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/ai/videos/reference-media", body)
			request.Header.Set("Content-Type", writer.FormDataContentType())
			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400, got %d body=%s", recorder.Code, recorder.Body.String())
			}
			if service.referenceUploadInput.UserID != 0 {
				t.Fatalf("service must not be called when client overrides provider config: %#v", service.referenceUploadInput)
			}
			if !strings.Contains(recorder.Body.String(), "客户端不能覆盖Canvas智能体模型") {
				t.Fatalf("response missing model override message: %s", recorder.Body.String())
			}
		})
	}
}

func TestCanvasVideoRoutesRejectClientModelConfigOverride(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	tests := []struct {
		name        string
		contentType string
		body        string
	}{
		{name: "json model", contentType: "application/json", body: `{"agent_id":10,"prompt":"clip","model":"client-model"}`},
		{name: "json provider", contentType: "application/json", body: `{"agent_id":10,"prompt":"clip","provider":"client-provider"}`},
		{name: "json api_key", contentType: "application/json", body: `{"agent_id":10,"prompt":"clip","api_key":"client-secret"}`},
		{name: "json base_url", contentType: "application/json", body: `{"agent_id":10,"prompt":"clip","base_url":"https://client.example.test"}`},
		{name: "form model", contentType: "application/x-www-form-urlencoded", body: `agent_id=10&prompt=clip&model=client-model`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &fakeCanvasVideoService{}
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 9, Platform: enum.PlatformCanvas})
			})
			RegisterRoutes(router, service)

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/ai/videos", strings.NewReader(tt.body))
			request.Header.Set("Content-Type", tt.contentType)
			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400, got %d body=%s", recorder.Code, recorder.Body.String())
			}
			if service.createInput.UserID != 0 || service.createInput.AgentID != 0 || service.createInput.Prompt != "" || service.createInput.ModelID != "" {
				t.Fatalf("service must not be called when client overrides model config: %#v", service.createInput)
			}
			if !strings.Contains(recorder.Body.String(), "客户端不能覆盖Canvas智能体模型") {
				t.Fatalf("response missing model override message: %s", recorder.Body.String())
			}
		})
	}
}

func TestCanvasVideoRoutesRejectWrongPlatformIdentity(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeCanvasVideoService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 9, Platform: enum.PlatformAdmin})
	})
	RegisterRoutes(router, service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/ai/videos", strings.NewReader(`{"agent_id":10,"prompt":"clip"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.createInput.UserID != 0 || service.statusUserID != 0 || service.contentUserID != 0 {
		t.Fatalf("service must not be called for wrong platform: %#v", service)
	}
}

func TestCanvasVideoContentFallsBackToOctetStreamForEmptyContentType(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeCanvasVideoService{contentBody: []byte("video"), contentType: ""}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 9, Platform: enum.PlatformCanvas})
	})
	RegisterRoutes(router, service)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/canvas/v1/ai/videos/99/content", nil))
	if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("expected octet-stream fallback, got code=%d type=%s body=%s", recorder.Code, recorder.Header().Get("Content-Type"), recorder.Body.String())
	}
}

func TestCanvasVideoRoutesNilServiceReturnsServiceMissing(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 9, Platform: enum.PlatformCanvas})
	})
	RegisterRoutes(router, nil)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/ai/videos", strings.NewReader(`{"agent_id":10,"prompt":"clip"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), "Canvas视频生成服务未配置") {
		t.Fatalf("expected service missing response, got code=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

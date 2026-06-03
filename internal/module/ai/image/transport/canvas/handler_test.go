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
	aiimagemodule "admin_back_go/internal/module/ai/image"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"

	"github.com/gin-gonic/gin"
)

type fakeCanvasImageService struct {
	createInput       aiimagemodule.CreateInput
	uploadInput       aiimagemodule.CreateWithUploadedAssetsInput
	detailUserID      uint64
	detailTaskID      uint64
	returnNilDetail   bool
	returnEmptyDetail bool
}

func (f *fakeCanvasImageService) PageInit(ctx context.Context) (*aiimagemodule.PageInitResponse, *apperror.Error) {
	return nil, apperror.InternalKey("unexpected", nil, "unexpected")
}

func (f *fakeCanvasImageService) List(ctx context.Context, userID uint64, query aiimagemodule.ListQuery) (*aiimagemodule.ListResponse, *apperror.Error) {
	return nil, apperror.InternalKey("unexpected", nil, "unexpected")
}

func (f *fakeCanvasImageService) Detail(ctx context.Context, userID uint64, taskID uint64) (*aiimagemodule.DetailResponse, *apperror.Error) {
	f.detailUserID = userID
	f.detailTaskID = taskID
	if f.returnNilDetail {
		return nil, nil
	}
	if f.returnEmptyDetail {
		return &aiimagemodule.DetailResponse{}, nil
	}
	return &aiimagemodule.DetailResponse{
		Task: aiimagemodule.TaskDTO{ID: taskID, Status: aiimagemodule.StatusSuccess},
		Outputs: []aiimagemodule.AssetDTO{{
			ID:         700,
			StorageURL: "https://example.test/cat.png",
			MimeType:   "image/png",
			SourceType: aiimagemodule.SourceTypeGenerated,
		}},
	}, nil
}

func (f *fakeCanvasImageService) RegisterAsset(ctx context.Context, input aiimagemodule.RegisterAssetInput) (*aiimagemodule.AssetDTO, *apperror.Error) {
	return nil, apperror.InternalKey("unexpected", nil, "unexpected")
}

func (f *fakeCanvasImageService) Create(ctx context.Context, input aiimagemodule.CreateInput) (*aiimagemodule.CreateTaskResponse, *apperror.Error) {
	f.createInput = input
	return &aiimagemodule.CreateTaskResponse{Task: aiimagemodule.TaskDTO{ID: 88, Status: aiimagemodule.StatusPending}}, nil
}

func (f *fakeCanvasImageService) CreateWithUploadedAssets(ctx context.Context, input aiimagemodule.CreateWithUploadedAssetsInput) (*aiimagemodule.CreateTaskResponse, *apperror.Error) {
	f.uploadInput = input
	return &aiimagemodule.CreateTaskResponse{Task: aiimagemodule.TaskDTO{ID: 89, Status: aiimagemodule.StatusPending}}, nil
}

func (f *fakeCanvasImageService) Favorite(ctx context.Context, input aiimagemodule.FavoriteInput) (*aiimagemodule.TaskDTO, *apperror.Error) {
	return nil, apperror.InternalKey("unexpected", nil, "unexpected")
}

func (f *fakeCanvasImageService) Delete(ctx context.Context, userID uint64, taskID uint64) *apperror.Error {
	return apperror.InternalKey("unexpected", nil, "unexpected")
}

func TestCanvasImageGenerationCreatesCanvasPlatformTask(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeCanvasImageService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 9, Platform: enum.PlatformCanvas})
	})
	RegisterRoutes(router, service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/ai/images/generations", strings.NewReader(`{"agent_id":8,"prompt":"cat","n":2}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.createInput.UserID != 9 || service.createInput.AgentID != 8 || service.createInput.Platform != enum.PlatformCanvas || service.createInput.N != 2 {
		t.Fatalf("unexpected create input: %#v", service.createInput)
	}
	for _, want := range []string{`"task_id":88`, `"status":"pending"`} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Fatalf("response missing %s: %s", want, recorder.Body.String())
		}
	}
}

func TestCanvasImageGenerationRejectsAdminPlatformIdentity(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeCanvasImageService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 9, Platform: enum.PlatformAdmin})
	})
	RegisterRoutes(router, service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/ai/images/generations", strings.NewReader(`{"agent_id":8,"prompt":"cat","n":2}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.createInput.UserID != 0 {
		t.Fatalf("service should not be called for wrong platform, got input: %#v", service.createInput)
	}
}

func TestCanvasImageGenerationRejectsEmptyPlatformIdentity(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeCanvasImageService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 9, Platform: ""})
	})
	RegisterRoutes(router, service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/ai/images/generations", strings.NewReader(`{"agent_id":8,"prompt":"cat","n":2}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code == http.StatusOK {
		t.Fatalf("expected non-200 status for empty platform, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.createInput.UserID != 0 {
		t.Fatalf("service should not be called for empty platform, got input: %#v", service.createInput)
	}
}

func TestCanvasImageEditUploadsReferencesAndCreatesCanvasPlatformTask(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeCanvasImageService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 9, Platform: enum.PlatformCanvas})
	})
	RegisterRoutes(router, service)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("agent_id", "8")
	_ = writer.WriteField("prompt", "use this reference")
	_ = writer.WriteField("n", "1")
	file, err := writer.CreateFormFile("image", "reference.png")
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := file.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/ai/images/edits", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.uploadInput.UserID != 9 || service.uploadInput.AgentID != 8 || service.uploadInput.Platform != enum.PlatformCanvas || service.uploadInput.Prompt != "use this reference" {
		t.Fatalf("unexpected upload input: %#v", service.uploadInput)
	}
	if len(service.uploadInput.Assets) != 1 || service.uploadInput.Assets[0].FileName != "reference.png" || len(service.uploadInput.Assets[0].Body) == 0 {
		t.Fatalf("expected uploaded reference image, got %#v", service.uploadInput.Assets)
	}
	if !strings.Contains(recorder.Body.String(), `"task_id":89`) {
		t.Fatalf("response missing task id: %s", recorder.Body.String())
	}
}

func TestCanvasImageStatusReturnsTaskAndOutputs(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeCanvasImageService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 9, Platform: enum.PlatformCanvas})
	})
	RegisterRoutes(router, service)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/canvas/v1/ai/images/88", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.detailUserID != 9 || service.detailTaskID != 88 {
		t.Fatalf("unexpected detail lookup user=%d task=%d", service.detailUserID, service.detailTaskID)
	}
	for _, want := range []string{`"task"`, `"outputs"`, `"storage_url":"https://example.test/cat.png"`} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Fatalf("response missing %s: %s", want, recorder.Body.String())
		}
	}
}

func TestCanvasImageStatusRejectsInvalidDetailResult(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	tests := []struct {
		name    string
		service *fakeCanvasImageService
	}{
		{name: "nil detail", service: &fakeCanvasImageService{returnNilDetail: true}},
		{name: "empty detail", service: &fakeCanvasImageService{returnEmptyDetail: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 9, Platform: enum.PlatformCanvas})
			})
			RegisterRoutes(router, tt.service)

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/canvas/v1/ai/images/88", nil))

			if recorder.Code == http.StatusOK {
				t.Fatalf("expected non-200 status for invalid detail result, got %d body=%s", recorder.Code, recorder.Body.String())
			}
			if tt.service.detailUserID != 9 || tt.service.detailTaskID != 88 {
				t.Fatalf("unexpected detail lookup user=%d task=%d", tt.service.detailUserID, tt.service.detailTaskID)
			}
			if !strings.Contains(recorder.Body.String(), "Canvas图片生成结果无效") {
				t.Fatalf("response missing invalid result message: %s", recorder.Body.String())
			}
		})
	}
}

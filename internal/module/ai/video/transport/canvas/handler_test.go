package canvas

import (
	"context"
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
	createInput   aivideomodule.CreateInput
	statusUserID  int64
	statusID      int64
	contentUserID int64
	contentID     int64
	contentType   string
	contentBody   []byte
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

func TestCanvasVideoRoutesUseCanvasIdentityAndService(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakeCanvasVideoService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 9, Platform: enum.PlatformCanvas})
	})
	RegisterRoutes(router, service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/ai/videos", strings.NewReader(`{"agent_id":10,"prompt":"clip","duration_seconds":4,"size":"1280x720","resolution_name":"720p","model":"client-model"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.createInput.UserID != 9 || service.createInput.AgentID != 10 || service.createInput.Prompt != "clip" || service.createInput.DurationSeconds != 4 || service.createInput.Size != "1280x720" || service.createInput.ResolutionName != "720p" || service.createInput.ModelID != "client-model" {
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

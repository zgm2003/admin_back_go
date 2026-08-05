package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	airunmodule "admin_back_go/internal/module/ai/run"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

type inputAttachmentPreviewHTTPService struct {
	nilHTTPService
	runID   int64
	ordinal int64
}

func (service *inputAttachmentPreviewHTTPService) InputAttachmentPreview(_ context.Context, runID, ordinal int64) (*airunmodule.InputAttachmentPreviewResponse, *apperror.Error) {
	service.runID = runID
	service.ordinal = ordinal
	return &airunmodule.InputAttachmentPreviewResponse{URL: "https://signed.example/a.png", ExpiresIn: 300}, nil
}

func TestInputAttachmentPreviewHandlerBindsBothPositiveIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &inputAttachmentPreviewHTTPService{}
	router := gin.New()
	router.GET("/api/admin/v1/ai-runs/:id/input-attachments/:ordinal/preview", NewHandler(service).InputAttachmentPreview)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/v1/ai-runs/44/input-attachments/2/preview", nil))

	if recorder.Code != http.StatusOK || service.runID != 44 || service.ordinal != 2 {
		t.Fatalf("status=%d call=%d/%d body=%s", recorder.Code, service.runID, service.ordinal, recorder.Body.String())
	}
	var body struct {
		Data airunmodule.InputAttachmentPreviewResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body.Data.ExpiresIn != 300 {
		t.Fatalf("response=%+v error=%v", body, err)
	}
}

func TestInputAttachmentPreviewRouteUsesRunListPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := adminroute.NewRegistry()
	Register(gin.New(), &inputAttachmentPreviewHTTPService{}, registry)
	for _, definition := range registry.Definitions() {
		if definition.Method == http.MethodGet && definition.Path == "/api/admin/v1/ai-runs/:id/input-attachments/:ordinal/preview" {
			if definition.Access.Kind != adminroute.AccessPermission || definition.Access.PermissionCode != "ai_run_list" {
				t.Fatalf("preview policy=%+v", definition.Access)
			}
			return
		}
	}
	t.Fatal("input attachment preview route is not registered")
}

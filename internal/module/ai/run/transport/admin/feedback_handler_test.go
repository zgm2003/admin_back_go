package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"admin_back_go/internal/middleware"
	airunmodule "admin_back_go/internal/module/ai/run"
	"admin_back_go/internal/server/adminroute"
	"admin_back_go/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

func TestFeedbackHandlerAllowsFalseAndReturnsExactNullableState(t *testing.T) {
	router, service := newFeedbackTestRouter()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/admin/v1/ai-runs/44/user-feedback", bytes.NewBufferString(`{"liked":false}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.feedbackUserID != 7 || service.feedbackRunID != 44 || service.feedbackLiked {
		t.Fatalf("feedback call=%d/%d/%v", service.feedbackUserID, service.feedbackRunID, service.feedbackLiked)
	}
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data) != 3 || body.Data["id"] != float64(44) || body.Data["liked"] != false {
		t.Fatalf("feedback response must contain exact state: %#v", body.Data)
	}
	if likedAt, exists := body.Data["liked_at"]; !exists || likedAt != nil {
		t.Fatalf("liked_at must be explicit null: %#v", body.Data)
	}
}

func TestFeedbackRouteDoesNotRequireRunListPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := adminroute.NewRegistry()
	Register(gin.New(), &fakeRunHTTPService{}, registry)
	definitions := make(map[string]adminroute.Definition)
	for _, definition := range registry.Definitions() {
		definitions[definition.Method+" "+definition.Path] = definition
	}
	feedback := definitions[http.MethodPut+" /api/admin/v1/ai-runs/:id/user-feedback"]
	if feedback.Access.Kind != adminroute.AccessAuthenticated || feedback.Access.PermissionCode != "" {
		t.Fatalf("feedback policy=%+v", feedback)
	}
	for _, path := range []string{"/api/admin/v1/ai-runs", "/api/admin/v1/ai-runs/:id"} {
		definition := definitions[http.MethodGet+" "+path]
		if definition.Access.Kind != adminroute.AccessPermission || definition.Access.PermissionCode != "ai_run_list" {
			t.Fatalf("management policy %s=%+v", path, definition)
		}
	}
}

func newFeedbackTestRouter() (*gin.Engine, *fakeRunHTTPService) {
	gin.SetMode(gin.TestMode)
	service := &fakeRunHTTPService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextAuthIdentity, &middleware.AuthIdentity{UserID: 7, Platform: "admin"})
	})
	Register(router, service)
	return router, service
}

type fakeRunHTTPService struct {
	managementRunHTTPService
	feedbackUserID int64
	feedbackRunID  int64
	feedbackLiked  bool
}

type managementRunHTTPService struct{}

var _ airunmodule.HTTPService = managementRunHTTPService{}

func (managementRunHTTPService) PageInit(context.Context, airunmodule.PageInitFilter) (*airunmodule.InitResponse, *apperror.Error) {
	return &airunmodule.InitResponse{}, nil
}
func (managementRunHTTPService) List(context.Context, airunmodule.ListQuery) (*airunmodule.ListResponse, *apperror.Error) {
	return &airunmodule.ListResponse{}, nil
}
func (managementRunHTTPService) Detail(context.Context, int64) (*airunmodule.DetailResponse, *apperror.Error) {
	return &airunmodule.DetailResponse{}, nil
}
func (managementRunHTTPService) InputAttachmentPreview(context.Context, int64, int64) (*airunmodule.InputAttachmentPreviewResponse, *apperror.Error) {
	return &airunmodule.InputAttachmentPreviewResponse{}, nil
}
func (managementRunHTTPService) Dashboard(context.Context, airunmodule.DashboardFilter) (*airunmodule.DashboardResponse, *apperror.Error) {
	return &airunmodule.DashboardResponse{}, nil
}
func (service *fakeRunHTTPService) SetUserFeedback(_ context.Context, userID int64, id int64, liked bool) (*airunmodule.FeedbackResponse, *apperror.Error) {
	service.feedbackUserID = userID
	service.feedbackRunID = id
	service.feedbackLiked = liked
	return &airunmodule.FeedbackResponse{ID: id, Liked: liked}, nil
}

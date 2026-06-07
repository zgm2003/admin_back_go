package canvas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	promptmodule "admin_back_go/internal/module/ai/prompt"
	"admin_back_go/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

type fakePromptService struct {
	query promptmodule.ListQuery
}

func (f *fakePromptService) PublicList(ctx context.Context, query promptmodule.ListQuery) (*promptmodule.ListResponse, *apperror.Error) {
	f.query = query
	return &promptmodule.ListResponse{List: []promptmodule.Item{{ID: 1, Slug: "cat", Title: "Cat"}}}, nil
}

func TestCanvasPromptRoutePassesQueryToPromptService(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := &fakePromptService{}
	router := gin.New()
	RegisterRoutes(router, service)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/canvas/v1/prompts?keyword=cat&category=style&tag=poster&current_page=2&page_size=5", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.query.Keyword != "cat" || service.query.Category != "style" || service.query.CurrentPage != 2 || service.query.PageSize != 5 {
		t.Fatalf("query scalar mismatch: %#v", service.query)
	}
	if len(service.query.Tags) != 1 || service.query.Tags[0] != "poster" {
		t.Fatalf("query tags mismatch: %#v", service.query.Tags)
	}
}

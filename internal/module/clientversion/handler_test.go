package clientversion

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"admin_back_go/internal/apperror"
	projecti18n "admin_back_go/internal/i18n"

	"github.com/gin-gonic/gin"
)

type fakeHTTPService struct {
	err *apperror.Error
}

func (f *fakeHTTPService) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return &InitResponse{}, f.err
}

func (f *fakeHTTPService) List(ctx context.Context, query ListQuery) (*ListResponse, *apperror.Error) {
	return &ListResponse{Page: Page{CurrentPage: query.CurrentPage, PageSize: query.PageSize}}, f.err
}

func (f *fakeHTTPService) Create(ctx context.Context, input CreateInput) (int64, *apperror.Error) {
	return 1, f.err
}

func (f *fakeHTTPService) Update(ctx context.Context, id int64, input UpdateInput) *apperror.Error {
	return f.err
}

func (f *fakeHTTPService) SetLatest(ctx context.Context, id int64) *apperror.Error {
	return f.err
}

func (f *fakeHTTPService) ForceUpdate(ctx context.Context, id int64, forceUpdate int) *apperror.Error {
	return f.err
}

func (f *fakeHTTPService) Delete(ctx context.Context, id int64) *apperror.Error {
	return f.err
}

func (f *fakeHTTPService) UpdateJSON(ctx context.Context, platform string) (any, *apperror.Error) {
	return []any{}, f.err
}

func (f *fakeHTTPService) CurrentCheck(ctx context.Context, query CurrentCheckQuery) (*CurrentCheckResponse, *apperror.Error) {
	return &CurrentCheckResponse{}, f.err
}

func TestHandlerListLocalizesInvalidQuery(t *testing.T) {
	router := newClientVersionLocalizedRouter(&fakeHTTPService{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/client-versions?current_page=abc", nil)
	request.Header.Set("Accept-Language", "en-US")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := decodeClientVersionBody(t, recorder)
	if body["msg"] != "Invalid client version list request" {
		t.Fatalf("expected localized list query error, got %#v", body["msg"])
	}
}

func newClientVersionLocalizedRouter(service HTTPService) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(projecti18n.Localize())
	RegisterRoutes(router, service)
	return router
}

func decodeClientVersionBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	return body
}

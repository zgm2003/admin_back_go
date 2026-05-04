package systemlog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/dict"

	"github.com/gin-gonic/gin"
)

type fakeService struct {
	filesCalled bool
	linesQuery  LinesQuery
}

func (f *fakeService) Init(ctx context.Context) (*InitResponse, *apperror.Error) {
	return &InitResponse{Dict: InitDict{LogLevelArr: []dict.Option[string]{{Label: "ERROR", Value: "ERROR"}}, LogTailArr: []dict.Option[int]{{Label: "最近 500 行", Value: 500}}}}, nil
}

func (f *fakeService) Files(ctx context.Context) (*FilesResponse, *apperror.Error) {
	f.filesCalled = true
	return &FilesResponse{List: []FileItem{{Name: "admin-api.log", Size: 10, SizeHuman: "10 B", MTime: "2026-05-04 10:00:00"}}}, nil
}

func (f *fakeService) Lines(ctx context.Context, query LinesQuery) (*LinesResponse, *apperror.Error) {
	f.linesQuery = query
	return &LinesResponse{Filename: query.Filename, Total: 1, Lines: []LineItem{{Number: 1, Level: "ERROR", Content: "ERROR boom"}}}, nil
}

func TestHandlerUsesRESTRoutes(t *testing.T) {
	service := &fakeService{}
	router := newTestRouter(service)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/system-logs/files", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected files status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !service.filesCalled {
		t.Fatalf("expected files service call")
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/admin/v1/system-logs/files/admin-api.log/lines?tail=1000&level=ERROR&keyword=boom", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected lines status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.linesQuery.Filename != "admin-api.log" || service.linesQuery.Tail != 1000 || service.linesQuery.Level != "ERROR" || service.linesQuery.Keyword != "boom" {
		t.Fatalf("lines query mismatch: %#v", service.linesQuery)
	}
}

func TestHandlerInitReturnsDictionaries(t *testing.T) {
	router := newTestRouter(&fakeService{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/system-logs/init", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected init status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := decodeBody(t, recorder)
	data := body["data"].(map[string]any)
	dictValue := data["dict"].(map[string]any)
	if len(dictValue["log_level_arr"].([]any)) == 0 || len(dictValue["log_tail_arr"].([]any)) == 0 {
		t.Fatalf("expected log dictionaries, got %#v", dictValue)
	}
}

func newTestRouter(service HTTPService) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	RegisterRoutes(router, service)
	return router
}

func decodeBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	return body
}

package uploadtoken

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
	input CreateInput
	err   *apperror.Error
}

func (f *fakeHTTPService) Create(ctx context.Context, input CreateInput) (*CreateResponse, *apperror.Error) {
	f.input = input
	if f.err != nil {
		return nil, f.err
	}
	return &CreateResponse{Provider: ProviderCOS, Bucket: "bucket-a", Region: "ap-nanjing", Key: "images/demo.png"}, nil
}

func TestHandlerCreateLocalizesInvalidRequest(t *testing.T) {
	router := newUploadTokenLocalizedTestRouter(&fakeHTTPService{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/upload-tokens", nil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept-Language", "en-US")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := decodeUploadTokenBody(t, recorder)
	if body["msg"] != "Invalid upload token request" {
		t.Fatalf("expected localized request error, got %#v", body["msg"])
	}
}

func newUploadTokenLocalizedTestRouter(service HTTPService) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(projecti18n.Localize())
	RegisterRoutes(router, service)
	return router
}

func decodeUploadTokenBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	return body
}

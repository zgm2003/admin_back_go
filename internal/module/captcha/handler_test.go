package captcha

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"admin_back_go/internal/apperror"

	"github.com/gin-gonic/gin"
)

type fakeHTTPService struct {
	result *ChallengeResponse
	err    *apperror.Error
}

func (f fakeHTTPService) Generate(ctx context.Context) (*ChallengeResponse, *apperror.Error) {
	return f.result, f.err
}

func TestHandlerGenerateReturnsSlideChallenge(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	RegisterRoutes(router, fakeHTTPService{result: &ChallengeResponse{
		CaptchaID:   "captcha-id",
		CaptchaType: TypeSlide,
		MasterImage: "data:image/jpeg;base64,master",
		TileImage:   "data:image/png;base64,tile",
		TileX:       7,
		TileY:       53,
		TileWidth:   62,
		TileHeight:  62,
		ImageWidth:  300,
		ImageHeight: 220,
		ExpiresIn:   120,
	}})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/v1/auth/captcha", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	data := body["data"].(map[string]any)
	if data["captcha_id"] != "captcha-id" || data["captcha_type"] != TypeSlide {
		t.Fatalf("unexpected captcha response: %#v", data)
	}
}

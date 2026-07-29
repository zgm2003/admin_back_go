package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	airunmodule "admin_back_go/internal/module/ai/run"
	"admin_back_go/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

type latencyHTTPService struct{ nilHTTPService }

func (latencyHTTPService) LatencyStats(context.Context) (*airunmodule.LatencyStatsResponse, *apperror.Error) {
	return &airunmodule.LatencyStatsResponse{WindowDays: 30, MaxSamples: 10000, List: []airunmodule.LatencyStatsItem{{
		ProviderID: 9, ModelID: "gpt-test", TTFT: airunmodule.LatencyDistribution{SampleCount: 20, P50MS: 10, P95MS: 20, P99MS: 30},
	}}}, nil
}

func TestLatencyStatsHandlerReturnsProviderModelPercentiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(latencyHTTPService{})
	router.GET("/api/admin/v1/ai-runs/stats/latency", handler.LatencyStats)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/v1/ai-runs/stats/latency", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Data airunmodule.LatencyStatsResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data.List) != 1 || body.Data.List[0].TTFT.P95MS != 20 {
		t.Fatalf("response=%+v", body)
	}
}

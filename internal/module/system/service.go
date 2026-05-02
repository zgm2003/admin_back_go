package system

import (
	"context"

	"admin_back_go/internal/readiness"
	"admin_back_go/internal/version"
)

type ReadinessChecker interface {
	Readiness(ctx context.Context) readiness.Report
}

type Service struct {
	readiness ReadinessChecker
}

func NewService(checker ReadinessChecker) *Service {
	return &Service{readiness: checker}
}

func (s *Service) Health() HealthResponse {
	return HealthResponse{
		Service: "admin-api",
		Status:  "ok",
		Version: version.Version,
	}
}

func (s *Service) Ping() PingResponse {
	return PingResponse{Message: "pong"}
}

func (s *Service) Ready(ctx context.Context) readiness.Report {
	if s.readiness == nil {
		return readiness.NewReport(map[string]readiness.Check{
			"database":    {Status: readiness.StatusDisabled},
			"redis":       {Status: readiness.StatusDisabled},
			"token_redis": {Status: readiness.StatusDisabled},
		})
	}
	return s.readiness.Readiness(ctx)
}

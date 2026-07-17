package runtime

import (
	"context"

	"admin_back_go/internal/readiness"
)

const (
	StatusReady    = readiness.StatusReady
	StatusNotReady = readiness.StatusNotReady

	StatusUp       = readiness.StatusUp
	StatusDown     = readiness.StatusDown
	StatusDisabled = readiness.StatusDisabled
)

type Check = readiness.Check
type Report = readiness.Report

func NewReport(checks map[string]Check) Report {
	return readiness.NewReport(checks)
}

type Runtime interface {
	Start(context.Context) error
	Shutdown(context.Context) error
	Health(context.Context) Report
}

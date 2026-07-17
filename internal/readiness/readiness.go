package readiness

import runtimepkg "admin_back_go/internal/runtime"

const (
	StatusReady    = runtimepkg.StatusReady
	StatusNotReady = runtimepkg.StatusNotReady

	StatusUp       = runtimepkg.StatusUp
	StatusDown     = runtimepkg.StatusDown
	StatusDisabled = runtimepkg.StatusDisabled
)

type Check = runtimepkg.Check
type Report = runtimepkg.Report

func NewReport(checks map[string]Check) Report {
	return runtimepkg.NewReport(checks)
}

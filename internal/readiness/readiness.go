package readiness

const (
	StatusReady    = "ready"
	StatusNotReady = "not_ready"

	StatusUp       = "up"
	StatusDown     = "down"
	StatusDisabled = "disabled"
)

type Check struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type Report struct {
	Status string           `json:"status"`
	Checks map[string]Check `json:"checks"`
}

func NewReport(checks map[string]Check) Report {
	status := StatusReady
	for _, check := range checks {
		if check.Status == StatusDown {
			status = StatusNotReady
			break
		}
	}

	return Report{
		Status: status,
		Checks: checks,
	}
}

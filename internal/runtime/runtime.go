package runtime

import "context"

type Check struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type Report struct {
	Status string           `json:"status"`
	Checks map[string]Check `json:"checks"`
}

type Runtime interface {
	Start(context.Context) error
	Shutdown(context.Context) error
	Health(context.Context) Report
}

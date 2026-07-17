package ai

import "errors"

var (
	ErrEngineDisabled  = errors.New("ai engine disabled")
	ErrInvalidConfig   = errors.New("ai engine invalid config")
	ErrUnauthorized    = errors.New("ai engine unauthorized")
	ErrRateLimited     = errors.New("ai engine rate limited")
	ErrUpstreamTimeout = errors.New("ai engine upstream timeout")
	ErrUpstreamFailed  = errors.New("ai engine upstream failed")
	ErrCanceled        = errors.New("ai engine canceled")
)

type ProviderOutcome string

const (
	ProviderOutcomeNotDispatched ProviderOutcome = "not_dispatched"
	ProviderOutcomeRejected      ProviderOutcome = "rejected"
	ProviderOutcomeUnknown       ProviderOutcome = "outcome_unknown"
)

type ProviderError struct {
	Outcome           ProviderOutcome
	ProviderRequestID string
	Cause             error
}

func NewProviderError(outcome ProviderOutcome, providerRequestID string, cause error) error {
	if cause == nil {
		return nil
	}
	return &ProviderError{Outcome: outcome, ProviderRequestID: providerRequestID, Cause: cause}
}

func (e *ProviderError) Error() string {
	if e == nil || e.Cause == nil {
		return "provider request failed"
	}
	return e.Cause.Error()
}

func (e *ProviderError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func ProviderOutcomeFromError(err error) (ProviderOutcome, bool) {
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr == nil {
		return "", false
	}
	return providerErr.Outcome, true
}

func ProviderRequestIDFromError(err error) string {
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr == nil {
		return ""
	}
	return providerErr.ProviderRequestID
}

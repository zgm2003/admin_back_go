package contextengine

import (
	"errors"

	infraai "admin_back_go/internal/infra/ai"
)

type MemoryPermanentError struct {
	Code  string
	Cause error
}

func (failure *MemoryPermanentError) Error() string {
	if failure == nil || failure.Cause == nil {
		return "permanent memory build failure"
	}
	return failure.Cause.Error()
}

func (failure *MemoryPermanentError) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.Cause
}

func permanentMemoryFailure(err error) (string, bool) {
	switch {
	case errors.Is(err, infraai.ErrUnauthorized):
		return "ai.context.memory_provider_unauthorized", true
	case errors.Is(err, infraai.ErrInvalidConfig), errors.Is(err, infraai.ErrEngineDisabled):
		return "ai.context.memory_provider_invalid", true
	default:
		return "", false
	}
}

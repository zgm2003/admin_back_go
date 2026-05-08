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

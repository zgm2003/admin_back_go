package payment

import (
	"net/url"
	"time"
)

const (
	callbackProcessPending = "pending"
	callbackProcessSuccess = "success"
	callbackProcessFailed  = "failed"
	callbackProcessIgnored = "ignored"
	callbackResultSuccess  = "success"
	callbackResultFail     = "fail"
)

type AlipayCallbackInput struct {
	Form url.Values
}

type AlipayCallbackResult struct {
	Text string
}

type CallbackEventResolution struct {
	EventID        int64
	DedupeKey      []byte
	SignatureValid int
	ProcessStatus  string
	ProcessMessage string
	ProcessedAt    time.Time
	PaidOrderID    int64
	AlipayTradeNo  string
	PaidAt         time.Time
}

type CallbackEventResolutionResult struct {
	Event     *CallbackEvent
	PaidOrder *PaidOrderFinalization
	Replay    bool
}

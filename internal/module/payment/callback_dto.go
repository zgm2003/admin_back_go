package payment

import "net/url"

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

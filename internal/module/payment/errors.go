package payment

import "errors"

var ErrRepositoryNotConfigured = errors.New("payment: repository not configured")
var ErrGatewayNotConfigured = errors.New("payment: gateway not configured")
var ErrOutTradeNoRequired = errors.New("payment: out trade no is required")

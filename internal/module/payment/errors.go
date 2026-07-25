package payment

import (
	"errors"
	"strings"
)

var ErrRepositoryNotConfigured = errors.New("payment: repository not configured")
var ErrGatewayNotConfigured = errors.New("payment: gateway not configured")
var ErrOutTradeNoRequired = errors.New("payment: out trade no is required")
var ErrPaymentStateChanged = errors.New("payment: state changed")
var ErrPaymentOrderNotFound = errors.New("payment: order not found")

const alipayTradeNotExistCode = "ACQ.TRADE_NOT_EXIST"

func isAlipayTradeNotExistError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), alipayTradeNotExistCode)
}

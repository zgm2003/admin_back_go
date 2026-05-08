package enum

const (
	PaymentProviderAlipay = "alipay"
)

var PaymentProviders = []string{PaymentProviderAlipay}
var PaymentProviderLabels = map[string]string{PaymentProviderAlipay: "支付宝"}

const (
	PaymentMethodWeb = "web"
	PaymentMethodH5  = "h5"
)

var PaymentMethods = []string{PaymentMethodWeb, PaymentMethodH5}
var PaymentMethodLabels = map[string]string{
	PaymentMethodWeb: "PC网页支付",
	PaymentMethodH5:  "H5支付",
}

const (
	PaymentOrderPending   = 1
	PaymentOrderPaying    = 2
	PaymentOrderSucceeded = 3
	PaymentOrderClosed    = 4
	PaymentOrderFailed    = 5
)

var PaymentOrderStatuses = []int{PaymentOrderPending, PaymentOrderPaying, PaymentOrderSucceeded, PaymentOrderClosed, PaymentOrderFailed}
var PaymentOrderStatusLabels = map[int]string{
	PaymentOrderPending:   "待支付",
	PaymentOrderPaying:    "支付中",
	PaymentOrderSucceeded: "支付成功",
	PaymentOrderClosed:    "已关闭",
	PaymentOrderFailed:    "支付失败",
}

const (
	PaymentEventCreate = "create"
	PaymentEventQuery  = "query"
	PaymentEventNotify = "notify"
	PaymentEventClose  = "close"
	PaymentEventSync   = "sync"
)

var PaymentEventTypes = []string{PaymentEventCreate, PaymentEventQuery, PaymentEventNotify, PaymentEventClose, PaymentEventSync}
var PaymentEventTypeLabels = map[string]string{
	PaymentEventCreate: "创建支付",
	PaymentEventQuery:  "查询支付",
	PaymentEventNotify: "支付回调",
	PaymentEventClose:  "关闭支付",
	PaymentEventSync:   "定时同步",
}

const (
	PaymentEventPending = 1
	PaymentEventSuccess = 2
	PaymentEventFailed  = 3
	PaymentEventIgnored = 4
)

var PaymentEventProcessStatuses = []int{PaymentEventPending, PaymentEventSuccess, PaymentEventFailed, PaymentEventIgnored}
var PaymentEventProcessStatusLabels = map[int]string{
	PaymentEventPending: "待处理",
	PaymentEventSuccess: "成功",
	PaymentEventFailed:  "失败",
	PaymentEventIgnored: "已忽略",
}

func IsPaymentProvider(value string) bool {
	for _, item := range PaymentProviders {
		if item == value {
			return true
		}
	}
	return false
}

func IsPaymentMethod(value string) bool {
	for _, item := range PaymentMethods {
		if item == value {
			return true
		}
	}
	return false
}

func IsPaymentOrderStatus(value int) bool {
	for _, item := range PaymentOrderStatuses {
		if item == value {
			return true
		}
	}
	return false
}

func IsPaymentEventType(value string) bool {
	for _, item := range PaymentEventTypes {
		if item == value {
			return true
		}
	}
	return false
}

func IsPaymentEventProcessStatus(value int) bool {
	for _, item := range PaymentEventProcessStatuses {
		if item == value {
			return true
		}
	}
	return false
}

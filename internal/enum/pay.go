package enum

import "strings"

const (
	PayChannelWechat = 1
	PayChannelAlipay = 2
)

const (
	PayMethodWeb  = "web"
	PayMethodH5   = "h5"
	PayMethodApp  = "app"
	PayMethodMini = "mini"
	PayMethodScan = "scan"
	PayMethodMP   = "mp"
)

var PayChannels = []int{
	PayChannelWechat,
	PayChannelAlipay,
}

var PayChannelLabels = map[int]string{
	PayChannelWechat: "微信支付",
	PayChannelAlipay: "支付宝",
}

var PayMethods = []string{
	PayMethodWeb,
	PayMethodH5,
	PayMethodApp,
	PayMethodMini,
	PayMethodScan,
	PayMethodMP,
}

var PayMethodLabels = map[string]string{
	PayMethodWeb:  "PC网页支付",
	PayMethodH5:   "H5支付",
	PayMethodApp:  "APP支付",
	PayMethodMini: "小程序支付",
	PayMethodScan: "扫码支付",
	PayMethodMP:   "公众号支付",
}

var PayChannelMethods = map[int][]string{
	PayChannelWechat: {
		PayMethodScan,
		PayMethodH5,
		PayMethodApp,
		PayMethodMini,
		PayMethodMP,
	},
	PayChannelAlipay: {
		PayMethodWeb,
		PayMethodH5,
		PayMethodApp,
		PayMethodScan,
		PayMethodMini,
	},
}

const (
	PayOrderRecharge = 1
	PayOrderConsume  = 2
	PayOrderGoods    = 3
)

const (
	PayStatusPending   = 1
	PayStatusPaying    = 2
	PayStatusPaid      = 3
	PayStatusClosed    = 4
	PayStatusException = 5
)

const (
	PayBizInit      = 1
	PayBizPending   = 2
	PayBizExecuting = 3
	PayBizSuccess   = 4
	PayBizFailed    = 5
	PayBizManual    = 6
)

const (
	PayTxnCreated = 1
	PayTxnWaiting = 2
	PayTxnSuccess = 3
	PayTxnFailed  = 4
	PayTxnClosed  = 5
)

var PayOrderTypes = []int{
	PayOrderRecharge,
	PayOrderConsume,
	PayOrderGoods,
}

var PayOrderTypeLabels = map[int]string{
	PayOrderRecharge: "充值",
	PayOrderConsume:  "消费",
	PayOrderGoods:    "商品购买",
}

var PayStatuses = []int{
	PayStatusPending,
	PayStatusPaying,
	PayStatusPaid,
	PayStatusClosed,
	PayStatusException,
}

var PayStatusLabels = map[int]string{
	PayStatusPending:   "待支付",
	PayStatusPaying:    "支付中",
	PayStatusPaid:      "已支付",
	PayStatusClosed:    "已关闭",
	PayStatusException: "支付异常",
}

var PayBizStatuses = []int{
	PayBizInit,
	PayBizPending,
	PayBizExecuting,
	PayBizSuccess,
	PayBizFailed,
	PayBizManual,
}

var PayBizStatusLabels = map[int]string{
	PayBizInit:      "初始化",
	PayBizPending:   "待履约",
	PayBizExecuting: "履约中",
	PayBizSuccess:   "履约成功",
	PayBizFailed:    "履约失败",
	PayBizManual:    "人工处理",
}

var RechargePresets = []OptionPreset{
	{Label: "10元", Value: 1000},
	{Label: "30元", Value: 3000},
	{Label: "50元", Value: 5000},
	{Label: "100元", Value: 10000},
	{Label: "300元", Value: 30000},
	{Label: "500元", Value: 50000},
}

var PayTxnStatuses = []int{
	PayTxnCreated,
	PayTxnWaiting,
	PayTxnSuccess,
	PayTxnFailed,
	PayTxnClosed,
}

var PayTxnStatusLabels = map[int]string{
	PayTxnCreated: "已创建",
	PayTxnWaiting: "等待支付",
	PayTxnSuccess: "支付成功",
	PayTxnFailed:  "支付失败",
	PayTxnClosed:  "已关闭",
}

type OptionPreset struct {
	Label string
	Value int
}

func IsPayChannel(value int) bool {
	for _, channel := range PayChannels {
		if channel == value {
			return true
		}
	}
	return false
}

func IsPayMethod(value string) bool {
	value = strings.TrimSpace(value)
	for _, method := range PayMethods {
		if method == value {
			return true
		}
	}
	return false
}

func IsPayTxnStatus(value int) bool {
	for _, status := range PayTxnStatuses {
		if status == value {
			return true
		}
	}
	return false
}

func IsPayOrderType(value int) bool {
	for _, orderType := range PayOrderTypes {
		if orderType == value {
			return true
		}
	}
	return false
}

func IsPayStatus(value int) bool {
	for _, status := range PayStatuses {
		if status == value {
			return true
		}
	}
	return false
}

func IsPayBizStatus(value int) bool {
	for _, status := range PayBizStatuses {
		if status == value {
			return true
		}
	}
	return false
}

func PayDefaultSupportedMethods(channel int) []string {
	methods := PayChannelMethods[channel]
	return append([]string(nil), methods...)
}

func NormalizePaySupportedMethods(channel int, methods []string) []string {
	allowedMethods := PayDefaultSupportedMethods(channel)
	if len(allowedMethods) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(allowedMethods))
	for _, method := range allowedMethods {
		allowed[method] = struct{}{}
	}

	seen := make(map[string]struct{}, len(methods))
	result := make([]string, 0, len(methods))
	for _, raw := range methods {
		method := strings.TrimSpace(raw)
		if method == "" {
			continue
		}
		if _, ok := allowed[method]; !ok {
			continue
		}
		if _, ok := seen[method]; ok {
			continue
		}
		seen[method] = struct{}{}
		result = append(result, method)
	}
	return result
}

func PaySupportedMethodsValid(channel int, methods []string) bool {
	if !IsPayChannel(channel) || len(methods) == 0 {
		return false
	}
	normalized := NormalizePaySupportedMethods(channel, methods)
	if len(normalized) == 0 {
		return false
	}

	validSubmitted := 0
	seen := make(map[string]struct{}, len(methods))
	for _, raw := range methods {
		method := strings.TrimSpace(raw)
		if method == "" {
			continue
		}
		if _, ok := seen[method]; ok {
			continue
		}
		seen[method] = struct{}{}
		validSubmitted++
	}
	return validSubmitted == len(normalized)
}

package enum

const (
	PaymentProviderAlipay = "alipay"

	PaymentMethodWeb = "web"
	PaymentMethodH5  = "h5"
)

var PaymentProviders = []string{PaymentProviderAlipay}
var PaymentProviderLabels = map[string]string{
	PaymentProviderAlipay: "支付宝",
}

var PaymentMethods = []string{PaymentMethodWeb, PaymentMethodH5}
var PaymentMethodLabels = map[string]string{
	PaymentMethodWeb: "PC网页支付",
	PaymentMethodH5:  "H5支付",
}

func IsPaymentMethod(value string) bool {
	for _, item := range PaymentMethods {
		if item == value {
			return true
		}
	}
	return false
}

func IsPaymentProvider(value string) bool {
	for _, item := range PaymentProviders {
		if item == value {
			return true
		}
	}
	return false
}

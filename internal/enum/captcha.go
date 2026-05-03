package enum

const CaptchaTypeSlide = "slide"

var CaptchaTypes = []string{CaptchaTypeSlide}

func IsCaptchaType(value string) bool {
	for _, item := range CaptchaTypes {
		if value == item {
			return true
		}
	}
	return false
}

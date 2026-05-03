package validate

import (
	"testing"

	"github.com/gin-gonic/gin/binding"
)

func TestRegisterAddsEnumBackedGinValidators(t *testing.T) {
	if err := Register(); err != nil {
		t.Fatalf("register validators: %v", err)
	}

	type request struct {
		Status      int    `binding:"required,common_status"`
		Platform    string `binding:"required,platform_scope"`
		Code        string `binding:"required,platform_code"`
		Type        int    `binding:"required,permission_type"`
		LoginType   string `binding:"required,auth_platform_login_type"`
		CaptchaType string `binding:"required,captcha_type"`
	}

	valid := request{
		Status:      1,
		Platform:    "admin",
		Code:        "mini_app",
		Type:        2,
		LoginType:   "password",
		CaptchaType: "slide",
	}
	if err := binding.Validator.ValidateStruct(valid); err != nil {
		t.Fatalf("expected valid request, got %v", err)
	}

	invalid := valid
	invalid.CaptchaType = "click"
	if err := binding.Validator.ValidateStruct(invalid); err == nil {
		t.Fatalf("expected invalid captcha_type to fail")
	}
}

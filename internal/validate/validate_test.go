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
		Scene       string `binding:"required,verify_code_scene"`
		Sex         int    `binding:"user_sex"`
		LogLevel    string `binding:"required,log_level"`
		ValueType   int    `binding:"required,system_setting_value_type"`
	}

	valid := request{
		Status:      1,
		Platform:    "admin",
		Code:        "mini_app",
		Type:        2,
		LoginType:   "password",
		CaptchaType: "slide",
		Scene:       "login",
		Sex:         1,
		LogLevel:    "ERROR",
		ValueType:   4,
	}
	if err := binding.Validator.ValidateStruct(valid); err != nil {
		t.Fatalf("expected valid request, got %v", err)
	}

	invalid := valid
	invalid.CaptchaType = "click"
	if err := binding.Validator.ValidateStruct(invalid); err == nil {
		t.Fatalf("expected invalid captcha_type to fail")
	}

	invalid = valid
	invalid.Sex = 9
	if err := binding.Validator.ValidateStruct(invalid); err == nil {
		t.Fatalf("expected invalid user sex to fail")
	}

	invalid = valid
	invalid.LogLevel = "TRACE"
	if err := binding.Validator.ValidateStruct(invalid); err == nil {
		t.Fatalf("expected invalid log level to fail")
	}

	invalid = valid
	invalid.ValueType = 9
	if err := binding.Validator.ValidateStruct(invalid); err == nil {
		t.Fatalf("expected invalid system setting value type to fail")
	}
}

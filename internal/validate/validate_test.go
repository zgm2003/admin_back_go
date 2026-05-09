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
		Status         int    `binding:"required,common_status"`
		Platform       string `binding:"required,platform_scope"`
		Code           string `binding:"required,platform_code"`
		Type           int    `binding:"required,permission_type"`
		LoginType      string `binding:"required,auth_platform_login_type"`
		CaptchaType    string `binding:"required,captcha_type"`
		Scene          string `binding:"required,verify_code_scene"`
		Sex            int    `binding:"user_sex"`
		LogLevel       string `binding:"required,log_level"`
		ValueType      int    `binding:"required,system_setting_value_type"`
		Driver         string `binding:"required,upload_driver"`
		ImageExt       string `binding:"required,upload_image_ext"`
		FileExt        string `binding:"required,upload_file_ext"`
		Folder         string `binding:"required,upload_folder"`
		VerifyType     string `binding:"required,user_verify_type"`
		NotifyType     int    `binding:"required,notification_type"`
		NotifyLevel    int    `binding:"required,notification_level"`
		TargetType     int    `binding:"required,notification_target_type"`
		TaskStatus     int    `binding:"required,notification_task_status"`
		TaskPlatform   string `binding:"required,notification_task_platform"`
		ClientPlatform string `binding:"required,client_platform"`
	}

	valid := request{
		Status:         1,
		Platform:       "admin",
		Code:           "mini_app",
		Type:           2,
		LoginType:      "password",
		CaptchaType:    "slide",
		Scene:          "login",
		Sex:            1,
		LogLevel:       "ERROR",
		ValueType:      4,
		Driver:         "cos",
		ImageExt:       "png",
		FileExt:        "pdf",
		Folder:         "ai-agents",
		VerifyType:     "password",
		NotifyType:     1,
		NotifyLevel:    2,
		TargetType:     3,
		TaskStatus:     4,
		TaskPlatform:   "all",
		ClientPlatform: "windows-x86_64",
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

	invalid = valid
	invalid.Folder = "private"
	if err := binding.Validator.ValidateStruct(invalid); err == nil {
		t.Fatalf("expected invalid upload folder to fail")
	}

	invalid = valid
	invalid.VerifyType = "totp"
	if err := binding.Validator.ValidateStruct(invalid); err == nil {
		t.Fatalf("expected invalid user verify type to fail")
	}

	invalid = valid
	invalid.NotifyType = 9
	if err := binding.Validator.ValidateStruct(invalid); err == nil {
		t.Fatalf("expected invalid notification type to fail")
	}

	invalid = valid
	invalid.NotifyLevel = 9
	if err := binding.Validator.ValidateStruct(invalid); err == nil {
		t.Fatalf("expected invalid notification level to fail")
	}

	invalid = valid
	invalid.TargetType = 9
	if err := binding.Validator.ValidateStruct(invalid); err == nil {
		t.Fatalf("expected invalid notification target type to fail")
	}

	invalid = valid
	invalid.TaskStatus = 9
	if err := binding.Validator.ValidateStruct(invalid); err == nil {
		t.Fatalf("expected invalid notification task status to fail")
	}

	invalid = valid
	invalid.TaskPlatform = "mini"
	if err := binding.Validator.ValidateStruct(invalid); err == nil {
		t.Fatalf("expected invalid notification task platform to fail")
	}

	invalid = valid
	invalid.ClientPlatform = "linux-x86_64"
	if err := binding.Validator.ValidateStruct(invalid); err == nil {
		t.Fatalf("expected invalid client platform to fail")
	}
}

func TestRegisterRejectsInvalidUploadValidators(t *testing.T) {
	if err := Register(); err != nil {
		t.Fatalf("register validators: %v", err)
	}

	type request struct {
		Driver   string `binding:"required,upload_driver"`
		ImageExt string `binding:"required,upload_image_ext"`
		FileExt  string `binding:"required,upload_file_ext"`
		Folder   string `binding:"required,upload_folder"`
	}

	valid := request{Driver: "oss", ImageExt: "webp", FileExt: "xlsx", Folder: "avatars"}
	if err := binding.Validator.ValidateStruct(valid); err != nil {
		t.Fatalf("expected valid upload request, got %v", err)
	}

	invalid := valid
	invalid.Driver = "s3"
	if err := binding.Validator.ValidateStruct(invalid); err == nil {
		t.Fatalf("expected invalid upload driver to fail")
	}

	invalid = valid
	invalid.ImageExt = "exe"
	if err := binding.Validator.ValidateStruct(invalid); err == nil {
		t.Fatalf("expected invalid image ext to fail")
	}

	invalid = valid
	invalid.FileExt = "php"
	if err := binding.Validator.ValidateStruct(invalid); err == nil {
		t.Fatalf("expected invalid file ext to fail")
	}

	invalid = valid
	invalid.Folder = "tmp"
	if err := binding.Validator.ValidateStruct(invalid); err == nil {
		t.Fatalf("expected invalid upload folder to fail")
	}
}

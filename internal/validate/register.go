package validate

import (
	"fmt"
	"sync"

	"github.com/gin-gonic/gin/binding"
	playground "github.com/go-playground/validator/v10"
)

var (
	registerOnce sync.Once
	registerErr  error
)

// Register installs project-owned validation tags into Gin's validator engine.
func Register() error {
	registerOnce.Do(func() {
		validatorEngine, ok := binding.Validator.Engine().(*playground.Validate)
		if !ok {
			registerErr = fmt.Errorf("gin binding validator engine is not go-playground validator")
			return
		}

		validators := map[string]playground.Func{
			"common_yes_no":             validateCommonYesNo,
			"common_status":             validateCommonStatus,
			"platform_scope":            validatePlatformScope,
			"platform_code":             validatePlatformCode,
			"permission_type":           validatePermissionType,
			"auth_platform_login_type":  validateAuthPlatformLoginType,
			"captcha_type":              validateCaptchaType,
			"verify_code_scene":         validateVerifyCodeScene,
			"user_sex":                  validateUserSex,
			"user_verify_type":          validateUserVerifyType,
			"log_level":                 validateLogLevel,
			"system_setting_value_type": validateSystemSettingValueType,
			"upload_driver":             validateUploadDriver,
			"upload_image_ext":          validateUploadImageExt,
			"upload_file_ext":           validateUploadFileExt,
			"upload_folder":             validateUploadFolder,
		}
		for tag, fn := range validators {
			if err := validatorEngine.RegisterValidation(tag, fn); err != nil {
				registerErr = err
				return
			}
		}
	})
	return registerErr
}

// MustRegister installs validators and panics only during application bootstrap.
func MustRegister() {
	if err := Register(); err != nil {
		panic(err)
	}
}

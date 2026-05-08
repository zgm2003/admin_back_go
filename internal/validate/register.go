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
			"common_yes_no":                validateCommonYesNo,
			"common_status":                validateCommonStatus,
			"platform_scope":               validatePlatformScope,
			"platform_code":                validatePlatformCode,
			"permission_type":              validatePermissionType,
			"auth_platform_login_type":     validateAuthPlatformLoginType,
			"captcha_type":                 validateCaptchaType,
			"verify_code_scene":            validateVerifyCodeScene,
			"user_sex":                     validateUserSex,
			"user_verify_type":             validateUserVerifyType,
			"log_level":                    validateLogLevel,
			"system_setting_value_type":    validateSystemSettingValueType,
			"upload_driver":                validateUploadDriver,
			"upload_image_ext":             validateUploadImageExt,
			"upload_file_ext":              validateUploadFileExt,
			"upload_folder":                validateUploadFolder,
			"client_platform":              validateClientPlatform,
			"notification_type":            validateNotificationType,
			"notification_level":           validateNotificationLevel,
			"notification_target_type":     validateNotificationTargetType,
			"notification_task_status":     validateNotificationTaskStatus,
			"notification_task_platform":   validateNotificationTaskPlatform,
			"pay_channel":                  validatePayChannel,
			"pay_method":                   validatePayMethod,
			"pay_notify_type":              validatePayNotifyType,
			"pay_notify_process_status":    validatePayNotifyProcessStatus,
			"pay_txn_status":               validatePayTxnStatus,
			"pay_order_type":               validatePayOrderType,
			"pay_status":                   validatePayStatus,
			"pay_biz_status":               validatePayBizStatus,
			"wallet_type":                  validateWalletType,
			"wallet_source":                validateWalletSource,
			"pay_reconcile_status":         validatePayReconcileStatus,
			"pay_reconcile_bill_type":      validatePayReconcileBillType,
			"payment_provider":             validatePaymentProvider,
			"payment_method":               validatePaymentMethod,
			"payment_order_status":         validatePaymentOrderStatus,
			"payment_event_type":           validatePaymentEventType,
			"payment_event_process_status": validatePaymentEventProcessStatus,
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

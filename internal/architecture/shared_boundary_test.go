package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigratedDictSettingCallSitesUseSharedBoundaries(t *testing.T) {
	root := backendRoot(t)
	for _, check := range []struct {
		rel       string
		mustHave  []string
		mustAvoid []string
	}{
		{
			rel:       "internal/module/systemsetting/service.go",
			mustHave:  []string{"admin_back_go/internal/shared/dict", "shareddict.SystemSettingValueTypeOptions()"},
			mustAvoid: []string{"admin_back_go/internal/" + "dict"},
		},
		{
			rel:       "internal/module/auth/captcha.go",
			mustHave:  []string{"admin_back_go/internal/shared/setting", "sharedsetting.AuthCaptchaTTLMinutes(ctx, p.repository)"},
			mustAvoid: []string{"CaptchaTTLSettingKey = \"auth.captcha.ttl_minutes\"", "p.positiveIntSetting(ctx, CaptchaTTLSettingKey)"},
		},
		{
			rel:       "internal/module/uploadtoken/policy.go",
			mustHave:  []string{"admin_back_go/internal/shared/setting", "sharedsetting.UploadTokenTTLMinutes(ctx, p.repo)"},
			mustAvoid: []string{"admin_back_go/internal/module/systemsetting", "UploadTokenTTLSettingKey = \"upload.token.ttl_minutes\"", "p.repo.SettingByKey"},
		},
	} {
		t.Run(check.rel, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(check.rel)))
			if err != nil {
				t.Fatalf("read %s: %v", check.rel, err)
			}
			text := string(body)
			for _, want := range check.mustHave {
				if !strings.Contains(text, want) {
					t.Fatalf("%s must contain %q", check.rel, want)
				}
			}
			for _, banned := range check.mustAvoid {
				if strings.Contains(text, banned) {
					t.Fatalf("%s must not contain %q", check.rel, banned)
				}
			}
		})
	}
}

func TestVerifyCodeTTLDoesNotUseSystemSettingRuntime(t *testing.T) {
	root := backendRoot(t)
	files := []string{
		"internal/module/auth/verify_code_policy.go",
		"internal/module/mail/service.go",
		"internal/module/mail/repository.go",
		"internal/module/sms/service.go",
		"internal/module/sms/repository.go",
		"internal/shared/setting/setting.go",
	}
	banned := []string{
		"AuthVerifyCodeTTLKey",
		"AuthVerifyCodeTTLMinutes",
		"AuthVerifyCodeTTLMinutesOrDefault",
		"SaveAuthVerifyCodeTTLMinutes",
		"SystemSettingVerifyCodePolicyProvider",
		"auth.verify_code.ttl_minutes",
	}
	for _, rel := range files {
		t.Run(rel, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
			if err != nil {
				t.Fatalf("read %s: %v", rel, err)
			}
			text := string(body)
			for _, token := range banned {
				if strings.Contains(text, token) {
					t.Fatalf("%s must not contain verify-code system-setting runtime token %q", rel, token)
				}
			}
		})
	}
}

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
			mustAvoid: []string{"admin_back_go/internal/dict"},
		},
		{
			rel:       "internal/module/auth/captcha.go",
			mustHave:  []string{"admin_back_go/internal/shared/setting", "sharedsetting.AuthCaptchaTTLMinutes(ctx, p.repository)"},
			mustAvoid: []string{"CaptchaTTLSettingKey = \"auth.captcha.ttl_minutes\"", "p.positiveIntSetting(ctx, CaptchaTTLSettingKey)"},
		},
		{
			rel:       "internal/module/auth/verify_code_policy.go",
			mustHave:  []string{"admin_back_go/internal/shared/setting", "sharedsetting.AuthVerifyCodeTTLMinutes(ctx, p.repository)"},
			mustAvoid: []string{"admin_back_go/internal/module/systemsetting", "VerifyCodeTTLSettingKey = \"auth.verify_code.ttl_minutes\"", "p.repository.SettingByKey"},
		},
		{
			rel:       "internal/module/uploadtoken/policy.go",
			mustHave:  []string{"admin_back_go/internal/shared/setting", "sharedsetting.UploadTokenTTLMinutes(ctx, p.repo)"},
			mustAvoid: []string{"admin_back_go/internal/module/systemsetting", "UploadTokenTTLSettingKey = \"upload.token.ttl_minutes\"", "p.repo.SettingByKey"},
		},
		{
			rel:       "internal/module/mail/service.go",
			mustHave:  []string{"admin_back_go/internal/shared/setting", "sharedsetting.AuthVerifyCodeTTLMinutesOrDefault(ctx, repo)", "sharedsetting.SaveAuthVerifyCodeTTLMinutes(ctx, repo, ttl)"},
			mustAvoid: []string{"verifyCodeTTLSettingKey = \"auth.verify_code.ttl_minutes\"", "repo.SettingByKey(ctx, verifyCodeTTLSettingKey)", "repo.SaveSetting(ctx, systemsetting.Setting"},
		},
		{
			rel:       "internal/module/sms/service.go",
			mustHave:  []string{"admin_back_go/internal/shared/setting", "sharedsetting.AuthVerifyCodeTTLMinutesOrDefault(ctx, repo)", "sharedsetting.SaveAuthVerifyCodeTTLMinutes(ctx, repo, ttl)"},
			mustAvoid: []string{"verifyCodeTTLSettingKey = \"auth.verify_code.ttl_minutes\"", "repo.SettingByKey(ctx, verifyCodeTTLSettingKey)", "repo.SaveSetting(ctx, systemsetting.Setting"},
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

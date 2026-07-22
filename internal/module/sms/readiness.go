package sms

import (
	"context"
	"encoding/json"
	"strings"

	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/dict"
	"admin_back_go/internal/shared/enum"
)

func (s *Service) VerifyCodeReady(ctx context.Context, scene string) (bool, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil {
		return false, appErr
	}
	if !enum.IsSmsTemplateScene(scene) {
		return false, badRequest("sms.scene.invalid", "无效的短信模板场景")
	}
	if s.sender == nil {
		return false, nil
	}
	cfg, err := repo.DefaultConfig(ctx)
	if err != nil {
		return false, wrapInternal("sms.config.query_failed", "查询短信配置失败", err)
	}
	if !smsConfigReady(cfg) {
		return false, nil
	}
	tmpl, err := repo.TemplateByScene(ctx, scene)
	if err != nil {
		return false, wrapInternal("sms.template.query_failed", "查询短信模板失败", err)
	}
	return smsTemplateReady(tmpl), nil
}

func smsConfigReady(cfg *Config) bool {
	if cfg == nil || cfg.Status != enum.CommonYes || cfg.IsDel != enum.CommonNo {
		return false
	}
	if strings.TrimSpace(cfg.SecretIDEnc) == "" || strings.TrimSpace(cfg.SecretKeyEnc) == "" {
		return false
	}
	if strings.TrimSpace(cfg.SmsSdkAppID) == "" || strings.TrimSpace(cfg.SignName) == "" {
		return false
	}
	if !dict.IsSmsRegion(cfg.Region) || strings.TrimSpace(cfg.Endpoint) == "" {
		return false
	}
	return cfg.VerifyCodeTTLMinutes >= minVerifyCodeTTLMinutes && cfg.VerifyCodeTTLMinutes <= maxVerifyCodeTTLMinutes
}

func smsTemplateReady(tmpl *Template) bool {
	if tmpl == nil || tmpl.Status != enum.CommonYes || tmpl.IsDel != enum.CommonNo {
		return false
	}
	if strings.TrimSpace(tmpl.TencentTemplateID) == "" {
		return false
	}
	var variables []string
	if err := json.Unmarshal([]byte(tmpl.VariablesJSON), &variables); err != nil {
		return false
	}
	return len(variables) == 2 && variables[0] == "code" && variables[1] == "ttl_minutes"
}

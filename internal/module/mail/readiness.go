package mail

import (
	"context"
	"encoding/json"
	"net/http"
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
	if !enum.IsMailTemplateScene(scene) {
		return false, apperror.BadRequest("无效的邮件模板场景")
	}
	if s.sender == nil {
		return false, nil
	}
	cfg, err := repo.DefaultConfig(ctx)
	if err != nil {
		return false, apperror.LegacyWrap(apperror.CodeInternal, http.StatusInternalServerError, "查询邮件配置失败", err)
	}
	if !mailConfigReady(cfg) {
		return false, nil
	}
	tmpl, err := repo.TemplateByScene(ctx, scene)
	if err != nil {
		return false, apperror.LegacyWrap(apperror.CodeInternal, http.StatusInternalServerError, "查询邮件模板失败", err)
	}
	return mailTemplateReady(tmpl), nil
}

func mailConfigReady(cfg *Config) bool {
	if cfg == nil || cfg.Status != enum.CommonYes || cfg.IsDel != enum.CommonNo {
		return false
	}
	if strings.TrimSpace(cfg.SecretIDEnc) == "" || strings.TrimSpace(cfg.SecretKeyEnc) == "" {
		return false
	}
	if !dict.IsMailRegion(cfg.Region) || strings.TrimSpace(cfg.Endpoint) == "" || !isEmail(cfg.FromEmail) {
		return false
	}
	if replyTo := strings.TrimSpace(cfg.ReplyTo); replyTo != "" && !isEmail(replyTo) {
		return false
	}
	return cfg.VerifyCodeTTLMinutes >= minVerifyCodeTTLMinutes && cfg.VerifyCodeTTLMinutes <= maxVerifyCodeTTLMinutes
}

func mailTemplateReady(tmpl *Template) bool {
	if tmpl == nil || tmpl.Status != enum.CommonYes || tmpl.IsDel != enum.CommonNo {
		return false
	}
	if tmpl.TencentTemplateID == 0 || strings.TrimSpace(tmpl.Subject) == "" {
		return false
	}
	var variables []string
	err := json.Unmarshal([]byte(tmpl.VariablesJSON), &variables)
	if err != nil {
		return false
	}
	return len(variables) == 2 && variables[0] == "code" && variables[1] == "ttl_minutes"
}

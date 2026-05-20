package uploadtoken

import (
	"context"
	"strconv"
	"strings"
	"time"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/module/systemsetting"
)

const (
	UploadTokenTTLSettingKey = "upload.token.ttl_minutes"
	DefaultTTL               = 15 * time.Minute

	minTTLMinutes = 1
	maxTTLMinutes = 1440
)

type TTLPolicyProvider interface {
	TTL(ctx context.Context) time.Duration
}

type TTLPolicyRepository interface {
	SettingByKey(ctx context.Context, key string) (*systemsetting.Setting, error)
}

type systemSettingTTLPolicyProvider struct {
	repo TTLPolicyRepository
}

func NewSystemSettingTTLPolicyProvider(repo TTLPolicyRepository) TTLPolicyProvider {
	return systemSettingTTLPolicyProvider{repo: repo}
}

func (p systemSettingTTLPolicyProvider) TTL(ctx context.Context) time.Duration {
	if p.repo == nil {
		return DefaultTTL
	}
	setting, err := p.repo.SettingByKey(ctx, UploadTokenTTLSettingKey)
	if err != nil || setting == nil {
		return DefaultTTL
	}
	if setting.IsDel != enum.CommonNo || setting.Status != enum.CommonYes || setting.ValueType != enum.SystemSettingValueNumber {
		return DefaultTTL
	}
	minutes, err := strconv.Atoi(strings.TrimSpace(setting.SettingValue))
	if err != nil || minutes < minTTLMinutes || minutes > maxTTLMinutes {
		return DefaultTTL
	}
	return time.Duration(minutes) * time.Minute
}

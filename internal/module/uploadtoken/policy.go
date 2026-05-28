package uploadtoken

import (
	"context"
	"time"

	sharedsetting "admin_back_go/internal/shared/setting"
)

const (
	DefaultTTL = sharedsetting.DefaultUploadTokenTTL
)

type TTLPolicyProvider interface {
	TTL(ctx context.Context) time.Duration
}

type TTLPolicyRepository interface {
	sharedsetting.Reader
}

type systemSettingTTLPolicyProvider struct {
	repo TTLPolicyRepository
}

func NewSystemSettingTTLPolicyProvider(repo TTLPolicyRepository) TTLPolicyProvider {
	return systemSettingTTLPolicyProvider{repo: repo}
}

func (p systemSettingTTLPolicyProvider) TTL(ctx context.Context) time.Duration {
	minutes := sharedsetting.UploadTokenTTLMinutes(ctx, p.repo)
	return time.Duration(minutes) * time.Minute
}

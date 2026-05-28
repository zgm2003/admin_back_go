package uploadtoken

import (
	"context"
	"errors"
	"testing"
	"time"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/module/systemsetting"
	sharedsetting "admin_back_go/internal/shared/setting"
)

type fakeTTLPolicyRepository struct {
	setting *systemsetting.Setting
	err     error
	key     string
}

func (f *fakeTTLPolicyRepository) SettingByKey(ctx context.Context, key string) (*systemsetting.Setting, error) {
	f.key = key
	if f.err != nil {
		return nil, f.err
	}
	return f.setting, nil
}

func TestSystemSettingTTLPolicyProviderReturnsConfiguredTTL(t *testing.T) {
	repo := &fakeTTLPolicyRepository{setting: &systemsetting.Setting{SettingValue: "20", ValueType: enum.SystemSettingValueNumber, Status: enum.CommonYes, IsDel: enum.CommonNo}}
	provider := NewSystemSettingTTLPolicyProvider(repo)

	got := provider.TTL(context.Background())

	if got != 20*time.Minute {
		t.Fatalf("expected 20m, got %s", got)
	}
	if repo.key != sharedsetting.UploadTokenTTLKey {
		t.Fatalf("expected key %s, got %s", sharedsetting.UploadTokenTTLKey, repo.key)
	}
}

func TestSystemSettingTTLPolicyProviderFallsBackToDefault(t *testing.T) {
	tests := []struct {
		name string
		repo TTLPolicyRepository
	}{
		{name: "nil repo", repo: nil},
		{name: "missing", repo: &fakeTTLPolicyRepository{}},
		{name: "deleted", repo: &fakeTTLPolicyRepository{setting: &systemsetting.Setting{SettingValue: "20", ValueType: enum.SystemSettingValueNumber, Status: enum.CommonYes, IsDel: enum.CommonYes}}},
		{name: "disabled", repo: &fakeTTLPolicyRepository{setting: &systemsetting.Setting{SettingValue: "20", ValueType: enum.SystemSettingValueNumber, Status: enum.CommonNo, IsDel: enum.CommonNo}}},
		{name: "wrong type", repo: &fakeTTLPolicyRepository{setting: &systemsetting.Setting{SettingValue: "20", ValueType: enum.SystemSettingValueString, Status: enum.CommonYes, IsDel: enum.CommonNo}}},
		{name: "parse fail", repo: &fakeTTLPolicyRepository{setting: &systemsetting.Setting{SettingValue: "abc", ValueType: enum.SystemSettingValueNumber, Status: enum.CommonYes, IsDel: enum.CommonNo}}},
		{name: "zero", repo: &fakeTTLPolicyRepository{setting: &systemsetting.Setting{SettingValue: "0", ValueType: enum.SystemSettingValueNumber, Status: enum.CommonYes, IsDel: enum.CommonNo}}},
		{name: "negative", repo: &fakeTTLPolicyRepository{setting: &systemsetting.Setting{SettingValue: "-1", ValueType: enum.SystemSettingValueNumber, Status: enum.CommonYes, IsDel: enum.CommonNo}}},
		{name: "too large", repo: &fakeTTLPolicyRepository{setting: &systemsetting.Setting{SettingValue: "1441", ValueType: enum.SystemSettingValueNumber, Status: enum.CommonYes, IsDel: enum.CommonNo}}},
		{name: "repo error", repo: &fakeTTLPolicyRepository{err: errors.New("boom")}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewSystemSettingTTLPolicyProvider(tt.repo)
			if got := provider.TTL(context.Background()); got != DefaultTTL {
				t.Fatalf("expected default %s, got %s", DefaultTTL, got)
			}
		})
	}
}

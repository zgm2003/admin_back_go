package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/module/systemsetting"
)

type fakeVerifyCodePolicyRepository struct {
	row *systemsetting.Setting
	err error
}

func (f fakeVerifyCodePolicyRepository) SettingByKey(ctx context.Context, key string) (*systemsetting.Setting, error) {
	return f.row, f.err
}

func TestSystemSettingVerifyCodePolicyProviderTTL(t *testing.T) {
	tests := []struct {
		name    string
		row     *systemsetting.Setting
		want    time.Duration
		wantErr string
	}{
		{
			name: "enabled",
			row: &systemsetting.Setting{
				SettingKey:   VerifyCodeTTLSettingKey,
				SettingValue: "7",
				ValueType:    enum.SystemSettingValueNumber,
				Status:       enum.CommonYes,
				IsDel:        enum.CommonNo,
			},
			want: 7 * time.Minute,
		},
		{name: "missing", row: nil, wantErr: "验证码有效期配置缺失"},
		{
			name: "disabled",
			row: &systemsetting.Setting{
				SettingKey:   VerifyCodeTTLSettingKey,
				SettingValue: "5",
				ValueType:    enum.SystemSettingValueNumber,
				Status:       enum.CommonNo,
				IsDel:        enum.CommonNo,
			},
			wantErr: "验证码有效期配置已禁用",
		},
		{
			name: "wrong type",
			row: &systemsetting.Setting{
				SettingKey:   VerifyCodeTTLSettingKey,
				SettingValue: "5",
				ValueType:    enum.SystemSettingValueString,
				Status:       enum.CommonYes,
				IsDel:        enum.CommonNo,
			},
			wantErr: "验证码有效期配置类型必须为数字",
		},
		{
			name: "zero",
			row: &systemsetting.Setting{
				SettingKey:   VerifyCodeTTLSettingKey,
				SettingValue: "0",
				ValueType:    enum.SystemSettingValueNumber,
				Status:       enum.CommonYes,
				IsDel:        enum.CommonNo,
			},
			wantErr: "验证码有效期必须在 1-60 分钟之间",
		},
		{
			name: "too large",
			row: &systemsetting.Setting{
				SettingKey:   VerifyCodeTTLSettingKey,
				SettingValue: "61",
				ValueType:    enum.SystemSettingValueNumber,
				Status:       enum.CommonYes,
				IsDel:        enum.CommonNo,
			},
			wantErr: "验证码有效期必须在 1-60 分钟之间",
		},
		{
			name: "decimal",
			row: &systemsetting.Setting{
				SettingKey:   VerifyCodeTTLSettingKey,
				SettingValue: "1.5",
				ValueType:    enum.SystemSettingValueNumber,
				Status:       enum.CommonYes,
				IsDel:        enum.CommonNo,
			},
			wantErr: "验证码有效期必须为整数分钟",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewSystemSettingVerifyCodePolicyProvider(fakeVerifyCodePolicyRepository{row: tt.row})
			got, appErr := provider.VerifyCodeTTL(context.Background())
			if tt.wantErr != "" {
				if appErr == nil || appErr.Message != tt.wantErr {
					t.Fatalf("want %q got %#v", tt.wantErr, appErr)
				}
				return
			}
			if appErr != nil || got != tt.want {
				t.Fatalf("ttl=%s err=%#v", got, appErr)
			}
		})
	}
}

func TestSystemSettingVerifyCodePolicyProviderWrapsRepositoryError(t *testing.T) {
	provider := NewSystemSettingVerifyCodePolicyProvider(fakeVerifyCodePolicyRepository{err: errors.New("db down")})
	_, appErr := provider.VerifyCodeTTL(context.Background())
	if appErr == nil || appErr.Message != "查询验证码有效期配置失败" {
		t.Fatalf("got %#v", appErr)
	}
}

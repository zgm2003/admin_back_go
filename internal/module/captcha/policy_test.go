package captcha

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/module/systemsetting"
)

type fakeCaptchaPolicyRepository struct {
	rows map[string]*systemsetting.Setting
	err  error
	keys []string
}

func (f *fakeCaptchaPolicyRepository) SettingByKey(ctx context.Context, key string) (*systemsetting.Setting, error) {
	f.keys = append(f.keys, key)
	if f.err != nil {
		return nil, f.err
	}
	return f.rows[key], nil
}

func TestSystemSettingCaptchaPolicyProviderReadsTTLAndPadding(t *testing.T) {
	repo := &fakeCaptchaPolicyRepository{rows: map[string]*systemsetting.Setting{
		CaptchaTTLSettingKey: {
			SettingKey:   CaptchaTTLSettingKey,
			SettingValue: "5",
			ValueType:    enum.SystemSettingValueNumber,
			Status:       enum.CommonYes,
			IsDel:        enum.CommonNo,
		},
		CaptchaSlidePaddingSettingKey: {
			SettingKey:   CaptchaSlidePaddingSettingKey,
			SettingValue: "12",
			ValueType:    enum.SystemSettingValueNumber,
			Status:       enum.CommonYes,
			IsDel:        enum.CommonNo,
		},
	}}
	provider := NewSystemSettingCaptchaPolicyProvider(repo)

	ttl, appErr := provider.TTL(context.Background())
	if appErr != nil {
		t.Fatalf("expected TTL read to succeed, got %v", appErr)
	}
	padding, appErr := provider.SlidePadding(context.Background())
	if appErr != nil {
		t.Fatalf("expected slide padding read to succeed, got %v", appErr)
	}

	if ttl != 5*time.Minute {
		t.Fatalf("expected ttl 5m from system setting, got %s", ttl)
	}
	if padding != 12 {
		t.Fatalf("expected padding 12 from system setting, got %d", padding)
	}
	wantKeys := []string{CaptchaTTLSettingKey, CaptchaSlidePaddingSettingKey}
	if !reflect.DeepEqual(repo.keys, wantKeys) {
		t.Fatalf("expected SettingByKey keys %#v, got %#v", wantKeys, repo.keys)
	}
}

func TestSystemSettingCaptchaPolicyProviderAllowsZeroSlidePadding(t *testing.T) {
	provider := NewSystemSettingCaptchaPolicyProvider(&fakeCaptchaPolicyRepository{rows: map[string]*systemsetting.Setting{
		CaptchaSlidePaddingSettingKey: validCaptchaSetting(CaptchaSlidePaddingSettingKey, "0", enum.CommonYes),
	}})

	padding, appErr := provider.SlidePadding(context.Background())

	if appErr != nil {
		t.Fatalf("expected zero slide padding to stay valid, got %v", appErr)
	}
	if padding != 0 {
		t.Fatalf("expected zero slide padding, got %d", padding)
	}
}

func TestSystemSettingCaptchaPolicyProviderFailsClosedForInvalidSettings(t *testing.T) {
	cases := []struct {
		name string
		rows map[string]*systemsetting.Setting
	}{
		{name: "missing", rows: map[string]*systemsetting.Setting{}},
		{name: "disabled", rows: map[string]*systemsetting.Setting{
			CaptchaTTLSettingKey: validCaptchaSetting(CaptchaTTLSettingKey, "2", enum.CommonNo),
		}},
		{name: "wrong type", rows: map[string]*systemsetting.Setting{
			CaptchaTTLSettingKey: {
				SettingKey:   CaptchaTTLSettingKey,
				SettingValue: "2",
				ValueType:    enum.SystemSettingValueString,
				Status:       enum.CommonYes,
				IsDel:        enum.CommonNo,
			},
		}},
		{name: "not integer", rows: map[string]*systemsetting.Setting{
			CaptchaTTLSettingKey: validCaptchaSetting(CaptchaTTLSettingKey, "1.5", enum.CommonYes),
		}},
		{name: "non positive", rows: map[string]*systemsetting.Setting{
			CaptchaTTLSettingKey: validCaptchaSetting(CaptchaTTLSettingKey, "0", enum.CommonYes),
		}},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewSystemSettingCaptchaPolicyProvider(&fakeCaptchaPolicyRepository{rows: tt.rows})

			ttl, appErr := provider.TTL(context.Background())

			if appErr == nil {
				t.Fatalf("expected invalid setting to fail closed")
			}
			if ttl != 0 {
				t.Fatalf("expected zero ttl on failure, got %s", ttl)
			}
		})
	}
}

func TestSystemSettingCaptchaPolicyProviderWrapsRepositoryErrors(t *testing.T) {
	repoErr := errors.New("db down")
	provider := NewSystemSettingCaptchaPolicyProvider(&fakeCaptchaPolicyRepository{err: repoErr})

	_, appErr := provider.TTL(context.Background())

	if appErr == nil || appErr.Code != apperror.CodeInternal || !errors.Is(appErr, repoErr) {
		t.Fatalf("expected wrapped repository error, got %#v", appErr)
	}
	if appErr.MessageID != "captcha.policy.query_failed" {
		t.Fatalf("expected keyed query error, got %#v", appErr)
	}
}

func validCaptchaSetting(key string, value string, status int) *systemsetting.Setting {
	return &systemsetting.Setting{
		SettingKey:   key,
		SettingValue: value,
		ValueType:    enum.SystemSettingValueNumber,
		Status:       status,
		IsDel:        enum.CommonNo,
	}
}

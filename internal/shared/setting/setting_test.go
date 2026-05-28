package setting

import (
	"context"
	"errors"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/module/systemsetting"
)

type fakeRepository struct {
	rows        map[string]*systemsetting.Setting
	err         error
	saved       *systemsetting.Setting
	invalidated []string
}

func (f *fakeRepository) SettingByKey(ctx context.Context, key string) (*systemsetting.Setting, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.rows[key], nil
}

func (f *fakeRepository) SaveSetting(ctx context.Context, row systemsetting.Setting) error {
	f.saved = &row
	return f.err
}

func (f *fakeRepository) InvalidateSettingCache(ctx context.Context, key string) error {
	f.invalidated = append(f.invalidated, key)
	return f.err
}

func TestAuthCaptchaTTLMinutesPreservesRequiredPositiveNumberPolicy(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepository{rows: map[string]*systemsetting.Setting{
		AuthCaptchaTTLKey: numberSetting(AuthCaptchaTTLKey, "3", enum.CommonYes),
	}}

	got, appErr := AuthCaptchaTTLMinutes(ctx, repo)
	if appErr != nil {
		t.Fatalf("expected valid captcha ttl, got %v", appErr)
	}
	if got != 3 {
		t.Fatalf("expected 3 minutes, got %d", got)
	}

	for _, tt := range []struct {
		name string
		row  *systemsetting.Setting
		code int
	}{
		{name: "missing", row: nil, code: apperror.CodeInternal},
		{name: "disabled", row: numberSetting(AuthCaptchaTTLKey, "3", enum.CommonNo), code: apperror.CodeBadRequest},
		{name: "wrong type", row: &systemsetting.Setting{SettingKey: AuthCaptchaTTLKey, SettingValue: "3", ValueType: enum.SystemSettingValueString, Status: enum.CommonYes, IsDel: enum.CommonNo}, code: apperror.CodeInternal},
		{name: "not integer", row: numberSetting(AuthCaptchaTTLKey, "1.5", enum.CommonYes), code: apperror.CodeBadRequest},
		{name: "zero", row: numberSetting(AuthCaptchaTTLKey, "0", enum.CommonYes), code: apperror.CodeBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, appErr := AuthCaptchaTTLMinutes(ctx, &fakeRepository{rows: map[string]*systemsetting.Setting{AuthCaptchaTTLKey: tt.row}})
			if appErr == nil || appErr.Code != tt.code {
				t.Fatalf("expected code %d, got %#v", tt.code, appErr)
			}
		})
	}
}

func TestAuthVerifyCodeTTLMinutesPreservesRequiredRangePolicy(t *testing.T) {
	ctx := context.Background()

	got, appErr := AuthVerifyCodeTTLMinutes(ctx, &fakeRepository{rows: map[string]*systemsetting.Setting{
		AuthVerifyCodeTTLKey: numberSetting(AuthVerifyCodeTTLKey, "11", enum.CommonYes),
	}})
	if appErr != nil || got != 11 {
		t.Fatalf("expected configured ttl 11, got ttl=%d err=%v", got, appErr)
	}

	for _, tt := range []struct {
		name string
		row  *systemsetting.Setting
		code int
	}{
		{name: "missing", row: nil, code: apperror.CodeInternal},
		{name: "deleted", row: &systemsetting.Setting{SettingKey: AuthVerifyCodeTTLKey, SettingValue: "11", ValueType: enum.SystemSettingValueNumber, Status: enum.CommonYes, IsDel: enum.CommonYes}, code: apperror.CodeInternal},
		{name: "disabled", row: numberSetting(AuthVerifyCodeTTLKey, "11", enum.CommonNo), code: apperror.CodeBadRequest},
		{name: "wrong type", row: &systemsetting.Setting{SettingKey: AuthVerifyCodeTTLKey, SettingValue: "11", ValueType: enum.SystemSettingValueString, Status: enum.CommonYes, IsDel: enum.CommonNo}, code: apperror.CodeInternal},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, appErr := AuthVerifyCodeTTLMinutes(ctx, &fakeRepository{rows: map[string]*systemsetting.Setting{AuthVerifyCodeTTLKey: tt.row}})
			if appErr == nil || appErr.Code != tt.code || got != 0 {
				t.Fatalf("expected code %d and zero ttl, got ttl=%d err=%#v", tt.code, got, appErr)
			}
		})
	}

	for _, tt := range []struct {
		name string
		row  *systemsetting.Setting
	}{
		{name: "not integer", row: numberSetting(AuthVerifyCodeTTLKey, "abc", enum.CommonYes)},
		{name: "too small", row: numberSetting(AuthVerifyCodeTTLKey, "0", enum.CommonYes)},
		{name: "too large", row: numberSetting(AuthVerifyCodeTTLKey, "61", enum.CommonYes)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, appErr := AuthVerifyCodeTTLMinutes(ctx, &fakeRepository{rows: map[string]*systemsetting.Setting{AuthVerifyCodeTTLKey: tt.row}})
			if appErr == nil || appErr.Code != apperror.CodeBadRequest {
				t.Fatalf("expected bad request, got %#v", appErr)
			}
		})
	}
}

func TestAuthVerifyCodeTTLMinutesOrDefaultPreservesConfigPageFallbackPolicy(t *testing.T) {
	ctx := context.Background()

	for _, tt := range []struct {
		name string
		row  *systemsetting.Setting
	}{
		{name: "missing", row: nil},
		{name: "deleted", row: &systemsetting.Setting{SettingKey: AuthVerifyCodeTTLKey, SettingValue: "11", ValueType: enum.SystemSettingValueNumber, Status: enum.CommonYes, IsDel: enum.CommonYes}},
		{name: "disabled", row: numberSetting(AuthVerifyCodeTTLKey, "11", enum.CommonNo)},
		{name: "empty", row: numberSetting(AuthVerifyCodeTTLKey, " ", enum.CommonYes)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, appErr := AuthVerifyCodeTTLMinutesOrDefault(ctx, &fakeRepository{rows: map[string]*systemsetting.Setting{AuthVerifyCodeTTLKey: tt.row}})
			if appErr != nil || got != DefaultAuthVerifyCodeTTLMinutes {
				t.Fatalf("expected default ttl %d, got ttl=%d err=%v", DefaultAuthVerifyCodeTTLMinutes, got, appErr)
			}
		})
	}
}

func TestUploadTokenTTLMinutesPreservesFallbackPolicy(t *testing.T) {
	ctx := context.Background()

	got := UploadTokenTTLMinutes(ctx, &fakeRepository{rows: map[string]*systemsetting.Setting{
		UploadTokenTTLKey: numberSetting(UploadTokenTTLKey, "20", enum.CommonYes),
	}})
	if got != 20 {
		t.Fatalf("expected configured ttl 20, got %d", got)
	}

	for _, tt := range []struct {
		name string
		row  *systemsetting.Setting
	}{
		{name: "missing", row: nil},
		{name: "deleted", row: &systemsetting.Setting{SettingKey: UploadTokenTTLKey, SettingValue: "20", ValueType: enum.SystemSettingValueNumber, Status: enum.CommonYes, IsDel: enum.CommonYes}},
		{name: "disabled", row: numberSetting(UploadTokenTTLKey, "20", enum.CommonNo)},
		{name: "wrong type", row: &systemsetting.Setting{SettingKey: UploadTokenTTLKey, SettingValue: "20", ValueType: enum.SystemSettingValueString, Status: enum.CommonYes, IsDel: enum.CommonNo}},
		{name: "not integer", row: numberSetting(UploadTokenTTLKey, "abc", enum.CommonYes)},
		{name: "too small", row: numberSetting(UploadTokenTTLKey, "0", enum.CommonYes)},
		{name: "too large", row: numberSetting(UploadTokenTTLKey, "1441", enum.CommonYes)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := UploadTokenTTLMinutes(ctx, &fakeRepository{rows: map[string]*systemsetting.Setting{UploadTokenTTLKey: tt.row}})
			if got != int(DefaultUploadTokenTTL/time.Minute) {
				t.Fatalf("expected default ttl minutes %d, got %d", int(DefaultUploadTokenTTL/time.Minute), got)
			}
		})
	}
}

func TestSaveAuthVerifyCodeTTLMinutesPersistsTypedSettingAndInvalidatesCache(t *testing.T) {
	repo := &fakeRepository{}

	appErr := SaveAuthVerifyCodeTTLMinutes(context.Background(), repo, 9)
	if appErr != nil {
		t.Fatalf("expected save to succeed, got %v", appErr)
	}
	if repo.saved == nil ||
		repo.saved.SettingKey != AuthVerifyCodeTTLKey ||
		repo.saved.SettingValue != "9" ||
		repo.saved.ValueType != enum.SystemSettingValueNumber ||
		repo.saved.Status != enum.CommonYes ||
		repo.saved.IsDel != enum.CommonNo {
		t.Fatalf("unexpected saved setting: %#v", repo.saved)
	}
	if len(repo.invalidated) != 1 || repo.invalidated[0] != AuthVerifyCodeTTLKey {
		t.Fatalf("expected auth verify code ttl cache invalidation, got %#v", repo.invalidated)
	}

	appErr = SaveAuthVerifyCodeTTLMinutes(context.Background(), repo, 61)
	if appErr == nil || appErr.Code != apperror.CodeBadRequest {
		t.Fatalf("expected invalid ttl to be rejected, got %#v", appErr)
	}
}

func TestSettingReaderWrapsRepositoryErrors(t *testing.T) {
	_, appErr := AuthVerifyCodeTTLMinutes(context.Background(), &fakeRepository{err: errors.New("db down")})
	if appErr == nil || appErr.Code != apperror.CodeInternal {
		t.Fatalf("expected repository error to be wrapped, got %#v", appErr)
	}
}

func numberSetting(key string, value string, status int) *systemsetting.Setting {
	return &systemsetting.Setting{
		SettingKey:   key,
		SettingValue: value,
		ValueType:    enum.SystemSettingValueNumber,
		Status:       status,
		IsDel:        enum.CommonNo,
	}
}

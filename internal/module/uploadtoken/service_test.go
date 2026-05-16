package uploadtoken

import (
	"context"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/secretbox"
	storagecos "admin_back_go/internal/platform/storage/cos"
)

type fakeRepository struct {
	config *EnabledConfig
	err    error
}

func (f fakeRepository) GetEnabledConfig(ctx context.Context) (*EnabledConfig, error) {
	return f.config, f.err
}

type fakeSigner struct {
	input storagecos.SignInput
	err   error
}

func (f *fakeSigner) Sign(ctx context.Context, input storagecos.SignInput) (*storagecos.Credentials, error) {
	f.input = input
	if f.err != nil {
		return nil, f.err
	}
	return &storagecos.Credentials{
		TmpSecretID:  "tmp-id",
		TmpSecretKey: "tmp-key",
		SessionToken: "token",
		StartTime:    100,
		ExpiredTime:  200,
	}, nil
}

func TestCreateRejectsMissingEnabledSetting(t *testing.T) {
	service := NewService(fakeRepository{}, secretbox.New([]byte("12345678901234567890123456789012")), &fakeSigner{}, Options{})

	_, appErr := service.Create(context.Background(), validInput())

	if appErr == nil || appErr.MessageID != "uploadtoken.setting_missing" {
		t.Fatalf("expected missing setting error, got %#v", appErr)
	}
}

func TestCreateRejectsNonCOSDriver(t *testing.T) {
	service := NewService(fakeRepository{config: validConfig(t, "oss")}, secretbox.New([]byte("12345678901234567890123456789012")), &fakeSigner{}, Options{})

	_, appErr := service.Create(context.Background(), validInput())

	if appErr == nil || appErr.MessageID != "uploadtoken.cos_runtime_disabled" {
		t.Fatalf("expected non COS error, got %#v", appErr)
	}
}

func TestCreateRejectsUnsupportedFolder(t *testing.T) {
	service := NewService(fakeRepository{config: validConfig(t, enum.UploadDriverCOS)}, secretbox.New([]byte("12345678901234567890123456789012")), &fakeSigner{}, Options{})

	_, appErr := service.Create(context.Background(), CreateInput{Folder: "bad", FileName: "a.png", FileSize: 1, FileKind: FileKindImage})

	if appErr == nil || appErr.MessageID != "uploadtoken.folder.unsupported" {
		t.Fatalf("expected folder error, got %#v", appErr)
	}
}

func TestCreateRejectsUnsupportedImageExtension(t *testing.T) {
	service := NewService(fakeRepository{config: validConfig(t, enum.UploadDriverCOS)}, secretbox.New([]byte("12345678901234567890123456789012")), &fakeSigner{}, Options{})

	_, appErr := service.Create(context.Background(), CreateInput{Folder: "images", FileName: "a.exe", FileSize: 1, FileKind: FileKindImage})

	if appErr == nil || appErr.MessageID != "uploadtoken.file_type.unsupported" {
		t.Fatalf("expected extension error, got %#v", appErr)
	}
}

func TestCreateRejectsOversizeFile(t *testing.T) {
	service := NewService(fakeRepository{config: validConfig(t, enum.UploadDriverCOS)}, secretbox.New([]byte("12345678901234567890123456789012")), &fakeSigner{}, Options{})

	_, appErr := service.Create(context.Background(), CreateInput{Folder: "images", FileName: "a.png", FileSize: 3 * 1024 * 1024, FileKind: FileKindImage})

	if appErr == nil || appErr.MessageID != "uploadtoken.file_size.exceeded" {
		t.Fatalf("expected oversize error, got %#v", appErr)
	}
}

func TestCreateBuildsSafeKeyAndSignsCOS(t *testing.T) {
	signer := &fakeSigner{}
	service := NewService(fakeRepository{config: validConfig(t, enum.UploadDriverCOS)}, secretbox.New([]byte("12345678901234567890123456789012")), signer, Options{
		TTL:         10 * time.Minute,
		RandomBytes: 4,
		Now:         func() time.Time { return time.Date(2026, 5, 5, 12, 30, 0, 0, time.Local) },
		Random:      func(b []byte) (int, error) { copy(b, []byte{0xa1, 0xb2, 0xc3, 0xd4}); return len(b), nil },
	})

	got, appErr := service.Create(context.Background(), CreateInput{Folder: "images", FileName: "../你 好.png", FileSize: 1024, FileKind: FileKindImage})

	if appErr != nil {
		t.Fatalf("unexpected error: %#v", appErr)
	}
	if got.Provider != ProviderCOS {
		t.Fatalf("expected cos provider, got %q", got.Provider)
	}
	if got.Key != "images/2026/05/05/1777955400000-a1b2c3d4-___.png" {
		t.Fatalf("unexpected key %q", got.Key)
	}
	if got.UploadPath != "images/2026/05/05/" {
		t.Fatalf("unexpected upload path %q", got.UploadPath)
	}
	if signer.input.SecretID != "sid-plain" || signer.input.SecretKey != "skey-plain" {
		t.Fatalf("unexpected signer secrets %#v", signer.input)
	}
	if signer.input.Key != got.Key || signer.input.TTL != 10*time.Minute {
		t.Fatalf("unexpected signer input %#v", signer.input)
	}
	if got.Credentials.TmpSecretID != "tmp-id" || got.Credentials.SessionToken != "token" {
		t.Fatalf("unexpected credentials %#v", got.Credentials)
	}
}

func TestCreateAcceptsAIAgentAvatarFolder(t *testing.T) {
	signer := &fakeSigner{}
	service := NewService(fakeRepository{config: validConfig(t, enum.UploadDriverCOS)}, secretbox.New([]byte("12345678901234567890123456789012")), signer, Options{
		Now:    func() time.Time { return time.Date(2026, 5, 9, 8, 0, 0, 0, time.UTC) },
		Random: func(b []byte) (int, error) { copy(b, []byte{0x01, 0x02, 0x03, 0x04}); return len(b), nil },
	})

	got, appErr := service.Create(context.Background(), CreateInput{Folder: "ai-agents", FileName: "avatar.jpg", FileSize: 1024, FileKind: FileKindImage})
	if appErr != nil {
		t.Fatalf("unexpected error: %#v", appErr)
	}
	if !strings.HasPrefix(got.Key, "ai-agents/2026/05/09/") || signer.input.Key != got.Key {
		t.Fatalf("unexpected ai agent avatar key: got=%#v signer=%#v", got, signer.input)
	}
}

func TestCreateDoesNotExposeDriverSecrets(t *testing.T) {
	service := NewService(fakeRepository{config: validConfig(t, enum.UploadDriverCOS)}, secretbox.New([]byte("12345678901234567890123456789012")), &fakeSigner{}, Options{})

	got, appErr := service.Create(context.Background(), validInput())

	if appErr != nil {
		t.Fatalf("unexpected error: %#v", appErr)
	}
	serialized := got.Credentials.TmpSecretID + got.Credentials.TmpSecretKey + got.Credentials.SessionToken + got.Key
	if strings.Contains(serialized, "sid-plain") || strings.Contains(serialized, "skey-plain") {
		t.Fatalf("response leaked driver secret: %#v", got)
	}
}

func TestCreateReturnsExplicitDisabledError(t *testing.T) {
	service := NewService(fakeRepository{config: validConfig(t, enum.UploadDriverCOS)}, secretbox.New([]byte("12345678901234567890123456789012")), storagecos.DisabledSigner{}, Options{})

	_, appErr := service.Create(context.Background(), validInput())

	if appErr == nil || appErr.MessageID != "uploadtoken.cos_sts_disabled" {
		t.Fatalf("expected disabled error, got %#v", appErr)
	}
}

func validInput() CreateInput {
	return CreateInput{Folder: "images", FileName: "demo.png", FileSize: 1024, FileKind: FileKindImage}
}

func validConfig(t *testing.T, driver string) *EnabledConfig {
	t.Helper()
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	secretID, err := box.Encrypt("sid-plain")
	if err != nil {
		t.Fatalf("encrypt secret id: %v", err)
	}
	secretKey, err := box.Encrypt("skey-plain")
	if err != nil {
		t.Fatalf("encrypt secret key: %v", err)
	}
	return &EnabledConfig{
		SettingID: 1, DriverID: 2, RuleID: 3, Driver: driver,
		SecretIDEnc: secretID, SecretKeyEnc: secretKey,
		Bucket: "bucket-a", Region: "ap-nanjing", AppID: "1314",
		MaxSizeMB: 2, ImageExts: `["png","jpg"]`, FileExts: `["pdf","txt"]`,
	}
}

package payment

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/enum"
	gateway "admin_back_go/internal/platform/payment"
)

func TestCreateConfigEncryptsPrivateKeyAndStoresHint(t *testing.T) {
	repo := newFakeConfigRepo()
	secret := &fakeSecretbox{}
	service := NewService(Dependencies{Repository: repo, Secretbox: secret, Gateway: &fakeGateway{}, CertResolver: fakeResolver{}, CertStore: &fakeCertStore{}, Now: fixedPaymentNow})

	id, appErr := service.CreateConfig(context.Background(), validConfigInput())
	if appErr != nil {
		t.Fatalf("CreateConfig error=%v", appErr)
	}
	if id != 1 || repo.config.PrivateKeyEnc != "enc:PRIVATE_KEY" || repo.config.PrivateKeyHint == "" {
		t.Fatalf("unexpected stored config: %#v", repo.config)
	}
}

func TestUpdateConfigKeepsExistingPrivateKeyWhenEmpty(t *testing.T) {
	repo := newFakeConfigRepo()
	repo.config = &Config{ID: 1, Provider: providerAlipay, Code: "alipay_default", PrivateKeyEnc: "enc:OLD", PrivateKeyHint: "***OLD", EnabledMethodsJSON: mustConfigJSON([]string{enum.PaymentMethodWeb}), Status: enum.CommonNo}
	service := NewService(Dependencies{Repository: repo, Secretbox: &fakeSecretbox{}, Gateway: &fakeGateway{}, CertResolver: fakeResolver{}, CertStore: &fakeCertStore{}, Now: fixedPaymentNow})

	input := validConfigInput()
	input.AppPrivateKey = ""
	if appErr := service.UpdateConfig(context.Background(), 1, input); appErr != nil {
		t.Fatalf("UpdateConfig error=%v", appErr)
	}
	if !repo.keepPrivateKey || repo.config.PrivateKeyEnc != "enc:OLD" {
		t.Fatalf("expected existing key to be kept, keep=%v cfg=%#v", repo.keepPrivateKey, repo.config)
	}
}

func TestChangeConfigStatusToEnabledRunsLocalConfigTest(t *testing.T) {
	repo := newFakeConfigRepo()
	repo.config = validStoredConfig()
	gw := &fakeGateway{}
	service := NewService(Dependencies{Repository: repo, Secretbox: &fakeSecretbox{}, Gateway: gw, CertResolver: fakeResolver{}, CertStore: &fakeCertStore{}, Now: fixedPaymentNow})

	if appErr := service.ChangeConfigStatus(context.Background(), 1, enum.CommonYes); appErr != nil {
		t.Fatalf("ChangeConfigStatus error=%v", appErr)
	}
	if gw.testCount != 1 || repo.status != enum.CommonYes {
		t.Fatalf("expected gateway test before enable, testCount=%d status=%d", gw.testCount, repo.status)
	}
}

func TestUploadCertificateDelegatesToStore(t *testing.T) {
	store := &fakeCertStore{}
	service := NewService(Dependencies{Repository: newFakeConfigRepo(), Secretbox: &fakeSecretbox{}, Gateway: &fakeGateway{}, CertResolver: fakeResolver{}, CertStore: store, Now: fixedPaymentNow})

	result, appErr := service.UploadCertificate(context.Background(), CertificateUploadInput{ConfigCode: "alipay_default", CertType: "app_cert", FileName: "app.crt", Size: 3, Reader: strings.NewReader("crt")})
	if appErr != nil {
		t.Fatalf("UploadCertificate error=%v", appErr)
	}
	if store.saved.ConfigCode != "alipay_default" || store.saved.CertType != "app_cert" || result.Path == "" {
		t.Fatalf("unexpected store call: saved=%#v result=%#v", store.saved, result)
	}
}

func TestTestConfigDecryptsResolvesAndBuildsGatewayConfig(t *testing.T) {
	repo := newFakeConfigRepo()
	repo.config = validStoredConfig()
	gw := &fakeGateway{}
	service := NewService(Dependencies{Repository: repo, Secretbox: &fakeSecretbox{}, Gateway: gw, CertResolver: fakeResolver{}, CertStore: &fakeCertStore{}, Now: fixedPaymentNow})

	result, appErr := service.TestConfig(context.Background(), 1)
	if appErr != nil {
		t.Fatalf("TestConfig error=%v", appErr)
	}
	if !result.OK || gw.cfg.PrivateKey != "PRIVATE_KEY" || !strings.HasPrefix(gw.cfg.AppCertPath, "/resolved/") {
		t.Fatalf("unexpected test result=%#v gateway=%#v", result, gw.cfg)
	}
}

func TestListConfigsDoesNotExposeEncryptedPrivateKey(t *testing.T) {
	repo := newFakeConfigRepo()
	repo.config = validStoredConfig()
	service := NewService(Dependencies{Repository: repo, Secretbox: &fakeSecretbox{}, Gateway: &fakeGateway{}, CertResolver: fakeResolver{}, CertStore: &fakeCertStore{}, Now: fixedPaymentNow})

	result, appErr := service.ListConfigs(context.Background(), ConfigListQuery{})
	if appErr != nil {
		t.Fatalf("ListConfigs error=%v", appErr)
	}
	if len(result.List) != 1 || result.List[0].Provider != providerAlipay || result.List[0].PrivateKeyHint == "" {
		t.Fatalf("unexpected list: %#v", result.List)
	}
}

func validConfigInput() ConfigMutationInput {
	return ConfigMutationInput{
		Provider:         providerAlipay,
		Code:             "alipay_default",
		Name:             "支付宝默认配置",
		AppID:            "2026000000000000",
		AppPrivateKey:    "PRIVATE_KEY",
		AppCertPath:      "runtime/app.crt",
		PlatformCertPath: "runtime/alipay.crt",
		RootCertPath:     "runtime/root.crt",
		NotifyURL:        "https://example.test/notify",
		Environment:      "sandbox",
		EnabledMethods:   []string{enum.PaymentMethodWeb},
		Status:           enum.CommonNo,
		Remark:           "",
	}
}

func validStoredConfig() *Config {
	return &Config{
		ID:                 1,
		Provider:           providerAlipay,
		Code:               "alipay_default",
		Name:               "支付宝默认配置",
		AppID:              "2026000000000000",
		PrivateKeyEnc:      "enc:PRIVATE_KEY",
		PrivateKeyHint:     "***KEY",
		AppCertPath:        "runtime/app.crt",
		PlatformCertPath:   "runtime/alipay.crt",
		RootCertPath:       "runtime/root.crt",
		NotifyURL:          "https://example.test/notify",
		Environment:        "sandbox",
		EnabledMethodsJSON: mustConfigJSON([]string{enum.PaymentMethodWeb}),
		Status:             enum.CommonNo,
		IsDel:              enum.CommonNo,
		CreatedAt:          fixedPaymentNow(),
		UpdatedAt:          fixedPaymentNow(),
	}
}

func fixedPaymentNow() time.Time { return time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC) }

type fakeConfigRepo struct {
	config         *Config
	keepPrivateKey bool
	status         int
}

func newFakeConfigRepo() *fakeConfigRepo { return &fakeConfigRepo{} }

func (r *fakeConfigRepo) ListConfigs(ctx context.Context, query ConfigListQuery) ([]Config, int64, error) {
	if r.config == nil {
		return nil, 0, nil
	}
	return []Config{*r.config}, 1, nil
}
func (r *fakeConfigRepo) GetConfig(ctx context.Context, id int64) (*Config, error) {
	if r.config == nil || r.config.ID != id {
		return nil, nil
	}
	copy := *r.config
	return &copy, nil
}
func (r *fakeConfigRepo) GetConfigByCode(ctx context.Context, code string) (*Config, error) {
	return nil, nil
}
func (r *fakeConfigRepo) CreateConfig(ctx context.Context, cfg Config) (int64, error) {
	cfg.ID = 1
	r.config = &cfg
	return cfg.ID, nil
}
func (r *fakeConfigRepo) UpdateConfig(ctx context.Context, cfg Config, keepPrivateKey bool) error {
	r.keepPrivateKey = keepPrivateKey
	if keepPrivateKey && r.config != nil {
		cfg.PrivateKeyEnc = r.config.PrivateKeyEnc
		cfg.PrivateKeyHint = r.config.PrivateKeyHint
	}
	r.config = &cfg
	return nil
}
func (r *fakeConfigRepo) ChangeConfigStatus(ctx context.Context, id int64, status int) error {
	r.status = status
	if r.config != nil {
		r.config.Status = status
	}
	return nil
}
func (r *fakeConfigRepo) DeleteConfig(ctx context.Context, id int64) error {
	if r.config != nil {
		r.config.IsDel = enum.CommonYes
		r.config.Status = enum.CommonNo
	}
	return nil
}
func (r *fakeConfigRepo) ListOrders(ctx context.Context, query OrderListQuery) ([]Order, int64, error) {
	return nil, 0, nil
}
func (r *fakeConfigRepo) GetOrder(ctx context.Context, id int64) (*Order, error) { return nil, nil }
func (r *fakeConfigRepo) CreateOrder(ctx context.Context, order Order) (int64, error) {
	return 0, nil
}
func (r *fakeConfigRepo) UpdateOrderPaying(ctx context.Context, id int64, payURL string) error {
	return nil
}
func (r *fakeConfigRepo) UpdateOrderFailed(ctx context.Context, id int64, reason string) error {
	return nil
}
func (r *fakeConfigRepo) UpdateOrderPaid(ctx context.Context, id int64, tradeNo string, paidAt time.Time) error {
	return nil
}
func (r *fakeConfigRepo) UpdateOrderClosed(ctx context.Context, id int64, closedAt time.Time) error {
	return nil
}
func (r *fakeConfigRepo) ListEnabledOrderConfigOptions(ctx context.Context) ([]Config, error) {
	if r.config == nil || r.config.Status != enum.CommonYes {
		return nil, nil
	}
	return []Config{*r.config}, nil
}

type fakeSecretbox struct{}

func (fakeSecretbox) Encrypt(plain string) (string, error) {
	if plain == "ERR" {
		return "", errors.New("encrypt")
	}
	return "enc:" + plain, nil
}
func (fakeSecretbox) Decrypt(ciphertext string) (string, error) {
	return strings.TrimPrefix(ciphertext, "enc:"), nil
}

type fakeResolver struct{}

func (fakeResolver) Resolve(path string) (string, error) {
	return "/resolved/" + strings.TrimSpace(path), nil
}

type fakeCertStore struct{ saved gateway.CertificateFile }

func (s *fakeCertStore) Save(ctx context.Context, file gateway.CertificateFile) (*gateway.CertificateSaveResult, error) {
	s.saved = file
	if file.Reader != nil {
		_, _ = io.ReadAll(file.Reader)
	}
	return &gateway.CertificateSaveResult{Path: "runtime/payment/certs/alipay/alipay_default/abc.crt", FileName: file.FileName, SHA256: "abc", Size: file.Size}, nil
}

type fakeGateway struct {
	testCount int
	cfg       gateway.ChannelConfig
}

func (g *fakeGateway) TestConfig(ctx context.Context, cfg gateway.ChannelConfig) error {
	g.testCount++
	g.cfg = cfg
	return nil
}
func (g *fakeGateway) Pay(ctx context.Context, cfg gateway.ChannelConfig, in gateway.PayInput) (*gateway.PayResult, error) {
	return &gateway.PayResult{PayURL: "https://example.test/pay"}, nil
}
func (g *fakeGateway) Query(ctx context.Context, cfg gateway.ChannelConfig, outTradeNo string) (*gateway.QueryResult, error) {
	return &gateway.QueryResult{Status: "WAIT_BUYER_PAY"}, nil
}
func (g *fakeGateway) Close(ctx context.Context, cfg gateway.ChannelConfig, outTradeNo string) error {
	return nil
}

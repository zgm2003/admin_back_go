package paychannel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/secretbox"
)

type fakeRepository struct {
	rows       []Channel
	total      int64
	rowByID    map[int64]Channel
	exists     bool
	referenced bool
	created    *Channel
	updates    []map[string]any
	statusID   int64
	status     int
	deletedID  int64
}

func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]Channel, int64, error) {
	return f.rows, f.total, nil
}

func (f *fakeRepository) Get(ctx context.Context, id int64) (*Channel, error) {
	if f.rowByID == nil {
		return nil, nil
	}
	row, ok := f.rowByID[id]
	if !ok {
		return nil, nil
	}
	return &row, nil
}

func (f *fakeRepository) ExistsUnique(ctx context.Context, channel int, mchID string, appID string, excludeID int64) (bool, error) {
	return f.exists, nil
}

func (f *fakeRepository) Create(ctx context.Context, row Channel) (int64, error) {
	f.created = &row
	return 11, nil
}

func (f *fakeRepository) Update(ctx context.Context, id int64, fields map[string]any) error {
	f.updates = append(f.updates, fields)
	return nil
}

func (f *fakeRepository) ChangeStatus(ctx context.Context, id int64, status int) error {
	f.statusID = id
	f.status = status
	return nil
}

func (f *fakeRepository) Delete(ctx context.Context, id int64) error {
	f.deletedID = id
	return nil
}

func (f *fakeRepository) Referenced(ctx context.Context, id int64) (bool, error) {
	return f.referenced, nil
}

func TestInitReturnsPayDicts(t *testing.T) {
	service := NewService(&fakeRepository{}, secretbox.New("vault-key"))

	got, appErr := service.Init(context.Background())
	if appErr != nil {
		t.Fatalf("expected init to succeed, got %v", appErr)
	}
	if len(got.Dict.ChannelArr) != 2 || got.Dict.ChannelArr[0].Value != enum.PayChannelWechat {
		t.Fatalf("unexpected channel dict: %#v", got.Dict.ChannelArr)
	}
	if len(got.Dict.PayMethodArr) != 6 || got.Dict.PayMethodArr[0].Value != enum.PayMethodWeb {
		t.Fatalf("unexpected method dict: %#v", got.Dict.PayMethodArr)
	}
	if len(got.Dict.CommonStatusArr) != 2 || got.Dict.CommonStatusArr[0].Value != enum.CommonYes {
		t.Fatalf("unexpected status dict: %#v", got.Dict.CommonStatusArr)
	}
}

func TestCreateRejectsUnsupportedChannel(t *testing.T) {
	service := NewService(&fakeRepository{}, secretbox.New("vault-key"))

	_, appErr := service.Create(context.Background(), CreateInput{Name: "bad", Channel: 9, SupportedMethods: []string{enum.PayMethodScan}, MchID: "mch"})
	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "无效的支付渠道" {
		t.Fatalf("expected invalid channel error, got %#v", appErr)
	}
}

func TestCreateRejectsUnsupportedMethods(t *testing.T) {
	service := NewService(&fakeRepository{}, secretbox.New("vault-key"))

	_, appErr := service.Create(context.Background(), CreateInput{Name: "wx", Channel: enum.PayChannelWechat, SupportedMethods: []string{enum.PayMethodWeb}, MchID: "mch"})
	if appErr == nil || appErr.Message != "所选支付方式包含当前渠道不支持的选项" {
		t.Fatalf("expected unsupported methods error, got %#v", appErr)
	}
}

func TestCreateNormalizesMethodsAndEncryptsPrivateKey(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, secretbox.New("vault-key"))

	id, appErr := service.Create(context.Background(), CreateInput{
		Name: " 微信渠道 ", Channel: enum.PayChannelWechat, SupportedMethods: []string{enum.PayMethodH5, enum.PayMethodScan, enum.PayMethodScan},
		MchID: " mch-1 ", AppID: " app-1 ", NotifyURL: " https://example.test/notify ", AppPrivateKey: "plain-private-key",
		PublicCertPath: " /cert/pub.pem ", PlatformCertPath: " /cert/platform.pem ", RootCertPath: " /cert/root.pem ", IsSandbox: enum.CommonNo, Status: enum.CommonYes,
	})
	if appErr != nil {
		t.Fatalf("expected create to succeed, got %v", appErr)
	}
	if id != 11 || repo.created == nil {
		t.Fatalf("expected created row, id=%d row=%#v", id, repo.created)
	}
	if repo.created.Name != "微信渠道" || repo.created.MchID != "mch-1" || repo.created.AppID != "app-1" {
		t.Fatalf("expected trimmed fields, got %#v", repo.created)
	}
	if repo.created.AppPrivateKeyEnc == "" || repo.created.AppPrivateKeyEnc == "plain-private-key" || repo.created.AppPrivateKeyHint != "***-key" {
		t.Fatalf("private key was not encrypted/hinted safely: %#v", repo.created)
	}
	methods := mustSupportedMethods(t, repo.created.ExtraConfig)
	if strings.Join(methods, ",") != "h5,scan" {
		t.Fatalf("unexpected normalized methods: %#v extra=%s", methods, repo.created.ExtraConfig)
	}
}

func TestCreatePrivateKeyFailsWithoutVaultKey(t *testing.T) {
	service := NewService(&fakeRepository{}, secretbox.New(""))

	_, appErr := service.Create(context.Background(), CreateInput{Name: "wx", Channel: enum.PayChannelWechat, SupportedMethods: []string{enum.PayMethodScan}, MchID: "mch", AppPrivateKey: "plain"})
	if appErr == nil || appErr.Code != apperror.CodeInternal || !strings.Contains(appErr.Message, "加密支付渠道私钥失败") {
		t.Fatalf("expected explicit encryption config error, got %#v", appErr)
	}
}

func TestUpdateBlankPrivateKeyKeepsExistingEncryptedKey(t *testing.T) {
	repo := &fakeRepository{rowByID: map[int64]Channel{5: {
		ID: 5, Name: "old", Channel: enum.PayChannelWechat, MchID: "mch", AppID: "app", ExtraConfig: `{"supported_methods":["scan"]}`,
		AppPrivateKeyEnc: "cipher-old", AppPrivateKeyHint: "***old", IsDel: enum.CommonNo,
	}}}
	service := NewService(repo, secretbox.New("vault-key"))

	appErr := service.Update(context.Background(), 5, UpdateInput{Name: "new", Channel: enum.PayChannelWechat, SupportedMethods: []string{enum.PayMethodScan}, MchID: "mch", AppID: "app", Status: enum.CommonYes, IsSandbox: enum.CommonNo})
	if appErr != nil {
		t.Fatalf("expected update to succeed, got %v", appErr)
	}
	if len(repo.updates) != 1 {
		t.Fatalf("expected one update, got %#v", repo.updates)
	}
	if _, ok := repo.updates[0]["app_private_key_enc"]; ok {
		t.Fatalf("blank private key must not overwrite encrypted key: %#v", repo.updates[0])
	}
	if _, ok := repo.updates[0]["app_private_key_hint"]; ok {
		t.Fatalf("blank private key must not overwrite key hint: %#v", repo.updates[0])
	}
}

func TestUpdateRejectsChangingChannelWithoutMethods(t *testing.T) {
	repo := &fakeRepository{rowByID: map[int64]Channel{5: {ID: 5, Channel: enum.PayChannelWechat, MchID: "mch", ExtraConfig: `{"supported_methods":["scan"]}`}}}
	service := NewService(repo, secretbox.New("vault-key"))

	appErr := service.Update(context.Background(), 5, UpdateInput{Name: "ali", Channel: enum.PayChannelAlipay, MchID: "mch", Status: enum.CommonYes, IsSandbox: enum.CommonNo})
	if appErr == nil || appErr.Message != "请至少选择一种支付方式" {
		t.Fatalf("expected missing supported methods error, got %#v", appErr)
	}
}

func TestDeleteRejectsReferencedChannel(t *testing.T) {
	repo := &fakeRepository{referenced: true, rowByID: map[int64]Channel{7: {ID: 7, Name: "支付宝"}}}
	service := NewService(repo, secretbox.New("vault-key"))

	appErr := service.Delete(context.Background(), 7)
	if appErr == nil || appErr.Message != "支付渠道已有订单或支付流水引用，请禁用而不是删除" {
		t.Fatalf("expected referenced delete rejection, got %#v", appErr)
	}
	if repo.deletedID != 0 {
		t.Fatalf("referenced channel must not be deleted")
	}
}

func TestListDTOExcludesPrivateKeyCiphertext(t *testing.T) {
	createdAt := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	repo := &fakeRepository{rows: []Channel{{
		ID: 1, Name: "支付宝", Channel: enum.PayChannelAlipay, MchID: "mch", AppID: "app", NotifyURL: "https://example.test/notify",
		AppPrivateKeyEnc: "cipher-secret", AppPrivateKeyHint: "***cret", ExtraConfig: `{"supported_methods":["web","h5"]}`,
		IsSandbox: enum.CommonYes, Status: enum.CommonYes, IsDel: enum.CommonNo, CreatedAt: createdAt,
	}}, total: 1}
	service := NewService(repo, secretbox.New("vault-key"))

	got, appErr := service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20})
	if appErr != nil {
		t.Fatalf("expected list to succeed, got %v", appErr)
	}
	if len(got.List) != 1 || got.List[0].SupportedMethodsText != "PC网页支付 / H5支付" {
		t.Fatalf("unexpected list response: %#v", got)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal list response: %v", err)
	}
	body := string(encoded)
	if strings.Contains(body, "app_private_key_enc") || strings.Contains(body, "cipher-secret") || strings.Contains(body, "plain") {
		t.Fatalf("list response leaked private key data: %s", body)
	}
}

func mustSupportedMethods(t *testing.T, extraConfig string) []string {
	t.Helper()
	var parsed struct {
		SupportedMethods []string `json:"supported_methods"`
	}
	if err := json.Unmarshal([]byte(extraConfig), &parsed); err != nil {
		t.Fatalf("decode extra config: %v", err)
	}
	return parsed.SupportedMethods
}

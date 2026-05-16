package uploadconfig

import (
	"context"
	"strings"
	"testing"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/secretbox"
)

type fakeRepository struct {
	drivers           []Driver
	driverTotal       int64
	driverExists      bool
	driverByID        map[int64]Driver
	driverReferenced  bool
	createdDriver     *Driver
	updatedDriverID   int64
	updatedDriverData map[string]any
	deletedDriverIDs  []int64
	rules             []Rule
	ruleTotal         int64
	ruleExists        bool
	ruleReferenced    bool
	createdRule       *Rule
	updatedRuleID     int64
	updatedRuleData   map[string]any
	deletedRuleIDs    []int64
	settingDriverDict []Driver
	settingRuleDict   []Rule
	settingExists     bool
	settingDriverOK   bool
	settingRuleOK     bool
	settingEnabled    bool
	createdSetting    *Setting
	updatedSettingID  int64
	updatedSetting    map[string]any
	deletedSettingIDs []int64
}

func (f *fakeRepository) ListDrivers(ctx context.Context, query DriverListQuery) ([]Driver, int64, error) {
	return f.drivers, f.driverTotal, nil
}

func (f *fakeRepository) ExistsDriverBucket(ctx context.Context, driver string, bucket string, excludeID int64) (bool, error) {
	return f.driverExists, nil
}

func (f *fakeRepository) CreateDriver(ctx context.Context, row Driver) (int64, error) {
	f.createdDriver = &row
	return 101, nil
}

func (f *fakeRepository) GetDriver(ctx context.Context, id int64) (*Driver, error) {
	if f.driverByID == nil {
		return nil, nil
	}
	row, ok := f.driverByID[id]
	if !ok {
		return nil, nil
	}
	return &row, nil
}

func (f *fakeRepository) UpdateDriver(ctx context.Context, id int64, fields map[string]any) error {
	f.updatedDriverID = id
	f.updatedDriverData = fields
	return nil
}

func (f *fakeRepository) DeleteDrivers(ctx context.Context, ids []int64) error {
	f.deletedDriverIDs = append([]int64{}, ids...)
	return nil
}

func (f *fakeRepository) DriverReferenced(ctx context.Context, ids []int64) (bool, error) {
	return f.driverReferenced, nil
}

func (f *fakeRepository) ListRules(ctx context.Context, query RuleListQuery) ([]Rule, int64, error) {
	return f.rules, f.ruleTotal, nil
}

func (f *fakeRepository) ExistsRuleTitle(ctx context.Context, title string, excludeID int64) (bool, error) {
	return f.ruleExists, nil
}

func (f *fakeRepository) CreateRule(ctx context.Context, row Rule) (int64, error) {
	f.createdRule = &row
	return 201, nil
}

func (f *fakeRepository) GetRule(ctx context.Context, id int64) (*Rule, error) {
	return &Rule{ID: id, Title: "old"}, nil
}

func (f *fakeRepository) UpdateRule(ctx context.Context, id int64, fields map[string]any) error {
	f.updatedRuleID = id
	f.updatedRuleData = fields
	return nil
}

func (f *fakeRepository) DeleteRules(ctx context.Context, ids []int64) error {
	f.deletedRuleIDs = append([]int64{}, ids...)
	return nil
}

func (f *fakeRepository) RuleReferenced(ctx context.Context, ids []int64) (bool, error) {
	return f.ruleReferenced, nil
}

func (f *fakeRepository) ListSettings(ctx context.Context, query SettingListQuery) ([]SettingListRow, int64, error) {
	return []SettingListRow{}, 0, nil
}

func (f *fakeRepository) DriverDict(ctx context.Context) ([]Driver, error) {
	return f.settingDriverDict, nil
}

func (f *fakeRepository) RuleDict(ctx context.Context) ([]Rule, error) {
	return f.settingRuleDict, nil
}

func (f *fakeRepository) DriverExists(ctx context.Context, id int64) (bool, error) {
	return f.settingDriverOK, nil
}

func (f *fakeRepository) RuleExists(ctx context.Context, id int64) (bool, error) {
	return f.settingRuleOK, nil
}

func (f *fakeRepository) ExistsSettingDriverRule(ctx context.Context, driverID int64, ruleID int64, excludeID int64) (bool, error) {
	return f.settingExists, nil
}

func (f *fakeRepository) CreateSetting(ctx context.Context, row Setting) (int64, error) {
	f.createdSetting = &row
	return 301, nil
}

func (f *fakeRepository) GetSetting(ctx context.Context, id int64) (*Setting, error) {
	return &Setting{ID: id, Status: enum.CommonNo}, nil
}

func (f *fakeRepository) UpdateSetting(ctx context.Context, id int64, fields map[string]any) error {
	f.updatedSettingID = id
	f.updatedSetting = fields
	return nil
}

func (f *fakeRepository) EnableSettingExclusive(ctx context.Context, id int64, row Setting, updateExisting bool) (int64, error) {
	if updateExisting {
		f.updatedSettingID = id
		f.updatedSetting = map[string]any{"driver_id": row.DriverID, "rule_id": row.RuleID, "status": row.Status, "remark": row.Remark}
		return id, nil
	}
	f.createdSetting = &row
	return 302, nil
}

func (f *fakeRepository) SettingEnabledIn(ctx context.Context, ids []int64) (bool, error) {
	return f.settingEnabled, nil
}

func (f *fakeRepository) DeleteSettings(ctx context.Context, ids []int64) error {
	f.deletedSettingIDs = append([]int64{}, ids...)
	return nil
}

func TestDriverInitReturnsEnumBackedDict(t *testing.T) {
	service := NewService(&fakeRepository{}, nil)

	got, appErr := service.DriverInit(context.Background())
	if appErr != nil {
		t.Fatalf("expected init to succeed, got %v", appErr)
	}
	options := got.Dict.UploadDriverArr
	if len(options) != 1 || options[0].Value != enum.UploadDriverCOS {
		t.Fatalf("unexpected upload driver dict: %#v", options)
	}
}

func TestRuleInitReturnsEnumBackedDict(t *testing.T) {
	service := NewService(&fakeRepository{}, nil)

	got, appErr := service.RuleInit(context.Background())
	if appErr != nil {
		t.Fatalf("expected init to succeed, got %v", appErr)
	}
	if len(got.Dict.UploadImageExtArr) == 0 || got.Dict.UploadImageExtArr[0].Value != "jpeg" {
		t.Fatalf("unexpected image ext dict: %#v", got.Dict.UploadImageExtArr)
	}
	if len(got.Dict.UploadFileExtArr) == 0 || got.Dict.UploadFileExtArr[0].Value != "docx" {
		t.Fatalf("unexpected file ext dict: %#v", got.Dict.UploadFileExtArr)
	}
}

func TestServiceRequiresRepositoryForDriverList(t *testing.T) {
	service := NewService(nil, nil)

	_, appErr := service.DriverList(context.Background(), DriverListQuery{CurrentPage: 1, PageSize: 20})
	if appErr == nil || appErr.Code != apperror.CodeInternal {
		t.Fatalf("expected repository not configured error, got %#v", appErr)
	}
}

func TestDriverCreateEncryptsSecretsAndReturnsID(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	repo := &fakeRepository{}
	service := NewService(repo, &box)

	id, appErr := service.CreateDriver(context.Background(), DriverCreateInput{
		Driver: enum.UploadDriverCOS, SecretID: "sid-123456", SecretKey: "skey-abcdef", Bucket: "bucket-a", Region: "ap-nanjing", AppID: "1314",
	})
	if appErr != nil {
		t.Fatalf("expected create to succeed, got %v", appErr)
	}
	if id != 101 || repo.createdDriver == nil {
		t.Fatalf("expected created driver id, id=%d row=%#v", id, repo.createdDriver)
	}
	if repo.createdDriver.SecretIDEnc == "" || repo.createdDriver.SecretIDEnc == "sid-123456" {
		t.Fatalf("secret_id must be encrypted, row=%#v", repo.createdDriver)
	}
	plain, err := box.Decrypt(repo.createdDriver.SecretIDEnc)
	if err != nil || plain != "sid-123456" {
		t.Fatalf("expected encrypted secret id to decrypt, plain=%q err=%v", plain, err)
	}
	if repo.createdDriver.SecretIDHint != "***3456" || repo.createdDriver.SecretKeyHint != "***cdef" {
		t.Fatalf("unexpected hints: %#v", repo.createdDriver)
	}
}

func TestDriverCreateRejectsDuplicateDriverBucket(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	service := NewService(&fakeRepository{driverExists: true}, &box)

	_, appErr := service.CreateDriver(context.Background(), DriverCreateInput{Driver: enum.UploadDriverCOS, SecretID: "sid", SecretKey: "skey", Bucket: "bucket-a", Region: "ap-nanjing", AppID: "1314"})
	if appErr == nil || appErr.Code != apperror.CodeBadRequest || !strings.Contains(appErr.Message, "同一驱动下该桶已存在") {
		t.Fatalf("expected duplicate bucket error, got %#v", appErr)
	}
}

func TestDriverCreateRejectsMissingCOSAppID(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	service := NewService(&fakeRepository{}, &box)

	_, appErr := service.CreateDriver(context.Background(), DriverCreateInput{Driver: enum.UploadDriverCOS, SecretID: "sid", SecretKey: "skey", Bucket: "bucket-a", Region: "ap-nanjing"})
	if appErr == nil || appErr.Message != "COS appid 不能为空" {
		t.Fatalf("expected missing appid error, got %#v", appErr)
	}
}

func TestDriverCreateRejectsNonCOSDriver(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	service := NewService(&fakeRepository{}, &box)

	_, appErr := service.CreateDriver(context.Background(), DriverCreateInput{Driver: "oss", SecretID: "sid", SecretKey: "skey", Bucket: "bucket-a", Region: "cn-hangzhou"})
	if appErr == nil || appErr.Message != "当前仅支持腾讯云 COS，请重新配置 COS" {
		t.Fatalf("expected non-COS rejection error, got %#v", appErr)
	}
}

func TestDriverUpdateKeepsSecretsWhenOmitted(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	repo := &fakeRepository{driverByID: map[int64]Driver{7: {ID: 7, Driver: enum.UploadDriverCOS, Bucket: "old", SecretIDEnc: "old-id", SecretKeyEnc: "old-key"}}}
	service := NewService(repo, &box)

	appErr := service.UpdateDriver(context.Background(), 7, DriverUpdateInput{Driver: enum.UploadDriverCOS, Bucket: "bucket-a", Region: "ap-nanjing", AppID: "1314"})
	if appErr != nil {
		t.Fatalf("expected update to succeed, got %v", appErr)
	}
	if _, ok := repo.updatedDriverData["secret_id_enc"]; ok {
		t.Fatalf("secret_id_enc should not be updated when omitted: %#v", repo.updatedDriverData)
	}
	if _, ok := repo.updatedDriverData["secret_key_enc"]; ok {
		t.Fatalf("secret_key_enc should not be updated when omitted: %#v", repo.updatedDriverData)
	}
}

func TestDriverUpdateRotatesProvidedSecret(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	repo := &fakeRepository{driverByID: map[int64]Driver{7: {ID: 7, Driver: enum.UploadDriverCOS, Bucket: "old", SecretIDEnc: "old-id", SecretKeyEnc: "old-key"}}}
	service := NewService(repo, &box)

	appErr := service.UpdateDriver(context.Background(), 7, DriverUpdateInput{Driver: enum.UploadDriverCOS, SecretID: "new-secret-id", Bucket: "bucket-a", Region: "ap-nanjing", AppID: "1314"})
	if appErr != nil {
		t.Fatalf("expected update to succeed, got %v", appErr)
	}
	ciphertext, ok := repo.updatedDriverData["secret_id_enc"].(string)
	if !ok || ciphertext == "" || ciphertext == "new-secret-id" {
		t.Fatalf("expected encrypted rotated secret id, fields=%#v", repo.updatedDriverData)
	}
	if repo.updatedDriverData["secret_id_hint"] != "***t-id" {
		t.Fatalf("expected rotated secret hint, fields=%#v", repo.updatedDriverData)
	}
	if _, ok := repo.updatedDriverData["secret_key_enc"]; ok {
		t.Fatalf("secret_key_enc should not be rotated when omitted: %#v", repo.updatedDriverData)
	}
}

func TestDriverListNeverReturnsPlaintextOrCiphertext(t *testing.T) {
	repo := &fakeRepository{drivers: []Driver{{ID: 1, Driver: enum.UploadDriverCOS, SecretIDEnc: "cipher-id", SecretKeyEnc: "cipher-key", SecretIDHint: "***1111", SecretKeyHint: "***2222", Bucket: "bucket-a", Region: "ap-nanjing"}}, driverTotal: 1}
	service := NewService(repo, nil)

	got, appErr := service.DriverList(context.Background(), DriverListQuery{CurrentPage: 1, PageSize: 20})
	if appErr != nil {
		t.Fatalf("expected list to succeed, got %v", appErr)
	}
	if len(got.List) != 1 {
		t.Fatalf("expected one row, got %#v", got.List)
	}
	item := got.List[0]
	if item.SecretIDHint != "***1111" || item.SecretKeyHint != "***2222" {
		t.Fatalf("expected hints only, got %#v", item)
	}
	if strings.Contains(item.SecretIDHint, "cipher") || strings.Contains(item.SecretKeyHint, "cipher") {
		t.Fatalf("ciphertext leaked in list item: %#v", item)
	}
}

func TestDriverDeleteRejectsReferencedDriver(t *testing.T) {
	service := NewService(&fakeRepository{driverReferenced: true}, nil)

	appErr := service.DeleteDrivers(context.Background(), []int64{1})
	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "上传驱动已被上传设置引用，无法删除" {
		t.Fatalf("expected referenced delete rejection, got %#v", appErr)
	}
}

func TestRuleCreateRejectsDuplicateTitle(t *testing.T) {
	service := NewService(&fakeRepository{ruleExists: true}, nil)

	_, appErr := service.CreateRule(context.Background(), RuleMutationInput{Title: "重复", MaxSizeMB: 10, ImageExts: []string{"png"}})
	if appErr == nil || appErr.Message != "规则标题已存在" {
		t.Fatalf("expected duplicate title error, got %#v", appErr)
	}
}

func TestRuleCreateRejectsInvalidMaxSize(t *testing.T) {
	service := NewService(&fakeRepository{}, nil)

	_, appErr := service.CreateRule(context.Background(), RuleMutationInput{Title: "bad", MaxSizeMB: 0, ImageExts: []string{"png"}})
	if appErr == nil || appErr.Message != "max_size_mb 必须在 1..10240 之间" {
		t.Fatalf("expected max size error, got %#v", appErr)
	}
}

func TestRuleCreateRejectsUnknownImageExt(t *testing.T) {
	service := NewService(&fakeRepository{}, nil)

	_, appErr := service.CreateRule(context.Background(), RuleMutationInput{Title: "bad", MaxSizeMB: 10, ImageExts: []string{"exe"}})
	if appErr == nil || !strings.Contains(appErr.Message, "图片扩展名不支持") {
		t.Fatalf("expected image ext error, got %#v", appErr)
	}
}

func TestRuleCreateRejectsUnknownFileExt(t *testing.T) {
	service := NewService(&fakeRepository{}, nil)

	_, appErr := service.CreateRule(context.Background(), RuleMutationInput{Title: "bad", MaxSizeMB: 10, FileExts: []string{"php"}})
	if appErr == nil || !strings.Contains(appErr.Message, "文件扩展名不支持") {
		t.Fatalf("expected file ext error, got %#v", appErr)
	}
}

func TestRuleCreateRejectsBothExtArraysEmpty(t *testing.T) {
	service := NewService(&fakeRepository{}, nil)

	_, appErr := service.CreateRule(context.Background(), RuleMutationInput{Title: "empty", MaxSizeMB: 10})
	if appErr == nil || appErr.Message != "image_exts 和 file_exts 不能同时为空" {
		t.Fatalf("expected empty ext arrays error, got %#v", appErr)
	}
}

func TestRuleCreateNormalizesLowercaseDedupeAndEnumOrder(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, nil)

	id, appErr := service.CreateRule(context.Background(), RuleMutationInput{Title: " 图片 ", MaxSizeMB: 10, ImageExts: []string{" PNG", "jpeg", "png"}, FileExts: []string{" PDF ", "docx"}})
	if appErr != nil {
		t.Fatalf("expected create rule to succeed, got %v", appErr)
	}
	if id != 201 || repo.createdRule == nil {
		t.Fatalf("expected created rule, id=%d row=%#v", id, repo.createdRule)
	}
	if repo.createdRule.Title != "图片" {
		t.Fatalf("expected trimmed title, got %#v", repo.createdRule)
	}
	if repo.createdRule.ImageExts != `["jpeg","png"]` {
		t.Fatalf("expected normalized image exts, got %q", repo.createdRule.ImageExts)
	}
	if repo.createdRule.FileExts != `["docx","pdf"]` {
		t.Fatalf("expected normalized file exts, got %q", repo.createdRule.FileExts)
	}
}

func TestRuleDeleteRejectsReferencedRule(t *testing.T) {
	service := NewService(&fakeRepository{ruleReferenced: true}, nil)

	appErr := service.DeleteRules(context.Background(), []int64{1})
	if appErr == nil || appErr.Message != "上传规则已被上传设置引用，无法删除" {
		t.Fatalf("expected referenced rule delete rejection, got %#v", appErr)
	}
}

func TestSettingInitReturnsDriverAndRuleDicts(t *testing.T) {
	repo := &fakeRepository{
		settingDriverDict: []Driver{{ID: 1, Driver: enum.UploadDriverCOS, Bucket: "bucket-a"}},
		settingRuleDict:   []Rule{{ID: 2, Title: "图片规则"}},
	}
	service := NewService(repo, nil)

	got, appErr := service.SettingInit(context.Background())
	if appErr != nil {
		t.Fatalf("expected setting init to succeed, got %v", appErr)
	}
	if len(got.Dict.UploadDriverList) != 1 || got.Dict.UploadDriverList[0].Label != "腾讯云 COS - bucket-a" || got.Dict.UploadDriverList[0].Value != 1 {
		t.Fatalf("unexpected driver dict: %#v", got.Dict.UploadDriverList)
	}
	if len(got.Dict.UploadRuleList) != 1 || got.Dict.UploadRuleList[0].Label != "图片规则" || got.Dict.UploadRuleList[0].Value != 2 {
		t.Fatalf("unexpected rule dict: %#v", got.Dict.UploadRuleList)
	}
	if len(got.Dict.CommonStatusArr) != 2 {
		t.Fatalf("unexpected status dict: %#v", got.Dict.CommonStatusArr)
	}
}

func TestSettingCreateRejectsMissingDriver(t *testing.T) {
	service := NewService(&fakeRepository{settingRuleOK: true}, nil)

	_, appErr := service.CreateSetting(context.Background(), SettingMutationInput{DriverID: 1, RuleID: 2, Status: enum.CommonNo})
	if appErr == nil || appErr.Message != "上传驱动不存在" {
		t.Fatalf("expected missing driver error, got %#v", appErr)
	}
}

func TestSettingCreateRejectsMissingRule(t *testing.T) {
	service := NewService(&fakeRepository{settingDriverOK: true}, nil)

	_, appErr := service.CreateSetting(context.Background(), SettingMutationInput{DriverID: 1, RuleID: 2, Status: enum.CommonNo})
	if appErr == nil || appErr.Message != "上传规则不存在" {
		t.Fatalf("expected missing rule error, got %#v", appErr)
	}
}

func TestSettingCreateRejectsDuplicateDriverRule(t *testing.T) {
	service := NewService(&fakeRepository{settingDriverOK: true, settingRuleOK: true, settingExists: true}, nil)

	_, appErr := service.CreateSetting(context.Background(), SettingMutationInput{DriverID: 1, RuleID: 2, Status: enum.CommonNo})
	if appErr == nil || appErr.Message != "该驱动与规则组合已存在" {
		t.Fatalf("expected duplicate setting error, got %#v", appErr)
	}
}

func TestSettingCreateEnabledDisablesOtherEnabledRows(t *testing.T) {
	repo := &fakeRepository{settingDriverOK: true, settingRuleOK: true}
	service := NewService(repo, nil)

	id, appErr := service.CreateSetting(context.Background(), SettingMutationInput{DriverID: 1, RuleID: 2, Status: enum.CommonYes, Remark: "启用"})
	if appErr != nil {
		t.Fatalf("expected create enabled setting to succeed, got %v", appErr)
	}
	if id != 302 || repo.createdSetting == nil || repo.createdSetting.Status != enum.CommonYes {
		t.Fatalf("expected exclusive create path, id=%d row=%#v", id, repo.createdSetting)
	}
}

func TestSettingStatusEnabledDisablesOtherEnabledRows(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, nil)

	appErr := service.ChangeSettingStatus(context.Background(), 9, enum.CommonYes)
	if appErr != nil {
		t.Fatalf("expected enable status to succeed, got %v", appErr)
	}
	if repo.updatedSettingID != 9 || repo.updatedSetting["status"] != enum.CommonYes {
		t.Fatalf("expected exclusive update status, got id=%d fields=%#v", repo.updatedSettingID, repo.updatedSetting)
	}
}

func TestSettingStatusDisabledOnlyDisablesCurrentRow(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, nil)

	appErr := service.ChangeSettingStatus(context.Background(), 9, enum.CommonNo)
	if appErr != nil {
		t.Fatalf("expected disable status to succeed, got %v", appErr)
	}
	if repo.updatedSettingID != 9 || repo.updatedSetting["status"] != enum.CommonNo {
		t.Fatalf("expected normal status update, got id=%d fields=%#v", repo.updatedSettingID, repo.updatedSetting)
	}
}

func TestSettingDeleteRejectsEnabledSetting(t *testing.T) {
	service := NewService(&fakeRepository{settingEnabled: true}, nil)

	appErr := service.DeleteSettings(context.Background(), []int64{1})
	if appErr == nil || appErr.Message != "包含启用的上传设置，无法删除" {
		t.Fatalf("expected enabled delete rejection, got %#v", appErr)
	}
}

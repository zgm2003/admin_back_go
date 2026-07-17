package aiprovider

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/ai/provider"
	"admin_back_go/internal/infra/secretbox"
	"admin_back_go/internal/shared/apperror"
)

type fakeRepository struct {
	rows               []Provider
	total              int64
	listQuery          ListQuery
	rowByID            map[uint64]Provider
	exists             bool
	created            *Provider
	updates            []map[string]any
	statusID           uint64
	status             int
	deletedID          uint64
	updateErr          error
	modelsByProvider   map[uint64][]ProviderModel
	replacedProviderID uint64
	replacedModels     []ProviderModel
}

func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]Provider, int64, error) {
	f.listQuery = query
	return f.rows, f.total, nil
}

func (f *fakeRepository) Get(ctx context.Context, id uint64) (*Provider, error) {
	if f.rowByID == nil {
		return nil, nil
	}
	row, ok := f.rowByID[id]
	if !ok {
		return nil, nil
	}
	return &row, nil
}

func (f *fakeRepository) ExistsByTypeName(ctx context.Context, engineType string, name string, excludeID uint64) (bool, error) {
	return f.exists, nil
}

func (f *fakeRepository) Create(ctx context.Context, row Provider) (uint64, error) {
	f.created = &row
	return 11, nil
}

func (f *fakeRepository) Update(ctx context.Context, id uint64, fields map[string]any) error {
	f.updates = append(f.updates, fields)
	return f.updateErr
}

func (f *fakeRepository) ListModels(ctx context.Context, providerID uint64) ([]ProviderModel, error) {
	if f.modelsByProvider == nil {
		return nil, nil
	}
	return f.modelsByProvider[providerID], nil
}

func (f *fakeRepository) ReplaceModels(ctx context.Context, providerID uint64, models []ProviderModel) error {
	f.replacedProviderID = providerID
	f.replacedModels = append([]ProviderModel(nil), models...)
	return nil
}

func (f *fakeRepository) ChangeStatus(ctx context.Context, id uint64, status int) error {
	f.statusID = id
	f.status = status
	return nil
}

func (f *fakeRepository) Delete(ctx context.Context, id uint64) error {
	f.deletedID = id
	return nil
}

type fakeModelDriver struct {
	config provider.Config
	err    error
}

func (f *fakeModelDriver) ListModels(ctx context.Context, cfg provider.Config) ([]provider.Model, error) {
	f.config = cfg
	if f.err != nil {
		return nil, f.err
	}
	return []provider.Model{{ID: "gpt-4.1-mini", Object: "model", OwnedBy: "openai"}}, nil
}

func (f *fakeModelDriver) TestConnection(ctx context.Context, cfg provider.Config) (*provider.TestResult, error) {
	f.config = cfg
	if f.err != nil {
		return &provider.TestResult{OK: false, Status: provider.HealthFailed, Message: f.err.Error()}, f.err
	}
	return &provider.TestResult{OK: true, Status: provider.HealthOK, LatencyMs: 12, Message: "ok", ModelCount: 1}, nil
}

type fakeTester struct {
	input infraai.TestConnectionInput
	err   error
}

func (f *fakeTester) TestConnection(ctx context.Context, input infraai.TestConnectionInput) (*infraai.TestConnectionResult, error) {
	f.input = input
	if f.err != nil {
		return &infraai.TestConnectionResult{OK: false, Status: "500", Message: f.err.Error()}, f.err
	}
	return &infraai.TestConnectionResult{OK: true, Status: "200 OK", LatencyMs: 12, Message: "ok"}, nil
}

func TestInitOnlyReturnsOpenAIDriver(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, secretbox.New([]byte("12345678901234567890123456789012")), nil)

	result, appErr := service.PageInit(context.Background())
	if appErr != nil {
		t.Fatalf("Init error = %v", appErr)
	}
	if len(result.Dict.EngineTypeArr) != 1 || result.Dict.EngineTypeArr[0].Value != "openai" {
		t.Fatalf("driver options = %+v, want openai only", result.Dict.EngineTypeArr)
	}
}

func TestCreateRequiresAPIKeyAndModels(t *testing.T) {
	service := NewService(&fakeRepository{}, secretbox.New([]byte("12345678901234567890123456789012")), nil)

	_, appErr := service.Create(context.Background(), CreateInput{Name: "OpenAI", EngineType: "openai", Status: 1})
	if appErr == nil || !strings.Contains(appErr.Message, "API Key") {
		t.Fatalf("Create error = %v, want API Key required", appErr)
	}
	_, appErr = service.Create(context.Background(), CreateInput{Name: "OpenAI", EngineType: "openai", APIKey: "sk-test", Status: 1})
	if appErr == nil || !strings.Contains(appErr.Message, "模型") {
		t.Fatalf("Create error = %v, want model required", appErr)
	}
}

func TestListDoesNotDefaultBlankEngineTypeFilter(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, secretbox.New([]byte("12345678901234567890123456789012")), nil)

	_, appErr := service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20})
	if appErr != nil {
		t.Fatalf("expected list to succeed, got %v", appErr)
	}
	if repo.listQuery.EngineType != "" {
		t.Fatalf("blank engine_type filter must stay blank, got %q", repo.listQuery.EngineType)
	}
}

func TestListRejectsInvalidStoredProviderStateInsteadOfInventingDTOFallback(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	validProvider := Provider{ID: 1, Name: "OpenAI", EngineType: "openai", HealthStatus: provider.HealthUnknown, LastModelSyncStatus: provider.HealthUnknown, Status: 1, CreatedAt: now, UpdatedAt: now}
	validModels := []ProviderModel{{ProviderID: 1, ModelID: "gpt-4.1-mini", Status: 1, CreatedAt: now, UpdatedAt: now}}
	cases := []struct {
		name     string
		row      Provider
		models   []ProviderModel
		errorMsg string
	}{
		{name: "blank engine_type", row: func() Provider { row := validProvider; row.EngineType = ""; return row }(), models: validModels, errorMsg: "AI供应商数据异常"},
		{name: "blank health_status", row: func() Provider { row := validProvider; row.HealthStatus = ""; return row }(), models: validModels, errorMsg: "AI供应商数据异常"},
		{name: "blank last_model_sync_status", row: func() Provider { row := validProvider; row.LastModelSyncStatus = ""; return row }(), models: validModels, errorMsg: "AI供应商数据异常"},
		{name: "invalid provider status", row: func() Provider { row := validProvider; row.Status = 99; return row }(), models: validModels, errorMsg: "AI供应商数据异常"},
		{name: "invalid model status", row: validProvider, models: []ProviderModel{{ProviderID: 1, ModelID: "gpt-4.1-mini", Status: 99, CreatedAt: now, UpdatedAt: now}}, errorMsg: "AI供应商模型数据异常"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepository{
				rows:             []Provider{tc.row},
				total:            1,
				modelsByProvider: map[uint64][]ProviderModel{1: tc.models},
			}
			service := NewService(repo, secretbox.New([]byte("12345678901234567890123456789012")), nil)

			got, appErr := service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20})
			if appErr == nil || appErr.Message != tc.errorMsg {
				t.Fatalf("expected %q error, got response=%#v error=%#v", tc.errorMsg, got, appErr)
			}
		})
	}
}

func TestCreateRequiresCanonicalEngineTypeInsteadOfDriverFallback(t *testing.T) {
	service := NewService(&fakeRepository{}, secretbox.New([]byte("12345678901234567890123456789012")), nil)

	_, appErr := service.Create(context.Background(), CreateInput{
		Name:     "OpenAI",
		APIKey:   "sk-test",
		ModelIDs: []string{"gpt-4.1-mini"},
		Status:   1,
	})
	if appErr == nil || appErr.Message != "AI驱动不能为空" {
		t.Fatalf("expected canonical engine_type error, got %#v", appErr)
	}
}

func TestPreviewModelsRequiresCanonicalEngineTypeInsteadOfDriverFallback(t *testing.T) {
	driver := &fakeModelDriver{}
	service := NewServiceWithDriver(&fakeRepository{}, secretbox.New([]byte("12345678901234567890123456789012")), nil, driver)

	_, appErr := service.PreviewModels(context.Background(), ModelOptionsInput{
		APIKey: "sk-test",
	})
	if appErr == nil || appErr.Message != "AI驱动不能为空" {
		t.Fatalf("expected canonical engine_type error, got %#v", appErr)
	}
	if driver.config.Driver != "" {
		t.Fatalf("driver should not be called for missing canonical engine_type, got %#v", driver.config)
	}
}

func TestProviderContractDoesNotExposeDriverAlias(t *testing.T) {
	for _, file := range []string{"dto.go", "service.go", "transport/admin/request.go"} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		source := string(raw)
		for _, forbidden := range []string{`json:"driver"`, "DriverName", "driverFromInput(", "normalizeDriver("} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("%s must not contain provider driver alias %q", file, forbidden)
			}
		}
	}
}

func TestCreatePersistsSelectedModels(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, secretbox.New([]byte("12345678901234567890123456789012")), nil)

	id, appErr := service.Create(context.Background(), CreateInput{
		Name:              "OpenAI",
		EngineType:        "openai",
		APIKey:            "sk-test",
		ModelIDs:          []string{"gpt-4.1-mini", "gpt-4.1", "gpt-4.1-mini"},
		ModelDisplayNames: map[string]string{"gpt-4.1-mini": "默认轻量模型"},
		Status:            1,
	})
	if appErr != nil {
		t.Fatalf("Create error = %v", appErr)
	}
	if id != 11 {
		t.Fatalf("id = %d, want 11", id)
	}
	if repo.replacedProviderID != 11 {
		t.Fatalf("replaced provider id = %d, want 11", repo.replacedProviderID)
	}
	if len(repo.replacedModels) != 2 {
		t.Fatalf("model count = %d, want 2: %#v", len(repo.replacedModels), repo.replacedModels)
	}
	if repo.replacedModels[0].DisplayName != "默认轻量模型" {
		t.Fatalf("display name not persisted: %#v", repo.replacedModels)
	}
	encoded, err := json.Marshal(repo.replacedModels)
	if err != nil {
		t.Fatalf("marshal replaced models: %v", err)
	}
	body := string(encoded)
	for _, forbidden := range []string{"RawJSON", "Source", "IsDel", "CreatedBy", "UpdatedBy"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("provider model snapshot must not carry fake metadata field %s: %s", forbidden, body)
		}
	}
}

func TestCreateNormalizesEncryptsAndMasksAPIKey(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, secretbox.New([]byte("12345678901234567890123456789012")), nil)

	id, appErr := service.Create(context.Background(), CreateInput{Name: " OpenAI ", EngineType: "openai", BaseURL: " https://api.openai.test/v1/ ", APIKey: "plain-secret-key", ModelIDs: []string{"gpt-4.1-mini"}, Status: 1})
	if appErr != nil {
		t.Fatalf("expected create to succeed, got %v", appErr)
	}
	if id != 11 || repo.created == nil {
		t.Fatalf("expected created row, id=%d row=%#v", id, repo.created)
	}
	if repo.created.Name != "OpenAI" || repo.created.EngineType != "openai" || repo.created.BaseURL != "https://api.openai.test/v1" {
		t.Fatalf("fields were not normalized: %#v", repo.created)
	}
	if repo.created.APIKeyEnc == "" || repo.created.APIKeyEnc == "plain-secret-key" || repo.created.APIKeyHint != "***-key" {
		t.Fatalf("api key was not encrypted safely: %#v", repo.created)
	}
	encoded, err := json.Marshal(repo.created)
	if err != nil {
		t.Fatalf("marshal created provider: %v", err)
	}
	body := string(encoded)
	for _, forbidden := range []string{"ConfigJSON", "CreatedBy", "UpdatedBy"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("provider connection must not carry fake/reserved field %s: %s", forbidden, body)
		}
	}
}

func TestCreateAppendsOpenAIVersionPathForOriginOnlyBaseURL(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, secretbox.New([]byte("12345678901234567890123456789012")), nil)

	_, appErr := service.Create(context.Background(), CreateInput{Name: "Local OpenAI", EngineType: "openai", BaseURL: " http://host.docker.internal:8317/ ", APIKey: "plain-secret-key", ModelIDs: []string{"gpt-5.4"}, Status: 1})
	if appErr != nil {
		t.Fatalf("expected create to succeed, got %v", appErr)
	}
	if repo.created == nil || repo.created.BaseURL != "http://host.docker.internal:8317/v1" {
		t.Fatalf("base url was not normalized to OpenAI v1 endpoint: %#v", repo.created)
	}
}

func TestPreviewModelsAppendsOpenAIVersionPathForOriginOnlyBaseURL(t *testing.T) {
	driver := &fakeModelDriver{}
	service := NewServiceWithDriver(&fakeRepository{}, secretbox.New([]byte("12345678901234567890123456789012")), nil, driver)

	_, appErr := service.PreviewModels(context.Background(), ModelOptionsInput{EngineType: "openai", BaseURL: "http://host.docker.internal:8317", APIKey: "plain-secret-key"})
	if appErr != nil {
		t.Fatalf("expected model preview to succeed, got %v", appErr)
	}
	if driver.config.BaseURL != "http://host.docker.internal:8317/v1" {
		t.Fatalf("preview models used base url %q, want /v1 normalized", driver.config.BaseURL)
	}
}

func TestListDTOExcludesEncryptedAndPlainAPIKey(t *testing.T) {
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	repo := &fakeRepository{
		rows:             []Provider{{ID: 1, Name: "OpenAI", EngineType: "openai", BaseURL: "", APIKeyEnc: "cipher-secret", APIKeyHint: "***cret", HealthStatus: "ok", LastModelSyncStatus: "unknown", Status: 1, CreatedAt: now, UpdatedAt: now}},
		total:            1,
		modelsByProvider: map[uint64][]ProviderModel{1: {{ProviderID: 1, ModelID: "gpt-4.1-mini", Status: 1}}},
	}
	service := NewService(repo, secretbox.New([]byte("12345678901234567890123456789012")), nil)

	got, appErr := service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20})
	if appErr != nil {
		t.Fatalf("expected list to succeed, got %v", appErr)
	}
	if len(got.List) != 1 || got.List[0].APIKeyMasked != "***cret" || got.List[0].Name != "OpenAI" {
		t.Fatalf("unexpected list response: %#v", got)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	body := string(encoded)
	if strings.Contains(body, "api_key_enc") || strings.Contains(body, "cipher-secret") || strings.Contains(body, "plain-secret") || strings.Contains(body, "api_key\"") {
		t.Fatalf("list response leaked api key data: %s", body)
	}
	if strings.Contains(body, "default_model_id") || strings.Contains(body, "is_default") {
		t.Fatalf("provider response must not expose default model concept: %s", body)
	}
	if strings.Contains(body, `"driver"`) || strings.Contains(body, `"driver_name"`) {
		t.Fatalf("provider response must not expose driver alias fields: %s", body)
	}
	for _, forbidden := range []string{`"source"`, `"raw"`, `"config_json"`} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("provider model response must not expose unused metadata field %s: %s", forbidden, body)
		}
	}
}

func TestUpdateBlankAPIKeyKeepsExistingEncryptedKey(t *testing.T) {
	repo := &fakeRepository{rowByID: map[uint64]Provider{5: {ID: 5, Name: "Old", EngineType: "openai", BaseURL: "", APIKeyEnc: "cipher-old", APIKeyHint: "***old", Status: 1}}}
	service := NewService(repo, secretbox.New([]byte("12345678901234567890123456789012")), nil)

	appErr := service.Update(context.Background(), 5, UpdateInput{Name: "New", EngineType: "openai", BaseURL: "", ModelIDs: []string{"gpt-4.1-mini"}, Status: 1})
	if appErr != nil {
		t.Fatalf("expected update to succeed, got %v", appErr)
	}
	if len(repo.updates) != 1 {
		t.Fatalf("expected one update, got %#v", repo.updates)
	}
	if _, ok := repo.updates[0]["api_key_enc"]; ok {
		t.Fatalf("blank api key must not overwrite encrypted key: %#v", repo.updates[0])
	}
	if _, ok := repo.updates[0]["api_key_hint"]; ok {
		t.Fatalf("blank api key must not overwrite key hint: %#v", repo.updates[0])
	}
}

func TestCreateRejectsDuplicateTypeName(t *testing.T) {
	service := NewService(&fakeRepository{exists: true}, secretbox.New([]byte("12345678901234567890123456789012")), nil)

	_, appErr := service.Create(context.Background(), CreateInput{Name: "OpenAI", EngineType: "openai", APIKey: "sk-test", ModelIDs: []string{"gpt-4.1-mini"}, Status: 1})
	if appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest || appErr.Message != "该驱动下已存在同名供应商" {
		t.Fatalf("expected duplicate error, got %#v", appErr)
	}
}

func TestPreviewStoredModelsUsesSavedEncryptedKey(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	cipher, err := box.Encrypt("plain-secret-key")
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	repo := &fakeRepository{rowByID: map[uint64]Provider{5: {ID: 5, Name: "OpenAI", EngineType: "openai", BaseURL: "https://api.openai.test/v1", APIKeyEnc: cipher, Status: 1}}}
	driver := &fakeModelDriver{}
	service := NewServiceWithDriver(repo, box, nil, driver)

	result, appErr := service.PreviewStoredModels(context.Background(), 5)
	if appErr != nil {
		t.Fatalf("expected stored model preview to succeed, got %v", appErr)
	}
	if result == nil || len(result.List) != 1 || result.List[0].ModelID != "gpt-4.1-mini" {
		t.Fatalf("unexpected model preview result: %#v", result)
	}
	if driver.config.APIKey != "plain-secret-key" || driver.config.BaseURL != "https://api.openai.test/v1" || driver.config.Driver != "openai" {
		t.Fatalf("stored preview did not use saved provider config: %#v", driver.config)
	}
	if len(repo.updates) != 0 {
		t.Fatalf("stored preview must not write sync/health state: %#v", repo.updates)
	}
}

func TestTestConnectionDecryptsSecretAndUpdatesHealth(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	cipher, err := box.Encrypt("plain-secret-key")
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	repo := &fakeRepository{rowByID: map[uint64]Provider{5: {ID: 5, Name: "OpenAI", EngineType: "openai", BaseURL: "", APIKeyEnc: cipher, Status: 1}}}
	tester := &fakeTester{}
	driver := &fakeModelDriver{}
	service := NewServiceWithDriver(repo, box, tester, driver)

	result, appErr := service.TestConnection(context.Background(), 5)
	if appErr != nil {
		t.Fatalf("expected test to succeed, got %v", appErr)
	}
	if result == nil || !result.OK || driver.config.APIKey != "plain-secret-key" || driver.config.BaseURL != "" || driver.config.Driver != "openai" {
		t.Fatalf("unexpected test result/input: result=%#v input=%#v", result, tester.input)
	}
	if len(repo.updates) != 1 || repo.updates[0]["health_status"] != "ok" {
		t.Fatalf("expected health update, got %#v", repo.updates)
	}
}

func TestTestConnectionAppendsOpenAIVersionPathForStoredOriginOnlyBaseURL(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	cipher, err := box.Encrypt("plain-secret-key")
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	repo := &fakeRepository{rowByID: map[uint64]Provider{5: {ID: 5, Name: "OpenAI", EngineType: "openai", BaseURL: "http://host.docker.internal:8317", APIKeyEnc: cipher, Status: 1}}}
	driver := &fakeModelDriver{}
	service := NewServiceWithDriver(repo, box, nil, driver)

	_, appErr := service.TestConnection(context.Background(), 5)
	if appErr != nil {
		t.Fatalf("expected test connection to succeed, got %v", appErr)
	}
	if driver.config.BaseURL != "http://host.docker.internal:8317/v1" {
		t.Fatalf("test connection used base url %q, want /v1 normalized", driver.config.BaseURL)
	}
}

func TestTestConnectionRejectsDisabledConnection(t *testing.T) {
	service := NewService(&fakeRepository{rowByID: map[uint64]Provider{5: {ID: 5, Name: "OpenAI", EngineType: "openai", BaseURL: "", Status: 2}}}, secretbox.New([]byte("12345678901234567890123456789012")), &fakeTester{})

	_, appErr := service.TestConnection(context.Background(), 5)
	if appErr == nil || appErr.Message != "AI供应商已禁用" {
		t.Fatalf("expected disabled error, got %#v", appErr)
	}
}

func TestTestConnectionReportsHealthUpdateFailure(t *testing.T) {
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	cipher, err := box.Encrypt("plain-secret-key")
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	repo := &fakeRepository{
		rowByID:   map[uint64]Provider{5: {ID: 5, Name: "OpenAI", EngineType: "openai", BaseURL: "", APIKeyEnc: cipher, Status: 1}},
		updateErr: errors.New("table not set"),
	}
	service := NewServiceWithDriver(repo, box, nil, &fakeModelDriver{})

	_, appErr := service.TestConnection(context.Background(), 5)
	if appErr == nil || appErr.Message != "更新AI供应商健康状态失败" || !errors.Is(appErr.Cause, repo.updateErr) {
		t.Fatalf("expected wrapped health update error, got %#v", appErr)
	}
}

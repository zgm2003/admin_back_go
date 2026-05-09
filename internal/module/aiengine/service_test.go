package aiengine

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	platformai "admin_back_go/internal/platform/ai"
	"admin_back_go/internal/platform/ai/provider"
	"admin_back_go/internal/platform/secretbox"
)

type fakeRepository struct {
	rows               []Connection
	total              int64
	rowByID            map[uint64]Connection
	exists             bool
	created            *Connection
	updates            []map[string]any
	statusID           uint64
	status             int
	deletedID          uint64
	updateErr          error
	modelsByProvider   map[uint64][]ProviderModel
	replacedProviderID uint64
	replacedModels     []ProviderModel
}

func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]Connection, int64, error) {
	return f.rows, f.total, nil
}

func (f *fakeRepository) Get(ctx context.Context, id uint64) (*Connection, error) {
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

func (f *fakeRepository) Create(ctx context.Context, row Connection) (uint64, error) {
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
	return []provider.Model{{ID: "gpt-4.1-mini", Object: "model", OwnedBy: "openai", Raw: map[string]any{"id": "gpt-4.1-mini"}}}, nil
}

func (f *fakeModelDriver) TestConnection(ctx context.Context, cfg provider.Config) (*provider.TestResult, error) {
	f.config = cfg
	if f.err != nil {
		return &provider.TestResult{OK: false, Status: provider.HealthFailed, Message: f.err.Error()}, f.err
	}
	return &provider.TestResult{OK: true, Status: provider.HealthOK, LatencyMs: 12, Message: "ok", ModelCount: 1}, nil
}

type fakeTester struct {
	input platformai.TestConnectionInput
	err   error
}

func (f *fakeTester) TestConnection(ctx context.Context, input platformai.TestConnectionInput) (*platformai.TestConnectionResult, error) {
	f.input = input
	if f.err != nil {
		return &platformai.TestConnectionResult{OK: false, Status: "500", Message: f.err.Error()}, f.err
	}
	return &platformai.TestConnectionResult{OK: true, Status: "200 OK", LatencyMs: 12, Message: "ok"}, nil
}

func TestInitOnlyReturnsOpenAIDriver(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, secretbox.New("vault-key"), nil)

	result, appErr := service.Init(context.Background())
	if appErr != nil {
		t.Fatalf("Init error = %v", appErr)
	}
	if len(result.Dict.EngineTypeArr) != 1 || result.Dict.EngineTypeArr[0].Value != "openai" {
		t.Fatalf("driver options = %+v, want openai only", result.Dict.EngineTypeArr)
	}
}

func TestCreateRequiresAPIKeyAndModels(t *testing.T) {
	service := NewService(&fakeRepository{}, secretbox.New("vault-key"), nil)

	_, appErr := service.Create(context.Background(), CreateInput{Name: "OpenAI", EngineType: "openai", Status: 1})
	if appErr == nil || !strings.Contains(appErr.Message, "API Key") {
		t.Fatalf("Create error = %v, want API Key required", appErr)
	}
	_, appErr = service.Create(context.Background(), CreateInput{Name: "OpenAI", EngineType: "openai", APIKey: "sk-test", Status: 1})
	if appErr == nil || !strings.Contains(appErr.Message, "模型") {
		t.Fatalf("Create error = %v, want model required", appErr)
	}
}

func TestCreatePersistsSelectedModels(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, secretbox.New("vault-key"), nil)

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
	for _, model := range repo.replacedModels {
		if model.RawJSON != nil {
			t.Fatalf("empty model raw json must be persisted as NULL, got %#v", model.RawJSON)
		}
	}
}

func TestCreateNormalizesEncryptsAndMasksAPIKey(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, secretbox.New("vault-key"), nil)

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
	if repo.created.ConfigJSON != nil {
		t.Fatalf("empty config json must be persisted as NULL, got %#v", repo.created.ConfigJSON)
	}
	if repo.created.APIKeyEnc == "" || repo.created.APIKeyEnc == "plain-secret-key" || repo.created.APIKeyHint != "***-key" {
		t.Fatalf("api key was not encrypted safely: %#v", repo.created)
	}
}

func TestListDTOExcludesEncryptedAndPlainAPIKey(t *testing.T) {
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	repo := &fakeRepository{
		rows:             []Connection{{ID: 1, Name: "OpenAI", EngineType: "openai", BaseURL: "", APIKeyEnc: "cipher-secret", APIKeyHint: "***cret", HealthStatus: "ok", Status: 1, CreatedAt: now, UpdatedAt: now}},
		total:            1,
		modelsByProvider: map[uint64][]ProviderModel{1: {{ProviderID: 1, ModelID: "gpt-4.1-mini", Status: 1}}},
	}
	service := NewService(repo, secretbox.New("vault-key"), nil)

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
}

func TestUpdateBlankAPIKeyKeepsExistingEncryptedKey(t *testing.T) {
	repo := &fakeRepository{rowByID: map[uint64]Connection{5: {ID: 5, Name: "Old", EngineType: "openai", BaseURL: "", APIKeyEnc: "cipher-old", APIKeyHint: "***old", Status: 1}}}
	service := NewService(repo, secretbox.New("vault-key"), nil)

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
	service := NewService(&fakeRepository{exists: true}, secretbox.New("vault-key"), nil)

	_, appErr := service.Create(context.Background(), CreateInput{Name: "OpenAI", EngineType: "openai", APIKey: "sk-test", ModelIDs: []string{"gpt-4.1-mini"}, Status: 1})
	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "该驱动下已存在同名供应商" {
		t.Fatalf("expected duplicate error, got %#v", appErr)
	}
}

func TestPreviewStoredModelsUsesSavedEncryptedKey(t *testing.T) {
	box := secretbox.New("vault-key")
	cipher, err := box.Encrypt("plain-secret-key")
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	repo := &fakeRepository{rowByID: map[uint64]Connection{5: {ID: 5, Name: "OpenAI", EngineType: "openai", BaseURL: "https://api.openai.test/v1", APIKeyEnc: cipher, Status: 1}}}
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
	box := secretbox.New("vault-key")
	cipher, err := box.Encrypt("plain-secret-key")
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	repo := &fakeRepository{rowByID: map[uint64]Connection{5: {ID: 5, Name: "OpenAI", EngineType: "openai", BaseURL: "", APIKeyEnc: cipher, Status: 1}}}
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

func TestTestConnectionRejectsDisabledConnection(t *testing.T) {
	service := NewService(&fakeRepository{rowByID: map[uint64]Connection{5: {ID: 5, Name: "OpenAI", EngineType: "openai", BaseURL: "", Status: 2}}}, secretbox.New("vault-key"), &fakeTester{})

	_, appErr := service.TestConnection(context.Background(), 5)
	if appErr == nil || appErr.Message != "AI供应商已禁用" {
		t.Fatalf("expected disabled error, got %#v", appErr)
	}
}

func TestTestConnectionReportsHealthUpdateFailure(t *testing.T) {
	box := secretbox.New("vault-key")
	cipher, err := box.Encrypt("plain-secret-key")
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	repo := &fakeRepository{
		rowByID:   map[uint64]Connection{5: {ID: 5, Name: "OpenAI", EngineType: "openai", BaseURL: "", APIKeyEnc: cipher, Status: 1}},
		updateErr: errors.New("table not set"),
	}
	service := NewServiceWithDriver(repo, box, nil, &fakeModelDriver{})

	_, appErr := service.TestConnection(context.Background(), 5)
	if appErr == nil || appErr.Message != "更新AI供应商健康状态失败" || !errors.Is(appErr.Cause, repo.updateErr) {
		t.Fatalf("expected wrapped health update error, got %#v", appErr)
	}
}

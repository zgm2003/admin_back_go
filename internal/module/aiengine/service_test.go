package aiengine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	platformai "admin_back_go/internal/platform/ai"
	"admin_back_go/internal/platform/secretbox"
)

type fakeRepository struct {
	rows      []Connection
	total     int64
	rowByID   map[uint64]Connection
	exists    bool
	created   *Connection
	updates   []map[string]any
	statusID  uint64
	status    int
	deletedID uint64
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

func TestCreateNormalizesEncryptsAndMasksAPIKey(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, secretbox.New("vault-key"), nil)

	id, appErr := service.Create(context.Background(), CreateInput{Name: " Dify ", EngineType: "dify", BaseURL: " https://api.dify.test/v1/ ", APIKey: "plain-secret-key", WorkspaceID: " ws ", Status: 1})
	if appErr != nil {
		t.Fatalf("expected create to succeed, got %v", appErr)
	}
	if id != 11 || repo.created == nil {
		t.Fatalf("expected created row, id=%d row=%#v", id, repo.created)
	}
	if repo.created.Name != "Dify" || repo.created.EngineType != "dify" || repo.created.BaseURL != "https://api.dify.test/v1" || repo.created.WorkspaceID != "ws" {
		t.Fatalf("fields were not normalized: %#v", repo.created)
	}
	if repo.created.APIKeyEnc == "" || repo.created.APIKeyEnc == "plain-secret-key" || repo.created.APIKeyHint != "***-key" {
		t.Fatalf("api key was not encrypted safely: %#v", repo.created)
	}
}

func TestListDTOExcludesEncryptedAndPlainAPIKey(t *testing.T) {
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	repo := &fakeRepository{rows: []Connection{{ID: 1, Name: "Dify", EngineType: "dify", BaseURL: "https://api.dify.test", APIKeyEnc: "cipher-secret", APIKeyHint: "***cret", HealthStatus: "ok", Status: 1, CreatedAt: now, UpdatedAt: now}}, total: 1}
	service := NewService(repo, secretbox.New("vault-key"), nil)

	got, appErr := service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20})
	if appErr != nil {
		t.Fatalf("expected list to succeed, got %v", appErr)
	}
	if len(got.List) != 1 || got.List[0].APIKeyMasked != "***cret" || got.List[0].Name != "Dify" {
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
}

func TestUpdateBlankAPIKeyKeepsExistingEncryptedKey(t *testing.T) {
	repo := &fakeRepository{rowByID: map[uint64]Connection{5: {ID: 5, Name: "Old", EngineType: "dify", BaseURL: "https://old.test", APIKeyEnc: "cipher-old", APIKeyHint: "***old", Status: 1}}}
	service := NewService(repo, secretbox.New("vault-key"), nil)

	appErr := service.Update(context.Background(), 5, UpdateInput{Name: "New", EngineType: "dify", BaseURL: "https://new.test", Status: 1})
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

	_, appErr := service.Create(context.Background(), CreateInput{Name: "Dify", EngineType: "dify", BaseURL: "https://api.dify.test", Status: 1})
	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "该引擎类型下已存在同名供应商" {
		t.Fatalf("expected duplicate error, got %#v", appErr)
	}
}

func TestTestConnectionDecryptsSecretAndUpdatesHealth(t *testing.T) {
	box := secretbox.New("vault-key")
	cipher, err := box.Encrypt("plain-secret-key")
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	repo := &fakeRepository{rowByID: map[uint64]Connection{5: {ID: 5, Name: "Dify", EngineType: "dify", BaseURL: "https://api.dify.test", APIKeyEnc: cipher, Status: 1}}}
	tester := &fakeTester{}
	service := NewService(repo, box, tester)

	result, appErr := service.TestConnection(context.Background(), 5)
	if appErr != nil {
		t.Fatalf("expected test to succeed, got %v", appErr)
	}
	if result == nil || !result.OK || tester.input.APIKey != "plain-secret-key" || tester.input.BaseURL != "https://api.dify.test" || tester.input.EngineType != platformai.EngineTypeDify {
		t.Fatalf("unexpected test result/input: result=%#v input=%#v", result, tester.input)
	}
	if len(repo.updates) != 1 || repo.updates[0]["health_status"] != "ok" {
		t.Fatalf("expected health update, got %#v", repo.updates)
	}
}

func TestTestConnectionRejectsDisabledConnection(t *testing.T) {
	service := NewService(&fakeRepository{rowByID: map[uint64]Connection{5: {ID: 5, Name: "Dify", EngineType: "dify", BaseURL: "https://api.dify.test", Status: 2}}}, secretbox.New("vault-key"), &fakeTester{})

	_, appErr := service.TestConnection(context.Background(), 5)
	if appErr == nil || appErr.Message != "AI供应商已禁用" {
		t.Fatalf("expected disabled error, got %#v", appErr)
	}
}

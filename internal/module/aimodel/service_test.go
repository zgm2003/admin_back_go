package aimodel

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
	rows      []Model
	total     int64
	rowByID   map[int64]Model
	exists    bool
	created   *Model
	updates   []map[string]any
	statusID  int64
	status    int
	deletedID int64
}

func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]Model, int64, error) {
	return f.rows, f.total, nil
}

func (f *fakeRepository) Get(ctx context.Context, id int64) (*Model, error) {
	if f.rowByID == nil {
		return nil, nil
	}
	row, ok := f.rowByID[id]
	if !ok {
		return nil, nil
	}
	return &row, nil
}

func (f *fakeRepository) ExistsByDriverName(ctx context.Context, driver string, name string, excludeID int64) (bool, error) {
	return f.exists, nil
}

func (f *fakeRepository) Create(ctx context.Context, row Model) (int64, error) {
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

func TestInitReturnsAIDicts(t *testing.T) {
	service := NewService(&fakeRepository{}, secretbox.New("vault-key"))

	got, appErr := service.Init(context.Background())
	if appErr != nil {
		t.Fatalf("expected init to succeed, got %v", appErr)
	}
	if len(got.Dict.AIDriverArr) != len(enum.AIDrivers) || got.Dict.AIDriverArr[0].Value != enum.AIDriverOpenAI {
		t.Fatalf("unexpected driver dict: %#v", got.Dict.AIDriverArr)
	}
	if len(got.Dict.CommonStatusArr) != 2 || got.Dict.CommonStatusArr[0].Value != enum.CommonYes {
		t.Fatalf("unexpected status dict: %#v", got.Dict.CommonStatusArr)
	}
}

func TestCreateRejectsInvalidDriver(t *testing.T) {
	service := NewService(&fakeRepository{}, secretbox.New("vault-key"))

	_, appErr := service.Create(context.Background(), CreateInput{Name: "bad", Driver: "goods", ModelCode: "x"})
	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "无效的AI驱动" {
		t.Fatalf("expected invalid driver error, got %#v", appErr)
	}
}

func TestCreateRejectsDuplicateDriverName(t *testing.T) {
	service := NewService(&fakeRepository{exists: true}, secretbox.New("vault-key"))

	_, appErr := service.Create(context.Background(), CreateInput{Name: "GPT", Driver: enum.AIDriverOpenAI, ModelCode: "gpt-5.5"})
	if appErr == nil || appErr.Message != "该驱动下已存在同名模型" {
		t.Fatalf("expected duplicate error, got %#v", appErr)
	}
}

func TestCreateNormalizesAndEncryptsAPIKey(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, secretbox.New("vault-key"))

	id, appErr := service.Create(context.Background(), CreateInput{Name: " GPT ", Driver: enum.AIDriverOpenAI, ModelCode: " gpt-5.5 ", Endpoint: " https://api.example.test ", APIKey: "plain-secret-key", Status: enum.CommonYes})
	if appErr != nil {
		t.Fatalf("expected create to succeed, got %v", appErr)
	}
	if id != 11 || repo.created == nil {
		t.Fatalf("expected created row, id=%d row=%#v", id, repo.created)
	}
	if repo.created.Name != "GPT" || repo.created.ModelCode != "gpt-5.5" || repo.created.Endpoint != "https://api.example.test" {
		t.Fatalf("fields were not normalized: %#v", repo.created)
	}
	if repo.created.APIKeyEnc == "" || repo.created.APIKeyEnc == "plain-secret-key" || repo.created.APIKeyHint != "***-key" {
		t.Fatalf("API key was not encrypted/hinted safely: %#v", repo.created)
	}
}

func TestCreateAPIKeyFailsWithoutVaultKey(t *testing.T) {
	service := NewService(&fakeRepository{}, secretbox.New(""))

	_, appErr := service.Create(context.Background(), CreateInput{Name: "GPT", Driver: enum.AIDriverOpenAI, ModelCode: "gpt", APIKey: "plain"})
	if appErr == nil || appErr.Code != apperror.CodeInternal || !strings.Contains(appErr.Message, "加密AI模型API Key失败") {
		t.Fatalf("expected explicit encryption error, got %#v", appErr)
	}
}

func TestUpdateBlankAPIKeyKeepsExistingEncryptedKey(t *testing.T) {
	repo := &fakeRepository{rowByID: map[int64]Model{5: {ID: 5, Name: "old", Driver: enum.AIDriverOpenAI, ModelCode: "gpt", APIKeyEnc: "cipher-old", APIKeyHint: "***old", Status: enum.CommonYes}}}
	service := NewService(repo, secretbox.New("vault-key"))

	appErr := service.Update(context.Background(), 5, UpdateInput{Name: "new", Driver: enum.AIDriverOpenAI, ModelCode: "gpt", Status: enum.CommonYes})
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

func TestListDTOExcludesAPIKeyCiphertext(t *testing.T) {
	createdAt := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	repo := &fakeRepository{rows: []Model{{ID: 1, Name: "OpenAI", Driver: enum.AIDriverOpenAI, ModelCode: "gpt-5.5", Endpoint: "https://api.example.test", APIKeyEnc: "cipher-secret", APIKeyHint: "***cret", Status: enum.CommonYes, CreatedAt: createdAt, UpdatedAt: createdAt}}, total: 1}
	service := NewService(repo, secretbox.New("vault-key"))

	got, appErr := service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20})
	if appErr != nil {
		t.Fatalf("expected list to succeed, got %v", appErr)
	}
	if len(got.List) != 1 || got.List[0].DriverName != "OpenAI" || got.List[0].APIKeyHint != "***cret" {
		t.Fatalf("unexpected list response: %#v", got)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal list response: %v", err)
	}
	body := string(encoded)
	if strings.Contains(body, "api_key_enc") || strings.Contains(body, "cipher-secret") || strings.Contains(body, "plain-secret") || strings.Contains(body, "api_key\"") {
		t.Fatalf("list response leaked API key data: %s", body)
	}
}

func TestChangeStatusRejectsInvalidStatus(t *testing.T) {
	service := NewService(&fakeRepository{rowByID: map[int64]Model{5: {ID: 5}}}, secretbox.New("vault-key"))

	appErr := service.ChangeStatus(context.Background(), 5, 9)
	if appErr == nil || appErr.Message != "无效的状态" {
		t.Fatalf("expected status error, got %#v", appErr)
	}
}

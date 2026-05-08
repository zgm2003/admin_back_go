package aiapp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	platformai "admin_back_go/internal/platform/ai"
	"admin_back_go/internal/platform/secretbox"
)

type fakeAIAppRepository struct {
	rows              []AppWithEngine
	total             int64
	rawByID           map[uint64]App
	rowByID           map[uint64]AppWithEngine
	activeConnections map[uint64]EngineConnection
	connections       []EngineConnection
	existsCode        bool
	existsBinding     bool
	created           *App
	updates           []map[string]any
	statusID          uint64
	status            int
	deletedID         uint64
	bindings          []Binding
	createdBinding    *Binding
	deletedBindingID  uint64
	visibleApps       []App
}

func (f *fakeAIAppRepository) List(ctx context.Context, query ListQuery) ([]AppWithEngine, int64, error) {
	return f.rows, f.total, nil
}

func (f *fakeAIAppRepository) Get(ctx context.Context, id uint64) (*AppWithEngine, error) {
	if f.rowByID == nil {
		return nil, nil
	}
	row, ok := f.rowByID[id]
	if !ok {
		return nil, nil
	}
	return &row, nil
}

func (f *fakeAIAppRepository) GetRaw(ctx context.Context, id uint64) (*App, error) {
	if f.rawByID == nil {
		return nil, nil
	}
	row, ok := f.rawByID[id]
	if !ok {
		return nil, nil
	}
	return &row, nil
}

func (f *fakeAIAppRepository) ListActiveConnections(ctx context.Context) ([]EngineConnection, error) {
	return f.connections, nil
}

func (f *fakeAIAppRepository) GetActiveConnection(ctx context.Context, id uint64) (*EngineConnection, error) {
	if f.activeConnections == nil {
		return nil, nil
	}
	row, ok := f.activeConnections[id]
	if !ok {
		return nil, nil
	}
	return &row, nil
}

func (f *fakeAIAppRepository) ExistsByCode(ctx context.Context, code string, excludeID uint64) (bool, error) {
	return f.existsCode, nil
}

func (f *fakeAIAppRepository) Create(ctx context.Context, row App) (uint64, error) {
	f.created = &row
	return 11, nil
}

func (f *fakeAIAppRepository) Update(ctx context.Context, id uint64, fields map[string]any) error {
	f.updates = append(f.updates, fields)
	return nil
}

func (f *fakeAIAppRepository) ChangeStatus(ctx context.Context, id uint64, status int) error {
	f.statusID = id
	f.status = status
	return nil
}

func (f *fakeAIAppRepository) Delete(ctx context.Context, id uint64) error {
	f.deletedID = id
	return nil
}

func (f *fakeAIAppRepository) ListBindings(ctx context.Context, appID uint64) ([]Binding, error) {
	return f.bindings, nil
}

func (f *fakeAIAppRepository) ExistsBinding(ctx context.Context, appID uint64, bindType string, bindKey string, excludeID uint64) (bool, error) {
	return f.existsBinding, nil
}

func (f *fakeAIAppRepository) CreateBinding(ctx context.Context, row Binding) (uint64, error) {
	f.createdBinding = &row
	return 22, nil
}

func (f *fakeAIAppRepository) DeleteBinding(ctx context.Context, id uint64) error {
	f.deletedBindingID = id
	return nil
}

func (f *fakeAIAppRepository) ListVisibleApps(ctx context.Context, query OptionQuery) ([]App, error) {
	return f.visibleApps, nil
}

type fakeAIAppTester struct {
	input platformai.TestConnectionInput
	err   error
}

func (f *fakeAIAppTester) TestConnection(ctx context.Context, input platformai.TestConnectionInput) (*platformai.TestConnectionResult, error) {
	f.input = input
	if f.err != nil {
		return &platformai.TestConnectionResult{OK: false, Status: "500", Message: f.err.Error()}, f.err
	}
	return &platformai.TestConnectionResult{OK: true, Status: "200 OK", Message: "ok"}, nil
}

func TestCreateRejectsMissingActiveEngineConnection(t *testing.T) {
	service := NewService(&fakeAIAppRepository{}, secretbox.New("vault-key"), nil)

	_, appErr := service.Create(context.Background(), CreateInput{
		EngineConnectionID:  99,
		Name:                "客服助手",
		Code:                "support_bot",
		AppType:             "chat",
		DefaultResponseMode: "streaming",
		RuntimeConfig:       map[string]any{},
		Status:              enum.CommonYes,
	})

	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "AI供应商不存在或已禁用" {
		t.Fatalf("expected missing active connection error, got %#v", appErr)
	}
}

func TestListDTOExcludesEncryptedAndPlainAppAPIKey(t *testing.T) {
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	repo := &fakeAIAppRepository{
		rows: []AppWithEngine{{
			App: App{
				ID:                  1,
				EngineConnectionID:  3,
				Name:                "客服助手",
				Code:                "support_bot",
				AppType:             "chat",
				EngineAppID:         "dify-app-id",
				EngineAppAPIKeyEnc:  "cipher-engine-app-key",
				EngineAppAPIKeyHint: "***-key",
				DefaultResponseMode: "streaming",
				RuntimeConfigJSON:   `{"scene":"support"}`,
				Status:              enum.CommonYes,
				IsDel:               enum.CommonNo,
				CreatedAt:           now,
				UpdatedAt:           now,
			},
			EngineConnectionName: "Dify",
			EngineType:           "dify",
		}},
		total: 1,
	}
	service := NewService(repo, secretbox.New("vault-key"), nil)

	got, appErr := service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20})
	if appErr != nil {
		t.Fatalf("expected list to succeed, got %v", appErr)
	}
	if len(got.List) != 1 || got.List[0].EngineAppAPIKeyMasked != "***-key" {
		t.Fatalf("unexpected list response: %#v", got)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal list response: %v", err)
	}
	body := string(encoded)
	for _, forbidden := range []string{"engine_app_api_key_enc", "cipher-engine-app-key", "plain-engine-app-key", "engine_app_api_key\""} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("list response leaked app key data %q in %s", forbidden, body)
		}
	}
}

func TestCreateRejectsDuplicateCode(t *testing.T) {
	repo := &fakeAIAppRepository{
		activeConnections: map[uint64]EngineConnection{1: {ID: 1, Name: "Dify", EngineType: "dify", Status: enum.CommonYes, IsDel: enum.CommonNo}},
		existsCode:        true,
	}
	service := NewService(repo, secretbox.New("vault-key"), nil)

	_, appErr := service.Create(context.Background(), CreateInput{EngineConnectionID: 1, Name: "客服助手", Code: "support_bot", AppType: "chat", DefaultResponseMode: "streaming", Status: enum.CommonYes})

	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "AI应用编码已存在" {
		t.Fatalf("expected duplicate code error, got %#v", appErr)
	}
}

func TestCreateBindingRejectsDuplicateScope(t *testing.T) {
	repo := &fakeAIAppRepository{
		rawByID:       map[uint64]App{7: {ID: 7, Status: enum.CommonYes, IsDel: enum.CommonNo}},
		existsBinding: true,
	}
	service := NewService(repo, secretbox.New("vault-key"), nil)

	_, appErr := service.CreateBinding(context.Background(), 7, BindingInput{BindType: "user", BindKey: "9", Status: enum.CommonYes})

	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "AI应用绑定已存在" {
		t.Fatalf("expected duplicate binding error, got %#v", appErr)
	}
}

func TestOptionsExcludeDisabledApps(t *testing.T) {
	repo := &fakeAIAppRepository{visibleApps: []App{
		{ID: 1, Name: "启用应用", Code: "enabled", Status: enum.CommonYes, IsDel: enum.CommonNo},
		{ID: 2, Name: "禁用应用", Code: "disabled", Status: enum.CommonNo, IsDel: enum.CommonNo},
		{ID: 3, Name: "删除应用", Code: "deleted", Status: enum.CommonYes, IsDel: enum.CommonYes},
	}}
	service := NewService(repo, secretbox.New("vault-key"), nil)

	got, appErr := service.Options(context.Background(), OptionQuery{UserID: 9, Platform: "admin"})
	if appErr != nil {
		t.Fatalf("expected options to succeed, got %v", appErr)
	}
	if len(got.List) != 1 || got.List[0].Value != 1 || got.List[0].Code != "enabled" {
		t.Fatalf("disabled/deleted apps must be excluded, got %#v", got.List)
	}
}

func TestUpdateBlankEngineAppAPIKeyKeepsExistingCiphertext(t *testing.T) {
	repo := &fakeAIAppRepository{
		rawByID:           map[uint64]App{5: {ID: 5, EngineAppAPIKeyEnc: "cipher-old", EngineAppAPIKeyHint: "***old", Status: enum.CommonYes, IsDel: enum.CommonNo}},
		activeConnections: map[uint64]EngineConnection{1: {ID: 1, Name: "Dify", EngineType: "dify", Status: enum.CommonYes, IsDel: enum.CommonNo}},
	}
	service := NewService(repo, secretbox.New("vault-key"), nil)

	appErr := service.Update(context.Background(), 5, UpdateInput{EngineConnectionID: 1, Name: "客服助手", Code: "support_bot", AppType: "chat", DefaultResponseMode: "streaming", Status: enum.CommonYes})
	if appErr != nil {
		t.Fatalf("expected update to succeed, got %v", appErr)
	}
	if len(repo.updates) != 1 {
		t.Fatalf("expected one update, got %#v", repo.updates)
	}
	if _, ok := repo.updates[0]["engine_app_api_key_enc"]; ok {
		t.Fatalf("blank app key must not overwrite ciphertext: %#v", repo.updates[0])
	}
	if _, ok := repo.updates[0]["engine_app_api_key_hint"]; ok {
		t.Fatalf("blank app key must not overwrite key hint: %#v", repo.updates[0])
	}
}

func TestTestDecryptsAppKeyAndUsesActiveEngineConnection(t *testing.T) {
	box := secretbox.New("vault-key")
	cipher, err := box.Encrypt("plain-app-key")
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	repo := &fakeAIAppRepository{
		rowByID: map[uint64]AppWithEngine{5: {
			App: App{ID: 5, EngineConnectionID: 1, EngineAppAPIKeyEnc: cipher, Status: enum.CommonYes, IsDel: enum.CommonNo},
		}},
		activeConnections: map[uint64]EngineConnection{1: {ID: 1, Name: "Dify", EngineType: "dify", BaseURL: "https://api.dify.test/v1", Status: enum.CommonYes, IsDel: enum.CommonNo}},
	}
	tester := &fakeAIAppTester{}
	service := NewService(repo, box, tester)

	result, appErr := service.Test(context.Background(), 5)
	if appErr != nil {
		t.Fatalf("expected test to succeed, got %v", appErr)
	}
	if result == nil || !result.OK {
		t.Fatalf("expected successful test result, got %#v", result)
	}
	if tester.input.APIKey != "plain-app-key" || tester.input.BaseURL != "https://api.dify.test/v1" || tester.input.EngineType != platformai.EngineTypeDify {
		t.Fatalf("unexpected tester input: %#v", tester.input)
	}
}

func TestTestReturnsUpstreamError(t *testing.T) {
	box := secretbox.New("vault-key")
	cipher, err := box.Encrypt("plain-app-key")
	if err != nil {
		t.Fatalf("encrypt fixture: %v", err)
	}
	repo := &fakeAIAppRepository{
		rowByID:           map[uint64]AppWithEngine{5: {App: App{ID: 5, EngineConnectionID: 1, EngineAppAPIKeyEnc: cipher, Status: enum.CommonYes, IsDel: enum.CommonNo}}},
		activeConnections: map[uint64]EngineConnection{1: {ID: 1, Name: "Dify", EngineType: "dify", BaseURL: "https://api.dify.test/v1", Status: enum.CommonYes, IsDel: enum.CommonNo}},
	}
	service := NewService(repo, box, &fakeAIAppTester{err: errors.New("upstream failed")})

	_, appErr := service.Test(context.Background(), 5)
	if appErr == nil || appErr.Code != apperror.CodeInternal || appErr.Message != "测试AI应用失败" {
		t.Fatalf("expected wrapped upstream error, got %#v", appErr)
	}
}

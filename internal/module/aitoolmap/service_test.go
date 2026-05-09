package aitoolmap

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
)

type fakeToolMapRepository struct {
	rows             []ToolMapWithEngine
	total            int64
	rawByID          map[uint64]ToolMap
	activeProviders  map[uint64]Provider
	connections      []Provider
	existsCode       bool
	existsPermission bool
	created          *ToolMap
	updates          []map[string]any
	statusID         uint64
	status           int
	deletedID        uint64
}

func (f *fakeToolMapRepository) List(ctx context.Context, query ListQuery) ([]ToolMapWithEngine, int64, error) {
	return f.rows, f.total, nil
}
func (f *fakeToolMapRepository) GetRaw(ctx context.Context, id uint64) (*ToolMap, error) {
	row, ok := f.rawByID[id]
	if !ok {
		return nil, nil
	}
	return &row, nil
}
func (f *fakeToolMapRepository) ListActiveProviders(ctx context.Context) ([]Provider, error) {
	return f.connections, nil
}
func (f *fakeToolMapRepository) GetActiveProvider(ctx context.Context, id uint64) (*Provider, error) {
	row, ok := f.activeProviders[id]
	if !ok {
		return nil, nil
	}
	return &row, nil
}
func (f *fakeToolMapRepository) ExistsByCode(ctx context.Context, code string, excludeID uint64) (bool, error) {
	return f.existsCode, nil
}
func (f *fakeToolMapRepository) ExistsPermissionCode(ctx context.Context, code string) (bool, error) {
	return f.existsPermission, nil
}
func (f *fakeToolMapRepository) Create(ctx context.Context, row ToolMap) (uint64, error) {
	f.created = &row
	return 11, nil
}
func (f *fakeToolMapRepository) Update(ctx context.Context, id uint64, fields map[string]any) error {
	f.updates = append(f.updates, fields)
	return nil
}
func (f *fakeToolMapRepository) ChangeStatus(ctx context.Context, id uint64, status int) error {
	f.statusID = id
	f.status = status
	return nil
}
func (f *fakeToolMapRepository) Delete(ctx context.Context, id uint64) error {
	f.deletedID = id
	return nil
}

func TestCreateDifyToolStoresEngineToolID(t *testing.T) {
	repo := &fakeToolMapRepository{activeProviders: map[uint64]Provider{3: {ID: 3, EngineType: "dify", Status: enum.CommonYes, IsDel: enum.CommonNo}}}
	service := NewService(repo)

	id, appErr := service.Create(context.Background(), MutationInput{ProviderID: 3, Name: "查订单", Code: "query_order", ToolType: ToolTypeDifyTool, EngineToolID: "tool-1", RiskLevel: RiskLow, ConfigJSON: json.RawMessage(`{"timeout":3}`), Status: enum.CommonYes})
	if appErr != nil {
		t.Fatalf("expected create to succeed, got %v", appErr)
	}
	if id != 11 {
		t.Fatalf("expected id 11, got %d", id)
	}
	if repo.created == nil || repo.created.EngineToolID != "tool-1" || repo.created.ConfigJSON != `{"timeout":3}` || repo.created.RiskLevel != RiskLow {
		t.Fatalf("unexpected created row: %#v", repo.created)
	}
}

func TestCreateAdminActionGatewayRequiresExistingPermissionCode(t *testing.T) {
	repo := &fakeToolMapRepository{activeProviders: map[uint64]Provider{3: {ID: 3, EngineType: "dify", Status: enum.CommonYes, IsDel: enum.CommonNo}}}
	service := NewService(repo)

	_, appErr := service.Create(context.Background(), MutationInput{ProviderID: 3, Name: "删用户", Code: "delete_user", ToolType: ToolTypeAdminActionGateway, RiskLevel: RiskHigh, Status: enum.CommonYes})
	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "admin_action_gateway 必须绑定有效权限编码" {
		t.Fatalf("expected missing permission error, got %#v", appErr)
	}

	_, appErr = service.Create(context.Background(), MutationInput{ProviderID: 3, Name: "删用户", Code: "delete_user", ToolType: ToolTypeAdminActionGateway, PermissionCode: "user_delete", RiskLevel: RiskHigh, Status: enum.CommonYes})
	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "权限编码不存在或已禁用" {
		t.Fatalf("expected unknown permission error, got %#v", appErr)
	}
}

func TestChangeStatusPreservesConfigJSON(t *testing.T) {
	repo := &fakeToolMapRepository{rawByID: map[uint64]ToolMap{7: {ID: 7, ConfigJSON: `{"a":1}`, Status: enum.CommonYes, IsDel: enum.CommonNo}}}
	service := NewService(repo)

	appErr := service.ChangeStatus(context.Background(), 7, enum.CommonNo)
	if appErr != nil {
		t.Fatalf("expected change status to succeed, got %v", appErr)
	}
	if repo.statusID != 7 || repo.status != enum.CommonNo || len(repo.updates) != 0 {
		t.Fatalf("status change must not update config json: statusID=%d status=%d updates=%#v", repo.statusID, repo.status, repo.updates)
	}
}

func TestListDoesNotLeakEngineSecret(t *testing.T) {
	now := time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)
	repo := &fakeToolMapRepository{rows: []ToolMapWithEngine{{ToolMap: ToolMap{ID: 1, ProviderID: 3, Name: "查订单", Code: "query_order", ToolType: ToolTypeDifyTool, EngineToolID: "tool-1", RiskLevel: RiskLow, ConfigJSON: `{"safe":true}`, Status: enum.CommonYes, IsDel: enum.CommonNo, CreatedAt: now, UpdatedAt: now}, ProviderName: "Dify", EngineType: "dify", EngineAPIKeyEnc: "cipher-secret"}}, total: 1}
	service := NewService(repo)

	got, appErr := service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20})
	if appErr != nil {
		t.Fatalf("expected list to succeed, got %v", appErr)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	body := string(encoded)
	for _, forbidden := range []string{"api_key", "api_key_enc", "cipher-secret"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("list response leaked %q in %s", forbidden, body)
		}
	}
	if len(got.List) != 1 || string(got.List[0].ConfigJSON) != `{"safe":true}` {
		t.Fatalf("unexpected list response: %#v", got.List)
	}
}

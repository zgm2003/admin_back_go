package aibilling

import (
	"context"
	"testing"

	"admin_back_go/internal/shared/enum"
)

func TestServiceCreateRuleTrimsSceneAndUnitAndRejectsUnknownUnit(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	id, appErr := service.CreateRule(context.Background(), CreateRuleInput{Scene: "  admin_image_generate  ", Unit: "  image  ", UnitPriceCents: 120, Status: RuleStatusEnabled})
	if appErr != nil {
		t.Fatalf("CreateRule returned error: %v", appErr)
	}
	if id != 101 {
		t.Fatalf("expected id 101, got %d", id)
	}
	if repo.created.Scene != SceneAdminImageGenerate || repo.created.Unit != UnitImage {
		t.Fatalf("expected trimmed scene/unit, got %#v", repo.created)
	}

	_, appErr = service.CreateRule(context.Background(), CreateRuleInput{Scene: SceneAdminImageGenerate, Unit: "token", UnitPriceCents: 1, Status: RuleStatusEnabled})
	if appErr == nil || appErr.MessageID != "aibilling.rule.unit.invalid" {
		t.Fatalf("expected invalid unit keyed error, got %#v", appErr)
	}
}

func TestServiceCreateRuleRejectsNonPositiveUnitPrice(t *testing.T) {
	service := NewService(&fakeRepository{})

	_, appErr := service.CreateRule(context.Background(), CreateRuleInput{Scene: SceneAdminImageGenerate, Unit: UnitRequest, UnitPriceCents: 0, Status: RuleStatusEnabled})
	if appErr == nil || appErr.MessageID != "aibilling.rule.unit_price.invalid" {
		t.Fatalf("expected unit_price invalid keyed error, got %#v", appErr)
	}
}

func TestServiceCreateAndUpdateRuleRejectMissingStatus(t *testing.T) {
	service := NewService(&fakeRepository{byID: &Rule{ID: 7, Scene: SceneAdminImageGenerate, Unit: UnitRequest, UnitPriceCents: 100, Status: RuleStatusEnabled, IsDel: enum.CommonNo}})

	_, appErr := service.CreateRule(context.Background(), CreateRuleInput{Scene: SceneAdminImageGenerate, Unit: UnitRequest, UnitPriceCents: 100})
	if appErr == nil || appErr.MessageID != "aibilling.rule.status.invalid" {
		t.Fatalf("expected create missing status to be rejected, got %#v", appErr)
	}

	appErr = service.UpdateRule(context.Background(), 7, UpdateRuleInput{Unit: UnitRequest, UnitPriceCents: 100})
	if appErr == nil || appErr.MessageID != "aibilling.rule.status.invalid" {
		t.Fatalf("expected update missing status to be rejected, got %#v", appErr)
	}
}

func TestServiceUpdateRuleKeepsSceneImmutable(t *testing.T) {
	repo := &fakeRepository{byID: &Rule{ID: 7, Scene: SceneAdminImageGenerate, Unit: UnitRequest, UnitPriceCents: 100, Status: RuleStatusEnabled, IsDel: enum.CommonNo}}
	service := NewService(repo)

	appErr := service.UpdateRule(context.Background(), 7, UpdateRuleInput{Scene: SceneCanvasImageGenerate, Unit: UnitImage, UnitPriceCents: 250, Status: RuleStatusDisabled})
	if appErr != nil {
		t.Fatalf("UpdateRule returned error: %v", appErr)
	}
	if _, ok := repo.updatedFields["scene"]; ok {
		t.Fatalf("UpdateRule must not update scene, fields=%#v", repo.updatedFields)
	}
	if repo.updatedFields["unit"] != UnitImage || repo.updatedFields["unit_price_cents"] != int64(250) || repo.updatedFields["status"] != RuleStatusDisabled {
		t.Fatalf("unexpected update fields: %#v", repo.updatedFields)
	}
}

func TestServiceDeleteRuleSoftDeletes(t *testing.T) {
	repo := &fakeRepository{byID: &Rule{ID: 9, Scene: SceneAdminImageGenerate, Unit: UnitRequest, UnitPriceCents: 100, Status: RuleStatusEnabled, IsDel: enum.CommonNo}}
	service := NewService(repo)

	appErr := service.DeleteRule(context.Background(), 9)
	if appErr != nil {
		t.Fatalf("DeleteRule returned error: %v", appErr)
	}
	if repo.deletedID != 9 {
		t.Fatalf("expected deleted id 9, got %d", repo.deletedID)
	}
}

func TestServiceEnabledRuleReturnsEnabledActiveRule(t *testing.T) {
	repo := &fakeRepository{enabled: &Rule{ID: 3, Scene: SceneAdminImageGenerate, Unit: UnitRequest, UnitPriceCents: 99, Status: RuleStatusEnabled, IsDel: enum.CommonNo}}
	service := NewService(repo)

	rule, appErr := service.EnabledRule(context.Background(), "  "+SceneAdminImageGenerate+"  ")
	if appErr != nil {
		t.Fatalf("EnabledRule returned error: %v", appErr)
	}
	if rule == nil || rule.ID != 3 || rule.UnitPriceCents != 99 {
		t.Fatalf("unexpected enabled rule: %#v", rule)
	}
	if repo.enabledScene != SceneAdminImageGenerate {
		t.Fatalf("expected trimmed scene lookup, got %q", repo.enabledScene)
	}
}

func TestServiceEnabledRuleMissingOrDisabledReturnsKeyedError(t *testing.T) {
	for _, tc := range []struct {
		name string
		rule *Rule
	}{
		{name: "missing", rule: nil},
		{name: "disabled", rule: &Rule{ID: 4, Scene: SceneAdminImageGenerate, Unit: UnitRequest, UnitPriceCents: 1, Status: RuleStatusDisabled, IsDel: enum.CommonNo}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := NewService(&fakeRepository{enabled: tc.rule})
			rule, appErr := service.EnabledRule(context.Background(), SceneAdminImageGenerate)
			if rule != nil {
				t.Fatalf("expected no rule, got %#v", rule)
			}
			if appErr == nil || appErr.MessageID != "aibilling.rule.not_configured" {
				t.Fatalf("expected not_configured keyed error, got %#v", appErr)
			}
		})
	}
}

type fakeRepository struct {
	created       Rule
	byID          *Rule
	enabled       *Rule
	enabledScene  string
	updatedFields map[string]any
	deletedID     uint64
}

func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]Rule, int64, error) {
	return nil, 0, nil
}
func (f *fakeRepository) Get(ctx context.Context, id uint64) (*Rule, error) { return f.byID, nil }
func (f *fakeRepository) GetByScene(ctx context.Context, scene string) (*Rule, error) {
	return nil, nil
}
func (f *fakeRepository) EnabledByScene(ctx context.Context, scene string) (*Rule, error) {
	f.enabledScene = scene
	return f.enabled, nil
}
func (f *fakeRepository) Create(ctx context.Context, row Rule) (uint64, error) {
	f.created = row
	return 101, nil
}
func (f *fakeRepository) Update(ctx context.Context, id uint64, fields map[string]any) error {
	f.updatedFields = fields
	return nil
}
func (f *fakeRepository) ChangeStatus(ctx context.Context, id uint64, status int) error {
	f.updatedFields = map[string]any{"status": status}
	return nil
}
func (f *fakeRepository) Delete(ctx context.Context, id uint64) error { f.deletedID = id; return nil }
func (f *fakeRepository) CreateRecord(ctx context.Context, row BillingRecord) (int64, error) {
	return 0, nil
}
func (f *fakeRepository) GetRecord(ctx context.Context, id int64) (*BillingRecord, error) {
	return nil, nil
}
func (f *fakeRepository) UpdateRecord(ctx context.Context, id int64, fields map[string]any) error {
	return nil
}

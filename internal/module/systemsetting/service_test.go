package systemsetting

import (
	"context"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
)

type fakeRepository struct {
	rows        []Setting
	total       int64
	gotList     ListQuery
	gotCreate   *Setting
	existsKey   bool
	byID        map[int64]Setting
	updates     []map[string]any
	deleted     []int64
	invalidated []string
}

func (f *fakeRepository) List(ctx context.Context, query ListQuery) ([]Setting, int64, error) {
	f.gotList = query
	return f.rows, f.total, nil
}

func (f *fakeRepository) Get(ctx context.Context, id int64) (*Setting, error) {
	if f.byID == nil {
		return nil, nil
	}
	row, ok := f.byID[id]
	if !ok {
		return nil, nil
	}
	return &row, nil
}

func (f *fakeRepository) SettingsByIDs(ctx context.Context, ids []int64) (map[int64]Setting, error) {
	rows := make(map[int64]Setting)
	for _, id := range ids {
		if f.byID == nil {
			continue
		}
		row, ok := f.byID[id]
		if ok {
			rows[id] = row
		}
	}
	return rows, nil
}

func (f *fakeRepository) ExistsByKey(ctx context.Context, key string, excludeID int64) (bool, error) {
	return f.existsKey, nil
}

func (f *fakeRepository) Create(ctx context.Context, row Setting) (int64, error) {
	f.gotCreate = &row
	return 11, nil
}

func (f *fakeRepository) Update(ctx context.Context, id int64, fields map[string]any) error {
	f.updates = append(f.updates, fields)
	return nil
}

func (f *fakeRepository) Delete(ctx context.Context, ids []int64) error {
	f.deleted = append([]int64{}, ids...)
	return nil
}

func (f *fakeRepository) InvalidateCache(ctx context.Context, key string) error {
	f.invalidated = append(f.invalidated, key)
	return nil
}

func TestInitReturnsEnumBackedDict(t *testing.T) {
	service := NewService(&fakeRepository{})

	got, appErr := service.Init(context.Background())
	if appErr != nil {
		t.Fatalf("expected init to succeed, got %v", appErr)
	}
	options := got.Dict.SystemSettingValueTypeArr
	if len(options) != 4 || options[0].Value != enum.SystemSettingValueString || options[3].Value != enum.SystemSettingValueJSON {
		t.Fatalf("unexpected value type dict: %#v", options)
	}
}

func TestListTrimsKeyAndReturnsLabels(t *testing.T) {
	createdAt := time.Date(2026, 5, 4, 10, 0, 0, 0, time.UTC)
	repo := &fakeRepository{
		rows: []Setting{{
			ID: 1, SettingKey: "user.default_avatar", SettingValue: "avatar.png", ValueType: enum.SystemSettingValueString,
			Remark: "默认头像", Status: enum.CommonYes, IsDel: enum.CommonNo, CreatedAt: createdAt, UpdatedAt: createdAt,
		}},
		total: 1,
	}
	service := NewService(repo)

	got, appErr := service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20, Key: " user.", Status: ptrInt(enum.CommonYes)})
	if appErr != nil {
		t.Fatalf("expected list to succeed, got %v", appErr)
	}
	if repo.gotList.Key != "user." {
		t.Fatalf("expected trimmed key prefix, got %#v", repo.gotList)
	}
	if len(got.List) != 1 || got.List[0].ValueTypeName != "字符串" || got.List[0].StatusName != "启用" {
		t.Fatalf("unexpected list response: %#v", got)
	}
	if got.Page.Total != 1 || got.Page.TotalPage != 1 {
		t.Fatalf("unexpected page: %#v", got.Page)
	}
}

func TestCreateRejectsDuplicateKey(t *testing.T) {
	service := NewService(&fakeRepository{existsKey: true})

	_, appErr := service.Create(context.Background(), CreateInput{Key: "user.default_avatar", Value: "x", Type: enum.SystemSettingValueString})
	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "配置 key [user.default_avatar] 已存在" {
		t.Fatalf("expected duplicate key error, got %#v", appErr)
	}
}

func TestCreatePreservesStringValueWhitespace(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	_, appErr := service.Create(context.Background(), CreateInput{Key: "ui.banner", Value: "  keep spaces  ", Type: enum.SystemSettingValueString})
	if appErr != nil {
		t.Fatalf("expected create to succeed, got %v", appErr)
	}
	if repo.gotCreate == nil || repo.gotCreate.SettingValue != "  keep spaces  " {
		t.Fatalf("system setting string value must not be silently trimmed: %#v", repo.gotCreate)
	}
}

func TestCreateValidatesValueTypeAndStoresRow(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	id, appErr := service.Create(context.Background(), CreateInput{Key: "feature.switch", Value: "true", Type: enum.SystemSettingValueBool, Remark: "开关"})
	if appErr != nil {
		t.Fatalf("expected create to succeed, got %v", appErr)
	}
	if id != 11 || repo.gotCreate == nil {
		t.Fatalf("expected created row, id=%d row=%#v", id, repo.gotCreate)
	}
	if repo.gotCreate.SettingKey != "feature.switch" || repo.gotCreate.SettingValue != "true" || repo.gotCreate.ValueType != enum.SystemSettingValueBool || repo.gotCreate.Status != enum.CommonYes || repo.gotCreate.IsDel != enum.CommonNo {
		t.Fatalf("unexpected created row: %#v", repo.gotCreate)
	}
}

func TestCreateRejectsInvalidTypedValue(t *testing.T) {
	service := NewService(&fakeRepository{})

	cases := []struct {
		name  string
		input CreateInput
		msg   string
	}{
		{name: "number", input: CreateInput{Key: "num", Value: "abc", Type: enum.SystemSettingValueNumber}, msg: "数值类型需为数字"},
		{name: "bool", input: CreateInput{Key: "bool", Value: "yes", Type: enum.SystemSettingValueBool}, msg: "布尔类型需为 true/false 或 0/1"},
		{name: "json", input: CreateInput{Key: "json", Value: `"string"`, Type: enum.SystemSettingValueJSON}, msg: "JSON 类型需为合法对象或数组"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, appErr := service.Create(context.Background(), tt.input)
			if appErr == nil || appErr.Message != tt.msg {
				t.Fatalf("expected %q, got %#v", tt.msg, appErr)
			}
		})
	}
}

func TestUpdateMissingRowReturnsNotFound(t *testing.T) {
	service := NewService(&fakeRepository{})

	appErr := service.Update(context.Background(), 99, UpdateInput{Value: "x", Type: enum.SystemSettingValueString})
	if appErr == nil || appErr.Code != apperror.CodeNotFound || appErr.Message != "配置项不存在" {
		t.Fatalf("expected not found, got %#v", appErr)
	}
}

func TestUpdateInvalidatesChangedKey(t *testing.T) {
	repo := &fakeRepository{byID: map[int64]Setting{2: {ID: 2, SettingKey: "user.default_avatar", Status: enum.CommonYes}}}
	service := NewService(repo)

	appErr := service.Update(context.Background(), 2, UpdateInput{Value: `{"url":"avatar.png"}`, Type: enum.SystemSettingValueJSON, Remark: "默认头像"})
	if appErr != nil {
		t.Fatalf("expected update to succeed, got %v", appErr)
	}
	if len(repo.updates) != 1 || repo.updates[0]["setting_value"] != `{"url":"avatar.png"}` || repo.updates[0]["value_type"] != enum.SystemSettingValueJSON {
		t.Fatalf("unexpected update fields: %#v", repo.updates)
	}
	if len(repo.invalidated) != 1 || repo.invalidated[0] != "user.default_avatar" {
		t.Fatalf("expected cache invalidation, got %#v", repo.invalidated)
	}
}

func TestChangeStatusInvalidatesKey(t *testing.T) {
	repo := &fakeRepository{byID: map[int64]Setting{3: {ID: 3, SettingKey: "feature.switch", Status: enum.CommonYes}}}
	service := NewService(repo)

	appErr := service.ChangeStatus(context.Background(), 3, enum.CommonNo)
	if appErr != nil {
		t.Fatalf("expected status change to succeed, got %v", appErr)
	}
	if len(repo.updates) != 1 || repo.updates[0]["status"] != enum.CommonNo {
		t.Fatalf("unexpected status update: %#v", repo.updates)
	}
	if len(repo.invalidated) != 1 || repo.invalidated[0] != "feature.switch" {
		t.Fatalf("expected cache invalidation, got %#v", repo.invalidated)
	}
}

func TestDeleteBatchNormalizesIDsAndInvalidatesKeys(t *testing.T) {
	repo := &fakeRepository{byID: map[int64]Setting{
		2: {ID: 2, SettingKey: "a"},
		3: {ID: 3, SettingKey: "b"},
	}}
	service := NewService(repo)

	appErr := service.Delete(context.Background(), []int64{3, 2, 3, 0})
	if appErr != nil {
		t.Fatalf("expected delete to succeed, got %v", appErr)
	}
	if len(repo.deleted) != 2 || repo.deleted[0] != 2 || repo.deleted[1] != 3 {
		t.Fatalf("expected normalized delete ids, got %#v", repo.deleted)
	}
	if len(repo.invalidated) != 2 || repo.invalidated[0] != "a" || repo.invalidated[1] != "b" {
		t.Fatalf("expected invalidated keys, got %#v", repo.invalidated)
	}
}

func ptrInt(value int) *int {
	return &value
}

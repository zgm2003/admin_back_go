package authplatform

import (
	"context"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
)

type fakeManagementRepository struct {
	active *Platform
	rows   []Platform
	total  int64
	gotCreate *Platform
	updates []map[string]any
	deleted []int64
	statusRows map[int64]Platform
	existsCode bool
}

func (f *fakeManagementRepository) FindActiveByCode(ctx context.Context, code string) (*Platform, error) {
	return f.active, nil
}

func (f *fakeManagementRepository) List(ctx context.Context, query ListQuery) ([]Platform, int64, error) {
	return f.rows, f.total, nil
}

func (f *fakeManagementRepository) Get(ctx context.Context, id int64) (*Platform, error) {
	if f.statusRows != nil {
		row, ok := f.statusRows[id]
		if ok {
			return &row, nil
		}
	}
	return nil, nil
}

func (f *fakeManagementRepository) PlatformsByIDs(ctx context.Context, ids []int64) (map[int64]Platform, error) {
	rows := make(map[int64]Platform)
	for _, id := range ids {
		if f.statusRows != nil {
			row, ok := f.statusRows[id]
			if ok {
				rows[id] = row
			}
		}
	}
	return rows, nil
}

func (f *fakeManagementRepository) ExistsByCode(ctx context.Context, code string, excludeID int64) (bool, error) {
	return f.existsCode, nil
}

func (f *fakeManagementRepository) Create(ctx context.Context, row Platform) (int64, error) {
	f.gotCreate = &row
	return 9, nil
}

func (f *fakeManagementRepository) Update(ctx context.Context, id int64, fields map[string]any) error {
	f.updates = append(f.updates, fields)
	return nil
}

func (f *fakeManagementRepository) Delete(ctx context.Context, ids []int64) error {
	f.deleted = append([]int64{}, ids...)
	return nil
}

func TestManagementInitReturnsEnumBackedDict(t *testing.T) {
	service := NewService(&fakeManagementRepository{})

	got, appErr := service.Init(context.Background())
	if appErr != nil {
		t.Fatalf("expected init to succeed, got %v", appErr)
	}
	if len(got.Dict.CommonStatusArr) != 2 || got.Dict.CommonStatusArr[0].Value != enum.CommonYes {
		t.Fatalf("missing common status dict: %#v", got.Dict)
	}
	if len(got.Dict.AuthPlatformLoginTypeArr) != 3 || got.Dict.AuthPlatformLoginTypeArr[0].Value != enum.LoginTypeEmail {
		t.Fatalf("missing login type dict: %#v", got.Dict)
	}
	if len(got.Dict.AuthPlatformCaptchaTypeArr) != 1 || got.Dict.AuthPlatformCaptchaTypeArr[0].Value != enum.CaptchaTypeSlide {
		t.Fatalf("missing captcha type dict: %#v", got.Dict)
	}
}

func TestManagementListIncludesCaptchaTypeWithoutFallbackFields(t *testing.T) {
	createdAt := time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC)
	repo := &fakeManagementRepository{
		rows: []Platform{{
			ID: 1, Code: "admin", Name: "PC后台", LoginTypes: `["password"]`, CaptchaType: enum.CaptchaTypeSlide,
			AccessTTL: 14400, RefreshTTL: 1209600, BindPlatform: enum.CommonYes, BindDevice: enum.CommonNo,
			BindIP: enum.CommonNo, SingleSession: enum.CommonYes, MaxSessions: 1, AllowRegister: enum.CommonYes,
			Status: enum.CommonYes, CreatedAt: createdAt, UpdatedAt: createdAt,
		}},
		total: 1,
	}
	service := NewService(repo)

	got, appErr := service.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 50})
	if appErr != nil {
		t.Fatalf("expected list to succeed, got %v", appErr)
	}
	if len(got.List) != 1 || got.List[0].CaptchaType != enum.CaptchaTypeSlide {
		t.Fatalf("captcha_type missing from list: %#v", got)
	}
	if got.Page.Total != 1 || got.Page.CurrentPage != 1 || got.Page.PageSize != 50 {
		t.Fatalf("unexpected page: %#v", got.Page)
	}
}

func TestManagementCreateValidatesCaptchaTypeAndStoresJSONLoginTypes(t *testing.T) {
	repo := &fakeManagementRepository{}
	service := NewService(repo)

	id, appErr := service.Create(context.Background(), CreateInput{
		Code: "mini", Name: "小程序", LoginTypes: []string{enum.LoginTypePassword, enum.LoginTypeEmail}, CaptchaType: enum.CaptchaTypeSlide,
		AccessTTL: 3600, RefreshTTL: 86400, BindPlatform: enum.CommonYes, BindDevice: enum.CommonNo,
		BindIP: enum.CommonNo, SingleSession: enum.CommonYes, MaxSessions: 1, AllowRegister: enum.CommonYes,
	})
	if appErr != nil {
		t.Fatalf("expected create to succeed, got %v", appErr)
	}
	if id != 9 || repo.gotCreate == nil {
		t.Fatalf("expected created row, id=%d row=%#v", id, repo.gotCreate)
	}
	if repo.gotCreate.CaptchaType != enum.CaptchaTypeSlide {
		t.Fatalf("expected captcha_type stored, got %#v", repo.gotCreate)
	}
	if repo.gotCreate.LoginTypes != `["email","password"]` {
		t.Fatalf("expected normalized login_types json, got %q", repo.gotCreate.LoginTypes)
	}
}

func TestManagementCreateRejectsUnsupportedCaptchaType(t *testing.T) {
	service := NewService(&fakeManagementRepository{})

	_, appErr := service.Create(context.Background(), CreateInput{
		Code: "mini", Name: "小程序", LoginTypes: []string{enum.LoginTypePassword}, CaptchaType: "click",
		AccessTTL: 3600, RefreshTTL: 86400, BindPlatform: enum.CommonYes, BindDevice: enum.CommonNo,
		BindIP: enum.CommonNo, SingleSession: enum.CommonYes, MaxSessions: 1, AllowRegister: enum.CommonYes,
	})
	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "无效的验证码类型" {
		t.Fatalf("expected captcha type validation error, got %#v", appErr)
	}
}

func TestManagementDeleteAndDisableProtectAdminPlatform(t *testing.T) {
	repo := &fakeManagementRepository{statusRows: map[int64]Platform{1: {ID: 1, Code: "admin", Status: enum.CommonYes}}}
	service := NewService(repo)

	if appErr := service.Delete(context.Background(), []int64{1}); appErr == nil || appErr.Message != "核心平台 [admin] 不允许删除" {
		t.Fatalf("expected admin delete protection, got %#v", appErr)
	}
	if appErr := service.ChangeStatus(context.Background(), 1, enum.CommonNo); appErr == nil || appErr.Message != "核心平台 [admin] 不允许禁用" {
		t.Fatalf("expected admin disable protection, got %#v", appErr)
	}
}

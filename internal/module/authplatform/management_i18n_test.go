package authplatform

import (
	"context"
	"errors"
	"testing"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	projecti18n "admin_back_go/internal/i18n"
)

type errorManagementRepository struct {
	rows       map[int64]Platform
	listErr    error
	getErr     error
	idsErr     error
	existsErr  error
	createErr  error
	updateErr  error
	deleteErr  error
	existsCode bool
}

func (r *errorManagementRepository) FindActiveByCode(ctx context.Context, code string) (*Platform, error) {
	return nil, nil
}

func (r *errorManagementRepository) List(ctx context.Context, query ListQuery) ([]Platform, int64, error) {
	return nil, 0, r.listErr
}

func (r *errorManagementRepository) Get(ctx context.Context, id int64) (*Platform, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	if row, ok := r.rows[id]; ok {
		copy := row
		return &copy, nil
	}
	return nil, nil
}

func (r *errorManagementRepository) PlatformsByIDs(ctx context.Context, ids []int64) (map[int64]Platform, error) {
	if r.idsErr != nil {
		return nil, r.idsErr
	}
	result := make(map[int64]Platform, len(ids))
	for _, id := range ids {
		if row, ok := r.rows[id]; ok {
			result[id] = row
		}
	}
	return result, nil
}

func (r *errorManagementRepository) ExistsByCode(ctx context.Context, code string, excludeID int64) (bool, error) {
	return r.existsCode, r.existsErr
}

func (r *errorManagementRepository) Create(ctx context.Context, row Platform) (int64, error) {
	return 9, r.createErr
}

func (r *errorManagementRepository) Update(ctx context.Context, id int64, fields map[string]any) error {
	return r.updateErr
}

func (r *errorManagementRepository) Delete(ctx context.Context, ids []int64) error {
	return r.deleteErr
}

func TestAuthPlatformCatalogKeysMatch(t *testing.T) {
	want := []string{
		"authplatform.service_missing",
		"authplatform.repository_missing",
		"authplatform.list.request.invalid",
		"authplatform.create.request.invalid",
		"authplatform.update.request.invalid",
		"authplatform.delete.empty",
		"authplatform.status.invalid",
		"authplatform.id.invalid",
		"authplatform.query_failed",
		"authplatform.code_check_failed",
		"authplatform.code.duplicate",
		"authplatform.create_failed",
		"authplatform.not_found",
		"authplatform.update_failed",
		"authplatform.delete.contains_missing",
		"authplatform.delete.admin_forbidden",
		"authplatform.delete_failed",
		"authplatform.status.disable_forbidden",
		"authplatform.status_update_failed",
		"authplatform.current_page.invalid",
		"authplatform.page_size.invalid",
		"authplatform.code.invalid",
		"authplatform.name.invalid",
		"authplatform.captcha_type.invalid",
		"authplatform.access_ttl.invalid",
		"authplatform.refresh_ttl.invalid",
		"authplatform.policy.invalid",
		"authplatform.max_sessions.invalid",
		"authplatform.login_types.empty",
		"authplatform.login_types.invalid",
		"authplatform.encode_login_types_failed",
	}

	for _, lang := range []string{"zh-CN", "en-US"} {
		keys, err := projecti18n.CatalogKeys(lang)
		if err != nil {
			t.Fatalf("load %s keys: %v", lang, err)
		}
		for _, key := range want {
			if _, ok := keys[key]; !ok {
				t.Fatalf("%s missing key %q", lang, key)
			}
		}
	}
}

func TestManagementListUsesKeyedErrors(t *testing.T) {
	service := NewService(&errorManagementRepository{})

	cases := []struct {
		name string
		q    ListQuery
		want string
	}{
		{name: "current page", q: ListQuery{CurrentPage: 0, PageSize: 20}, want: "authplatform.current_page.invalid"},
		{name: "page size", q: ListQuery{CurrentPage: 1, PageSize: 51}, want: "authplatform.page_size.invalid"},
		{name: "status", q: ListQuery{CurrentPage: 1, PageSize: 20, Status: intPtr(9)}, want: "authplatform.status.invalid"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, appErr := service.List(context.Background(), tt.q)
			assertMessageID(t, appErr, tt.want)
		})
	}

	queryErrService := NewService(&errorManagementRepository{listErr: errors.New("boom")})
	_, appErr := queryErrService.List(context.Background(), ListQuery{CurrentPage: 1, PageSize: 20})
	assertMessageID(t, appErr, "authplatform.query_failed")
}

func TestManagementCreateUsesKeyedErrors(t *testing.T) {
	service := NewService(&errorManagementRepository{})

	cases := []struct {
		name  string
		input CreateInput
		want  string
	}{
		{name: "code", input: mutateCreateInput(func(input *CreateInput) { input.Code = "1" }), want: "authplatform.code.invalid"},
		{name: "name", input: mutateCreateInput(func(input *CreateInput) { input.Name = "" }), want: "authplatform.name.invalid"},
		{name: "login empty", input: mutateCreateInput(func(input *CreateInput) { input.LoginTypes = nil }), want: "authplatform.login_types.empty"},
		{name: "login invalid", input: mutateCreateInput(func(input *CreateInput) { input.LoginTypes = []string{"sms"} }), want: "authplatform.login_types.invalid"},
		{name: "captcha", input: mutateCreateInput(func(input *CreateInput) { input.CaptchaType = "click" }), want: "authplatform.captcha_type.invalid"},
		{name: "access ttl", input: mutateCreateInput(func(input *CreateInput) { input.AccessTTL = 59 }), want: "authplatform.access_ttl.invalid"},
		{name: "refresh ttl", input: mutateCreateInput(func(input *CreateInput) { input.RefreshTTL = 59 }), want: "authplatform.refresh_ttl.invalid"},
		{name: "policy", input: mutateCreateInput(func(input *CreateInput) { input.BindPlatform = 0 }), want: "authplatform.policy.invalid"},
		{name: "max sessions", input: mutateCreateInput(func(input *CreateInput) { input.MaxSessions = 101 }), want: "authplatform.max_sessions.invalid"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, appErr := service.Create(context.Background(), tt.input)
			assertMessageID(t, appErr, tt.want)
		})
	}

	duplicateService := NewService(&errorManagementRepository{existsCode: true})
	_, appErr := duplicateService.Create(context.Background(), validCreateInput())
	assertMessageID(t, appErr, "authplatform.code.duplicate")
	if appErr.TemplateData["code"] != "mini" {
		t.Fatalf("expected duplicate code template data, got %#v", appErr.TemplateData)
	}

	existsErrService := NewService(&errorManagementRepository{existsErr: errors.New("boom")})
	_, appErr = existsErrService.Create(context.Background(), validCreateInput())
	assertMessageID(t, appErr, "authplatform.code_check_failed")

	createErrService := NewService(&errorManagementRepository{createErr: errors.New("boom")})
	_, appErr = createErrService.Create(context.Background(), validCreateInput())
	assertMessageID(t, appErr, "authplatform.create_failed")
}

func TestManagementUpdateUsesKeyedErrors(t *testing.T) {
	baseRepo := &errorManagementRepository{rows: map[int64]Platform{2: {ID: 2, Code: "mini"}}}
	service := NewService(baseRepo)

	appErr := service.Update(context.Background(), 0, validUpdateInput())
	assertMessageID(t, appErr, "authplatform.id.invalid")

	appErr = service.Update(context.Background(), 99, validUpdateInput())
	assertMessageID(t, appErr, "authplatform.not_found")

	queryErrService := NewService(&errorManagementRepository{rows: map[int64]Platform{2: {ID: 2, Code: "mini"}}, getErr: errors.New("boom")})
	appErr = queryErrService.Update(context.Background(), 2, validUpdateInput())
	assertMessageID(t, appErr, "authplatform.query_failed")

	updateErrService := NewService(&errorManagementRepository{rows: map[int64]Platform{2: {ID: 2, Code: "mini"}}, updateErr: errors.New("boom")})
	appErr = updateErrService.Update(context.Background(), 2, validUpdateInput())
	assertMessageID(t, appErr, "authplatform.update_failed")
}

func TestManagementDeleteUsesKeyedErrors(t *testing.T) {
	service := NewService(&errorManagementRepository{})

	appErr := service.Delete(context.Background(), nil)
	assertMessageID(t, appErr, "authplatform.delete.empty")

	appErr = service.Delete(context.Background(), []int64{1, 2})
	assertMessageID(t, appErr, "authplatform.delete.contains_missing")

	adminRepo := &errorManagementRepository{rows: map[int64]Platform{1: {ID: 1, Code: enum.PlatformAdmin}}}
	adminService := NewService(adminRepo)
	appErr = adminService.Delete(context.Background(), []int64{1})
	assertMessageID(t, appErr, "authplatform.delete.admin_forbidden")

	queryErrService := NewService(&errorManagementRepository{rows: map[int64]Platform{1: {ID: 1, Code: "mini"}}, idsErr: errors.New("boom")})
	appErr = queryErrService.Delete(context.Background(), []int64{1})
	assertMessageID(t, appErr, "authplatform.query_failed")

	deleteErrService := NewService(&errorManagementRepository{rows: map[int64]Platform{1: {ID: 1, Code: "mini"}}, deleteErr: errors.New("boom")})
	appErr = deleteErrService.Delete(context.Background(), []int64{1})
	assertMessageID(t, appErr, "authplatform.delete_failed")
}

func TestManagementChangeStatusUsesKeyedErrors(t *testing.T) {
	service := NewService(&errorManagementRepository{})

	appErr := service.ChangeStatus(context.Background(), 0, enum.CommonYes)
	assertMessageID(t, appErr, "authplatform.id.invalid")

	appErr = service.ChangeStatus(context.Background(), 1, 9)
	assertMessageID(t, appErr, "authplatform.status.invalid")

	appErr = service.ChangeStatus(context.Background(), 99, enum.CommonYes)
	assertMessageID(t, appErr, "authplatform.not_found")

	adminRepo := &errorManagementRepository{rows: map[int64]Platform{1: {ID: 1, Code: enum.PlatformAdmin}}}
	adminService := NewService(adminRepo)
	appErr = adminService.ChangeStatus(context.Background(), 1, enum.CommonNo)
	assertMessageID(t, appErr, "authplatform.status.disable_forbidden")

	queryErrService := NewService(&errorManagementRepository{rows: map[int64]Platform{1: {ID: 1, Code: "mini"}}, getErr: errors.New("boom")})
	appErr = queryErrService.ChangeStatus(context.Background(), 1, enum.CommonNo)
	assertMessageID(t, appErr, "authplatform.query_failed")

	updateErrService := NewService(&errorManagementRepository{rows: map[int64]Platform{1: {ID: 1, Code: "mini"}}, updateErr: errors.New("boom")})
	appErr = updateErrService.ChangeStatus(context.Background(), 1, enum.CommonNo)
	assertMessageID(t, appErr, "authplatform.status_update_failed")
}

func validCreateInput() CreateInput {
	return CreateInput{
		Code:          "mini",
		Name:          "小程序",
		LoginTypes:    []string{enum.LoginTypePassword},
		CaptchaType:   enum.CaptchaTypeSlide,
		AccessTTL:     3600,
		RefreshTTL:    86400,
		BindPlatform:  enum.CommonYes,
		BindDevice:    enum.CommonNo,
		BindIP:        enum.CommonNo,
		SingleSession: enum.CommonYes,
		MaxSessions:   1,
		AllowRegister: enum.CommonYes,
	}
}

func mutateCreateInput(mutator func(*CreateInput)) CreateInput {
	input := validCreateInput()
	mutator(&input)
	return input
}

func intPtr(value int) *int {
	return &value
}

func assertMessageID(t *testing.T, appErr *apperror.Error, want string) {
	t.Helper()
	if appErr == nil {
		t.Fatalf("expected error %q, got nil", want)
	}
	if appErr.Code != apperror.CodeBadRequest && appErr.Code != apperror.CodeNotFound && appErr.Code != apperror.CodeInternal {
		t.Fatalf("unexpected code for %q: %#v", want, appErr)
	}
	if appErr.MessageID != want {
		t.Fatalf("expected message id %q, got %#v", want, appErr)
	}
}

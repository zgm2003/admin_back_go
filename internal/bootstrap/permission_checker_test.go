package bootstrap

import (
	"context"
	"errors"
	"testing"
	"time"

	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/permission"
	"admin_back_go/internal/module/user"
	"admin_back_go/internal/shared/apperror"
)

type fakePermissionUserRepository struct {
	user           *user.User
	role           *user.Role
	err            error
	findRoleCalled bool
}

func (f fakePermissionUserRepository) FindUser(ctx context.Context, userID int64) (*user.User, error) {
	return f.user, f.err
}

func (f *fakePermissionUserRepository) FindRole(ctx context.Context, roleID int64) (*user.Role, error) {
	f.findRoleCalled = true
	return f.role, f.err
}

type fakePermissionContextBuilder struct {
	called bool
	ctx    permission.Context
	err    *apperror.Error
}

func (f *fakePermissionContextBuilder) BuildContextByRole(ctx context.Context, roleID int64, platform string) (permission.Context, *apperror.Error) {
	f.called = true
	return f.ctx, f.err
}

type fakePermissionRouteAccessGrantCache struct {
	getValues []string
	getHit    bool
	getErr    error

	getKey    string
	setKey    string
	setValues []string
	setTTL    time.Duration
	setErr    error
}

func (f *fakePermissionRouteAccessGrantCache) Get(ctx context.Context, key string) ([]string, bool, error) {
	f.getKey = key
	return f.getValues, f.getHit, f.getErr
}

func (f *fakePermissionRouteAccessGrantCache) Set(ctx context.Context, key string, values []string, ttl time.Duration) error {
	f.setKey = key
	f.setValues = values
	f.setTTL = ttl
	return f.setErr
}

func TestPermissionCheckerAllowsOwnedRouteAccessCode(t *testing.T) {
	checker := PermissionCheckerFor(
		&fakePermissionUserRepository{user: &user.User{ID: 1, RoleID: 7}, role: &user.Role{ID: 7, Name: "管理员"}},
		&fakePermissionContextBuilder{ctx: permission.Context{RouteAccessCodes: []string{"permission_permission_add"}}},
		nil,
		0,
	)

	appErr := checker(context.Background(), middleware.PermissionInput{
		UserID:   1,
		Platform: "admin",
		Code:     "permission_permission_add",
	})

	if appErr != nil {
		t.Fatalf("expected permission allowed, got %v", appErr)
	}
}

func TestPermissionCheckerAllowsPageRouteAccessCode(t *testing.T) {
	checker := PermissionCheckerFor(
		&fakePermissionUserRepository{user: &user.User{ID: 12, RoleID: 3}, role: &user.Role{ID: 3}},
		&fakePermissionContextBuilder{ctx: permission.Context{ButtonCodes: []string{}, RouteAccessCodes: []string{"payment_config_list"}}},
		nil,
		0,
	)

	appErr := checker(context.Background(), middleware.PermissionInput{UserID: 12, Platform: "admin", Code: "payment_config_list"})

	if appErr != nil {
		t.Fatalf("expected PAGE route access code to pass PermissionCheck, got %#v", appErr)
	}
}

func TestPermissionCheckerAllowsButtonRouteAccessCode(t *testing.T) {
	checker := PermissionCheckerFor(
		&fakePermissionUserRepository{user: &user.User{ID: 12, RoleID: 3}, role: &user.Role{ID: 3}},
		&fakePermissionContextBuilder{ctx: permission.Context{ButtonCodes: []string{"payment_config_edit"}, RouteAccessCodes: []string{"payment_config_list", "payment_config_edit"}}},
		nil,
		0,
	)

	appErr := checker(context.Background(), middleware.PermissionInput{UserID: 12, Platform: "admin", Code: "payment_config_edit"})

	if appErr != nil {
		t.Fatalf("expected BUTTON route access code to pass PermissionCheck, got %#v", appErr)
	}
}

func TestPermissionCheckerDeniesMissingRouteAccessCode(t *testing.T) {
	checker := PermissionCheckerFor(
		&fakePermissionUserRepository{user: &user.User{ID: 1, RoleID: 7}, role: &user.Role{ID: 7, Name: "管理员"}},
		&fakePermissionContextBuilder{ctx: permission.Context{RouteAccessCodes: []string{"other"}}},
		nil,
		0,
	)

	appErr := checker(context.Background(), middleware.PermissionInput{UserID: 1, Platform: "admin", Code: "permission_permission_add"})

	if appErr == nil || appErr.Code != apperror.CodeForbidden {
		t.Fatalf("expected forbidden, got %#v", appErr)
	}
}

func TestPermissionCheckerDoesNotFallbackWhenUserLookupFails(t *testing.T) {
	checker := PermissionCheckerFor(
		&fakePermissionUserRepository{err: errors.New("db down")},
		&fakePermissionContextBuilder{},
		nil,
		0,
	)

	appErr := checker(context.Background(), middleware.PermissionInput{UserID: 1, Platform: "admin", Code: "permission_permission_add"})

	if appErr == nil || appErr.Code != apperror.CodeInternal {
		t.Fatalf("expected internal error, got %#v", appErr)
	}
}

func TestPermissionCheckerRejectsMissingUserWithoutBuildingContext(t *testing.T) {
	builder := &fakePermissionContextBuilder{ctx: permission.Context{RouteAccessCodes: []string{"permission_permission_add"}}}
	checker := PermissionCheckerFor(
		&fakePermissionUserRepository{user: nil, role: &user.Role{ID: 7, Name: "管理员"}},
		builder,
		nil,
		0,
	)

	appErr := checker(context.Background(), middleware.PermissionInput{UserID: 1, Platform: "admin", Code: "permission_permission_add"})

	if appErr == nil || appErr.Code != apperror.CodeUnauthorized {
		t.Fatalf("expected unauthorized, got %#v", appErr)
	}
	if builder.called {
		t.Fatalf("expected permission builder to be skipped when user is missing")
	}
}

func TestPermissionCheckerRejectsMissingRoleWithoutBuildingContext(t *testing.T) {
	repo := &fakePermissionUserRepository{user: &user.User{ID: 1, RoleID: 7}, role: nil}
	builder := &fakePermissionContextBuilder{ctx: permission.Context{RouteAccessCodes: []string{"permission_permission_add"}}}
	checker := PermissionCheckerFor(repo, builder, nil, 0)

	appErr := checker(context.Background(), middleware.PermissionInput{UserID: 1, Platform: "admin", Code: "permission_permission_add"})

	if appErr == nil || appErr.Code != apperror.CodeForbidden {
		t.Fatalf("expected forbidden, got %#v", appErr)
	}
	if !repo.findRoleCalled {
		t.Fatalf("expected role lookup before permission check")
	}
	if builder.called {
		t.Fatalf("expected permission builder to be skipped when role is missing")
	}
}

func TestPermissionCheckerFallsBackToPermissionBuilderWhenCacheGetFails(t *testing.T) {
	cache := &fakePermissionRouteAccessGrantCache{getErr: errors.New("redis down")}
	builder := &fakePermissionContextBuilder{ctx: permission.Context{RouteAccessCodes: []string{"permission_permission_add"}}}
	checker := PermissionCheckerFor(
		&fakePermissionUserRepository{user: &user.User{ID: 12, RoleID: 7}, role: &user.Role{ID: 7, Name: "管理员"}},
		builder,
		cache,
		time.Minute,
	)

	appErr := checker(context.Background(), middleware.PermissionInput{UserID: 12, Platform: "admin", Code: "permission_permission_add"})

	if appErr != nil {
		t.Fatalf("expected permission allowed after cache error fallback, got %v", appErr)
	}
	if !builder.called {
		t.Fatalf("expected permission builder to run when cache get fails")
	}
	if cache.getKey != "auth_perm_uid_12_admin_rbac_route_access_grants" || cache.setKey != "auth_perm_uid_12_admin_rbac_route_access_grants" {
		t.Fatalf("cache keys mismatch: get=%q set=%q", cache.getKey, cache.setKey)
	}
}

func TestPermissionCheckerFailsClosedWhenPermissionBuilderFails(t *testing.T) {
	builderErr := apperror.Internal("权限上下文计算失败")
	builder := &fakePermissionContextBuilder{err: builderErr}
	checker := PermissionCheckerFor(
		&fakePermissionUserRepository{user: &user.User{ID: 12, RoleID: 7}, role: &user.Role{ID: 7, Name: "管理员"}},
		builder,
		nil,
		time.Minute,
	)

	appErr := checker(context.Background(), middleware.PermissionInput{UserID: 12, Platform: "admin", Code: "permission_permission_add"})

	if appErr != builderErr {
		t.Fatalf("expected builder error to be returned, got %#v", appErr)
	}
	if !builder.called {
		t.Fatalf("expected permission builder to run")
	}
}

func TestPermissionCheckerAllowsCachedRouteAccessCodeWithoutBuildingContext(t *testing.T) {
	cache := &fakePermissionRouteAccessGrantCache{
		getValues: []string{"permission_permission_add"},
		getHit:    true,
	}
	builder := &fakePermissionContextBuilder{ctx: permission.Context{RouteAccessCodes: []string{"other"}}}
	checker := PermissionCheckerFor(
		&fakePermissionUserRepository{user: &user.User{ID: 12, RoleID: 7}, role: &user.Role{ID: 7, Name: "管理员"}},
		builder,
		cache,
		0,
	)

	appErr := checker(context.Background(), middleware.PermissionInput{UserID: 12, Platform: "admin", Code: "permission_permission_add"})

	if appErr != nil {
		t.Fatalf("expected cached permission allowed, got %v", appErr)
	}
	if builder.called {
		t.Fatalf("expected permission builder to be skipped on cache hit")
	}
	if cache.getKey != "auth_perm_uid_12_admin_rbac_route_access_grants" {
		t.Fatalf("cache key mismatch: %q", cache.getKey)
	}
	if cache.setKey != "" {
		t.Fatalf("expected cache set to be skipped on hit, got %q", cache.setKey)
	}
}

func TestPermissionCheckerBuildsAndCachesRouteAccessCodesOnCacheMiss(t *testing.T) {
	cache := &fakePermissionRouteAccessGrantCache{}
	builder := &fakePermissionContextBuilder{ctx: permission.Context{RouteAccessCodes: []string{"permission_permission_add"}}}
	checker := PermissionCheckerFor(
		&fakePermissionUserRepository{user: &user.User{ID: 12, RoleID: 7}, role: &user.Role{ID: 7, Name: "管理员"}},
		builder,
		cache,
		time.Minute,
	)

	appErr := checker(context.Background(), middleware.PermissionInput{UserID: 12, Platform: "admin", Code: "permission_permission_add"})

	if appErr != nil {
		t.Fatalf("expected permission allowed after cache miss build, got %v", appErr)
	}
	if !builder.called {
		t.Fatalf("expected permission builder to run on cache miss")
	}
	if cache.setKey != "auth_perm_uid_12_admin_rbac_route_access_grants" {
		t.Fatalf("cache set key mismatch: %q", cache.setKey)
	}
	if len(cache.setValues) != 1 || cache.setValues[0] != "permission_permission_add" || cache.setTTL != time.Minute {
		t.Fatalf("cache set payload mismatch: values=%#v ttl=%s", cache.setValues, cache.setTTL)
	}
}

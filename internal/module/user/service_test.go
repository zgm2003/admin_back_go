package user

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/module/permission"
)

type fakeUserRepository struct {
	user    *User
	profile *Profile
	role    *Role
	entries []QuickEntry
	err     error
}

func (f fakeUserRepository) FindUser(ctx context.Context, userID int64) (*User, error) {
	return f.user, f.err
}

func (f fakeUserRepository) FindProfile(ctx context.Context, userID int64) (*Profile, error) {
	return f.profile, f.err
}

func (f fakeUserRepository) FindRole(ctx context.Context, roleID int64) (*Role, error) {
	return f.role, f.err
}

func (f fakeUserRepository) QuickEntries(ctx context.Context, userID int64) ([]QuickEntry, error) {
	return f.entries, f.err
}

type fakePermissionBuilder struct {
	called   bool
	roleID   int64
	platform string
	ctx      permission.Context
	err      *apperror.Error
}

func (f *fakePermissionBuilder) BuildContextByRole(ctx context.Context, roleID int64, platform string) (permission.Context, *apperror.Error) {
	f.called = true
	f.roleID = roleID
	f.platform = platform
	return f.ctx, f.err
}

type fakeButtonCache struct {
	called bool
	key    string
	values []string
	ttl    time.Duration
	err    error
}

func (f *fakeButtonCache) Set(ctx context.Context, key string, values []string, ttl time.Duration) error {
	f.called = true
	f.key = key
	f.values = values
	f.ttl = ttl
	return f.err
}

func TestServiceInitReturnsLegacyResponseAndCachesButtons(t *testing.T) {
	builder := &fakePermissionBuilder{ctx: permission.Context{
		Permissions: []permission.MenuItem{{Index: "1", Label: "系统", Children: []permission.MenuItem{}}},
		Router:      []permission.RouteItem{{Name: "menu_2", Path: "/system/user", ViewKey: "system/user/index", Meta: map[string]string{"menuId": "2"}}},
		ButtonCodes: []string{"user_add"},
	}}
	cache := &fakeButtonCache{}
	svc := NewService(fakeUserRepository{
		user:    &User{ID: 1, Username: "admin", RoleID: 7},
		profile: &Profile{UserID: 1, Avatar: "avatar.png"},
		role:    &Role{ID: 7, Name: "管理员"},
		entries: []QuickEntry{{ID: 3, PermissionID: 2, Sort: 1}},
	}, builder, cache, 30*time.Minute)

	got, appErr := svc.Init(context.Background(), InitInput{UserID: 1, Platform: "admin"})

	if appErr != nil {
		t.Fatalf("expected no app error, got %v", appErr)
	}
	if got.UserID != 1 || got.Username != "admin" || got.Avatar != "avatar.png" || got.RoleName != "管理员" {
		t.Fatalf("base response mismatch: %#v", got)
	}
	if !reflect.DeepEqual(got.ButtonCodes, []string{"user_add"}) || len(got.Permissions) != 1 || len(got.Router) != 1 {
		t.Fatalf("permission response mismatch: %#v", got)
	}
	if len(got.QuickEntry) != 1 || got.QuickEntry[0].ID != 3 || got.QuickEntry[0].PermissionID != 2 || got.QuickEntry[0].Sort != 1 {
		t.Fatalf("quick_entry mismatch: %#v", got.QuickEntry)
	}
	if builder.roleID != 7 || builder.platform != "admin" {
		t.Fatalf("permission builder input mismatch: role=%d platform=%q", builder.roleID, builder.platform)
	}
	if cache.key != "auth_perm_uid_1_admin_rbac_page_grants" || !reflect.DeepEqual(cache.values, []string{"user_add"}) || cache.ttl != 30*time.Minute {
		t.Fatalf("button cache mismatch: %#v", cache)
	}
}

func TestServiceInitIgnoresButtonCacheFailure(t *testing.T) {
	builder := &fakePermissionBuilder{ctx: permission.Context{ButtonCodes: []string{"user_add"}}}
	svc := NewService(fakeUserRepository{
		user: &User{ID: 1, Username: "admin", RoleID: 7},
		role: &Role{ID: 7, Name: "管理员"},
	}, builder, &fakeButtonCache{err: errors.New("redis down")}, time.Minute)

	_, appErr := svc.Init(context.Background(), InitInput{UserID: 1, Platform: "admin"})

	if appErr != nil {
		t.Fatalf("expected cache failure to be ignored, got %v", appErr)
	}
}

func TestServiceInitReturnsNotFoundWhenUserMissing(t *testing.T) {
	svc := NewService(fakeUserRepository{}, &fakePermissionBuilder{}, nil, time.Minute)

	_, appErr := svc.Init(context.Background(), InitInput{UserID: 404, Platform: "admin"})

	if appErr == nil || appErr.Code != 404 {
		t.Fatalf("expected not found, got %#v", appErr)
	}
}

func TestServiceInitSkipsPermissionBuildWhenRoleMissing(t *testing.T) {
	builder := &fakePermissionBuilder{ctx: permission.Context{ButtonCodes: []string{"user_add"}}}
	cache := &fakeButtonCache{}
	svc := NewService(fakeUserRepository{
		user:    &User{ID: 1, Username: "admin", RoleID: 7},
		profile: &Profile{UserID: 1, Avatar: "avatar.png"},
		role:    nil,
		entries: []QuickEntry{{ID: 3, PermissionID: 2, Sort: 1}},
	}, builder, cache, time.Minute)

	got, appErr := svc.Init(context.Background(), InitInput{UserID: 1, Platform: "admin"})

	if appErr != nil {
		t.Fatalf("expected init to succeed with empty permissions, got %v", appErr)
	}
	if builder.called {
		t.Fatalf("expected permission builder to be skipped when role is missing")
	}
	if cache.called {
		t.Fatalf("expected button cache write to be skipped when role is missing")
	}
	if got.RoleName != "" || len(got.ButtonCodes) != 0 || len(got.Permissions) != 0 || len(got.Router) != 0 {
		t.Fatalf("expected empty permission payload when role is missing, got %#v", got)
	}
	if len(got.QuickEntry) != 1 || got.QuickEntry[0].ID != 3 {
		t.Fatalf("quick_entry should still be returned, got %#v", got.QuickEntry)
	}
}

func TestServiceInitWrapsRepositoryError(t *testing.T) {
	svc := NewService(fakeUserRepository{err: errors.New("db down")}, &fakePermissionBuilder{}, nil, time.Minute)

	_, appErr := svc.Init(context.Background(), InitInput{UserID: 1, Platform: "admin"})

	if appErr == nil || appErr.Code != 500 {
		t.Fatalf("expected internal error, got %#v", appErr)
	}
}

func TestServiceInitPropagatesPermissionError(t *testing.T) {
	builder := &fakePermissionBuilder{err: apperror.BadRequest("无效的平台标识: unknown")}
	svc := NewService(fakeUserRepository{
		user: &User{ID: 1, Username: "admin", RoleID: 7},
		role: &Role{ID: 7, Name: "管理员"},
	}, builder, nil, time.Minute)

	_, appErr := svc.Init(context.Background(), InitInput{UserID: 1, Platform: "unknown"})

	if appErr == nil || appErr.Code != 100 {
		t.Fatalf("expected permission app error, got %#v", appErr)
	}
}

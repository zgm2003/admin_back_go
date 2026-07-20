package permission

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/infra/redisclient"

	"gorm.io/gorm"
)

var _ PrincipalVersionBumper = (*GormRepository)(nil)

type fakePrincipalRepository struct {
	mu       sync.Mutex
	snapshot PrincipalSnapshot
	loads    int
	current  []PrincipalVersion
}

func (f *fakePrincipalRepository) LoadSnapshot(context.Context, int64, string) (PrincipalSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.loads++
	return f.snapshot, nil
}

func (f *fakePrincipalRepository) CurrentVersions(context.Context, []PrincipalSubject) ([]PrincipalVersion, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]PrincipalVersion(nil), f.current...), nil
}

func (f *fakePrincipalRepository) AllVersions(context.Context) ([]PrincipalVersion, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]PrincipalVersion(nil), f.current...), nil
}

func (f *fakePrincipalRepository) setSnapshot(snapshot PrincipalSnapshot) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.snapshot = snapshot
}

func (f *fakePrincipalRepository) loadCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.loads
}

type fakePrincipalCache struct {
	mu           sync.Mutex
	snapshot     *PrincipalSnapshot
	invalidating bool
	fail         error
	token        string
}

func (f *fakePrincipalCache) Load(context.Context, int64, string) (*PrincipalSnapshot, PrincipalCacheState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return nil, PrincipalCacheMiss, f.fail
	}
	if f.invalidating {
		return nil, PrincipalCacheInvalidating, nil
	}
	if f.snapshot == nil {
		return nil, PrincipalCacheMiss, nil
	}
	copy := *f.snapshot
	copy.RouteCodes = append([]string(nil), f.snapshot.RouteCodes...)
	copy.ButtonCodes = append([]string(nil), f.snapshot.ButtonCodes...)
	return &copy, PrincipalCacheHit, nil
}

func (f *fakePrincipalCache) Store(_ context.Context, snapshot PrincipalSnapshot, _ time.Duration) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return false, f.fail
	}
	if f.invalidating {
		return false, nil
	}
	copy := snapshot
	f.snapshot = &copy
	return true, nil
}

func (f *fakePrincipalCache) Begin(_ context.Context, _ []PrincipalVersion, token string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return f.fail
	}
	f.invalidating = true
	f.token = token
	return nil
}

func (f *fakePrincipalCache) Publish(_ context.Context, _ []PrincipalVersion, next []PrincipalVersion, token string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return f.fail
	}
	if !f.invalidating || f.token != token {
		return errors.New("principal invalidation token mismatch")
	}
	f.invalidating = false
	f.token = ""
	f.snapshot = nil
	_ = next
	return nil
}

func (f *fakePrincipalCache) Abort(_ context.Context, _ []PrincipalVersion, token string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return f.fail
	}
	if f.token == token {
		f.invalidating = false
		f.token = ""
	}
	return nil
}

func (f *fakePrincipalCache) Reconcile(_ context.Context, _ []PrincipalVersion) error {
	return f.fail
}

func TestPrincipalKeyIsVersionAndRoleScoped(t *testing.T) {
	got := PrincipalKey(12, 7, "admin", 3)
	want := "authz:principal:v1:admin:12:7:3"
	if got != want {
		t.Fatalf("PrincipalKey() = %q, want %q", got, want)
	}
}

func TestPrincipalCacheHitAuthorizesWithZeroRepositoryLoads(t *testing.T) {
	snapshot := PrincipalSnapshot{
		UserID:      12,
		RoleID:      7,
		Platform:    "admin",
		Version:     3,
		UserActive:  true,
		RoleActive:  true,
		RouteCodes:  []string{"user_list"},
		ButtonCodes: []string{"user_edit"},
	}
	repository := &fakePrincipalRepository{snapshot: snapshot}
	cache := &fakePrincipalCache{snapshot: &snapshot}
	service := NewPrincipalService(repository, cache, PrincipalServiceOptions{})

	if appErr := service.Authorize(context.Background(), 12, "admin", "user_list"); appErr != nil {
		t.Fatalf("Authorize() error = %v", appErr)
	}
	if got := repository.loadCount(); got != 0 {
		t.Fatalf("cache hit issued %d repository loads, want zero", got)
	}
}

func TestPrincipalCacheMissBuildsAndThenAuthorizesWithoutSQL(t *testing.T) {
	snapshot := PrincipalSnapshot{UserID: 12, RoleID: 7, Platform: "admin", Version: 3, UserActive: true, RoleActive: true, RouteCodes: []string{"user_list"}}
	repository := &fakePrincipalRepository{snapshot: snapshot}
	cache := &fakePrincipalCache{}
	service := NewPrincipalService(repository, cache, PrincipalServiceOptions{})

	if appErr := service.Authorize(context.Background(), 12, "admin", "user_list"); appErr != nil {
		t.Fatalf("first Authorize() error = %v", appErr)
	}
	if appErr := service.Authorize(context.Background(), 12, "admin", "user_list"); appErr != nil {
		t.Fatalf("second Authorize() error = %v", appErr)
	}
	if got := repository.loadCount(); got != 1 {
		t.Fatalf("repository loads = %d, want one cache-miss load", got)
	}
}

func TestPrincipalCacheFailureFailsClosed(t *testing.T) {
	repository := &fakePrincipalRepository{snapshot: PrincipalSnapshot{UserID: 12, RoleID: 7, Platform: "admin", Version: 1, UserActive: true, RoleActive: true, RouteCodes: []string{"user_list"}}}
	service := NewPrincipalService(repository, &fakePrincipalCache{fail: errors.New("redis down")}, PrincipalServiceOptions{})

	if appErr := service.Authorize(context.Background(), 12, "admin", "user_list"); appErr == nil {
		t.Fatal("Authorize() allowed while Redis was unavailable")
	}
	if got := repository.loadCount(); got != 0 {
		t.Fatalf("Redis failure fell back to SQL with %d loads", got)
	}
}

func TestPrincipalServiceRejectsUnregisteredPlatformBeforeCacheOrRepository(t *testing.T) {
	repository := &fakePrincipalRepository{snapshot: PrincipalSnapshot{
		UserID: 12, RoleID: 7, Platform: "partner_portal", Version: 1,
		UserActive: true, RoleActive: true, RouteCodes: []string{"user_list"},
	}}
	service := NewPrincipalService(repository, &fakePrincipalCache{}, PrincipalServiceOptions{})

	if appErr := service.Authorize(context.Background(), 12, "partner_portal", "user_list"); appErr == nil {
		t.Fatal("Authorize() allowed an unregistered platform")
	}
	if got := repository.loadCount(); got != 0 {
		t.Fatalf("unregistered platform reached repository %d times", got)
	}
}

func TestPrincipalMutationRejectsUnregisteredPlatformBeforeCallback(t *testing.T) {
	repository := &fakePrincipalRepository{current: []PrincipalVersion{{UserID: 12, RoleID: 7, Platform: "partner_portal", Version: 1}}}
	service := NewPrincipalService(repository, &fakePrincipalCache{}, PrincipalServiceOptions{MutationToken: func() (string, error) { return "mutation-unknown", nil }})
	called := false

	err := service.Mutate(context.Background(), []PrincipalSubject{{UserID: 12, Platform: "partner_portal"}}, func() ([]PrincipalVersion, error) {
		called = true
		return []PrincipalVersion{{UserID: 12, RoleID: 7, Platform: "partner_portal", Version: 2}}, nil
	})
	if err == nil {
		t.Fatal("Mutate() allowed an unregistered platform")
	}
	if called {
		t.Fatal("unregistered platform mutation reached callback")
	}
}

func TestPrincipalMutationRejectsMalformedSubjectBeforeCallback(t *testing.T) {
	service := NewPrincipalService(&fakePrincipalRepository{}, &fakePrincipalCache{}, PrincipalServiceOptions{})
	called := false
	err := service.Mutate(context.Background(), []PrincipalSubject{{UserID: 12, Platform: "  "}}, func() ([]PrincipalVersion, error) {
		called = true
		return nil, nil
	})
	if err == nil || called {
		t.Fatalf("malformed principal subject was not rejected: err=%v called=%v", err, called)
	}
}

func TestPrincipalMutationKeepsOldSnapshotUnusable(t *testing.T) {
	oldSnapshot := PrincipalSnapshot{UserID: 12, RoleID: 7, Platform: "admin", Version: 3, UserActive: true, RoleActive: true, RouteCodes: []string{"user_list"}}
	newSnapshot := PrincipalSnapshot{UserID: 12, RoleID: 8, Platform: "admin", Version: 4, UserActive: true, RoleActive: true, RouteCodes: []string{"role_list"}}
	current := []PrincipalVersion{{UserID: 12, RoleID: 7, Platform: "admin", Version: 3}}
	repository := &fakePrincipalRepository{snapshot: oldSnapshot, current: current}
	cache := &fakePrincipalCache{snapshot: &oldSnapshot}
	service := NewPrincipalService(repository, cache, PrincipalServiceOptions{MutationToken: func() (string, error) { return "mutation-1", nil }})

	err := service.Mutate(context.Background(), []PrincipalSubject{{UserID: 12, Platform: "admin"}}, func() ([]PrincipalVersion, error) {
		if appErr := service.Authorize(context.Background(), 12, "admin", "user_list"); appErr == nil {
			t.Fatal("old snapshot remained usable behind invalidation gate")
		}
		repository.setSnapshot(newSnapshot)
		return []PrincipalVersion{{UserID: 12, RoleID: 8, Platform: "admin", Version: 4}}, nil
	})
	if err != nil {
		t.Fatalf("Mutate() error = %v", err)
	}
	if appErr := service.Authorize(context.Background(), 12, "admin", "role_list"); appErr != nil {
		t.Fatalf("new snapshot was not usable after publish: %v", appErr)
	}
	if appErr := service.Authorize(context.Background(), 12, "admin", "user_list"); appErr == nil {
		t.Fatal("old route code remained authorized after version publish")
	}
}

type principalIntegrationResources struct {
	db    *database.Client
	redis *redisclient.Client
}

func openPrincipalIntegrationResources(t *testing.T) principalIntegrationResources {
	t.Helper()
	if os.Getenv("ADMIN_IDENTITY_INTEGRATION") != "1" {
		t.Skip("Docker-only principal integration test")
	}
	redisDB, err := strconv.Atoi(os.Getenv("REDIS_DB"))
	if err != nil {
		redisDB = 0
	}
	db, err := database.Open(config.MySQLConfig{
		DSN: os.Getenv("MYSQL_DSN"), MaxOpenConns: 20, MaxIdleConns: 10, ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf("open integration MySQL: %v", err)
	}
	redisClient, err := redisclient.Open(config.RedisConfig{Addr: os.Getenv("REDIS_ADDR"), Password: os.Getenv("REDIS_PASSWORD"), DB: redisDB})
	if err != nil {
		_ = db.Close()
		t.Fatalf("open integration Redis: %v", err)
	}
	t.Cleanup(func() {
		_ = redisClient.Close()
		_ = db.Close()
	})
	return principalIntegrationResources{db: db, redis: redisClient}
}

type principalIntegrationFixture struct {
	userID       int64
	firstRoleID  int64
	secondRoleID int64
	firstCode    string
	secondCode   string
	thirdCode    string
	firstPermID  int64
	secondPermID int64
	thirdPermID  int64
}

func createPrincipalIntegrationFixture(t *testing.T, resources principalIntegrationResources) principalIntegrationFixture {
	t.Helper()
	ctx := context.Background()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	insert := func(query string, args ...any) int64 {
		result, err := resources.db.SQL.ExecContext(ctx, query, args...)
		if err != nil {
			t.Fatalf("integration insert: %v", err)
		}
		id, err := result.LastInsertId()
		if err != nil {
			t.Fatalf("integration insert id: %v", err)
		}
		return id
	}
	firstRoleID := insert("INSERT INTO roles (name, is_default, is_del) VALUES (?, 2, 2)", "p04-principal-a-"+suffix)
	secondRoleID := insert("INSERT INTO roles (name, is_default, is_del) VALUES (?, 2, 2)", "p04-principal-b-"+suffix)
	firstCode := "p04_principal_first_" + suffix
	secondCode := "p04_principal_second_" + suffix
	thirdCode := "p04_principal_third_" + suffix
	insertPermission := func(name, code string) int64 {
		return insert("INSERT INTO permissions (name, parent_id, platform, type, sort, code, i18n_key, show_menu, status, is_del) VALUES (?, 0, 'admin', 2, 1, ?, ?, 1, 1, 2)", name, code, "p04."+code)
	}
	firstPermID := insertPermission("P04 first "+suffix, firstCode)
	secondPermID := insertPermission("P04 second "+suffix, secondCode)
	thirdPermID := insertPermission("P04 third "+suffix, thirdCode)
	insert("INSERT INTO role_permissions (role_id, permission_id, is_del) VALUES (?, ?, 2)", firstRoleID, firstPermID)
	insert("INSERT INTO role_permissions (role_id, permission_id, is_del) VALUES (?, ?, 2)", secondRoleID, secondPermID)
	userID := insert("INSERT INTO users (role_id, username, status, is_del) VALUES (?, ?, 1, 2)", firstRoleID, "p04-principal-"+suffix)
	if _, err := resources.db.SQL.ExecContext(ctx, "INSERT INTO authz_principal_versions (user_id, platform, version, updated_at) VALUES (?, 'admin', 1, UTC_TIMESTAMP(6))", userID); err != nil {
		t.Fatalf("insert principal version: %v", err)
	}

	t.Cleanup(func() {
		_, _ = resources.db.SQL.ExecContext(ctx, "DELETE FROM authz_principal_versions WHERE user_id = ?", userID)
		_, _ = resources.db.SQL.ExecContext(ctx, "DELETE FROM users WHERE id = ?", userID)
		_, _ = resources.db.SQL.ExecContext(ctx, "DELETE FROM role_permissions WHERE role_id IN (?, ?)", firstRoleID, secondRoleID)
		_, _ = resources.db.SQL.ExecContext(ctx, "DELETE FROM permissions WHERE id IN (?, ?, ?)", firstPermID, secondPermID, thirdPermID)
		_, _ = resources.db.SQL.ExecContext(ctx, "DELETE FROM roles WHERE id IN (?, ?)", firstRoleID, secondRoleID)
	})
	return principalIntegrationFixture{
		userID: userID, firstRoleID: firstRoleID, secondRoleID: secondRoleID,
		firstCode: firstCode, secondCode: secondCode, thirdCode: thirdCode,
		firstPermID: firstPermID, secondPermID: secondPermID, thirdPermID: thirdPermID,
	}
}

type countingPrincipalRepository struct {
	PrincipalRepository
	mu    sync.Mutex
	loads int
}

func (r *countingPrincipalRepository) LoadSnapshot(ctx context.Context, userID int64, platform string) (PrincipalSnapshot, error) {
	r.mu.Lock()
	r.loads++
	r.mu.Unlock()
	return r.PrincipalRepository.LoadSnapshot(ctx, userID, platform)
}

func (r *countingPrincipalRepository) loadCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loads
}

func TestPrincipalIntegrationCacheHitUsesZeroSQLAndMutationsInvalidate(t *testing.T) {
	resources := openPrincipalIntegrationResources(t)
	fixture := createPrincipalIntegrationFixture(t, resources)
	prefix := fmt.Sprintf("p04:principal:%d:", fixture.userID)
	t.Cleanup(func() {
		keys, _ := resources.redis.Redis.Keys(context.Background(), prefix+"*").Result()
		if len(keys) > 0 {
			_ = resources.redis.Redis.Del(context.Background(), keys...).Err()
		}
	})
	repository := &countingPrincipalRepository{PrincipalRepository: NewGormPrincipalRepository(resources.db)}
	cache := NewRedisPrincipalCache(resources.redis, PrincipalCacheConfig{RedisPrefix: prefix})
	service := NewPrincipalService(repository, cache, PrincipalServiceOptions{})

	if err := service.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if appErr := service.Authorize(context.Background(), fixture.userID, "admin", fixture.firstCode); appErr != nil {
		t.Fatalf("cache miss authorization error = %v", appErr)
	}
	if appErr := service.Authorize(context.Background(), fixture.userID, "admin", fixture.firstCode); appErr != nil {
		t.Fatalf("cache hit authorization error = %v", appErr)
	}
	if got := repository.loadCount(); got != 1 {
		t.Fatalf("snapshot SQL loads = %d, want exactly one before cache hit", got)
	}

	subjects := []PrincipalSubject{{UserID: fixture.userID, Platform: "admin"}}
	if err := service.Mutate(context.Background(), subjects, func() ([]PrincipalVersion, error) {
		if appErr := service.Authorize(context.Background(), fixture.userID, "admin", fixture.firstCode); appErr == nil || appErr.Code != "permission.principal_invalidating" {
			t.Fatalf("authorization during mutation = %#v, want invalidating denial", appErr)
		}
		var versions []PrincipalVersion
		err := resources.db.Gorm.WithContext(context.Background()).Transaction(func(tx *gorm.DB) error {
			if err := tx.Table("users").Where("id = ?", fixture.userID).Update("role_id", fixture.secondRoleID).Error; err != nil {
				return err
			}
			var err error
			versions, err = BumpPrincipalVersions(context.Background(), tx, subjects)
			return err
		})
		return versions, err
	}); err != nil {
		t.Fatalf("role mutation error = %v", err)
	}
	if appErr := service.Authorize(context.Background(), fixture.userID, "admin", fixture.firstCode); appErr == nil {
		t.Fatal("old role permission remained authorized")
	}
	if appErr := service.Authorize(context.Background(), fixture.userID, "admin", fixture.secondCode); appErr != nil {
		t.Fatalf("new role permission authorization error = %v", appErr)
	}

	if err := service.Mutate(context.Background(), subjects, func() ([]PrincipalVersion, error) {
		var versions []PrincipalVersion
		err := resources.db.Gorm.WithContext(context.Background()).Transaction(func(tx *gorm.DB) error {
			if err := tx.Table("role_permissions").Where("role_id = ? AND permission_id = ?", fixture.secondRoleID, fixture.secondPermID).Update("is_del", CommonYes).Error; err != nil {
				return err
			}
			if err := tx.Exec("INSERT INTO role_permissions (role_id, permission_id, is_del) VALUES (?, ?, 2)", fixture.secondRoleID, fixture.thirdPermID).Error; err != nil {
				return err
			}
			var err error
			versions, err = BumpPrincipalVersions(context.Background(), tx, subjects)
			return err
		})
		return versions, err
	}); err != nil {
		t.Fatalf("permission grant mutation error = %v", err)
	}
	if appErr := service.Authorize(context.Background(), fixture.userID, "admin", fixture.secondCode); appErr == nil {
		t.Fatal("old role grant remained authorized")
	}
	if appErr := service.Authorize(context.Background(), fixture.userID, "admin", fixture.thirdCode); appErr != nil {
		t.Fatalf("new role grant authorization error = %v", appErr)
	}
}

func TestPrincipalIntegrationNextMutationRepairsCommittedGate(t *testing.T) {
	resources := openPrincipalIntegrationResources(t)
	fixture := createPrincipalIntegrationFixture(t, resources)
	prefix := fmt.Sprintf("p04:principal-repair:%d:", fixture.userID)
	t.Cleanup(func() {
		keys, _ := resources.redis.Redis.Keys(context.Background(), prefix+"*").Result()
		if len(keys) > 0 {
			_ = resources.redis.Redis.Del(context.Background(), keys...).Err()
		}
	})
	repository := NewGormPrincipalRepository(resources.db)
	cache := NewRedisPrincipalCache(resources.redis, PrincipalCacheConfig{RedisPrefix: prefix})
	service := NewPrincipalService(repository, cache, PrincipalServiceOptions{})
	if err := service.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	subjects := []PrincipalSubject{{UserID: fixture.userID, Platform: "admin"}}
	current, err := repository.CurrentVersions(context.Background(), subjects)
	if err != nil {
		t.Fatalf("CurrentVersions() error = %v", err)
	}
	if err := cache.Begin(context.Background(), current, "crashed-mutation"); err != nil {
		t.Fatalf("begin simulated crashed mutation: %v", err)
	}
	if err := resources.db.Gorm.WithContext(context.Background()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("users").Where("id = ?", fixture.userID).Update("role_id", fixture.secondRoleID).Error; err != nil {
			return err
		}
		_, err := BumpPrincipalVersions(context.Background(), tx, subjects)
		return err
	}); err != nil {
		t.Fatalf("commit simulated crashed mutation: %v", err)
	}
	if appErr := service.Authorize(context.Background(), fixture.userID, "admin", fixture.secondCode); appErr == nil || appErr.Code != "permission.principal_invalidating" {
		t.Fatalf("crashed gate did not remain fail closed: %#v", appErr)
	}

	err = service.Mutate(context.Background(), subjects, func() ([]PrincipalVersion, error) {
		var versions []PrincipalVersion
		err := resources.db.Gorm.WithContext(context.Background()).Transaction(func(tx *gorm.DB) error {
			var bumpErr error
			versions, bumpErr = BumpPrincipalVersions(context.Background(), tx, subjects)
			return bumpErr
		})
		return versions, err
	})
	if err != nil {
		t.Fatalf("next mutation did not repair committed gate: %v", err)
	}
	if appErr := service.Authorize(context.Background(), fixture.userID, "admin", fixture.secondCode); appErr != nil {
		t.Fatalf("repaired principal did not authorize new role: %v", appErr)
	}
}

func TestPrincipalIntegrationRejectsStaleMutationVersion(t *testing.T) {
	resources := openPrincipalIntegrationResources(t)
	prefix := fmt.Sprintf("p04:principal-stale:%d:", time.Now().UnixNano())
	t.Cleanup(func() {
		keys, _ := resources.redis.Redis.Keys(context.Background(), prefix+"*").Result()
		if len(keys) > 0 {
			_ = resources.redis.Redis.Del(context.Background(), keys...).Err()
		}
	})
	cache := NewRedisPrincipalCache(resources.redis, PrincipalCacheConfig{RedisPrefix: prefix})
	current := []PrincipalVersion{{UserID: 42, RoleID: 7, Platform: "admin", Version: 1}}
	next := []PrincipalVersion{{UserID: 42, RoleID: 7, Platform: "admin", Version: 2}}
	if err := cache.Begin(context.Background(), current, "winner"); err != nil {
		t.Fatalf("winner Begin() error = %v", err)
	}
	if err := cache.Publish(context.Background(), current, next, "winner"); err != nil {
		t.Fatalf("winner Publish() error = %v", err)
	}
	if err := cache.Begin(context.Background(), current, "stale"); err == nil {
		t.Fatal("stale mutation downgraded the published principal version")
	}
}

func TestPrincipalIntegrationReconcilePreservesActiveGate(t *testing.T) {
	resources := openPrincipalIntegrationResources(t)
	prefix := fmt.Sprintf("p04:principal-active:%d:", time.Now().UnixNano())
	t.Cleanup(func() {
		keys, _ := resources.redis.Redis.Keys(context.Background(), prefix+"*").Result()
		if len(keys) > 0 {
			_ = resources.redis.Redis.Del(context.Background(), keys...).Err()
		}
	})
	cache := NewRedisPrincipalCache(resources.redis, PrincipalCacheConfig{RedisPrefix: prefix})
	current := []PrincipalVersion{{UserID: 42, RoleID: 7, Platform: "admin", Version: 1}}
	if err := cache.Begin(context.Background(), current, "active"); err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if err := cache.Reconcile(context.Background(), current); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if _, state, err := cache.Load(context.Background(), 42, "admin"); err != nil || state != PrincipalCacheInvalidating {
		t.Fatalf("active gate after reconcile = state:%d err:%v", state, err)
	}
}

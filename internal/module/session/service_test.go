package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/config"
)

type fakeSessionCache struct {
	values      map[string]string
	getKeys     []string
	setKey      string
	setValue    string
	setTTL      time.Duration
	expireKey   string
	expireTTL   time.Duration
	deletedKey  string
	deletedKeys []string
}

func (f *fakeSessionCache) Get(ctx context.Context, key string) (string, error) {
	f.getKeys = append(f.getKeys, key)
	return f.values[key], nil
}

func (f *fakeSessionCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	f.setKey = key
	f.setValue = value
	f.setTTL = ttl
	return nil
}

func (f *fakeSessionCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	f.expireKey = key
	f.expireTTL = ttl
	return nil
}

func (f *fakeSessionCache) Del(ctx context.Context, key string) error {
	f.deletedKey = key
	f.deletedKeys = append(f.deletedKeys, key)
	return nil
}

type fakeSessionRepository struct {
	findHash        string
	findRefreshHash string
	findLatestKey   string
	session         *Session
	refreshSession  *Session
	latestSession   *Session
	rotatedID       int64
	rotation        Rotation
	revokedID       int64
	revokedAt       time.Time
	err             error
}

func (f *fakeSessionRepository) FindValidByAccessHash(ctx context.Context, accessHash string, now time.Time) (*Session, error) {
	f.findHash = accessHash
	return f.session, f.err
}

func (f *fakeSessionRepository) FindLatestActiveByUserPlatform(ctx context.Context, userID int64, platform string, now time.Time) (*Session, error) {
	f.findLatestKey = platform
	return f.latestSession, f.err
}

func (f *fakeSessionRepository) FindValidByRefreshHash(ctx context.Context, refreshHash string, now time.Time) (*Session, error) {
	f.findRefreshHash = refreshHash
	return f.refreshSession, f.err
}

func (f *fakeSessionRepository) Rotate(ctx context.Context, sessionID int64, rotation Rotation) error {
	f.rotatedID = sessionID
	f.rotation = rotation
	return f.err
}

func (f *fakeSessionRepository) Revoke(ctx context.Context, sessionID int64, revokedAt time.Time) error {
	f.revokedID = sessionID
	f.revokedAt = revokedAt
	return f.err
}

type fakePolicyProvider struct {
	policies map[string]*AuthPolicy
	err      error
}

func (f fakePolicyProvider) Policy(ctx context.Context, platform string) (*AuthPolicy, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.policies[platform], nil
}

func allowPolicies() fakePolicyProvider {
	return fakePolicyProvider{policies: map[string]*AuthPolicy{
		"admin": {BindPlatform: false, AccessTTL: 4 * time.Hour, RefreshTTL: 14 * 24 * time.Hour},
		"app":   {BindPlatform: false, AccessTTL: 8 * time.Hour, RefreshTTL: 30 * 24 * time.Hour},
		"web":   {BindPlatform: false, AccessTTL: 4 * time.Hour, RefreshTTL: 14 * 24 * time.Hour},
	}}
}

type sequenceTokenGenerator struct {
	values []string
}

func (g *sequenceTokenGenerator) MakeToken(bytes int) (string, error) {
	if len(g.values) == 0 {
		return "", errors.New("empty token sequence")
	}
	value := g.values[0]
	g.values = g.values[1:]
	return value, nil
}

func TestHashTokenMatchesLegacyPHPAlgorithm(t *testing.T) {
	got, err := HashToken("access-token", "pepper-value")
	if err != nil {
		t.Fatalf("expected hash token to succeed, got %v", err)
	}

	want := "b34e920808f14cffc2003f5ee7c8a3f29cb02961e39d52c64383a097b8c2be95"
	if got != want {
		t.Fatalf("expected legacy sha256 hash %s, got %s", want, got)
	}
}

func TestHashTokenRejectsUnsafePepper(t *testing.T) {
	for _, pepper := range []string{"", "change_me_to_long_random"} {
		if _, err := HashToken("token", pepper); err == nil {
			t.Fatalf("expected unsafe pepper %q to be rejected", pepper)
		}
	}
}

func TestAuthenticatorResolvesCachedSessionAndRefreshesTTL(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.Local)
	hash, err := HashToken("valid-token", "pepper-value")
	if err != nil {
		t.Fatalf("hash token: %v", err)
	}
	cacheKey := "token:" + hash
	cache := &fakeSessionCache{values: map[string]string{
		cacheKey: "12|2026-05-02 12:30:00|127.0.0.1|admin|device-1|34",
	}}
	repo := &fakeSessionRepository{}
	auth := NewAuthenticator(AuthenticatorDeps{
		Config:         config.TokenConfig{Pepper: "pepper-value", RedisPrefix: "token:", SessionCacheTTL: 30 * time.Minute},
		Cache:          cache,
		Repository:     repo,
		PolicyProvider: allowPolicies(),
		Now:            func() time.Time { return now },
	})

	identity, appErr := auth.Authenticate(context.Background(), TokenInput{
		AccessToken: "valid-token",
		Platform:    "web",
		DeviceID:    "device-1",
		ClientIP:    "127.0.0.1",
	})

	if appErr != nil {
		t.Fatalf("expected authenticate to succeed, got %v", appErr)
	}
	if identity.UserID != 12 || identity.SessionID != 34 || identity.Platform != "admin" {
		t.Fatalf("unexpected identity: %#v", identity)
	}
	if repo.findHash != "" {
		t.Fatalf("expected cached session to avoid mysql lookup, got hash %q", repo.findHash)
	}
	if cache.expireKey != cacheKey || cache.expireTTL != 30*time.Minute {
		t.Fatalf("expected token cache ttl refresh, got key=%q ttl=%s", cache.expireKey, cache.expireTTL)
	}
}

func TestAuthenticatorFallsBackToMySQLAndWritesRedis(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.Local)
	hash, err := HashToken("valid-token", "pepper-value")
	if err != nil {
		t.Fatalf("hash token: %v", err)
	}
	cache := &fakeSessionCache{values: map[string]string{}}
	repo := &fakeSessionRepository{session: &Session{
		ID:        55,
		UserID:    44,
		Platform:  "app",
		DeviceID:  "device-2",
		IP:        "10.0.0.8",
		ExpiresAt: now.Add(10 * time.Minute),
	}}
	auth := NewAuthenticator(AuthenticatorDeps{
		Config:         config.TokenConfig{Pepper: "pepper-value", RedisPrefix: "token:", SessionCacheTTL: 30 * time.Minute},
		Cache:          cache,
		Repository:     repo,
		PolicyProvider: allowPolicies(),
		Now:            func() time.Time { return now },
	})

	identity, appErr := auth.Authenticate(context.Background(), TokenInput{AccessToken: "valid-token", Platform: "app"})

	if appErr != nil {
		t.Fatalf("expected authenticate to succeed, got %v", appErr)
	}
	if identity.UserID != 44 || identity.SessionID != 55 || identity.Platform != "app" {
		t.Fatalf("unexpected identity: %#v", identity)
	}
	if repo.findHash != hash {
		t.Fatalf("expected mysql lookup by hash %q, got %q", hash, repo.findHash)
	}
	if cache.setKey != "token:"+hash {
		t.Fatalf("expected redis set key token hash, got %q", cache.setKey)
	}
	if cache.setValue != "44|2026-05-02 12:10:00|10.0.0.8|app|device-2|55" {
		t.Fatalf("unexpected redis cache value: %q", cache.setValue)
	}
}

func TestAuthenticatorRejectsInvalidCurrentPlatform(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.Local)
	hash, err := HashToken("valid-token", "pepper-value")
	if err != nil {
		t.Fatalf("hash token: %v", err)
	}
	cache := &fakeSessionCache{values: map[string]string{
		"token:" + hash: "12|2026-05-02 12:30:00|127.0.0.1|admin|device-1|34",
	}}
	auth := NewAuthenticator(AuthenticatorDeps{
		Config: config.TokenConfig{Pepper: "pepper-value", RedisPrefix: "token:", SessionCacheTTL: 30 * time.Minute},
		Cache:  cache,
		PolicyProvider: fakePolicyProvider{policies: map[string]*AuthPolicy{
			"admin": {BindPlatform: true},
		}},
		Now: func() time.Time { return now },
	})

	identity, appErr := auth.Authenticate(context.Background(), TokenInput{AccessToken: "valid-token", Platform: "missing"})

	if identity != nil {
		t.Fatalf("expected nil identity, got %#v", identity)
	}
	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "无效的平台标识" {
		t.Fatalf("expected invalid platform app error, got %#v", appErr)
	}
}

func TestAuthenticatorRejectsPlatformMismatchWhenPolicyBindsPlatform(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.Local)
	hash, err := HashToken("valid-token", "pepper-value")
	if err != nil {
		t.Fatalf("hash token: %v", err)
	}
	cache := &fakeSessionCache{values: map[string]string{
		"token:" + hash: "12|2026-05-02 12:30:00|127.0.0.1|admin|device-1|34",
	}}
	auth := NewAuthenticator(AuthenticatorDeps{
		Config: config.TokenConfig{Pepper: "pepper-value", RedisPrefix: "token:", SessionCacheTTL: 30 * time.Minute},
		Cache:  cache,
		PolicyProvider: fakePolicyProvider{policies: map[string]*AuthPolicy{
			"admin": {BindPlatform: true},
			"app":   {BindPlatform: false},
		}},
		Now: func() time.Time { return now },
	})

	identity, appErr := auth.Authenticate(context.Background(), TokenInput{AccessToken: "valid-token", Platform: "app"})

	if identity != nil {
		t.Fatalf("expected nil identity, got %#v", identity)
	}
	if appErr == nil || appErr.Code != apperror.CodeUnauthorized || appErr.Message != "平台不匹配" {
		t.Fatalf("expected platform mismatch app error, got %#v", appErr)
	}
}

func TestAuthenticatorRejectsDeviceMismatchWhenPolicyBindsDevice(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.Local)
	hash, err := HashToken("valid-token", "pepper-value")
	if err != nil {
		t.Fatalf("hash token: %v", err)
	}
	cache := &fakeSessionCache{values: map[string]string{
		"token:" + hash: "12|2026-05-02 12:30:00|127.0.0.1|admin|device-1|34",
	}}
	auth := NewAuthenticator(AuthenticatorDeps{
		Config: config.TokenConfig{Pepper: "pepper-value", RedisPrefix: "token:", SessionCacheTTL: 30 * time.Minute},
		Cache:  cache,
		PolicyProvider: fakePolicyProvider{policies: map[string]*AuthPolicy{
			"admin": {BindPlatform: true, BindDevice: true},
		}},
		Now: func() time.Time { return now },
	})

	identity, appErr := auth.Authenticate(context.Background(), TokenInput{
		AccessToken: "valid-token",
		Platform:    "admin",
		DeviceID:    "device-2",
		ClientIP:    "127.0.0.1",
	})

	if identity != nil {
		t.Fatalf("expected nil identity, got %#v", identity)
	}
	if appErr == nil || appErr.Code != apperror.CodeUnauthorized || appErr.Message != "设备变更，请重新登录" {
		t.Fatalf("expected device mismatch app error, got %#v", appErr)
	}
}

func TestAuthenticatorRejectsIPMismatchWhenPolicyBindsIPAndDeletesRedis(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.Local)
	hash, err := HashToken("valid-token", "pepper-value")
	if err != nil {
		t.Fatalf("hash token: %v", err)
	}
	cacheKey := "token:" + hash
	cache := &fakeSessionCache{values: map[string]string{
		cacheKey: "12|2026-05-02 12:30:00|127.0.0.1|admin|device-1|34",
	}}
	auth := NewAuthenticator(AuthenticatorDeps{
		Config: config.TokenConfig{Pepper: "pepper-value", RedisPrefix: "token:", SessionCacheTTL: 30 * time.Minute},
		Cache:  cache,
		PolicyProvider: fakePolicyProvider{policies: map[string]*AuthPolicy{
			"admin": {BindPlatform: true, BindIP: true},
		}},
		Now: func() time.Time { return now },
	})

	identity, appErr := auth.Authenticate(context.Background(), TokenInput{
		AccessToken: "valid-token",
		Platform:    "admin",
		DeviceID:    "device-1",
		ClientIP:    "10.0.0.1",
	})

	if identity != nil {
		t.Fatalf("expected nil identity, got %#v", identity)
	}
	if appErr == nil || appErr.Code != apperror.CodeUnauthorized || appErr.Message != "IP地址变动" {
		t.Fatalf("expected ip mismatch app error, got %#v", appErr)
	}
	if cache.deletedKey != cacheKey {
		t.Fatalf("expected mismatched ip to delete token cache key, got %q", cache.deletedKey)
	}
}

func TestAuthenticatorRejectsStaleSingleSessionAndDeletesRedis(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.Local)
	hash, err := HashToken("valid-token", "pepper-value")
	if err != nil {
		t.Fatalf("hash token: %v", err)
	}
	cacheKey := "token:" + hash
	pointerKey := "token:cur_sess:admin:12"
	cache := &fakeSessionCache{values: map[string]string{
		cacheKey:   "12|2026-05-02 12:30:00|127.0.0.1|admin|device-1|34",
		pointerKey: "99",
	}}
	repo := &fakeSessionRepository{latestSession: &Session{ID: 99, UserID: 12, Platform: "admin"}}
	auth := NewAuthenticator(AuthenticatorDeps{
		Config: config.TokenConfig{
			Pepper:                  "pepper-value",
			RedisPrefix:             "token:",
			SessionCacheTTL:         30 * time.Minute,
			SingleSessionPointerTTL: 30 * 24 * time.Hour,
		},
		Cache:      cache,
		Repository: repo,
		PolicyProvider: fakePolicyProvider{policies: map[string]*AuthPolicy{
			"admin": {BindPlatform: true, SingleSessionPerPlatform: true},
		}},
		Now: func() time.Time { return now },
	})

	identity, appErr := auth.Authenticate(context.Background(), TokenInput{
		AccessToken: "valid-token",
		Platform:    "admin",
		DeviceID:    "device-1",
		ClientIP:    "127.0.0.1",
	})

	if identity != nil {
		t.Fatalf("expected nil identity, got %#v", identity)
	}
	if appErr == nil || appErr.Code != apperror.CodeUnauthorized || appErr.Message != "账号已在其他设备登录" {
		t.Fatalf("expected stale single session app error, got %#v", appErr)
	}
	if cache.deletedKey != cacheKey {
		t.Fatalf("expected stale single session to delete token cache key, got %q", cache.deletedKey)
	}
}

func TestAuthenticatorRebuildsSingleSessionPointerFromRepository(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.Local)
	hash, err := HashToken("valid-token", "pepper-value")
	if err != nil {
		t.Fatalf("hash token: %v", err)
	}
	cacheKey := "token:" + hash
	cache := &fakeSessionCache{values: map[string]string{
		cacheKey: "12|2026-05-02 12:30:00|127.0.0.1|admin|device-1|34",
	}}
	repo := &fakeSessionRepository{latestSession: &Session{ID: 34, UserID: 12, Platform: "admin"}}
	auth := NewAuthenticator(AuthenticatorDeps{
		Config: config.TokenConfig{
			Pepper:                  "pepper-value",
			RedisPrefix:             "token:",
			SessionCacheTTL:         30 * time.Minute,
			SingleSessionPointerTTL: 30 * 24 * time.Hour,
		},
		Cache:      cache,
		Repository: repo,
		PolicyProvider: fakePolicyProvider{policies: map[string]*AuthPolicy{
			"admin": {BindPlatform: true, SingleSessionPerPlatform: true},
		}},
		Now: func() time.Time { return now },
	})

	identity, appErr := auth.Authenticate(context.Background(), TokenInput{
		AccessToken: "valid-token",
		Platform:    "admin",
		DeviceID:    "device-1",
		ClientIP:    "127.0.0.1",
	})

	if appErr != nil {
		t.Fatalf("expected authenticate to succeed, got %v", appErr)
	}
	if identity.UserID != 12 || identity.SessionID != 34 || identity.Platform != "admin" {
		t.Fatalf("unexpected identity: %#v", identity)
	}
	if cache.setKey != "token:cur_sess:admin:12" || cache.setValue != "34" || cache.setTTL != 30*24*time.Hour {
		t.Fatalf("expected single session pointer rebuild, key=%q value=%q ttl=%s", cache.setKey, cache.setValue, cache.setTTL)
	}
}

func TestAuthenticatorRejectsExpiredCachedSessionAndDeletesRedis(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.Local)
	hash, err := HashToken("expired-token", "pepper-value")
	if err != nil {
		t.Fatalf("hash token: %v", err)
	}
	cacheKey := "token:" + hash
	cache := &fakeSessionCache{values: map[string]string{
		cacheKey: "12|2026-05-02 11:59:59|127.0.0.1|admin|device-1|34",
	}}
	auth := NewAuthenticator(AuthenticatorDeps{
		Config: config.TokenConfig{Pepper: "pepper-value", RedisPrefix: "token:", SessionCacheTTL: 30 * time.Minute},
		Cache:  cache,
		Now:    func() time.Time { return now },
	})

	identity, appErr := auth.Authenticate(context.Background(), TokenInput{AccessToken: "expired-token"})

	if identity != nil {
		t.Fatalf("expected nil identity, got %#v", identity)
	}
	if appErr == nil || appErr.Code != apperror.CodeUnauthorized || appErr.Message != "Token已过期" {
		t.Fatalf("expected token expired app error, got %#v", appErr)
	}
	if cache.deletedKey != cacheKey {
		t.Fatalf("expected expired cache key to be deleted, got %q", cache.deletedKey)
	}
}

func TestAuthenticatorFailsClosedWithoutRepositoryOnCacheMiss(t *testing.T) {
	auth := NewAuthenticator(AuthenticatorDeps{
		Config: config.TokenConfig{Pepper: "pepper-value", RedisPrefix: "token:", SessionCacheTTL: 30 * time.Minute},
		Cache:  &fakeSessionCache{values: map[string]string{}},
		Now:    func() time.Time { return time.Date(2026, 5, 2, 12, 0, 0, 0, time.Local) },
	})

	identity, appErr := auth.Authenticate(context.Background(), TokenInput{AccessToken: "valid-token"})

	if identity != nil {
		t.Fatalf("expected nil identity, got %#v", identity)
	}
	if appErr == nil || appErr.Code != apperror.CodeUnauthorized || appErr.Message != "Token认证未配置" {
		t.Fatalf("expected token auth not configured, got %#v", appErr)
	}
}

func TestAuthenticatorReturnsServerErrorOnRepositoryFailure(t *testing.T) {
	auth := NewAuthenticator(AuthenticatorDeps{
		Config:     config.TokenConfig{Pepper: "pepper-value", RedisPrefix: "token:", SessionCacheTTL: 30 * time.Minute},
		Cache:      &fakeSessionCache{values: map[string]string{}},
		Repository: &fakeSessionRepository{err: errors.New("mysql down")},
		Now:        func() time.Time { return time.Date(2026, 5, 2, 12, 0, 0, 0, time.Local) },
	})

	identity, appErr := auth.Authenticate(context.Background(), TokenInput{AccessToken: "valid-token"})

	if identity != nil {
		t.Fatalf("expected nil identity, got %#v", identity)
	}
	if appErr == nil || appErr.Code != apperror.CodeInternal {
		t.Fatalf("expected internal error, got %#v", appErr)
	}
}

func TestAuthenticatorRefreshRotatesTokensAndDeletesOldAccessCache(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.Local)
	refreshHash, err := HashToken("old-refresh-token", "pepper-value")
	if err != nil {
		t.Fatalf("hash refresh token: %v", err)
	}
	oldAccessHash, err := HashToken("old-access-token", "pepper-value")
	if err != nil {
		t.Fatalf("hash access token: %v", err)
	}
	generator := &sequenceTokenGenerator{values: []string{"new-access-token", "new-refresh-token"}}
	cache := &fakeSessionCache{values: map[string]string{}}
	repo := &fakeSessionRepository{refreshSession: &Session{
		ID:               55,
		UserID:           44,
		AccessTokenHash:  oldAccessHash,
		RefreshTokenHash: refreshHash,
		Platform:         "admin",
		RefreshExpiresAt: now.Add(2 * time.Hour),
	}}
	auth := NewAuthenticator(AuthenticatorDeps{
		Config:         config.TokenConfig{Pepper: "pepper-value", RedisPrefix: "token:", SessionCacheTTL: 30 * time.Minute},
		Cache:          cache,
		Repository:     repo,
		PolicyProvider: allowPolicies(),
		TokenGenerator: generator.MakeToken,
		Now:            func() time.Time { return now },
	})

	result, appErr := auth.Refresh(context.Background(), RefreshInput{
		RefreshToken: "old-refresh-token",
		ClientIP:     "10.0.0.9",
		UserAgent:    "test-agent",
	})

	if appErr != nil {
		t.Fatalf("expected refresh to succeed, got %v", appErr)
	}
	if result.AccessToken != "new-access-token" || result.RefreshToken != "new-refresh-token" {
		t.Fatalf("unexpected refresh tokens: %#v", result)
	}
	if result.ExpiresIn != int((4*time.Hour).Seconds()) || result.RefreshExpiresIn != int((14*24*time.Hour).Seconds()) {
		t.Fatalf("unexpected token ttl result: %#v", result)
	}
	if repo.findRefreshHash != refreshHash {
		t.Fatalf("expected refresh lookup by hash %q, got %q", refreshHash, repo.findRefreshHash)
	}
	if repo.rotatedID != 55 {
		t.Fatalf("expected rotated session 55, got %d", repo.rotatedID)
	}
	if repo.rotation.AccessTokenHash == oldAccessHash || repo.rotation.AccessTokenHash == "" {
		t.Fatalf("expected new access token hash, got %q", repo.rotation.AccessTokenHash)
	}
	if repo.rotation.RefreshTokenHash == refreshHash || repo.rotation.RefreshTokenHash == "" {
		t.Fatalf("expected new refresh token hash, got %q", repo.rotation.RefreshTokenHash)
	}
	if !repo.rotation.ExpiresAt.Equal(now.Add(4 * time.Hour)) {
		t.Fatalf("expected access expiry %s, got %s", now.Add(4*time.Hour), repo.rotation.ExpiresAt)
	}
	if !repo.rotation.RefreshExpiresAt.Equal(now.Add(2 * time.Hour)) {
		t.Fatalf("expected refresh expiry unchanged, got %s", repo.rotation.RefreshExpiresAt)
	}
	if repo.rotation.IP != "10.0.0.9" || repo.rotation.UserAgent != "test-agent" {
		t.Fatalf("expected ip/ua rotation, got %#v", repo.rotation)
	}
	if cache.deletedKey != "token:"+oldAccessHash {
		t.Fatalf("expected old access cache deleted, got %q", cache.deletedKey)
	}
}

func TestAuthenticatorRefreshRejectsInvalidRefreshToken(t *testing.T) {
	auth := NewAuthenticator(AuthenticatorDeps{
		Config: config.TokenConfig{Pepper: "pepper-value", RedisPrefix: "token:"},
	})

	result, appErr := auth.Refresh(context.Background(), RefreshInput{})

	if result != nil {
		t.Fatalf("expected nil result, got %#v", result)
	}
	if appErr == nil || appErr.Code != apperror.CodeUnauthorized || appErr.Message != "缺少刷新令牌" {
		t.Fatalf("expected missing refresh token error, got %#v", appErr)
	}
}

func TestAuthenticatorRefreshRejectsExpiredRefreshSession(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.Local)
	refreshHash, err := HashToken("old-refresh-token", "pepper-value")
	if err != nil {
		t.Fatalf("hash refresh token: %v", err)
	}
	repo := &fakeSessionRepository{refreshSession: &Session{
		ID:               55,
		UserID:           44,
		RefreshTokenHash: refreshHash,
		Platform:         "admin",
		RefreshExpiresAt: now.Add(-time.Second),
	}}
	auth := NewAuthenticator(AuthenticatorDeps{
		Config:     config.TokenConfig{Pepper: "pepper-value", RedisPrefix: "token:"},
		Repository: repo,
		Now:        func() time.Time { return now },
	})

	result, appErr := auth.Refresh(context.Background(), RefreshInput{RefreshToken: "old-refresh-token"})

	if result != nil {
		t.Fatalf("expected nil result, got %#v", result)
	}
	if appErr == nil || appErr.Code != apperror.CodeUnauthorized || appErr.Message != "刷新令牌已过期，请重新登录" {
		t.Fatalf("expected expired refresh token error, got %#v", appErr)
	}
}

func TestAuthenticatorRefreshRejectsStaleSingleSession(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.Local)
	refreshHash, err := HashToken("old-refresh-token", "pepper-value")
	if err != nil {
		t.Fatalf("hash refresh token: %v", err)
	}
	cache := &fakeSessionCache{values: map[string]string{
		"token:cur_sess:admin:44": "99",
	}}
	repo := &fakeSessionRepository{refreshSession: &Session{
		ID:               55,
		UserID:           44,
		RefreshTokenHash: refreshHash,
		Platform:         "admin",
		RefreshExpiresAt: now.Add(time.Hour),
	}, latestSession: &Session{ID: 99, UserID: 44, Platform: "admin"}}
	auth := NewAuthenticator(AuthenticatorDeps{
		Config:     config.TokenConfig{Pepper: "pepper-value", RedisPrefix: "token:", SingleSessionPointerTTL: 30 * 24 * time.Hour},
		Cache:      cache,
		Repository: repo,
		PolicyProvider: fakePolicyProvider{policies: map[string]*AuthPolicy{
			"admin": {SingleSessionPerPlatform: true, AccessTTL: 4 * time.Hour, RefreshTTL: 14 * 24 * time.Hour},
		}},
		Now: func() time.Time { return now },
	})

	result, appErr := auth.Refresh(context.Background(), RefreshInput{RefreshToken: "old-refresh-token"})

	if result != nil {
		t.Fatalf("expected nil result, got %#v", result)
	}
	if appErr == nil || appErr.Code != apperror.CodeUnauthorized || appErr.Message != "账号已在其他设备登录，请重新登录" {
		t.Fatalf("expected stale session error, got %#v", appErr)
	}
}

func TestAuthenticatorLogoutRevokesSessionAndClearsTokenAndPointer(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.Local)
	accessHash, err := HashToken("access-token", "pepper-value")
	if err != nil {
		t.Fatalf("hash access token: %v", err)
	}
	cache := &fakeSessionCache{values: map[string]string{
		"token:cur_sess:admin:44": "55",
	}}
	repo := &fakeSessionRepository{session: &Session{
		ID:              55,
		UserID:          44,
		AccessTokenHash: accessHash,
		Platform:        "admin",
		ExpiresAt:       now.Add(time.Hour),
	}}
	auth := NewAuthenticator(AuthenticatorDeps{
		Config:     config.TokenConfig{Pepper: "pepper-value", RedisPrefix: "token:"},
		Cache:      cache,
		Repository: repo,
		Now:        func() time.Time { return now },
	})

	appErr := auth.Logout(context.Background(), "access-token")

	if appErr != nil {
		t.Fatalf("expected logout to succeed, got %v", appErr)
	}
	if repo.findHash != accessHash {
		t.Fatalf("expected lookup by access hash %q, got %q", accessHash, repo.findHash)
	}
	if repo.revokedID != 55 || !repo.revokedAt.Equal(now) {
		t.Fatalf("expected revoke session 55 at now, got id=%d at=%s", repo.revokedID, repo.revokedAt)
	}
	if !containsString(cache.deletedKeys, "token:"+accessHash) {
		t.Fatalf("expected token cache deletion, got %#v", cache.deletedKeys)
	}
	if !containsString(cache.deletedKeys, "token:cur_sess:admin:44") {
		t.Fatalf("expected pointer deletion, got %#v", cache.deletedKeys)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

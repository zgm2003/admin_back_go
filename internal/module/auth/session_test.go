package auth

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/infra/accesstoken"
	"admin_back_go/internal/shared/apperror"
	"admin_back_go/internal/shared/enum"
)

var _ Lifecycle = (*SessionLifecycle)(nil)

func TestSessionLifecyclePublicContractIssuesCredentials(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	repository := &fakeSessionRepository{createdID: 42}
	lifecycle := NewSessionLifecycle(LifecycleDeps{
		Config:         config.TokenConfig{RedisPrefix: "token:", SessionCacheTTL: 30 * time.Minute},
		Repository:     repository,
		PolicyProvider: allowPolicies(),
		AccessCodec:    testJWTCodec(),
		TokenPepper:    "0123456789abcdef0123456789abcdef",
		TokenGenerator: (&sequenceTokenGenerator{values: []string{"refresh-token"}}).MakeToken,
		Now:            func() time.Time { return now },
	})

	credentials, appErr := lifecycle.Issue(context.Background(), IssueCommand{
		UserID:   7,
		Platform: "admin",
		DeviceID: "device-a",
	})
	if appErr != nil {
		t.Fatalf("issue credentials: %v", appErr)
	}
	if credentials == nil || credentials.AccessToken == "" || credentials.RefreshToken != "refresh-token" {
		t.Fatalf("unexpected credentials: %#v", credentials)
	}
}

func TestCachedSessionUsesVersionedJSONPayload(t *testing.T) {
	revokedAt := time.Date(2026, 5, 2, 12, 5, 0, 0, time.UTC)
	session := &Session{
		ID:               42,
		UserID:           7,
		Platform:         "admin",
		DeviceID:         "device-a",
		IP:               "127.0.0.1",
		ExpiresAt:        time.Date(2026, 5, 2, 13, 0, 0, 0, time.UTC),
		RefreshExpiresAt: time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC),
		RevokedAt:        &revokedAt,
	}

	encoded := cacheValue(session)
	var payload CachedSession
	if err := json.Unmarshal([]byte(encoded), &payload); err != nil {
		t.Fatalf("decode cache JSON: %v; payload=%q", err, encoded)
	}
	if payload.SchemaVersion != SessionCacheSchemaVersion {
		t.Fatalf("schema version = %d, want %d", payload.SchemaVersion, SessionCacheSchemaVersion)
	}
	decoded, err := parseCachedSession(encoded, time.UTC)
	if err != nil {
		t.Fatalf("parse cached session: %v", err)
	}
	if decoded.ID != session.ID || decoded.UserID != session.UserID || decoded.RefreshExpiresAt != session.RefreshExpiresAt || decoded.RevokedAt == nil || !decoded.RevokedAt.Equal(revokedAt) {
		t.Fatalf("round-trip mismatch: %#v", decoded)
	}
}

func TestCachedSessionRejectsUnknownSchemaVersion(t *testing.T) {
	_, err := parseCachedSession(`{"session_id":42,"user_id":7,"schema_version":999}`, time.UTC)
	if !errors.Is(err, ErrUnsupportedSessionCacheSchema) {
		t.Fatalf("error = %v, want unsupported schema", err)
	}
}

func TestAuthenticateRejectsPersistedClaimMismatches(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	baseSession := Session{
		ID:               42,
		UserID:           7,
		Platform:         "admin",
		DeviceID:         "device-a",
		LastSeenAt:       now,
		ExpiresAt:        now.Add(time.Hour),
		RefreshExpiresAt: now.Add(24 * time.Hour),
		IsDel:            commonNo,
		UserStatus:       commonYes,
		UserIsDel:        commonNo,
	}
	baseClaims := accesstoken.Claims{
		SessionID: 42,
		UserID:    7,
		Issuer:    accessTokenIssuer,
		Platform:  "admin",
		DeviceID:  "device-a",
		IssuedAt:  now,
		NotBefore: now,
		ExpiresAt: now.Add(time.Hour),
	}
	revokedAt := now.Add(-time.Minute)
	tests := []struct {
		name   string
		mutate func(*Session, *accesstoken.Claims)
	}{
		{name: "session id", mutate: func(_ *Session, claims *accesstoken.Claims) { claims.SessionID++ }},
		{name: "subject", mutate: func(_ *Session, claims *accesstoken.Claims) { claims.UserID++ }},
		{name: "issuer", mutate: func(_ *Session, claims *accesstoken.Claims) { claims.Issuer = "other" }},
		{name: "platform", mutate: func(_ *Session, claims *accesstoken.Claims) { claims.Platform = "app" }},
		{name: "device id", mutate: func(_ *Session, claims *accesstoken.Claims) { claims.DeviceID = "other-device" }},
		{name: "issued at", mutate: func(_ *Session, claims *accesstoken.Claims) { claims.IssuedAt = claims.IssuedAt.Add(-time.Second) }},
		{name: "not before", mutate: func(_ *Session, claims *accesstoken.Claims) { claims.NotBefore = claims.NotBefore.Add(time.Second) }},
		{name: "access expiry", mutate: func(_ *Session, claims *accesstoken.Claims) { claims.ExpiresAt = claims.ExpiresAt.Add(time.Second) }},
		{name: "revoked", mutate: func(session *Session, _ *accesstoken.Claims) { session.RevokedAt = &revokedAt }},
		{name: "deleted", mutate: func(session *Session, _ *accesstoken.Claims) { session.IsDel = commonYes }},
		{name: "user disabled", mutate: func(session *Session, _ *accesstoken.Claims) { session.UserStatus = commonNo }},
		{name: "user deleted", mutate: func(session *Session, _ *accesstoken.Claims) { session.UserIsDel = commonYes }},
		{name: "refresh expired", mutate: func(session *Session, _ *accesstoken.Claims) { session.RefreshExpiresAt = now }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session := baseSession
			claims := baseClaims
			test.mutate(&session, &claims)
			appErr := matchClaims(&session, claims, now)
			if appErr == nil || appErr.Category != apperror.CategoryAuthentication {
				t.Fatalf("error = %#v, want authentication rejection", appErr)
			}
		})
	}
}

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
	findSessionID   int64
	updatedAccessID int64
	updatedHash     string
	updatedExpires  time.Time
	findRefreshHash string
	findLatestKey   string
	session         *Session
	refreshSession  *Session
	latestSession   *Session
	activeSessions  []Session
	created         SessionCreate
	createdID       int64
	rotatedID       int64
	rotation        SessionRotation
	rotateLost      bool
	revokedID       int64
	revokedAt       time.Time
	revokedPlatform string
	revokedUserID   int64
	err             error
}

func (f *fakeSessionRepository) WithUserLock(ctx context.Context, userID int64, platform string, operation func(SessionRepository) error) error {
	f.revokedUserID = userID
	f.revokedPlatform = platform
	if f.err != nil {
		return f.err
	}
	return operation(f)
}

func (f *fakeSessionRepository) ListActiveForUpdate(ctx context.Context, userID int64, platform string, now time.Time) ([]Session, error) {
	f.findLatestKey = platform
	return f.activeSessions, f.err
}

func (f *fakeSessionRepository) Insert(ctx context.Context, input SessionCreate) (int64, error) {
	return f.Create(ctx, input)
}

func (f *fakeSessionRepository) RevokeIDs(ctx context.Context, sessionIDs []int64, revokedAt time.Time) error {
	if len(sessionIDs) > 0 {
		f.revokedID = sessionIDs[len(sessionIDs)-1]
	}
	f.revokedAt = revokedAt
	return f.err
}

func (f *fakeSessionRepository) FindValidByID(ctx context.Context, sessionID int64, now time.Time) (*Session, error) {
	f.findSessionID = sessionID
	return normalizeFakePersistedSession(f.session, now), f.err
}

func normalizeFakePersistedSession(session *Session, now time.Time) *Session {
	if session == nil {
		return nil
	}
	clone := *session
	if clone.LastSeenAt.IsZero() {
		clone.LastSeenAt = now
	}
	if clone.RefreshExpiresAt.IsZero() {
		clone.RefreshExpiresAt = now.Add(24 * time.Hour)
	}
	if clone.IsDel == 0 {
		clone.IsDel = commonNo
	}
	if clone.UserStatus == 0 {
		clone.UserStatus = commonYes
	}
	if clone.UserIsDel == 0 {
		clone.UserIsDel = commonNo
	}
	return &clone
}

func (f *fakeSessionRepository) UpdateAccessToken(ctx context.Context, sessionID int64, accessHash string, expiresAt time.Time) error {
	f.updatedAccessID = sessionID
	f.updatedHash = accessHash
	f.updatedExpires = expiresAt
	return f.err
}

func (f *fakeSessionRepository) FindLatestActiveByUserPlatform(ctx context.Context, userID int64, platform string, now time.Time) (*Session, error) {
	f.findLatestKey = platform
	return f.latestSession, f.err
}

func (f *fakeSessionRepository) FindValidByRefreshHash(ctx context.Context, refreshHash string, now time.Time) (*Session, error) {
	f.findRefreshHash = refreshHash
	return normalizeFakePersistedSession(f.refreshSession, now), f.err
}

func (f *fakeSessionRepository) RotateIfRefreshHash(ctx context.Context, sessionID int64, previousHash string, rotation SessionRotation) (bool, error) {
	f.rotatedID = sessionID
	f.rotation = rotation
	return !f.rotateLost, f.err
}

func (f *fakeSessionRepository) Revoke(ctx context.Context, sessionID int64, revokedAt time.Time) error {
	f.revokedID = sessionID
	f.revokedAt = revokedAt
	return f.err
}

func (f *fakeSessionRepository) Create(ctx context.Context, input SessionCreate) (int64, error) {
	f.created = input
	if f.createdID == 0 {
		f.createdID = 77
	}
	return f.createdID, f.err
}

func (f *fakeSessionRepository) ListActiveByUserPlatform(ctx context.Context, userID int64, platform string, now time.Time) ([]Session, error) {
	f.findLatestKey = platform
	return f.activeSessions, f.err
}

func (f *fakeSessionRepository) RevokeByUserPlatform(ctx context.Context, userID int64, platform string, revokedAt time.Time) error {
	f.revokedUserID = userID
	f.revokedPlatform = platform
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

func testJWTCodec() accesstoken.Codec {
	return accesstoken.NewJWTCodec([]byte("12345678901234567890123456789012"), accesstoken.Options{Issuer: "admin_go"})
}

func issueTestAccessToken(t *testing.T, now time.Time, sessionID int64, userID int64, platform string, deviceID string, ttl time.Duration) string {
	t.Helper()
	token, err := testJWTCodec().Issue(accesstoken.Claims{
		SessionID: sessionID,
		UserID:    userID,
		Platform:  platform,
		DeviceID:  deviceID,
		IssuedAt:  now,
		ExpiresAt: now.Add(ttl),
	})
	if err != nil {
		t.Fatalf("issue access token: %v", err)
	}
	return token
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

func TestNewAuthenticatorNormalizesTokenConfigDefaults(t *testing.T) {
	auth := NewAuthenticator(AuthenticatorDeps{
		Config: config.TokenConfig{
			RedisPrefix:             "   ",
			SessionCacheTTL:         -time.Second,
			SingleSessionPointerTTL: -time.Hour,
		},
	})

	if auth.cfg.RedisPrefix != config.DefaultTokenRedisPrefix {
		t.Fatalf("expected default redis prefix %q, got %q", config.DefaultTokenRedisPrefix, auth.cfg.RedisPrefix)
	}
	if auth.cfg.SessionCacheTTL != config.DefaultTokenSessionCacheTTL {
		t.Fatalf("expected default session cache ttl %s, got %s", config.DefaultTokenSessionCacheTTL, auth.cfg.SessionCacheTTL)
	}
	if auth.cfg.SingleSessionPointerTTL != config.DefaultTokenSingleSessionPointerTTL {
		t.Fatalf("expected default pointer ttl %s, got %s", config.DefaultTokenSingleSessionPointerTTL, auth.cfg.SingleSessionPointerTTL)
	}
}

func TestAccessHashIsNotPersistedDuringIssue(t *testing.T) {
	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	generator := &sequenceTokenGenerator{values: []string{"refresh-token"}}
	repo := &fakeSessionRepository{createdID: 42}
	auth := NewAuthenticator(AuthenticatorDeps{
		Config:         config.TokenConfig{RedisPrefix: "token:"},
		Repository:     repo,
		PolicyProvider: allowPolicies(),
		TokenGenerator: generator.MakeToken,
		AccessCodec:    testJWTCodec(),
		TokenPepper:    "pepper-value",
		Now:            func() time.Time { return now },
	})

	result, appErr := auth.Create(context.Background(), CreateInput{UserID: 7, Platform: "admin", DeviceID: "device-a"})

	if appErr != nil {
		t.Fatalf("expected create to succeed, got %v", appErr)
	}
	if strings.Count(result.AccessToken, ".") != 2 {
		t.Fatalf("expected JWT access token, got %q", result.AccessToken)
	}
	if strings.Count(result.RefreshToken, ".") != 0 {
		t.Fatalf("expected opaque refresh token, got %q", result.RefreshToken)
	}
	if repo.created.LegacyNonce == "" || repo.updatedAccessID != 0 || repo.updatedHash != "" {
		t.Fatalf("access credential must not be hashed into persistence: created=%#v updated_id=%d hash=%q", repo.created, repo.updatedAccessID, repo.updatedHash)
	}
}

func TestAuthenticatorAuthenticateUsesJWTSessionID(t *testing.T) {
	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	accessToken := issueTestAccessToken(t, now, 42, 7, "admin", "device-a", time.Hour)
	cache := &fakeSessionCache{values: map[string]string{}}
	repo := &fakeSessionRepository{session: &Session{ID: 42, UserID: 7, Platform: "admin", DeviceID: "device-a", ExpiresAt: now.Add(time.Hour)}}
	auth := NewAuthenticator(AuthenticatorDeps{
		Config:         config.TokenConfig{RedisPrefix: "token:", SessionCacheTTL: 30 * time.Minute},
		Cache:          cache,
		Repository:     repo,
		PolicyProvider: allowPolicies(),
		AccessCodec:    testJWTCodec(),
		TokenPepper:    "pepper-value",
		Now:            func() time.Time { return now },
	})

	identity, appErr := auth.Authenticate(context.Background(), TokenInput{AccessToken: accessToken, Platform: "admin", DeviceID: "device-a"})

	if appErr != nil {
		t.Fatalf("expected authenticate to succeed, got %v", appErr)
	}
	if identity.UserID != 7 || identity.SessionID != 42 || identity.Platform != "admin" {
		t.Fatalf("unexpected identity: %#v", identity)
	}
	if repo.findSessionID != 42 {
		t.Fatalf("expected repository lookup by session id 42, got %d", repo.findSessionID)
	}
	if cache.setKey != "token:session:42" {
		t.Fatalf("expected session cache key, got %q", cache.setKey)
	}
}

func TestAuthenticatorResolvesCachedSessionAndRefreshesTTL(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.Local)
	accessToken := issueTestAccessToken(t, now, 34, 12, "admin", "device-1", 30*time.Minute)
	cacheKey := "token:session:34"
	cache := &fakeSessionCache{values: map[string]string{
		cacheKey: cacheValue(&Session{ID: 34, UserID: 12, Platform: "admin", DeviceID: "device-1", IP: "127.0.0.1", LastSeenAt: now, ExpiresAt: now.Add(30 * time.Minute), RefreshExpiresAt: now.Add(24 * time.Hour), IsDel: commonNo, UserStatus: commonYes, UserIsDel: commonNo}),
	}}
	repo := &fakeSessionRepository{}
	auth := NewAuthenticator(AuthenticatorDeps{
		Config:         config.TokenConfig{RedisPrefix: "token:", SessionCacheTTL: 30 * time.Minute},
		AccessCodec:    testJWTCodec(),
		TokenPepper:    "pepper-value",
		Cache:          cache,
		Repository:     repo,
		PolicyProvider: allowPolicies(),
		Now:            func() time.Time { return now },
	})

	identity, appErr := auth.Authenticate(context.Background(), TokenInput{
		AccessToken: accessToken,
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
	if repo.findSessionID != 0 {
		t.Fatalf("expected cached session to avoid mysql lookup, sid=%d", repo.findSessionID)
	}
	if cache.expireKey != cacheKey || cache.expireTTL != 30*time.Minute {
		t.Fatalf("expected session cache ttl refresh, got key=%q ttl=%s", cache.expireKey, cache.expireTTL)
	}
}

func TestAuthenticatorFallsBackToMySQLAndWritesRedis(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.Local)
	accessToken := issueTestAccessToken(t, now, 55, 44, "app", "device-2", 10*time.Minute)
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
		Config:         config.TokenConfig{RedisPrefix: "token:", SessionCacheTTL: 30 * time.Minute},
		AccessCodec:    testJWTCodec(),
		TokenPepper:    "pepper-value",
		Cache:          cache,
		Repository:     repo,
		PolicyProvider: allowPolicies(),
		Now:            func() time.Time { return now },
	})

	identity, appErr := auth.Authenticate(context.Background(), TokenInput{AccessToken: accessToken, Platform: "app", DeviceID: "device-2"})

	if appErr != nil {
		t.Fatalf("expected authenticate to succeed, got %v", appErr)
	}
	if identity.UserID != 44 || identity.SessionID != 55 || identity.Platform != "app" {
		t.Fatalf("unexpected identity: %#v", identity)
	}
	if repo.findSessionID != 55 {
		t.Fatalf("expected mysql lookup by session id 55, got %d", repo.findSessionID)
	}
	if cache.setKey != "token:session:55" {
		t.Fatalf("expected redis session key, got %q", cache.setKey)
	}
	if cache.setValue != cacheValue(normalizeFakePersistedSession(repo.session, now)) {
		t.Fatalf("unexpected redis cache value: %q", cache.setValue)
	}
}

func TestAuthenticatorRejectsInvalidCurrentPlatform(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.Local)
	accessToken := issueTestAccessToken(t, now, 34, 12, "admin", "device-1", 30*time.Minute)
	cache := &fakeSessionCache{values: map[string]string{
		"token:session:34": cacheValue(&Session{ID: 34, UserID: 12, Platform: "admin", DeviceID: "device-1", IP: "127.0.0.1", LastSeenAt: now, ExpiresAt: now.Add(30 * time.Minute), RefreshExpiresAt: now.Add(24 * time.Hour), IsDel: commonNo, UserStatus: commonYes, UserIsDel: commonNo}),
	}}
	auth := NewAuthenticator(AuthenticatorDeps{
		Config:      config.TokenConfig{RedisPrefix: "token:", SessionCacheTTL: 30 * time.Minute},
		AccessCodec: testJWTCodec(),
		TokenPepper: "pepper-value",
		Cache:       cache,
		PolicyProvider: fakePolicyProvider{policies: map[string]*AuthPolicy{
			"admin": {BindPlatform: true},
		}},
		Now: func() time.Time { return now },
	})

	identity, appErr := auth.Authenticate(context.Background(), TokenInput{AccessToken: accessToken, Platform: "missing"})

	if identity != nil {
		t.Fatalf("expected nil identity, got %#v", identity)
	}
	if appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest || appErr.Message != "无效的平台标识" {
		t.Fatalf("expected invalid platform app error, got %#v", appErr)
	}
}

func TestAuthenticatorRejectsPlatformMismatchWhenPolicyBindsPlatform(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.Local)
	accessToken := issueTestAccessToken(t, now, 34, 12, "admin", "device-1", 30*time.Minute)
	cache := &fakeSessionCache{values: map[string]string{
		"token:session:34": cacheValue(&Session{ID: 34, UserID: 12, Platform: "admin", DeviceID: "device-1", IP: "127.0.0.1", LastSeenAt: now, ExpiresAt: now.Add(30 * time.Minute), RefreshExpiresAt: now.Add(24 * time.Hour), IsDel: commonNo, UserStatus: commonYes, UserIsDel: commonNo}),
	}}
	auth := NewAuthenticator(AuthenticatorDeps{
		Config:      config.TokenConfig{RedisPrefix: "token:", SessionCacheTTL: 30 * time.Minute},
		AccessCodec: testJWTCodec(),
		TokenPepper: "pepper-value",
		Cache:       cache,
		PolicyProvider: fakePolicyProvider{policies: map[string]*AuthPolicy{
			"admin": {BindPlatform: true},
			"app":   {BindPlatform: false},
		}},
		Now: func() time.Time { return now },
	})

	identity, appErr := auth.Authenticate(context.Background(), TokenInput{AccessToken: accessToken, Platform: "app"})

	if identity != nil {
		t.Fatalf("expected nil identity, got %#v", identity)
	}
	if appErr == nil || appErr.LegacyCode != apperror.CodeUnauthorized || appErr.Message != "平台不匹配" {
		t.Fatalf("expected platform mismatch app error, got %#v", appErr)
	}
}

func TestAuthenticatorRejectsDeviceMismatchWhenPolicyBindsDevice(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.Local)
	accessToken := issueTestAccessToken(t, now, 34, 12, "admin", "device-1", 30*time.Minute)
	cache := &fakeSessionCache{values: map[string]string{
		"token:session:34": cacheValue(&Session{ID: 34, UserID: 12, Platform: "admin", DeviceID: "device-1", IP: "127.0.0.1", LastSeenAt: now, ExpiresAt: now.Add(30 * time.Minute), RefreshExpiresAt: now.Add(24 * time.Hour), IsDel: commonNo, UserStatus: commonYes, UserIsDel: commonNo}),
	}}
	auth := NewAuthenticator(AuthenticatorDeps{
		Config:      config.TokenConfig{RedisPrefix: "token:", SessionCacheTTL: 30 * time.Minute},
		AccessCodec: testJWTCodec(),
		TokenPepper: "pepper-value",
		Cache:       cache,
		PolicyProvider: fakePolicyProvider{policies: map[string]*AuthPolicy{
			"admin": {BindPlatform: true, BindDevice: true},
		}},
		Now: func() time.Time { return now },
	})

	identity, appErr := auth.Authenticate(context.Background(), TokenInput{
		AccessToken: accessToken,
		Platform:    "admin",
		DeviceID:    "device-2",
		ClientIP:    "127.0.0.1",
	})

	if identity != nil {
		t.Fatalf("expected nil identity, got %#v", identity)
	}
	if appErr == nil || appErr.LegacyCode != apperror.CodeUnauthorized || appErr.Message != "设备变更，请重新登录" {
		t.Fatalf("expected device mismatch app error, got %#v", appErr)
	}
}

func TestAuthenticatorRejectsIPMismatchWhenPolicyBindsIPAndDeletesRedis(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.Local)
	accessToken := issueTestAccessToken(t, now, 34, 12, "admin", "device-1", 30*time.Minute)
	cacheKey := "token:session:34"
	cache := &fakeSessionCache{values: map[string]string{
		cacheKey: cacheValue(&Session{ID: 34, UserID: 12, Platform: "admin", DeviceID: "device-1", IP: "127.0.0.1", LastSeenAt: now, ExpiresAt: now.Add(30 * time.Minute), RefreshExpiresAt: now.Add(24 * time.Hour), IsDel: commonNo, UserStatus: commonYes, UserIsDel: commonNo}),
	}}
	auth := NewAuthenticator(AuthenticatorDeps{
		Config:      config.TokenConfig{RedisPrefix: "token:", SessionCacheTTL: 30 * time.Minute},
		AccessCodec: testJWTCodec(),
		TokenPepper: "pepper-value",
		Cache:       cache,
		PolicyProvider: fakePolicyProvider{policies: map[string]*AuthPolicy{
			"admin": {BindPlatform: true, BindIP: true},
		}},
		Now: func() time.Time { return now },
	})

	identity, appErr := auth.Authenticate(context.Background(), TokenInput{
		AccessToken: accessToken,
		Platform:    "admin",
		DeviceID:    "device-1",
		ClientIP:    "10.0.0.1",
	})

	if identity != nil {
		t.Fatalf("expected nil identity, got %#v", identity)
	}
	if appErr == nil || appErr.LegacyCode != apperror.CodeUnauthorized || appErr.Message != "IP地址变动" {
		t.Fatalf("expected ip mismatch app error, got %#v", appErr)
	}
	if cache.deletedKey != cacheKey {
		t.Fatalf("expected mismatched ip to delete session cache key, got %q", cache.deletedKey)
	}
}

func TestAuthenticatorRejectsStaleSingleSessionAndDeletesRedis(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.Local)
	accessToken := issueTestAccessToken(t, now, 34, 12, "admin", "device-1", 30*time.Minute)
	cacheKey := "token:session:34"
	pointerKey := "token:cur_sess:admin:12"
	cache := &fakeSessionCache{values: map[string]string{
		cacheKey:   cacheValue(&Session{ID: 34, UserID: 12, Platform: "admin", DeviceID: "device-1", IP: "127.0.0.1", LastSeenAt: now, ExpiresAt: now.Add(30 * time.Minute), RefreshExpiresAt: now.Add(24 * time.Hour), IsDel: commonNo, UserStatus: commonYes, UserIsDel: commonNo}),
		pointerKey: "99",
	}}
	repo := &fakeSessionRepository{latestSession: &Session{ID: 99, UserID: 12, Platform: "admin"}}
	auth := NewAuthenticator(AuthenticatorDeps{
		Config: config.TokenConfig{
			RedisPrefix:             "token:",
			SessionCacheTTL:         30 * time.Minute,
			SingleSessionPointerTTL: 30 * 24 * time.Hour,
		},
		AccessCodec: testJWTCodec(),
		TokenPepper: "pepper-value",
		Cache:       cache,
		Repository:  repo,
		PolicyProvider: fakePolicyProvider{policies: map[string]*AuthPolicy{
			"admin": {BindPlatform: true, SingleSessionPerPlatform: true},
		}},
		Now: func() time.Time { return now },
	})

	identity, appErr := auth.Authenticate(context.Background(), TokenInput{
		AccessToken: accessToken,
		Platform:    "admin",
		DeviceID:    "device-1",
		ClientIP:    "127.0.0.1",
	})

	if identity != nil {
		t.Fatalf("expected nil identity, got %#v", identity)
	}
	if appErr == nil || appErr.LegacyCode != apperror.CodeUnauthorized || appErr.Message != "账号已在其他设备登录" {
		t.Fatalf("expected stale single session app error, got %#v", appErr)
	}
	if cache.deletedKey != cacheKey {
		t.Fatalf("expected stale single session to delete session cache key, got %q", cache.deletedKey)
	}
}

func TestAuthenticatorRebuildsSingleSessionPointerFromRepository(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.Local)
	accessToken := issueTestAccessToken(t, now, 34, 12, "admin", "device-1", 30*time.Minute)
	cacheKey := "token:session:34"
	cache := &fakeSessionCache{values: map[string]string{
		cacheKey: cacheValue(&Session{ID: 34, UserID: 12, Platform: "admin", DeviceID: "device-1", IP: "127.0.0.1", LastSeenAt: now, ExpiresAt: now.Add(30 * time.Minute), RefreshExpiresAt: now.Add(24 * time.Hour), IsDel: commonNo, UserStatus: commonYes, UserIsDel: commonNo}),
	}}
	repo := &fakeSessionRepository{latestSession: &Session{ID: 34, UserID: 12, Platform: "admin"}}
	auth := NewAuthenticator(AuthenticatorDeps{
		Config: config.TokenConfig{
			RedisPrefix:             "token:",
			SessionCacheTTL:         30 * time.Minute,
			SingleSessionPointerTTL: 30 * 24 * time.Hour,
		},
		AccessCodec: testJWTCodec(),
		TokenPepper: "pepper-value",
		Cache:       cache,
		Repository:  repo,
		PolicyProvider: fakePolicyProvider{policies: map[string]*AuthPolicy{
			"admin": {BindPlatform: true, SingleSessionPerPlatform: true},
		}},
		Now: func() time.Time { return now },
	})

	identity, appErr := auth.Authenticate(context.Background(), TokenInput{
		AccessToken: accessToken,
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
	accessToken := issueTestAccessToken(t, now.Add(-time.Hour), 34, 12, "admin", "device-1", 30*time.Minute)
	cacheKey := "token:session:34"
	cache := &fakeSessionCache{values: map[string]string{
		cacheKey: cacheValue(&Session{ID: 34, UserID: 12, Platform: "admin", DeviceID: "device-1", IP: "127.0.0.1", LastSeenAt: now.Add(-time.Hour), ExpiresAt: now.Add(-time.Second), RefreshExpiresAt: now.Add(24 * time.Hour), IsDel: commonNo, UserStatus: commonYes, UserIsDel: commonNo}),
	}}
	auth := NewAuthenticator(AuthenticatorDeps{
		Config:      config.TokenConfig{RedisPrefix: "token:", SessionCacheTTL: 30 * time.Minute},
		AccessCodec: testJWTCodec(),
		TokenPepper: "pepper-value",
		Cache:       cache,
		Now:         func() time.Time { return now },
	})

	identity, appErr := auth.Authenticate(context.Background(), TokenInput{AccessToken: accessToken})

	if identity != nil {
		t.Fatalf("expected nil identity, got %#v", identity)
	}
	if appErr == nil || appErr.LegacyCode != apperror.CodeUnauthorized {
		t.Fatalf("expected token expired app error, got %#v", appErr)
	}
}

func TestAuthenticatorFailsClosedWithoutRepositoryOnCacheMiss(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.Local)
	accessToken := issueTestAccessToken(t, now, 34, 12, "admin", "device-1", 30*time.Minute)
	auth := NewAuthenticator(AuthenticatorDeps{
		Config:      config.TokenConfig{RedisPrefix: "token:", SessionCacheTTL: 30 * time.Minute},
		AccessCodec: testJWTCodec(),
		TokenPepper: "pepper-value",
		Cache:       &fakeSessionCache{values: map[string]string{}},
		Now:         func() time.Time { return now },
	})

	identity, appErr := auth.Authenticate(context.Background(), TokenInput{AccessToken: accessToken})

	if identity != nil {
		t.Fatalf("expected nil identity, got %#v", identity)
	}
	if appErr == nil || appErr.LegacyCode != apperror.CodeUnauthorized || appErr.Message != "Token认证未配置" {
		t.Fatalf("expected token auth not configured, got %#v", appErr)
	}
}

func TestAuthenticatorReturnsServerErrorOnRepositoryFailure(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.Local)
	accessToken := issueTestAccessToken(t, now, 34, 12, "admin", "device-1", 30*time.Minute)
	auth := NewAuthenticator(AuthenticatorDeps{
		Config:      config.TokenConfig{RedisPrefix: "token:", SessionCacheTTL: 30 * time.Minute},
		AccessCodec: testJWTCodec(),
		TokenPepper: "pepper-value",
		Cache:       &fakeSessionCache{values: map[string]string{}},
		Repository:  &fakeSessionRepository{err: errors.New("mysql down")},
		Now:         func() time.Time { return now },
	})

	identity, appErr := auth.Authenticate(context.Background(), TokenInput{AccessToken: accessToken})

	if identity != nil {
		t.Fatalf("expected nil identity, got %#v", identity)
	}
	if appErr == nil || appErr.LegacyCode != apperror.CodeInternal {
		t.Fatalf("expected internal error, got %#v", appErr)
	}
}

func TestAuthenticatorRefreshRotatesTokensAndDeletesOldAccessCache(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.Local)
	refreshHash, err := HashToken("old-refresh-token", "pepper-value")
	if err != nil {
		t.Fatalf("hash refresh token: %v", err)
	}
	generator := &sequenceTokenGenerator{values: []string{"new-refresh-token"}}
	cache := &fakeSessionCache{values: map[string]string{}}
	repo := &fakeSessionRepository{refreshSession: &Session{
		ID:               55,
		UserID:           44,
		RefreshTokenHash: refreshHash,
		Platform:         "admin",
		RefreshExpiresAt: now.Add(2 * time.Hour),
	}}
	auth := NewAuthenticator(AuthenticatorDeps{
		Config:         config.TokenConfig{RedisPrefix: "token:", SessionCacheTTL: 30 * time.Minute},
		AccessCodec:    testJWTCodec(),
		TokenPepper:    "pepper-value",
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
	if strings.Count(result.AccessToken, ".") != 2 || result.RefreshToken != "new-refresh-token" {
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
	if cache.deletedKey != "token:session:55" {
		t.Fatalf("expected old session cache deleted, got %q", cache.deletedKey)
	}
}

func TestAuthenticatorCreateStoresSessionAndSingleSessionPointer(t *testing.T) {
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.Local)
	generator := &sequenceTokenGenerator{values: []string{"new-refresh-token"}}
	cache := &fakeSessionCache{values: map[string]string{}}
	repo := &fakeSessionRepository{
		createdID: 88,
		activeSessions: []Session{
			{ID: 55, UserID: 44, Platform: "admin"},
		},
	}
	auth := NewAuthenticator(AuthenticatorDeps{
		Config: config.TokenConfig{
			RedisPrefix:             "token:",
			SingleSessionPointerTTL: 30 * 24 * time.Hour,
		},
		Cache:      cache,
		Repository: repo,
		PolicyProvider: fakePolicyProvider{policies: map[string]*AuthPolicy{
			"admin": {BindPlatform: true, SingleSessionPerPlatform: true, AccessTTL: 4 * time.Hour, RefreshTTL: 14 * 24 * time.Hour},
		}},
		TokenGenerator: generator.MakeToken,
		AccessCodec:    testJWTCodec(),
		TokenPepper:    "pepper-value",
		Now:            func() time.Time { return now },
	})

	result, appErr := auth.Create(context.Background(), CreateInput{
		UserID:    44,
		Platform:  "admin",
		DeviceID:  "device-1",
		ClientIP:  "127.0.0.1",
		UserAgent: "test-agent",
	})

	if appErr != nil {
		t.Fatalf("expected create to succeed, got %v", appErr)
	}
	if strings.Count(result.AccessToken, ".") != 2 || result.RefreshToken != "new-refresh-token" {
		t.Fatalf("unexpected token result: %#v", result)
	}
	if repo.revokedUserID != 44 || repo.revokedPlatform != "admin" {
		t.Fatalf("expected old admin sessions to be revoked, got user=%d platform=%q", repo.revokedUserID, repo.revokedPlatform)
	}
	if !containsString(cache.deletedKeys, "token:session:55") {
		t.Fatalf("expected old session cache to be deleted, got %#v", cache.deletedKeys)
	}
	if repo.created.UserID != 44 || repo.created.Platform != "admin" || repo.created.DeviceID != "device-1" {
		t.Fatalf("unexpected created session: %#v", repo.created)
	}
	if !repo.created.ExpiresAt.Equal(now.Add(4*time.Hour)) || !repo.created.RefreshExpiresAt.Equal(now.Add(14*24*time.Hour)) {
		t.Fatalf("unexpected session expiry: %#v", repo.created)
	}
	if cache.setKey != "token:cur_sess:admin:44" || cache.setValue != "88" {
		t.Fatalf("expected single session pointer for new session, key=%q value=%q", cache.setKey, cache.setValue)
	}
}

func TestAuthenticatorCreateRejectsPolicyWithoutTokenTTL(t *testing.T) {
	auth := NewAuthenticator(AuthenticatorDeps{
		Config: config.TokenConfig{
			RedisPrefix: "token:",
		},
		Repository: &fakeSessionRepository{},
		PolicyProvider: fakePolicyProvider{policies: map[string]*AuthPolicy{
			"admin": {BindPlatform: true},
		}},
		TokenGenerator: (&sequenceTokenGenerator{values: []string{"new-access-token", "new-refresh-token"}}).MakeToken,
	})

	result, appErr := auth.Create(context.Background(), CreateInput{UserID: 44, Platform: "admin"})

	if result != nil {
		t.Fatalf("expected nil token result, got %#v", result)
	}
	if appErr == nil || appErr.LegacyCode != apperror.CodeInternal || appErr.Message != "认证平台Token有效期未配置" {
		t.Fatalf("expected missing platform ttl error, got %#v", appErr)
	}
}

func TestAuthenticatorRefreshRejectsPolicyWithoutAccessTTL(t *testing.T) {
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
		RefreshExpiresAt: now.Add(2 * time.Hour),
	}}
	auth := NewAuthenticator(AuthenticatorDeps{
		Config:      config.TokenConfig{RedisPrefix: "token:"},
		AccessCodec: testJWTCodec(),
		TokenPepper: "pepper-value",
		Repository:  repo,
		PolicyProvider: fakePolicyProvider{policies: map[string]*AuthPolicy{
			"admin": {RefreshTTL: 14 * 24 * time.Hour},
		}},
		TokenGenerator: (&sequenceTokenGenerator{values: []string{"new-access-token", "new-refresh-token"}}).MakeToken,
		Now:            func() time.Time { return now },
	})

	result, appErr := auth.Refresh(context.Background(), RefreshInput{RefreshToken: "old-refresh-token"})

	if result != nil {
		t.Fatalf("expected nil token result, got %#v", result)
	}
	if appErr == nil || appErr.LegacyCode != apperror.CodeInternal || appErr.Message != "认证平台Token有效期未配置" {
		t.Fatalf("expected missing platform ttl error, got %#v", appErr)
	}
}

func TestAuthenticatorRefreshRejectsInvalidRefreshToken(t *testing.T) {
	auth := NewAuthenticator(AuthenticatorDeps{
		Config:      config.TokenConfig{RedisPrefix: "token:"},
		AccessCodec: testJWTCodec(),
		TokenPepper: "pepper-value",
	})

	result, appErr := auth.Refresh(context.Background(), RefreshInput{})

	if result != nil {
		t.Fatalf("expected nil result, got %#v", result)
	}
	if appErr == nil || appErr.LegacyCode != apperror.CodeUnauthorized || appErr.Message != "缺少刷新令牌" {
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
		Config:      config.TokenConfig{RedisPrefix: "token:"},
		AccessCodec: testJWTCodec(),
		TokenPepper: "pepper-value",
		Repository:  repo,
		Now:         func() time.Time { return now },
	})

	result, appErr := auth.Refresh(context.Background(), RefreshInput{RefreshToken: "old-refresh-token"})

	if result != nil {
		t.Fatalf("expected nil result, got %#v", result)
	}
	if appErr == nil || appErr.LegacyCode != apperror.CodeUnauthorized || appErr.Message != "刷新令牌已过期，请重新登录" {
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
		Config:      config.TokenConfig{RedisPrefix: "token:", SingleSessionPointerTTL: 30 * 24 * time.Hour},
		AccessCodec: testJWTCodec(),
		TokenPepper: "pepper-value",
		Cache:       cache,
		Repository:  repo,
		PolicyProvider: fakePolicyProvider{policies: map[string]*AuthPolicy{
			"admin": {SingleSessionPerPlatform: true, AccessTTL: 4 * time.Hour, RefreshTTL: 14 * 24 * time.Hour},
		}},
		Now: func() time.Time { return now },
	})

	result, appErr := auth.Refresh(context.Background(), RefreshInput{RefreshToken: "old-refresh-token"})

	if result != nil {
		t.Fatalf("expected nil result, got %#v", result)
	}
	if appErr == nil || appErr.LegacyCode != apperror.CodeUnauthorized || appErr.Message != "账号已在其他设备登录，请重新登录" {
		t.Fatalf("expected stale session error, got %#v", appErr)
	}
}

func TestAuthenticatorLogoutRevokesSessionAndClearsTokenAndPointer(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.Local)
	accessToken := issueTestAccessToken(t, now, 55, 44, "admin", "", time.Hour)
	cache := &fakeSessionCache{values: map[string]string{
		"token:cur_sess:admin:44": "55",
	}}
	repo := &fakeSessionRepository{session: &Session{
		ID:        55,
		UserID:    44,
		Platform:  "admin",
		ExpiresAt: now.Add(time.Hour),
	}}
	auth := NewAuthenticator(AuthenticatorDeps{
		Config:      config.TokenConfig{RedisPrefix: "token:"},
		AccessCodec: testJWTCodec(),
		TokenPepper: "pepper-value",
		Cache:       cache,
		Repository:  repo,
		Now:         func() time.Time { return now },
	})

	appErr := auth.Logout(context.Background(), accessToken)

	if appErr != nil {
		t.Fatalf("expected logout to succeed, got %v", appErr)
	}
	if repo.findSessionID != 55 {
		t.Fatalf("expected lookup by session id 55, got %d", repo.findSessionID)
	}
	if repo.revokedID != 55 || !repo.revokedAt.Equal(now) {
		t.Fatalf("expected revoke session 55 at now, got id=%d at=%s", repo.revokedID, repo.revokedAt)
	}
	if !containsString(cache.deletedKeys, "token:session:55") {
		t.Fatalf("expected session cache deletion, got %#v", cache.deletedKeys)
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

type fakeRevocationCache struct {
	values      map[string]string
	deletedKeys []string
}

func (f *fakeRevocationCache) Get(ctx context.Context, key string) (string, error) {
	return f.values[key], nil
}

func (f *fakeRevocationCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return nil
}

func (f *fakeRevocationCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return nil
}

func (f *fakeRevocationCache) Del(ctx context.Context, key string) error {
	f.deletedKeys = append(f.deletedKeys, key)
	delete(f.values, key)
	return nil
}

func TestSessionRevocationServiceDeletesSessionCacheAndMatchingPointer(t *testing.T) {
	cache := &fakeRevocationCache{values: map[string]string{"token:cur_sess:admin:44": "99"}}
	service := NewSessionRevocationService(cache, SessionRevocationConfig{RedisPrefix: "token:"})

	err := service.RevokeCache(context.Background(), Session{ID: 99, UserID: 44, Platform: "admin"})
	if err != nil {
		t.Fatalf("RevokeCache returned error: %v", err)
	}

	if !revocationContains(cache.deletedKeys, "token:session:99") {
		t.Fatalf("session cache was not deleted: %#v", cache.deletedKeys)
	}
	if !revocationContains(cache.deletedKeys, "token:cur_sess:admin:44") {
		t.Fatalf("matching pointer was not deleted: %#v", cache.deletedKeys)
	}
}

func TestSessionRevocationServiceKeepsNonMatchingPointer(t *testing.T) {
	cache := &fakeRevocationCache{values: map[string]string{"token:cur_sess:admin:44": "100"}}
	service := NewSessionRevocationService(cache, SessionRevocationConfig{RedisPrefix: "token:"})

	err := service.RevokeCache(context.Background(), Session{ID: 99, UserID: 44, Platform: "admin"})
	if err != nil {
		t.Fatalf("RevokeCache returned error: %v", err)
	}

	if !revocationContains(cache.deletedKeys, "token:session:99") {
		t.Fatalf("session cache was not deleted: %#v", cache.deletedKeys)
	}
	if revocationContains(cache.deletedKeys, "token:cur_sess:admin:44") {
		t.Fatalf("non-matching pointer must not be deleted: %#v", cache.deletedKeys)
	}
}

func revocationContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// Session admin management tests merged from usersession/service_test.go.
type fakeSessionAdminRepository struct {
	listQuery    SessionListQuery
	listRows     []SessionListRow
	listTotal    int64
	statsRows    []SessionStatsRow
	recordsByID  map[int64]SessionAdminRecord
	markedIDs    []int64
	markedAt     time.Time
	markAffected int64
	listErr      error
	statsErr     error
	recordErr    error
	markErr      error
}

func (f *fakeSessionAdminRepository) List(ctx context.Context, query SessionListQuery) ([]SessionListRow, int64, error) {
	f.listQuery = query
	return f.listRows, f.listTotal, f.listErr
}

func (f *fakeSessionAdminRepository) Stats(ctx context.Context, now time.Time) ([]SessionStatsRow, error) {
	return f.statsRows, f.statsErr
}

func (f *fakeSessionAdminRepository) GetByID(ctx context.Context, id int64) (*SessionAdminRecord, error) {
	if f.recordErr != nil {
		return nil, f.recordErr
	}
	row, ok := f.recordsByID[id]
	if !ok {
		return nil, nil
	}
	return &row, nil
}

func (f *fakeSessionAdminRepository) GetByIDs(ctx context.Context, ids []int64) ([]SessionAdminRecord, error) {
	if f.recordErr != nil {
		return nil, f.recordErr
	}
	rows := make([]SessionAdminRecord, 0, len(ids))
	for _, id := range ids {
		if row, ok := f.recordsByID[id]; ok {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func (f *fakeSessionAdminRepository) MarkRevoked(ctx context.Context, ids []int64, revokedAt time.Time) (int64, error) {
	f.markedIDs = append([]int64(nil), ids...)
	f.markedAt = revokedAt
	if f.markErr != nil {
		return 0, f.markErr
	}
	if f.markAffected == 0 {
		return int64(len(ids)), nil
	}
	return f.markAffected, nil
}

type fakeSessionAdminCacheRevoker struct {
	rows  []Session
	calls int
	err   error
}

func TestSessionAdmin_PageInitUsesRegisteredPlatformOptions(t *testing.T) {
	got, appErr := NewSessionAdminService(&fakeSessionAdminRepository{}).PageInit(context.Background())
	if appErr != nil {
		t.Fatalf("page init failed: %v", appErr)
	}
	if len(got.Dict.PlatformArr) != 1 || got.Dict.PlatformArr[0].Value != enum.PlatformAdmin {
		t.Fatalf("session platform options must come from registered adapters: %#v", got.Dict.PlatformArr)
	}
}

func (f *fakeSessionAdminCacheRevoker) RevokeCache(ctx context.Context, row Session) error {
	f.calls++
	f.rows = append(f.rows, row)
	return f.err
}

func (f *fakeSessionAdminCacheRevoker) RevokeCaches(ctx context.Context, rows []Session) error {
	f.calls++
	f.rows = append(f.rows, rows...)
	return f.err
}

func TestSessionAdmin_ListNormalizesQueryAndDerivesStatus(t *testing.T) {
	now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.Local)
	expiredAt := now.Add(-time.Minute)
	activeAt := now.Add(time.Hour)
	revokedAt := now.Add(-2 * time.Hour)
	repo := &fakeSessionAdminRepository{
		listTotal: 3,
		listRows: []SessionListRow{
			{
				ID: 1, UserID: 11, Username: "active-user", Platform: "admin", DeviceID: "dev-1",
				IP: "127.0.0.1", UserAgent: "ua-1", LastSeenAt: now,
				ExpiresAt: activeAt, RefreshExpiresAt: activeAt, CreatedAt: now.Add(-24 * time.Hour),
			},
			{
				ID: 2, UserID: 12, Username: "expired-user", Platform: "app", DeviceID: "dev-2",
				IP: "127.0.0.2", UserAgent: "ua-2", LastSeenAt: now.Add(-time.Hour),
				ExpiresAt: expiredAt, RefreshExpiresAt: expiredAt, CreatedAt: now.Add(-48 * time.Hour),
			},
			{
				ID: 3, UserID: 13, Username: "revoked-user", Platform: "admin", DeviceID: "dev-3",
				IP: "::1", UserAgent: "ua-3", LastSeenAt: now.Add(-2 * time.Hour),
				ExpiresAt: activeAt, RefreshExpiresAt: activeAt, RevokedAt: &revokedAt, CreatedAt: now.Add(-72 * time.Hour),
			},
		},
	}
	service := NewSessionAdminService(repo, WithSessionAdminNow(func() time.Time { return now }))

	got, appErr := service.List(context.Background(), SessionListQuery{
		CurrentPage: -1,
		PageSize:    999,
		Username:    " active-user ",
		Platform:    "admin",
		Status:      "active",
	})
	if appErr != nil {
		t.Fatalf("expected list to succeed, got %v", appErr)
	}
	if repo.listQuery.CurrentPage != 1 || repo.listQuery.PageSize != 50 {
		t.Fatalf("query was not normalized: %#v", repo.listQuery)
	}
	if !repo.listQuery.Now.Equal(now) {
		t.Fatalf("query now mismatch: %s", repo.listQuery.Now)
	}
	if repo.listQuery.Username != "active-user" || repo.listQuery.Platform != "admin" || repo.listQuery.Status != "active" {
		t.Fatalf("query filters mismatch: %#v", repo.listQuery)
	}
	if got.Page.Total != 3 || got.Page.TotalPage != 1 {
		t.Fatalf("page mismatch: %#v", got.Page)
	}
	if got.List[0].Status != SessionStatusActive || got.List[1].Status != SessionStatusExpired || got.List[2].Status != SessionStatusRevoked {
		t.Fatalf("status mismatch: %#v", got.List)
	}
	if got.List[0].PlatformName != "admin" || got.List[1].PlatformName != "app" {
		t.Fatalf("platform name mismatch: %#v", got.List)
	}
}

func TestSessionAdmin_ListRejectsInvalidStatusAndPlatform(t *testing.T) {
	service := NewSessionAdminService(&fakeSessionAdminRepository{}, WithSessionAdminNow(func() time.Time {
		return time.Date(2026, 5, 8, 10, 0, 0, 0, time.Local)
	}))

	if _, appErr := service.List(context.Background(), SessionListQuery{CurrentPage: 1, PageSize: 20, Status: "bad"}); appErr == nil || appErr.MessageID != "usersession.status.invalid" {
		t.Fatalf("expected keyed invalid status error, got %#v", appErr)
	}
	if _, appErr := service.List(context.Background(), SessionListQuery{CurrentPage: 1, PageSize: 20, Platform: "mini"}); appErr == nil || appErr.MessageID != "usersession.platform.invalid" {
		t.Fatalf("expected keyed invalid platform error, got %#v", appErr)
	}
}

func TestSessionAdmin_ListWrapsSessionAdminRepositoryError(t *testing.T) {
	service := NewSessionAdminService(&fakeSessionAdminRepository{listErr: errors.New("db down")})

	if _, appErr := service.List(context.Background(), SessionListQuery{CurrentPage: 1, PageSize: 20}); appErr == nil || appErr.MessageID != "usersession.query_failed" {
		t.Fatalf("expected keyed list repository error, got %#v", appErr)
	}
}

func TestSessionAdmin_StatsIncludesRegisteredPlatformsOnly(t *testing.T) {
	now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.Local)
	repo := &fakeSessionAdminRepository{statsRows: []SessionStatsRow{
		{Platform: "admin", Total: 2},
		{Platform: "partner_portal", Total: 7},
	}}
	service := NewSessionAdminService(repo, WithSessionAdminNow(func() time.Time { return now }))

	got, appErr := service.Stats(context.Background())
	if appErr != nil {
		t.Fatalf("expected stats to succeed, got %v", appErr)
	}
	if got.TotalActive != 2 {
		t.Fatalf("total_active mismatch: %d", got.TotalActive)
	}
	if len(got.PlatformDistribution) != 1 || got.PlatformDistribution[enum.PlatformAdmin] != 2 {
		t.Fatalf("platform_distribution mismatch: %#v", got.PlatformDistribution)
	}
}

func TestSessionAdmin_RevokeRejectsCurrentSession(t *testing.T) {
	service := NewSessionAdminService(&fakeSessionAdminRepository{}, WithSessionAdminNow(func() time.Time {
		return time.Date(2026, 5, 8, 10, 0, 0, 0, time.Local)
	}))

	if _, appErr := service.Revoke(context.Background(), 55, 55); appErr == nil || appErr.MessageID != "usersession.revoke_current_forbidden" {
		t.Fatalf("expected keyed current session revoke error, got %#v", appErr)
	}
}

func TestSessionAdmin_BatchRevokeRejectsMoreThanOneHundredIDs(t *testing.T) {
	service := NewSessionAdminService(&fakeSessionAdminRepository{})
	ids := make([]int64, 101)
	for i := range ids {
		ids[i] = int64(i + 1)
	}

	if _, appErr := service.BatchRevoke(context.Background(), SessionBatchRevokeInput{IDs: ids}, 55); appErr == nil || appErr.MessageID != "usersession.batch_too_many" {
		t.Fatalf("expected keyed batch limit error, got %#v", appErr)
	}
}

func TestSessionAdmin_RevokeReturnsFalseForAlreadyRevokedSession(t *testing.T) {
	now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.Local)
	revokedAt := now.Add(-time.Hour)
	repo := &fakeSessionAdminRepository{recordsByID: map[int64]SessionAdminRecord{
		77: {ID: 77, UserID: 44, Platform: "admin", RevokedAt: &revokedAt},
	}}
	revoker := &fakeSessionAdminCacheRevoker{}
	service := NewSessionAdminService(repo, WithSessionAdminNow(func() time.Time { return now }), WithSessionAdminCacheRevoker(revoker))

	got, appErr := service.Revoke(context.Background(), 77, 55)
	if appErr != nil {
		t.Fatalf("expected already revoked session to be idempotent, got %v", appErr)
	}
	if got.ID != 77 || got.Revoked {
		t.Fatalf("unexpected revoke response: %#v", got)
	}
	if len(repo.markedIDs) != 0 || revoker.calls != 0 {
		t.Fatalf("already revoked session must not touch db/cache, marked=%#v calls=%d", repo.markedIDs, revoker.calls)
	}
}

func TestSessionAdmin_RevokeMarksSessionAndRevokesCache(t *testing.T) {
	now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.Local)
	repo := &fakeSessionAdminRepository{recordsByID: map[int64]SessionAdminRecord{
		77: {ID: 77, UserID: 44, Platform: "admin"},
	}}
	revoker := &fakeSessionAdminCacheRevoker{}
	service := NewSessionAdminService(repo, WithSessionAdminNow(func() time.Time { return now }), WithSessionAdminCacheRevoker(revoker))

	got, appErr := service.Revoke(context.Background(), 77, 55)
	if appErr != nil {
		t.Fatalf("expected revoke to succeed, got %v", appErr)
	}
	if got.ID != 77 || !got.Revoked {
		t.Fatalf("unexpected revoke response: %#v", got)
	}
	if len(repo.markedIDs) != 1 || repo.markedIDs[0] != 77 || !repo.markedAt.Equal(now) {
		t.Fatalf("mark revoked mismatch: ids=%#v at=%s", repo.markedIDs, repo.markedAt)
	}
	if revoker.calls != 1 || len(revoker.rows) != 1 || revoker.rows[0].ID != 77 || revoker.rows[0].Platform != "admin" || revoker.rows[0].UserID != 44 {
		t.Fatalf("cache revoker mismatch: calls=%d rows=%#v", revoker.calls, revoker.rows)
	}
}

func TestSessionAdmin_BatchRevokeDeduplicatesAndSkipsCurrentAndAlreadyRevoked(t *testing.T) {
	now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.Local)
	revokedAt := now.Add(-time.Hour)
	repo := &fakeSessionAdminRepository{recordsByID: map[int64]SessionAdminRecord{
		1: {ID: 1, UserID: 11, Platform: "admin"},
		2: {ID: 2, UserID: 12, Platform: "admin", RevokedAt: &revokedAt},
		3: {ID: 3, UserID: 13, Platform: "app"},
	}}
	revoker := &fakeSessionAdminCacheRevoker{}
	service := NewSessionAdminService(repo, WithSessionAdminNow(func() time.Time { return now }), WithSessionAdminCacheRevoker(revoker))

	got, appErr := service.BatchRevoke(context.Background(), SessionBatchRevokeInput{IDs: []int64{1, 2, 1, 3, 99}}, 3)
	if appErr != nil {
		t.Fatalf("expected batch revoke to succeed, got %v", appErr)
	}
	if got.Count != 1 || got.SkippedCurrent != 1 || got.SkippedAlreadyRevoked != 1 {
		t.Fatalf("batch response mismatch: %#v", got)
	}
	if len(repo.markedIDs) != 1 || repo.markedIDs[0] != 1 {
		t.Fatalf("only session 1 should be marked, got %#v", repo.markedIDs)
	}
	if revoker.calls != 1 || len(revoker.rows) != 1 || revoker.rows[0].ID != 1 {
		t.Fatalf("only session 1 cache should be revoked, calls=%d rows=%#v", revoker.calls, revoker.rows)
	}
}

package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/infra/accesstoken"
	"admin_back_go/internal/infra/secretkey"
	"admin_back_go/internal/module/permission"

	"gorm.io/gorm"
)

const multiNodePropagationSLA = 2 * time.Second

func TestMultiNodeSessionRevocationPropagatesWithinTwoSeconds(t *testing.T) {
	resources := openSessionIntegrationResources(t)
	userID := createSessionIntegrationUser(t, resources)
	prefix := fmt.Sprintf("p04:multinode:revoke:%d:", userID)
	cleanupIntegrationRedisPrefix(t, resources, prefix)
	policy := &AuthPolicy{AccessTTL: time.Hour, RefreshTTL: 24 * time.Hour}
	nodeA := newIntegrationSessionLifecycle(resources, prefix, policy)
	nodeB := newIntegrationSessionLifecycle(resources, prefix, policy)

	issued, appErr := nodeA.Issue(context.Background(), IssueCommand{
		UserID: userID, Platform: "admin", DeviceID: "node-a-device", ClientIP: "127.0.0.1", UserAgent: "p04-node-a",
	})
	if appErr != nil {
		t.Fatalf("issue on node A: %v", appErr)
	}
	credential := AccessCredential{AccessToken: issued.AccessToken, Platform: "admin", DeviceID: "node-a-device", ClientIP: "127.0.0.1"}
	if _, appErr := nodeB.Authenticate(context.Background(), credential); appErr != nil {
		claims, parseErr := nodeB.accessCodec.Parse(issued.AccessToken, nodeB.now())
		session, repositoryErr := nodeB.repository.FindValidByID(context.Background(), claims.SessionID, nodeB.now())
		t.Fatalf("warm node B session cache: app=%v parse=%v claims=%#v session=%#v repository=%v match=%v", appErr, parseErr, claims, session, repositoryErr, matchClaims(session, claims, nodeB.now()))
	}
	if value, err := resources.redis.Redis.Get(context.Background(), prefix+"session:"+sessionIDFromAccessToken(t, issued.AccessToken)).Result(); err != nil || value == "" {
		t.Fatalf("node B did not warm the shared session cache: value=%q err=%v", value, err)
	}

	started := time.Now()
	if appErr := nodeA.Revoke(context.Background(), RevokeCommand{AccessToken: issued.AccessToken}); appErr != nil {
		t.Fatalf("revoke on node A: %v", appErr)
	}
	requireMultiNodeCondition(t, multiNodePropagationSLA, func() bool {
		identity, authErr := nodeB.Authenticate(context.Background(), credential)
		return identity == nil && authErr != nil
	})
	if elapsed := time.Since(started); elapsed > multiNodePropagationSLA {
		t.Fatalf("cross-node revocation took %s, SLA is %s", elapsed, multiNodePropagationSLA)
	}
}

func TestMultiNodeRefreshReusePropagatesWithinTwoSeconds(t *testing.T) {
	resources := openSessionIntegrationResources(t)
	userID := createSessionIntegrationUser(t, resources)
	prefix := fmt.Sprintf("p04:multinode:refresh:%d:", userID)
	cleanupIntegrationRedisPrefix(t, resources, prefix)
	policy := &AuthPolicy{AccessTTL: time.Hour, RefreshTTL: 24 * time.Hour}
	nodeA := newIntegrationSessionLifecycle(resources, prefix, policy)
	nodeB := newIntegrationSessionLifecycle(resources, prefix, policy)

	issued, appErr := nodeA.Issue(context.Background(), IssueCommand{UserID: userID, Platform: "admin", DeviceID: "refresh-device"})
	if appErr != nil {
		t.Fatalf("issue refresh credential: %v", appErr)
	}
	if _, appErr := nodeA.Rotate(context.Background(), RotateCommand{RefreshToken: issued.RefreshToken}); appErr != nil {
		t.Fatalf("rotate on node A: %v", appErr)
	}
	started := time.Now()
	requireMultiNodeCondition(t, multiNodePropagationSLA, func() bool {
		credentials, rotateErr := nodeB.Rotate(context.Background(), RotateCommand{RefreshToken: issued.RefreshToken})
		return credentials == nil && rotateErr != nil && rotateErr.Code == "auth.refresh_reused"
	})
	if elapsed := time.Since(started); elapsed > multiNodePropagationSLA {
		t.Fatalf("cross-node refresh reuse took %s, SLA is %s", elapsed, multiNodePropagationSLA)
	}
}

func TestMultiNodePrincipalChangesPropagateWithinTwoSeconds(t *testing.T) {
	resources := openSessionIntegrationResources(t)
	fixture := createMultiNodePrincipalFixture(t, resources)
	prefix := fmt.Sprintf("p04:multinode:principal:%d:", fixture.userID)
	cleanupIntegrationRedisPrefix(t, resources, prefix)
	repositoryA := permission.NewGormPrincipalRepository(resources.db)
	repositoryB := permission.NewGormPrincipalRepository(resources.db)
	serviceA := permission.NewPrincipalService(repositoryA, permission.NewRedisPrincipalCache(resources.redis, permission.PrincipalCacheConfig{RedisPrefix: prefix}), permission.PrincipalServiceOptions{})
	serviceB := permission.NewPrincipalService(repositoryB, permission.NewRedisPrincipalCache(resources.redis, permission.PrincipalCacheConfig{RedisPrefix: prefix}), permission.PrincipalServiceOptions{})
	ctx := context.Background()
	if err := serviceA.Reconcile(ctx); err != nil {
		t.Fatalf("reconcile node A: %v", err)
	}
	if appErr := serviceB.Authorize(ctx, fixture.userID, "admin", fixture.firstCode); appErr != nil {
		t.Fatalf("warm node B principal cache: %v", appErr)
	}
	subjects := []permission.PrincipalSubject{{UserID: fixture.userID, Platform: "admin"}}

	started := time.Now()
	if err := mutateMultiNodePrincipal(ctx, resources, serviceA, subjects, func(tx *gorm.DB) error {
		return tx.Table("users").Where("id = ?", fixture.userID).Update("role_id", fixture.secondRoleID).Error
	}); err != nil {
		t.Fatalf("role change on node A: %v", err)
	}
	requireMultiNodeCondition(t, multiNodePropagationSLA, func() bool {
		oldErr := serviceB.Authorize(ctx, fixture.userID, "admin", fixture.firstCode)
		newErr := serviceB.Authorize(ctx, fixture.userID, "admin", fixture.secondCode)
		return oldErr != nil && newErr == nil
	})
	if elapsed := time.Since(started); elapsed > multiNodePropagationSLA {
		t.Fatalf("cross-node role change took %s, SLA is %s", elapsed, multiNodePropagationSLA)
	}

	started = time.Now()
	if err := mutateMultiNodePrincipal(ctx, resources, serviceA, subjects, func(tx *gorm.DB) error {
		return tx.Table("users").Where("id = ?", fixture.userID).Update("status", permission.CommonNo).Error
	}); err != nil {
		t.Fatalf("user disable on node A: %v", err)
	}
	requireMultiNodeCondition(t, multiNodePropagationSLA, func() bool {
		appErr := serviceB.Authorize(ctx, fixture.userID, "admin", fixture.secondCode)
		return appErr != nil && appErr.MessageID == "auth.user_inactive"
	})
	if elapsed := time.Since(started); elapsed > multiNodePropagationSLA {
		t.Fatalf("cross-node user disable took %s, SLA is %s", elapsed, multiNodePropagationSLA)
	}
}

func TestMultiNodeSecretRotationRehearsal(t *testing.T) {
	resources := openSessionIntegrationResources(t)
	userID := createSessionIntegrationUser(t, resources)
	prefix := fmt.Sprintf("p04:multinode:rotation:%d:", userID)
	cleanupIntegrationRedisPrefix(t, resources, prefix)
	oldSecret := rotationSecret("P04_ROTATION_OLD_SECRET", "old")
	newSecret := rotationSecret("P04_ROTATION_NEW_SECRET", "new")
	if oldSecret == newSecret {
		t.Fatal("rotation test secrets must differ")
	}
	oldRing, err := secretkey.NewKeyRing(oldSecret)
	if err != nil {
		t.Fatalf("old key ring: %v", err)
	}
	dualRing, err := secretkey.NewKeyRingWithPrevious(newSecret, []string{oldSecret})
	if err != nil {
		t.Fatalf("dual key ring: %v", err)
	}
	newRing, err := secretkey.NewKeyRing(newSecret)
	if err != nil {
		t.Fatalf("new-only key ring: %v", err)
	}
	policy := &AuthPolicy{AccessTTL: time.Hour, RefreshTTL: 24 * time.Hour}
	oldNode := newMultiNodeLifecycleForKeyRing(t, resources, prefix, policy, oldRing)
	dualNode := newMultiNodeLifecycleForKeyRing(t, resources, prefix, policy, dualRing)
	newOnlyNode := newMultiNodeLifecycleForKeyRing(t, resources, prefix, policy, newRing)

	oldCredentials, appErr := oldNode.Issue(context.Background(), IssueCommand{UserID: userID, Platform: "admin", DeviceID: "old-node"})
	if appErr != nil {
		t.Fatalf("old node issue: %v", appErr)
	}
	if got := accessTokenKeyID(t, oldCredentials.AccessToken); got != oldRing.JWTSigningKeyID() {
		t.Fatalf("old token kid = %q, want %q", got, oldRing.JWTSigningKeyID())
	}
	oldInput := AccessCredential{AccessToken: oldCredentials.AccessToken, Platform: "admin", DeviceID: "old-node"}
	if _, appErr := dualNode.Authenticate(context.Background(), oldInput); appErr != nil {
		claims, parseErr := dualNode.accessCodec.Parse(oldCredentials.AccessToken, dualNode.now())
		session, repositoryErr := dualNode.repository.FindValidByID(context.Background(), claims.SessionID, dualNode.now())
		t.Fatalf("dual node rejected old access token: app=%v parse=%v claims=%#v session=%#v repository=%v match=%v", appErr, parseErr, claims, session, repositoryErr, matchClaims(session, claims, dualNode.now()))
	}
	if credentials, rotateErr := dualNode.Rotate(context.Background(), RotateCommand{RefreshToken: oldCredentials.RefreshToken}); credentials != nil || rotateErr == nil || rotateErr.Code != "auth.refresh_reused" {
		t.Fatalf("old refresh credential survived root-secret cutover: credentials=%#v error=%#v", credentials, rotateErr)
	}
	if _, appErr := newOnlyNode.Authenticate(context.Background(), oldInput); appErr == nil {
		t.Fatal("new-only node accepted an old-key access token")
	}

	currentCredentials, appErr := dualNode.Issue(context.Background(), IssueCommand{UserID: userID, Platform: "admin", DeviceID: "dual-node"})
	if appErr != nil {
		t.Fatalf("dual node issue: %v", appErr)
	}
	if got := accessTokenKeyID(t, currentCredentials.AccessToken); got != newRing.JWTSigningKeyID() {
		t.Fatalf("dual token kid = %q, want current %q", got, newRing.JWTSigningKeyID())
	}
	rotated, appErr := dualNode.Rotate(context.Background(), RotateCommand{RefreshToken: currentCredentials.RefreshToken})
	if appErr != nil {
		t.Fatalf("dual node rotate current refresh credential: %v", appErr)
	}
	currentInput := AccessCredential{AccessToken: rotated.AccessToken, Platform: "admin", DeviceID: "dual-node"}
	if _, appErr := newOnlyNode.Authenticate(context.Background(), currentInput); appErr != nil {
		t.Fatalf("new-only node rejected current token: %v", appErr)
	}
	if appErr := dualNode.Revoke(context.Background(), RevokeCommand{AccessToken: rotated.AccessToken}); appErr != nil {
		t.Fatalf("dual node revoke: %v", appErr)
	}
	requireMultiNodeCondition(t, multiNodePropagationSLA, func() bool {
		identity, authErr := newOnlyNode.Authenticate(context.Background(), currentInput)
		return identity == nil && authErr != nil
	})
}

type multiNodePrincipalFixture struct {
	userID       int64
	firstRoleID  int64
	secondRoleID int64
	firstCode    string
	secondCode   string
}

func createMultiNodePrincipalFixture(t *testing.T, resources sessionIntegrationResources) multiNodePrincipalFixture {
	t.Helper()
	ctx := context.Background()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	insert := func(query string, args ...any) int64 {
		result, err := resources.db.SQL.ExecContext(ctx, query, args...)
		if err != nil {
			t.Fatalf("insert multi-node principal fixture: %v", err)
		}
		id, err := result.LastInsertId()
		if err != nil {
			t.Fatalf("read fixture insert ID: %v", err)
		}
		return id
	}
	firstRoleID := insert("INSERT INTO roles (name, is_default, is_del) VALUES (?, 2, 2)", "p04-node-role-a-"+suffix)
	secondRoleID := insert("INSERT INTO roles (name, is_default, is_del) VALUES (?, 2, 2)", "p04-node-role-b-"+suffix)
	firstCode := "p04_node_first_" + suffix
	secondCode := "p04_node_second_" + suffix
	insertPermission := func(name, code string) int64 {
		return insert("INSERT INTO permissions (name, parent_id, platform, type, sort, code, i18n_key, show_menu, status, is_del) VALUES (?, 0, 'admin', 2, 1, ?, ?, 1, 1, 2)", name, code, "p04."+code)
	}
	firstPermissionID := insertPermission("P04 node first "+suffix, firstCode)
	secondPermissionID := insertPermission("P04 node second "+suffix, secondCode)
	insert("INSERT INTO role_permissions (role_id, permission_id, is_del) VALUES (?, ?, 2)", firstRoleID, firstPermissionID)
	insert("INSERT INTO role_permissions (role_id, permission_id, is_del) VALUES (?, ?, 2)", secondRoleID, secondPermissionID)
	userID := insert("INSERT INTO users (role_id, username, status, is_del) VALUES (?, ?, 1, 2)", firstRoleID, "p04-node-user-"+suffix)
	if _, err := resources.db.SQL.ExecContext(ctx, "INSERT INTO authz_principal_versions (user_id, platform, version, updated_at) VALUES (?, 'admin', 1, UTC_TIMESTAMP(6))", userID); err != nil {
		t.Fatalf("insert multi-node principal version: %v", err)
	}
	t.Cleanup(func() {
		_, _ = resources.db.SQL.ExecContext(ctx, "DELETE FROM authz_principal_versions WHERE user_id = ?", userID)
		_, _ = resources.db.SQL.ExecContext(ctx, "DELETE FROM users WHERE id = ?", userID)
		_, _ = resources.db.SQL.ExecContext(ctx, "DELETE FROM role_permissions WHERE role_id IN (?, ?)", firstRoleID, secondRoleID)
		_, _ = resources.db.SQL.ExecContext(ctx, "DELETE FROM permissions WHERE id IN (?, ?)", firstPermissionID, secondPermissionID)
		_, _ = resources.db.SQL.ExecContext(ctx, "DELETE FROM roles WHERE id IN (?, ?)", firstRoleID, secondRoleID)
	})
	return multiNodePrincipalFixture{userID: userID, firstRoleID: firstRoleID, secondRoleID: secondRoleID, firstCode: firstCode, secondCode: secondCode}
}

func mutateMultiNodePrincipal(ctx context.Context, resources sessionIntegrationResources, service *permission.PrincipalService, subjects []permission.PrincipalSubject, mutation func(*gorm.DB) error) error {
	return service.Mutate(ctx, subjects, func() ([]permission.PrincipalVersion, error) {
		var versions []permission.PrincipalVersion
		err := resources.db.Gorm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := mutation(tx); err != nil {
				return err
			}
			var err error
			versions, err = permission.BumpPrincipalVersions(ctx, tx, subjects)
			return err
		})
		return versions, err
	})
}

func newMultiNodeLifecycleForKeyRing(t *testing.T, resources sessionIntegrationResources, prefix string, policy *AuthPolicy, keys *secretkey.KeyRing) *SessionLifecycle {
	t.Helper()
	codec, err := accesstoken.NewRotatingJWTCodec(keys.JWTSigningKeyID(), keys.JWTVerificationKeys(), accesstoken.Options{Issuer: "admin_go"})
	if err != nil {
		t.Fatalf("build multi-node access codec: %v", err)
	}
	return NewSessionLifecycle(LifecycleDeps{
		Config: configForMultiNode(prefix), Cache: NewSessionRedisCache(resources.redis), Repository: NewSessionGormRepository(resources.db),
		PolicyProvider: integrationPolicyProvider{policy: policy}, AccessCodec: codec, TokenPepper: keys.TokenPepper(),
	})
}

func configForMultiNode(prefix string) config.TokenConfig {
	return config.TokenConfig{RedisPrefix: prefix, SessionCacheTTL: 30 * time.Minute, SingleSessionPointerTTL: time.Hour}
}

func cleanupIntegrationRedisPrefix(t *testing.T, resources sessionIntegrationResources, prefix string) {
	t.Helper()
	t.Cleanup(func() {
		keys, _ := resources.redis.Redis.Keys(context.Background(), prefix+"*").Result()
		if len(keys) > 0 {
			_ = resources.redis.Redis.Del(context.Background(), keys...).Err()
		}
	})
}

func requireMultiNodeCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if condition() {
			return
		}
		if !time.Now().Before(deadline) {
			t.Fatalf("condition did not propagate within %s", timeout)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func sessionIDFromAccessToken(t *testing.T, token string) string {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("invalid JWT segment count")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode JWT payload: %v", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatalf("decode JWT claims: %v", err)
	}
	return fmt.Sprint(claims["sid"])
}

func accessTokenKeyID(t *testing.T, token string) string {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("invalid JWT segment count")
	}
	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode JWT header: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(header, &fields); err != nil {
		t.Fatalf("decode JWT header JSON: %v", err)
	}
	keyID, _ := fields["kid"].(string)
	return keyID
}

func rotationSecret(envName string, label string) string {
	if value := os.Getenv(envName); strings.TrimSpace(value) != "" {
		return value
	}
	return strings.Repeat(label+"-p04-", 12)
}

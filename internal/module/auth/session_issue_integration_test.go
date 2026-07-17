package auth

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/infra/accesstoken"
	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/infra/redisclient"
)

const identityIntegrationEnv = "ADMIN_IDENTITY_INTEGRATION"

const integrationTokenPepper = "p04-session-integration-token-pepper"

type sessionIntegrationResources struct {
	db    *database.Client
	redis *redisclient.Client
	cfg   config.Config
}

func openSessionIntegrationResources(t *testing.T) sessionIntegrationResources {
	t.Helper()
	if os.Getenv(identityIntegrationEnv) != "1" {
		t.Skip("Docker-only identity integration test")
	}
	tokenRedisDB := config.DefaultTokenRedisDB
	if raw := os.Getenv("TOKEN_REDIS_DB"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			t.Fatalf("parse TOKEN_REDIS_DB: %v", err)
		}
		tokenRedisDB = parsed
	}
	cfg := config.Config{
		MySQL: config.MySQLConfig{
			DSN:             os.Getenv("MYSQL_DSN"),
			MaxOpenConns:    50,
			MaxIdleConns:    20,
			ConnMaxLifetime: time.Minute,
		},
		Redis: config.RedisConfig{
			Addr:     os.Getenv("REDIS_ADDR"),
			Password: os.Getenv("REDIS_PASSWORD"),
		},
		Token: config.NormalizeTokenConfig(config.TokenConfig{RedisDB: tokenRedisDB}),
	}
	var err error
	db, err := database.Open(cfg.MySQL)
	if err != nil {
		t.Fatalf("open integration MySQL: %v", err)
	}
	redisCfg := cfg.Redis
	redisCfg.DB = cfg.Token.RedisDB
	redisClient, err := redisclient.Open(redisCfg)
	if err != nil {
		_ = db.Close()
		t.Fatalf("open integration Redis: %v", err)
	}
	t.Cleanup(func() {
		_ = redisClient.Close()
		_ = db.Close()
	})
	return sessionIntegrationResources{db: db, redis: redisClient, cfg: cfg}
}

func createSessionIntegrationUser(t *testing.T, resources sessionIntegrationResources) int64 {
	t.Helper()
	username := fmt.Sprintf("p04-session-%d", time.Now().UnixNano())
	result, err := resources.db.SQL.ExecContext(context.Background(),
		"INSERT INTO users (username, status, is_del) VALUES (?, 1, 2)", username)
	if err != nil {
		t.Fatalf("insert integration user: %v", err)
	}
	userID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("integration user id: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = resources.db.SQL.ExecContext(ctx, "DELETE FROM user_sessions WHERE user_id = ?", userID)
		_, _ = resources.db.SQL.ExecContext(ctx, "DELETE FROM authz_principal_versions WHERE user_id = ?", userID)
		_, _ = resources.db.SQL.ExecContext(ctx, "DELETE FROM users WHERE id = ?", userID)
	})
	return userID
}

func newIntegrationSessionLifecycle(resources sessionIntegrationResources, prefix string, policy *AuthPolicy) *SessionLifecycle {
	return NewSessionLifecycle(LifecycleDeps{
		Config: config.TokenConfig{
			RedisPrefix:             prefix,
			SessionCacheTTL:         30 * time.Minute,
			SingleSessionPointerTTL: time.Hour,
		},
		Cache:          NewSessionRedisCache(resources.redis),
		Repository:     NewSessionGormRepository(resources.db),
		PolicyProvider: integrationPolicyProvider{policy: policy},
		AccessCodec: accesstoken.NewJWTCodec(
			[]byte("p04-session-integration-signing-key"),
			accesstoken.Options{Issuer: "admin_go"},
		),
		TokenPepper: integrationTokenPepper,
	})
}

type integrationPolicyProvider struct {
	policy *AuthPolicy
}

func (p integrationPolicyProvider) Policy(context.Context, string) (*AuthPolicy, error) {
	return p.policy, nil
}

func issueConcurrently(t *testing.T, lifecycle *SessionLifecycle, userID int64, count int) {
	t.Helper()
	start := make(chan struct{})
	errors := make(chan error, count)
	var wait sync.WaitGroup
	for index := 0; index < count; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			_, appErr := lifecycle.Issue(context.Background(), IssueCommand{
				UserID:    userID,
				Platform:  "admin",
				DeviceID:  fmt.Sprintf("device-%02d", index),
				ClientIP:  "127.0.0.1",
				UserAgent: "p04-integration",
			})
			if appErr != nil {
				errors <- appErr
			}
		}(index)
	}
	close(start)
	wait.Wait()
	close(errors)
	for err := range errors {
		t.Errorf("concurrent issue: %v", err)
	}
}

func loadIntegrationSessions(t *testing.T, resources sessionIntegrationResources, userID int64) []Session {
	t.Helper()
	var sessions []Session
	if err := resources.db.Gorm.WithContext(context.Background()).
		Where("user_id = ? AND platform = ? AND is_del = ?", userID, "admin", commonNo).
		Order("id ASC").
		Find(&sessions).Error; err != nil {
		t.Fatalf("query integration sessions: %v", err)
	}
	return sessions
}

func TestConcurrentIssueEnforcesSessionLimits(t *testing.T) {
	resources := openSessionIntegrationResources(t)

	t.Run("single session has one active row and current pointer", func(t *testing.T) {
		userID := createSessionIntegrationUser(t, resources)
		prefix := fmt.Sprintf("p04:issue:%d:", userID)
		t.Cleanup(func() {
			_ = resources.redis.Redis.Del(context.Background(), prefix+"cur_sess:admin:"+fmt.Sprint(userID)).Err()
		})
		lifecycle := newIntegrationSessionLifecycle(resources, prefix, &AuthPolicy{
			SingleSessionPerPlatform: true,
			AccessTTL:                time.Hour,
			RefreshTTL:               24 * time.Hour,
		})

		issueConcurrently(t, lifecycle, userID, 20)
		sessions := loadIntegrationSessions(t, resources, userID)
		active := make([]Session, 0, 1)
		for _, session := range sessions {
			if session.RevokedAt == nil {
				active = append(active, session)
			}
		}
		if len(active) != 1 {
			t.Fatalf("active sessions = %d, want 1; total=%d", len(active), len(sessions))
		}
		pointer, err := resources.redis.Redis.Get(context.Background(), prefix+"cur_sess:admin:"+fmt.Sprint(userID)).Result()
		if err != nil {
			t.Fatalf("read single-session pointer: %v", err)
		}
		if pointer != fmt.Sprint(active[0].ID) {
			t.Fatalf("single-session pointer = %s, active session = %d", pointer, active[0].ID)
		}
	})

	t.Run("max sessions keeps the three newest active", func(t *testing.T) {
		userID := createSessionIntegrationUser(t, resources)
		prefix := fmt.Sprintf("p04:max:%d:", userID)
		lifecycle := newIntegrationSessionLifecycle(resources, prefix, &AuthPolicy{
			MaxSessions: 3,
			AccessTTL:   time.Hour,
			RefreshTTL:  24 * time.Hour,
		})

		issueConcurrently(t, lifecycle, userID, 20)
		sessions := loadIntegrationSessions(t, resources, userID)
		if len(sessions) != 20 {
			t.Fatalf("total sessions = %d, want 20", len(sessions))
		}
		activeIDs := make([]int64, 0, 3)
		for _, session := range sessions {
			if session.RevokedAt == nil {
				activeIDs = append(activeIDs, session.ID)
			}
		}
		if len(activeIDs) != 3 {
			t.Fatalf("active sessions = %d, want 3; ids=%v", len(activeIDs), activeIDs)
		}
		want := []int64{sessions[len(sessions)-3].ID, sessions[len(sessions)-2].ID, sessions[len(sessions)-1].ID}
		sort.Slice(activeIDs, func(i, j int) bool { return activeIDs[i] < activeIDs[j] })
		for index := range want {
			if activeIDs[index] != want[index] {
				t.Fatalf("active ids = %v, want newest %v", activeIDs, want)
			}
		}
	})
}

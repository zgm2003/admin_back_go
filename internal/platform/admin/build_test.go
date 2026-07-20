package admin

import (
	"os"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/infra/accesstoken"
	"admin_back_go/internal/infra/database"
	infrarealtime "admin_back_go/internal/infra/realtime"
	"admin_back_go/internal/infra/redisclient"
	"admin_back_go/internal/infra/secretkey"
)

func TestBuildAIMessageRepositoryUsesDurableRealtimeSink(t *testing.T) {
	body, err := os.ReadFile("build.go")
	if err != nil {
		t.Fatalf("read admin composition: %v", err)
	}
	compact := strings.Join(strings.Fields(string(body)), " ")
	want := "aimessage.NewGormRepository( resources.DB, replycommand.WithDurableEventSink(realtimeEventSink), )"
	if !strings.Contains(compact, want) {
		t.Fatal("Admin API cancellation must commit through the shared durable realtime sink")
	}
}

func TestAccessTokenCodecForKeysSupportsDualKeyWindow(t *testing.T) {
	oldRing, err := secretkey.NewKeyRing(strings.Repeat("o", 64))
	if err != nil {
		t.Fatalf("old key ring: %v", err)
	}
	dualRing, err := secretkey.NewKeyRingWithPrevious(strings.Repeat("n", 64), []string{strings.Repeat("o", 64)})
	if err != nil {
		t.Fatalf("dual key ring: %v", err)
	}
	oldCodec, err := accessTokenCodecForKeys(oldRing)
	if err != nil {
		t.Fatalf("old access codec: %v", err)
	}
	dualCodec, err := accessTokenCodecForKeys(dualRing)
	if err != nil {
		t.Fatalf("dual access codec: %v", err)
	}
	now := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	token, err := oldCodec.Issue(accesstoken.Claims{SessionID: 1, UserID: 2, Platform: "admin", IssuedAt: now, ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatalf("issue old token: %v", err)
	}
	if _, err := dualCodec.Parse(token, now.Add(time.Minute)); err != nil {
		t.Fatalf("dual codec rejected old token: %v", err)
	}
}

func TestBuildRejectsMissingRequiredResources(t *testing.T) {
	keys, err := secretkey.NewKeyRing(strings.Repeat("k", 64))
	if err != nil {
		t.Fatalf("key ring: %v", err)
	}
	result, err := Build(BuildInput{Keys: keys})
	if result != nil || err == nil || !strings.Contains(err.Error(), "resources") {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestBuildProducesCompleteAdminGraph(t *testing.T) {
	keys, err := secretkey.NewKeyRing(strings.Repeat("k", 64))
	if err != nil {
		t.Fatalf("key ring: %v", err)
	}
	resources := buildTestResources()

	result, err := Build(BuildInput{
		Config: config.Config{
			Logging: config.DefaultLoggingConfig(),
			AI:      config.NormalizeAIConfig(config.AIConfig{}),
		},
		Resources:         resources,
		Keys:              keys,
		Providers:         &ProviderSet{},
		RealtimePublisher: infrarealtime.NoopPublisher{},
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if result == nil {
		t.Fatal("build returned nil result")
	}
	if err := result.Graph.Validate(); err != nil {
		t.Fatalf("graph validation: %v", err)
	}
	if result.Authenticator == nil || result.PermissionChecker == nil {
		t.Fatalf("runtime seams incomplete: %+v", result)
	}
}

func TestBuildRejectsMissingEnabledRuntimeAdapters(t *testing.T) {
	keys, err := secretkey.NewKeyRing(strings.Repeat("k", 64))
	if err != nil {
		t.Fatalf("key ring: %v", err)
	}
	base := BuildInput{
		Resources: buildTestResources(),
		Keys:      keys,
		Providers: &ProviderSet{},
	}

	queueInput := base
	queueInput.Config.Queue.Enabled = true
	if result, err := Build(queueInput); result != nil || err == nil || !strings.Contains(err.Error(), "queue") {
		t.Fatalf("queue result=%+v err=%v", result, err)
	}

	realtimeInput := base
	realtimeInput.Config.Realtime.Enabled = true
	if result, err := Build(realtimeInput); result != nil || err == nil || !strings.Contains(err.Error(), "realtime") {
		t.Fatalf("realtime result=%+v err=%v", result, err)
	}
}

func buildTestResources() *BuildResources {
	return &BuildResources{
		DB:         &database.Client{},
		Redis:      &redisclient.Client{},
		TokenRedis: &redisclient.Client{},
		QueueRedis: &redisclient.Client{},
	}
}

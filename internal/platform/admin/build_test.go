package admin

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/infra/accesstoken"
	"admin_back_go/internal/infra/database"
	infrarealtime "admin_back_go/internal/infra/realtime"
	"admin_back_go/internal/infra/redisclient"
	"admin_back_go/internal/infra/secretbox"
	"admin_back_go/internal/infra/secretkey"
)

func TestProviderSetCarriesMailDiagnosticBox(t *testing.T) {
	field, ok := reflect.TypeOf(ProviderSet{}).FieldByName("MailDiagnosticBox")
	if !ok || field.Type != reflect.TypeOf(secretbox.VersionedBox{}) {
		t.Fatal("Admin ProviderSet must carry a versioned mail diagnostic box")
	}
}

func TestBuildWiresMailDiagnosticDependencies(t *testing.T) {
	compact := compactAdminBuild(t)
	for _, want := range []string{
		"mailService := mail.NewServiceWithDependencies(mail.ServiceDependencies{",
		"CredentialBox: providers.Secretbox",
		"DiagnosticBox: providers.MailDiagnosticBox",
	} {
		if !strings.Contains(compact, want) {
			t.Fatalf("admin composition missing %q", want)
		}
	}
	if strings.Contains(compact, "mail.NewService(") {
		t.Fatal("Admin Build still uses the positional Mail constructor")
	}
	if strings.Contains(compact, "MAIL_DIAGNOSTIC_SECRET") {
		t.Fatal("Admin Build introduced a separate diagnostic root")
	}
}

func TestBuildUsesSingleVerificationClock(t *testing.T) {
	compact := compactAdminBuild(t)
	for _, want := range []string{
		"sharedClock := clock.SystemClock{}",
		"Clock: sharedClock",
		"auth.WithClock(sharedClock)",
	} {
		if !strings.Contains(compact, want) {
			t.Fatalf("admin composition missing %q", want)
		}
	}
	if strings.Count(compact, "clock.SystemClock{}") != 1 {
		t.Fatal("Admin Build must instantiate exactly one system clock")
	}
}

func TestBuildWiresRedeemCodesWithSharedWalletRepositoryClockAndTelemetry(t *testing.T) {
	compact := compactAdminBuild(t)
	for _, want := range []string{
		"walletRepository := walletmodule.NewGormRepository(resources.DB)",
		"walletService := walletmodule.NewService(walletRepository)",
		"paymentmodule.NewGormRepository(resources.DB, walletRepository)",
		"redeemcode.NewGormRepository(resources.DB, walletRepository, sharedClock)",
		"redeemcode.NewRedisAttemptLimiter(resources.Redis.Redis)",
		"redeemcode.WithClock(sharedClock)",
		"redeemcode.WithTelemetry(recorder)",
		"RedeemCodes: redeemCodeService",
	} {
		if !strings.Contains(compact, want) {
			t.Fatalf("admin redeem code composition missing %q", want)
		}
	}
}

func TestBuildRequiresWalletParticipantForPaymentComposition(t *testing.T) {
	compact := compactAdminBuild(t)
	if strings.Contains(compact, "paymentmodule.NewGormRepository(resources.DB)") {
		t.Fatal("payment repository must not be constructed without the wallet participant")
	}
}

func compactAdminBuild(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile("build.go")
	if err != nil {
		t.Fatalf("read admin composition: %v", err)
	}
	return strings.Join(strings.Fields(string(body)), " ")
}

func TestBuildAIMessageRepositoryUsesDurableRealtimeSink(t *testing.T) {
	body, err := os.ReadFile("build.go")
	if err != nil {
		t.Fatalf("read admin composition: %v", err)
	}
	compact := strings.Join(strings.Fields(string(body)), " ")
	for _, want := range []string{
		"aiReplyRepository := replycommand.NewGormRepository( resources.DB, replycommand.WithDurableEventSink(realtimeEventSink), )",
		"replycommand.NewHistoryParticipant(aiReplyRepository)",
		"aimessage.NewGormRepository( resources.DB, aiReplyRepository,",
	} {
		if !strings.Contains(compact, want) {
			t.Fatalf("Admin AI message composition missing %q", want)
		}
	}
}

func TestBuildWiresAuthVerificationChannelCapabilities(t *testing.T) {
	body, err := os.ReadFile("build.go")
	if err != nil {
		t.Fatalf("read admin composition: %v", err)
	}
	compact := strings.Join(strings.Fields(string(body)), " ")
	for _, want := range []string{
		"auth.WithVerifyCodeMailSender(mailService)",
		"auth.WithVerifyCodePhoneSender(smsService)",
		"auth.WithVerifyCodeReadinessProvider(auth.NewChannelVerifyCodeReadinessProvider(mailService, smsService))",
		"auth.WithVerifyCodePolicyProvider(auth.NewChannelVerifyCodePolicyProvider(mailService, smsService))",
	} {
		if !strings.Contains(compact, want) {
			t.Fatalf("admin auth composition missing %q", want)
		}
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

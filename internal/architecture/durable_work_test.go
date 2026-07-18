package architecture

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestDurableWorkUsesDatabaseTruthFencingAndTypedRealtime(t *testing.T) {
	root := backendRoot(t)
	replyRepository := durableRead(t, root, "internal", "module", "ai", "replycommand", "repository.go")
	for _, required := range []string{
		"lease_owner = ? AND lease_token = ?",
		"WithDurableEventSink",
		"TypeAIResponseCanceledV1",
		"PublishBestEffort",
	} {
		if !strings.Contains(replyRepository, required) {
			t.Fatalf("reply repository is missing durable/fenced invariant %q", required)
		}
	}

	messageService := durableRead(t, root, "internal", "module", "ai", "message", "service.go")
	for _, forbidden := range []string{"go func", "sync.Map", "map[string]context.CancelFunc"} {
		if strings.Contains(messageService, forbidden) {
			t.Fatalf("AI message service keeps process-local reply state %q", forbidden)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "internal", "bootstrap", "ai_reply_dispatcher.go")); !os.IsNotExist(err) {
		t.Fatal("process-local AI reply dispatcher must stay deleted")
	}

	realtimeEvents := durableRead(t, root, "internal", "module", "realtime", "event.go")
	for _, required := range []string{"type EventRegistry struct", "TypeResyncRequiredV1", "TypeAIResponseCanceledV1", "DisallowUnknownFields"} {
		if !strings.Contains(realtimeEvents, required) {
			t.Fatalf("typed realtime registry is missing %q", required)
		}
	}
	realtimeRepository := durableRead(t, root, "internal", "module", "realtime", "repository.go")
	for _, required := range []string{
		"DurableEventRetention",
		"realtime_event_retention_watermarks",
		"DeletedThroughSequence",
		"Transaction(func(tx *gorm.DB)",
	} {
		if !strings.Contains(realtimeRepository, required) {
			t.Fatalf("realtime repository is missing durable recovery invariant %q", required)
		}
	}
	redisPubSub := durableRead(t, root, "internal", "infra", "realtime", "redis_pubsub.go")
	if strings.Contains(redisPubSub, "realtime_events") || strings.Contains(redisPubSub, "RetentionWatermark") {
		t.Fatal("Redis realtime adapter must not become durable terminal truth")
	}
}

func TestDurableWorkTasksAreCentrallyOwnedAndScheduled(t *testing.T) {
	root := backendRoot(t)
	jobs := durableRead(t, root, "internal", "jobs", "noop.go")
	worker := durableRead(t, root, "internal", "runtime", "worker.go")
	cron := durableRead(t, root, "internal", "module", "crontask", "registry.go")
	for _, required := range []string{
		"modulerealtime.RegisterTaskDefinitions",
		"RealtimeRetentionService",
	} {
		if !strings.Contains(jobs, required) {
			t.Fatalf("TaskRegistry composition is missing %q", required)
		}
	}
	for _, required := range []string{"NewRetentionService", "WithDurableEventSink"} {
		if !strings.Contains(worker, required) {
			t.Fatalf("Worker composition is missing %q", required)
		}
	}
	for _, required := range []string{"realtime_event_retention_cleanup", "TypeCleanupExpiredV1"} {
		if !strings.Contains(cron, required) {
			t.Fatalf("cron task registry is missing %q", required)
		}
	}
}

func TestDurableWorkMigrationClosesRetentionAndRequestIDContract(t *testing.T) {
	root := backendRoot(t)
	migration := durableRead(t, root, "database", "migrations", "202607150102_realtime_retention.sql")
	reconciliation := durableRead(t, root, "database", "reconciliation", "044_realtime_retention.sql")
	combined := strings.ToLower(migration + "\n" + reconciliation)
	for _, required := range []string{
		"realtime_event_retention_watermarks",
		"deleted_through_sequence",
		"varchar(128)",
		"date_add(`occurred_at`, interval 7 day)",
		"idx_realtime_expiry",
		"realtime:cleanup-expired:v1",
		"0 15 3 * * *",
	} {
		if !strings.Contains(combined, required) {
			t.Fatalf("P05 database contract is missing %q", required)
		}
	}
	if regexp.MustCompile(`(?i)\b(drop|truncate)\b`).MatchString(reconciliation) {
		t.Fatal("P05 reconciliation must stay non-destructive")
	}
}

func TestDurableWorkRestartGateIsDockerOnlyAndBlocking(t *testing.T) {
	root := backendRoot(t)
	restart := durableRead(t, root, "scripts", "tests", "durable-work-restart.tests.ps1")
	verify := durableRead(t, root, "scripts", "verify-durable-work.ps1")
	backend := durableRead(t, root, "scripts", "verify-backend.ps1")
	for _, required := range []string{
		"admin-api", "admin-worker", "ai_provider_attempts", "P05_PAUSE",
		"docker", "kill", "realtime_events", "lease_expires_at",
	} {
		if !strings.Contains(restart, required) {
			t.Fatalf("restart gate is missing scenario evidence %q", required)
		}
	}
	for _, source := range []struct{ name, body string }{
		{"restart", restart}, {"verify", verify},
	} {
		if regexp.MustCompile(`(?m)^\s*(?:&\s*)?go(?:\.exe)?\s`).MatchString(source.body) {
			t.Fatalf("%s script executes host Go instead of Docker", source.name)
		}
	}
	if !strings.Contains(verify, "durable-work-restart.tests.ps1") || !strings.Contains(backend, "verify-durable-work.ps1") {
		t.Fatal("durable restart gate is not blocking repository verification")
	}
}

func durableRead(t *testing.T, root string, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{root}, parts...)...)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(body)
}

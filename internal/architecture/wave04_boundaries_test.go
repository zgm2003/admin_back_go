package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWave04WorkerHasNoStaticScheduleAdapter(t *testing.T) {
	root := repositoryRoot(t)
	jobs, err := os.ReadFile(filepath.Join(root, "internal", "jobs", "noop.go"))
	if err != nil {
		t.Fatal(err)
	}
	worker, err := os.ReadFile(filepath.Join(root, "internal", "runtime", "worker.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"type ScheduleRegistrar interface",
		"type ScheduledTaskDefinition struct",
		"func RegisterSchedules(",
		"func registerScheduleDefinitions(",
		"func scheduledEnqueueTask(",
	} {
		if strings.Contains(string(jobs), forbidden) {
			t.Fatalf("jobs/noop.go still contains removed adapter %q", forbidden)
		}
	}
	if strings.Contains(string(worker), "jobs.RegisterSchedules(") {
		t.Fatal("worker runtime still invokes the removed static schedule adapter")
	}
}

func TestWave04WorkerDoesNotOwnRealtimeSessions(t *testing.T) {
	root := repositoryRoot(t)
	worker, err := os.ReadFile(filepath.Join(root, "internal", "runtime", "worker.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(worker), "infrarealtime.NewManager(") {
		t.Fatal("Worker must publish realtime events but must not own WebSocket sessions")
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

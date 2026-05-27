package architecture_test

import (
	"os"
	"strings"
	"testing"
)

func TestPlatformIsNotAuthModule(t *testing.T) {
	entries, err := os.ReadDir("../module")
	if err != nil {
		t.Fatalf("read module dir: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if name == "auth" {
			continue
		}
		if strings.HasSuffix(name, "auth") {
			t.Fatalf("platform-named auth module must not exist: internal/module/%s", entry.Name())
		}
	}
}

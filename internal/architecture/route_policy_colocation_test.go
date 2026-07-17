package architecture

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestRoutePolicyIsColocatedWithEveryTransportRegistration(t *testing.T) {
	root := backendRoot(t)
	for _, retired := range []string{"internal/bootstrap/route_meta.go", "internal/bootstrap/route_meta_test.go"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(retired))); !os.IsNotExist(err) {
			t.Fatalf("legacy route metadata bridge still exists: %s", retired)
		}
	}

	directRegistration := regexp.MustCompile(`\.(GET|POST|PUT|PATCH|DELETE|Any)\s*\(`)
	var offenders []string
	err := filepath.WalkDir(filepath.Join(root, "internal", "module"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		name := filepath.Base(path)
		if name != "route.go" && name != "task_route.go" {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if directRegistration.Match(body) {
			relative, _ := filepath.Rel(root, path)
			offenders = append(offenders, filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk route files: %v", err)
	}
	if len(offenders) > 0 {
		t.Fatalf("routes bypass adminroute.Registry:\n  %s", strings.Join(offenders, "\n  "))
	}
}

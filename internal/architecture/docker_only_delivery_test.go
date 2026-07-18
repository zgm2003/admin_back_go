package architecture

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestDockerOnlyDeliveryBoundary(t *testing.T) {
	root := backendRoot(t)
	for _, forbidden := range []string{".github", ".worktrees"} {
		if _, err := os.Stat(filepath.Join(root, forbidden)); err == nil || !os.IsNotExist(err) {
			t.Fatalf("%s must not exist in the backend repository", forbidden)
		}
	}

	gitignore := string(readBackendArchitectureFile(t, ".gitignore"))
	if regexp.MustCompile(`(?m)^\s*\.worktrees/?\s*$`).MatchString(gitignore) {
		t.Fatal(".gitignore must not preserve a hidden worktree convention")
	}

	if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
		command := exec.Command("git", "-C", root, "worktree", "list", "--porcelain")
		output, commandErr := command.CombinedOutput()
		if commandErr != nil {
			t.Fatalf("list registered worktrees: %v: %s", commandErr, output)
		}
		count := 0
		for _, line := range strings.Split(string(output), "\n") {
			if strings.HasPrefix(strings.TrimSuffix(line, "\r"), "worktree ") {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("registered worktrees=%d, want exactly the master checkout", count)
		}
	}
}

func TestDockerComposeIsTheOnlyApplicationRuntimeEntry(t *testing.T) {
	compose := string(readBackendArchitectureFile(t, "deploy/docker-first/docker-compose.yml"))
	lifecycle := string(readBackendArchitectureFile(t, "scripts/docker-platform.ps1"))
	dockerfile := string(readBackendArchitectureFile(t, "Dockerfile"))

	for _, required := range []string{
		"context: ../../../admin_front_ts",
		"context: ../..",
		"image: admin-go-backend:local",
		"BUILD_REVISION: ${ADMIN_BACKEND_BUILD_REVISION:-unknown}",
	} {
		if !strings.Contains(compose, required) {
			t.Fatalf("application Compose contract is missing %q", required)
		}
	}
	for _, required := range []string{
		"Invoke-Docker @('compose', '-f', $appCompose, 'build', 'admin-api', 'frontend')",
		"Invoke-Docker @('compose', '-f', $appCompose, 'up', '-d', '--no-build'",
		"Resolve-GitRevision -Repository $repoRoot",
	} {
		if !strings.Contains(lifecycle, required) {
			t.Fatalf("Docker lifecycle is missing %q", required)
		}
	}
	if regexp.MustCompile(`(?im)^\s*(?:&\s*)?(?:go(?:\.exe)?\s+run|(?:npm|pnpm|yarn)\s+run\s+dev|vite(?:\.cmd)?\s)`).MatchString(lifecycle) {
		t.Fatal("Docker lifecycle must not start host Go or Vite processes")
	}
	for _, required := range []string{
		"go test ./...",
		"FROM test AS build",
		"LABEL org.opencontainers.image.revision=\"${BUILD_REVISION}\"",
		"USER app",
	} {
		if !strings.Contains(dockerfile, required) {
			t.Fatalf("backend Docker image contract is missing %q", required)
		}
	}
}

package architecture

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestAdminReleaseManifestSchema(t *testing.T) {
	root := backendRoot(t)
	schemaPath := filepath.Join(root, "release", "admin-only", "release-manifest.schema.json")
	body, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read release manifest schema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(body, &schema); err != nil {
		t.Fatalf("parse release manifest schema: %v", err)
	}
	if schema["additionalProperties"] != false {
		t.Fatal("release manifest schema must reject additional properties")
	}
	wantRequired := []string{"schema_version", "release_id", "backend", "frontend", "contract", "database", "evidence"}
	if got := releaseStringArray(t, schema["required"], "release manifest required"); strings.Join(got, "|") != strings.Join(wantRequired, "|") {
		t.Fatalf("release manifest required fields=%v want %v", got, wantRequired)
	}

	properties := releaseObject(t, schema["properties"], "release manifest properties")
	if releaseObject(t, properties["schema_version"], "schema_version")["const"] != float64(1) {
		t.Fatal("release manifest schema_version must be one")
	}
	if got := releaseObject(t, properties["database"], "database"); releaseObject(t, got["properties"], "database properties")["atlas_version"] == nil {
		t.Fatal("release manifest database must declare atlas_version")
	}
	for _, section := range []string{"backend", "frontend", "contract", "database", "evidence"} {
		object := releaseObject(t, properties[section], section)
		if object["additionalProperties"] != false {
			t.Errorf("%s must reject additional properties", section)
		}
		if len(releaseStringArray(t, object["required"], section+" required")) == 0 {
			t.Errorf("%s must declare required fields", section)
		}
	}

	defs := releaseObject(t, schema["$defs"], "release manifest definitions")
	if releaseObject(t, defs["gitSha"], "gitSha")["pattern"] != "^[0-9a-f]{40}$" {
		t.Fatal("release manifest Git SHA definition is not strict")
	}
	if releaseObject(t, defs["sha256"], "sha256")["pattern"] != "^[0-9a-f]{64}$" {
		t.Fatal("release manifest SHA-256 definition is not strict")
	}
}

func TestReleaseManifestGeneratorAvoidsImportedManifestParameter(t *testing.T) {
	root := backendRoot(t)
	path := filepath.Join(root, "scripts", "release", "new-release-manifest.ps1")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read release manifest generator: %v", err)
	}

	assignment := regexp.MustCompile(`(?im)^\s*\$manifest\s*=\s*\[ordered\]@\{`)
	if assignment.Match(body) {
		t.Fatal("release manifest generator must not reuse the imported typed $Manifest parameter")
	}
}

func TestAdminReleaseMachineryIsImmutableAndFailClosed(t *testing.T) {
	root := backendRoot(t)
	files := []string{
		"scripts/release/new-release-manifest.ps1",
		"scripts/release/check-release-manifest.ps1",
		"scripts/release/check-platform-kernel.ps1",
		"scripts/release/export-docker-images.ps1",
		"scripts/release/deploy-admin-only.ps1",
		"scripts/release/rollback-admin-only.ps1",
	}
	sources := make(map[string]string, len(files))
	for _, relative := range files {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("read %s: %v", relative, err)
		}
		sources[relative] = string(body)
		assertPowerShellParses(t, filepath.Join(root, filepath.FromSlash(relative)))
	}

	required := map[string][]string{
		"scripts/release/new-release-manifest.ps1": {
			"input-lock.json", "platform-kernel-proof.json", "images\\metadata.json",
			"release-manifest.json", "[ordered]@{", "Move-Item -LiteralPath $temporaryPath",
		},
		"scripts/release/check-release-manifest.ps1": {
			"[switch]$SchemaOnly", "Test-Json -SchemaFile", "Test-LockedCommitAncestor",
			"org.opencontainers.image.revision", "archive_sha256", "platform_kernel_sha256",
		},
		"scripts/release/check-platform-kernel.ps1": {
			"TestPlatformKernel", "053_verify_admin_only.sql", "auth-platforms",
			"permission_platform_arr", "platform-kernel-proof.json", "Move-Item -LiteralPath $temporaryPath",
		},
		"scripts/release/export-docker-images.ps1": {
			"git status --porcelain=v1 --untracked-files=all", "git merge-base --is-ancestor",
			"org.opencontainers.image.revision", "RepoDigests", "docker save", "docker load",
			"metadata.json", "Get-FileSha256", "scripts\\verify-frontend.ps1",
		},
		"scripts/release/deploy-admin-only.ps1": {
			"check-release-manifest.ps1", "contract-admin-only.ps1", "-DestructiveApproval",
			"--no-build", "--wait", "Invoke-AdminReleaseSmoke", "deployment-state.json",
			"previous_manifest", "maintenance window",
		},
		"scripts/release/rollback-admin-only.ps1": {
			"Get-ReleaseManifestDocument", "previous_manifest", "docker load",
			"Invoke-AdminReleaseSmoke", "full database rollback", "recovery rehearsal",
			"--no-build", "--wait",
		},
	}
	for relative, tokens := range required {
		for _, token := range tokens {
			if !strings.Contains(sources[relative], token) {
				t.Errorf("%s is missing %q", relative, token)
			}
		}
	}

	combined := strings.Join([]string{
		sources["scripts/release/new-release-manifest.ps1"],
		sources["scripts/release/check-release-manifest.ps1"],
		sources["scripts/release/check-platform-kernel.ps1"],
		sources["scripts/release/export-docker-images.ps1"],
		sources["scripts/release/deploy-admin-only.ps1"],
		sources["scripts/release/rollback-admin-only.ps1"],
	}, "\n")
	for _, forbidden := range []*regexp.Regexp{
		regexp.MustCompile(`(?i)git\s+(?:checkout|switch)\s+`),
		regexp.MustCompile(`(?i)docker\s+(?:compose\s+)?(?:down\s+-v|volume\s+rm)`),
		regexp.MustCompile(`(?i)Write-(?:Host|Output).*\$(?:password|token|cookie|dsn|secret)`),
		regexp.MustCompile(`(?i)github|workflow|tauri|client[_-]?variant`),
	} {
		if forbidden.MatchString(combined) {
			t.Errorf("release scripts contain forbidden pattern %s", forbidden)
		}
	}
}

func TestAdminReleaseComposeUsesOnlyImmutableArtifacts(t *testing.T) {
	compose := string(readBackendArchitectureFile(t, "deploy/admin-only/docker-compose.yml"))
	for _, required := range []string{
		"image: ${ADMIN_FRONTEND_IMAGE:?ADMIN_FRONTEND_IMAGE is required}",
		"image: ${ADMIN_BACKEND_IMAGE:?ADMIN_BACKEND_IMAGE is required}",
		"pull_policy: never",
		"org.opencontainers.image.revision",
		"healthcheck:",
		"external: true",
	} {
		if !strings.Contains(compose, required) {
			t.Errorf("Admin release Compose is missing %q", required)
		}
	}
	for _, forbidden := range []string{"build:", ":latest", "down -v"} {
		if strings.Contains(compose, forbidden) {
			t.Errorf("Admin release Compose contains forbidden %q", forbidden)
		}
	}
}

func releaseObject(t *testing.T, value any, label string) map[string]any {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s must be an object", label)
	}
	return object
}

func releaseStringArray(t *testing.T, value any, label string) []string {
	t.Helper()
	values, ok := value.([]any)
	if !ok {
		t.Fatalf("%s must be an array", label)
	}
	result := make([]string, len(values))
	for index, value := range values {
		text, ok := value.(string)
		if !ok {
			t.Fatalf("%s[%d] must be a string", label, index)
		}
		result[index] = text
	}
	return result
}

func assertPowerShellParses(t *testing.T, path string) {
	t.Helper()
	powerShell, err := exec.LookPath("pwsh")
	if err != nil {
		t.Logf("pwsh unavailable; static release assertions still apply: %v", err)
		return
	}
	script := `$tokens=$null; $errors=$null; [Management.Automation.Language.Parser]::ParseFile($env:ADMIN_RELEASE_PARSE_PATH, [ref]$tokens, [ref]$errors) | Out-Null; if ($errors.Count -ne 0) { $errors | ForEach-Object { Write-Error $_.Message }; exit 1 }`
	command := exec.Command(powerShell, "-NoProfile", "-Command", script)
	command.Env = append(os.Environ(), "ADMIN_RELEASE_PARSE_PATH="+path)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("PowerShell parse failed for %s: %v: %s", path, err, output)
	}
}

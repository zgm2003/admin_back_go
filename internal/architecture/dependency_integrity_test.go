package architecture

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const asynqmonV072Sum = "github.com/hibiken/asynqmon v0.7.2 h1:YohWgTIPwtMyZ6khBDcVUz9BdSdQW2Dxn8SoxtbmjSg="

func requireWindowsPowerShell(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("Windows PowerShell behavior test")
	}
	powerShell, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Skipf("powershell.exe is unavailable: %v", err)
	}
	return powerShell
}

func copyTestFile(t *testing.T, source, destination string) {
	t.Helper()
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeFakeGoCommand(t *testing.T, directory string) {
	t.Helper()
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "@echo off\r\nif \"%FAKE_GO_EXIT%\"==\"23\" exit /b 23\r\nexit /b 0\r\n"
	if err := os.WriteFile(filepath.Join(directory, "go.cmd"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func runBackendVerificationWithFakeGo(t *testing.T, powerShell, sandbox, script, fakeBin string) ([]byte, error) {
	t.Helper()
	harness := filepath.Join(sandbox, "run-backend-verification.ps1")
	harnessSource := `param(
  [Parameter(Mandatory = $true)]
  [string]$ScriptPath,
  [Parameter(Mandatory = $true)]
  [string]$FakeBin
)

$ErrorActionPreference = "Stop"
$env:Path = $FakeBin + [IO.Path]::PathSeparator + $env:Path
$env:FAKE_GO_EXIT = "23"
& $ScriptPath
`
	if err := os.WriteFile(harness, []byte(harnessSource), 0o600); err != nil {
		t.Fatal(err)
	}

	command := exec.Command(
		powerShell,
		"-NoProfile",
		"-ExecutionPolicy", "Bypass",
		"-File", harness,
		"-ScriptPath", script,
		"-FakeBin", fakeBin,
	)
	return command.CombinedOutput()
}

func containsExactLine(data []byte, expected string) bool {
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSuffix(line, "\r") == expected {
			return true
		}
	}
	return false
}

func goModLineFields(line string) []string {
	if comment := strings.Index(line, "//"); comment >= 0 {
		line = line[:comment]
	}
	line = strings.ReplaceAll(line, "(", " ( ")
	line = strings.ReplaceAll(line, ")", " ) ")
	return strings.Fields(line)
}

func goModTokenValue(token string) (string, bool) {
	if value, err := strconv.Unquote(token); err == nil {
		return value, true
	}
	return strings.Trim(token, "\"`"), !strings.ContainsAny(token, "\"`")
}

func goModValue(data []byte, key string) (string, bool) {
	inRequireBlock := false
	for _, line := range strings.Split(string(data), "\n") {
		fields := goModLineFields(line)
		if len(fields) == 0 {
			continue
		}
		if inRequireBlock {
			if fields[0] == ")" {
				inRequireBlock = false
				continue
			}
			if len(fields) >= 2 && fields[0] == key {
				return fields[1], true
			}
			continue
		}
		if key == "go" && len(fields) >= 2 && fields[0] == "go" {
			return fields[1], true
		}
		if fields[0] != "require" || len(fields) < 2 {
			continue
		}
		if fields[1] == "(" {
			inRequireBlock = true
			continue
		}
		if len(fields) >= 3 && fields[1] == key {
			return fields[2], true
		}
	}
	return "", false
}

func protectedGoModReplacements(data []byte) []string {
	var protected []string
	inReplaceBlock := false
	for _, line := range strings.Split(string(data), "\n") {
		fields := goModLineFields(line)
		if len(fields) == 0 {
			continue
		}
		original := ""
		if inReplaceBlock {
			if fields[0] == ")" {
				inReplaceBlock = false
				continue
			}
			original = fields[0]
		} else if fields[0] == "replace" && len(fields) >= 2 {
			if fields[1] == "(" {
				inReplaceBlock = true
				continue
			}
			original = fields[1]
		}
		original, valid := goModTokenValue(original)
		for _, modulePath := range []string{"github.com/quic-go/quic-go", "golang.org/x/image"} {
			if original == modulePath || (!valid && strings.HasPrefix(original, modulePath)) {
				protected = append(protected, modulePath)
				break
			}
		}
	}
	return protected
}

func exactTrimmedLineCount(data []byte, expected string) int {
	count := 0
	inHTMLComment := false
	for _, line := range strings.Split(string(data), "\n") {
		commented := inHTMLComment
		remaining := line
		for {
			marker := "<!--"
			if inHTMLComment {
				marker = "-->"
			}
			index := strings.Index(remaining, marker)
			if index < 0 {
				break
			}
			commented = true
			inHTMLComment = !inHTMLComment
			remaining = remaining[index+len(marker):]
		}
		if !commented && strings.TrimSpace(line) == expected {
			count++
		}
	}
	return count
}

func requireExactTrimmedLineCount(t *testing.T, root, rel, expected string, want int) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	if got := exactTrimmedLineCount(data, expected); got != want {
		t.Errorf("%s contains exact trimmed line %q %d times, want %d", rel, expected, got, want)
	}
}

func TestSecureFoundationGuardRejectsProtectedReplacements(t *testing.T) {
	for name, testCase := range map[string]struct {
		goMod string
		want  string
	}{
		"quic single line with original version": {
			goMod: "replace github.com/quic-go/quic-go v0.59.1 => github.com/quic-go/quic-go v0.59.0\r\n",
			want:  "github.com/quic-go/quic-go",
		},
		"image block without original version": {
			goMod: "replace (\r\n\tgolang.org/x/image => golang.org/x/image v0.42.0\r\n)\r\n",
			want:  "golang.org/x/image",
		},
		"quic no-space block": {
			goMod: "replace(\r\n\tgithub.com/quic-go/quic-go => github.com/quic-go/quic-go v0.59.0\r\n)\r\n",
			want:  "github.com/quic-go/quic-go",
		},
		"quic quoted single line": {
			goMod: "replace \"github.com/quic-go/quic-go\" => github.com/quic-go/quic-go v0.59.0\r\n",
			want:  "github.com/quic-go/quic-go",
		},
		"image quoted block": {
			goMod: "replace (\r\n\t\"golang.org/x/image\" v0.43.0 => golang.org/x/image v0.42.0\r\n)\r\n",
			want:  "golang.org/x/image",
		},
		"quic malformed quoted token": {
			goMod: "replace \"github.com/quic-go/quic-go\\q\" => github.com/quic-go/quic-go v0.59.0\r\n",
			want:  "github.com/quic-go/quic-go",
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := protectedGoModReplacements([]byte(testCase.goMod))
			if len(got) != 1 || got[0] != testCase.want {
				t.Fatalf("protected replacements = %v, want [%s]", got, testCase.want)
			}
		})
	}
}

func TestSecureFoundationGuardRejectsBuildSurfaceCommentDecoys(t *testing.T) {
	for name, testCase := range map[string]struct {
		content  string
		expected string
	}{
		"Dockerfile": {
			content:  "ARG GO_BUILD_IMAGE=golang:1.25.0-bookworm\r\n# ARG GO_BUILD_IMAGE=golang:1.26.5-bookworm\r\n",
			expected: "ARG GO_BUILD_IMAGE=golang:1.26.5-bookworm",
		},
		"Compose": {
			content:  strings.Repeat("GO_BUILD_IMAGE: docker.m.daocloud.io/library/golang:1.25.0-bookworm\r\n# GO_BUILD_IMAGE: docker.m.daocloud.io/library/golang:1.26.5-bookworm\r\n", 2),
			expected: "GO_BUILD_IMAGE: docker.m.daocloud.io/library/golang:1.26.5-bookworm",
		},
		"README multiline HTML comment": {
			content:  "| Language | Go `1.25.0` |\r\n<!--\r\n| Language | Go `1.26.5` |\r\n-->\r\n",
			expected: "| Language | Go `1.26.5` |",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := exactTrimmedLineCount([]byte(testCase.content), testCase.expected); got != 0 {
				t.Fatalf("comment decoy count = %d, want 0", got)
			}
		})
	}
}

func TestSecureFoundationGuardAcceptsCRLF(t *testing.T) {
	const line = "ARG GO_BUILD_IMAGE=golang:1.26.5-bookworm"
	if got := exactTrimmedLineCount([]byte("  "+line+"\r\n"), line); got != 1 {
		t.Fatalf("CRLF exact line count = %d, want 1", got)
	}
	goMod := []byte("go 1.26.5\r\nrequire (\r\n\tgithub.com/quic-go/quic-go v0.59.1\r\n\tgolang.org/x/image v0.43.0\r\n)\r\n")
	for key, want := range map[string]string{
		"go":                         "1.26.5",
		"github.com/quic-go/quic-go": "v0.59.1",
		"golang.org/x/image":         "v0.43.0",
	} {
		if got, ok := goModValue(goMod, key); !ok || got != want {
			t.Errorf("CRLF go.mod %s=%s, found=%v, want %s", key, got, ok, want)
		}
	}
}

func TestSecureGoFoundationVersions(t *testing.T) {
	root := backendRoot(t)
	goMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"go":                         "1.26.5",
		"github.com/quic-go/quic-go": "v0.59.1",
		"golang.org/x/image":         "v0.43.0",
	} {
		got, ok := goModValue(goMod, key)
		if !ok {
			t.Errorf("go.mod value %q not found", key)
		} else if got != want {
			t.Errorf("go.mod %s=%s, want %s", key, got, want)
		}
	}
	for _, modulePath := range protectedGoModReplacements(goMod) {
		t.Errorf("go.mod replace directive for protected module %s is forbidden", modulePath)
	}

	requireExactTrimmedLineCount(t, root, "Dockerfile", "ARG GO_BUILD_IMAGE=golang:1.26.5-bookworm", 1)
	requireExactTrimmedLineCount(t, root, "deploy/docker-first/docker-compose.yml", "GO_BUILD_IMAGE: docker.m.daocloud.io/library/golang:1.26.5-bookworm", 2)
	requireExactTrimmedLineCount(t, root, "README.md", "| Language | Go `1.26.5` |", 1)
}

func TestAsynqmonChecksumMatchesTransparencyLog(t *testing.T) {
	root := backendRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	if !containsExactLine(data, asynqmonV072Sum) {
		t.Fatalf("go.sum must contain verified sum %q", asynqmonV072Sum)
	}
}

func TestBackendVerificationEntrypointsExist(t *testing.T) {
	root := backendRoot(t)
	for _, rel := range []string{
		"scripts/verify-go-clean.ps1",
		"scripts/verify-backend.ps1",
	} {
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Errorf("backend verification entrypoint %s: %v", rel, err)
			continue
		}
		if !info.Mode().IsRegular() {
			t.Errorf("backend verification entrypoint %s must be a regular file", rel)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("backend verification entrypoint %s must not be empty", rel)
		}
	}
}

func TestBackendVerificationDocumentationCoversSupportedPowerShellHosts(t *testing.T) {
	root := backendRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}

	for _, command := range []string{
		"pwsh -NoProfile -File scripts/verify-backend.ps1",
		"powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/verify-backend.ps1",
		"pwsh -NoProfile -File scripts/verify-go-clean.ps1",
		"powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/verify-go-clean.ps1",
		"pwsh -NoProfile -File scripts/verify-go-clean.ps1 -KeepScratch",
		"powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/verify-go-clean.ps1 -KeepScratch",
	} {
		if !strings.Contains(string(data), command) {
			t.Errorf("README.md must document %q", command)
		}
	}
}

func TestCleanVerificationRestoresGOMODCACHEState(t *testing.T) {
	powerShell := requireWindowsPowerShell(t)
	root := backendRoot(t)
	sandbox := t.TempDir()
	cleanScript := filepath.Join(sandbox, "scripts", "verify-go-clean.ps1")
	copyTestFile(t, filepath.Join(root, "scripts", "verify-go-clean.ps1"), cleanScript)
	fakeBin := filepath.Join(sandbox, "fake-bin")
	writeFakeGoCommand(t, fakeBin)

	harness := filepath.Join(sandbox, "verify-gomodcache-state.ps1")
	harnessSource := `[CmdletBinding()]
param(
  [Parameter(Mandatory = $true)]
  [string]$ScriptPath,
  [Parameter(Mandatory = $true)]
  [string]$FakeBin,
  [Parameter(Mandatory = $true)]
  [ValidateSet("unset", "empty", "nonempty")]
  [string]$State,
  [switch]$ExpectFailure
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if ($null -eq ("AdminGoVerificationTestNativeEnvironment" -as [type])) {
  Add-Type -TypeDefinition @'
using System.Runtime.InteropServices;

public static class AdminGoVerificationTestNativeEnvironment
{
    [DllImport("kernel32.dll", EntryPoint = "SetEnvironmentVariableW", CharSet = CharSet.Unicode, SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    public static extern bool SetEnvironmentVariable(string name, string value);
}
'@
}

$env:Path = $FakeBin + [IO.Path]::PathSeparator + $env:Path
if ($ExpectFailure) {
  $env:FAKE_GO_EXIT = "23"
} else {
  $env:FAKE_GO_EXIT = "0"
}

switch ($State) {
  "unset" {
    [Environment]::SetEnvironmentVariable("GOMODCACHE", $null, [EnvironmentVariableTarget]::Process)
    $expectedPresent = $false
    $expectedValue = $null
  }
  "empty" {
    if (-not [AdminGoVerificationTestNativeEnvironment]::SetEnvironmentVariable("GOMODCACHE", [string]::Empty)) {
      throw "SetEnvironmentVariableW failed with Win32 error $([Runtime.InteropServices.Marshal]::GetLastWin32Error())."
    }
    $expectedPresent = $true
    $expectedValue = ""
  }
  "nonempty" {
    $expectedValue = "admin-go-original-modcache"
    [Environment]::SetEnvironmentVariable("GOMODCACHE", $expectedValue, [EnvironmentVariableTarget]::Process)
    $expectedPresent = $true
  }
}

$failure = $null
try {
  & $ScriptPath
} catch {
  $failure = $_
}

if ($ExpectFailure) {
  if ($null -eq $failure -or -not $failure.Exception.Message.Contains("exit code 23")) {
    throw "expected fake go exit code 23"
  }
} elseif ($null -ne $failure) {
  throw $failure
}

$environment = [Environment]::GetEnvironmentVariables([EnvironmentVariableTarget]::Process)
$actualPresent = $environment.Contains("GOMODCACHE")
if ($actualPresent -ne $expectedPresent) {
  throw "GOMODCACHE presence mismatch for state $State after script"
}
if ($expectedPresent) {
  $actualValue = [Environment]::GetEnvironmentVariable("GOMODCACHE", [EnvironmentVariableTarget]::Process)
  if ($actualValue -cne $expectedValue) {
    throw "GOMODCACHE value mismatch for state $State after script"
  }
}
`
	if err := os.WriteFile(harness, []byte(harnessSource), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, state := range []string{"unset", "empty", "nonempty"} {
		for _, expectFailure := range []bool{false, true} {
			name := state + "/success"
			if expectFailure {
				name = state + "/failure"
			}
			t.Run(name, func(t *testing.T) {
				arguments := []string{
					"-NoProfile",
					"-ExecutionPolicy", "Bypass",
					"-File", harness,
					"-ScriptPath", cleanScript,
					"-FakeBin", fakeBin,
					"-State", state,
				}
				if expectFailure {
					arguments = append(arguments, "-ExpectFailure")
				}
				command := exec.Command(powerShell, arguments...)
				if output, err := command.CombinedOutput(); err != nil {
					t.Fatalf("PowerShell harness failed: %v\n%s", err, output)
				}
			})
		}
	}
}

func TestBackendVerificationClearsStaleBinariesBeforeRunning(t *testing.T) {
	powerShell := requireWindowsPowerShell(t)
	root := backendRoot(t)
	sandbox := t.TempDir()
	backendScript := filepath.Join(sandbox, "scripts", "verify-backend.ps1")
	copyTestFile(t, filepath.Join(root, "scripts", "verify-backend.ps1"), backendScript)
	fakeBin := filepath.Join(sandbox, "fake-bin")
	writeFakeGoCommand(t, fakeBin)

	verifyBin := filepath.Join(sandbox, ".tmp", "verify-bin")
	if err := os.MkdirAll(verifyBin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"admin-api.exe", "admin-worker.exe"} {
		if err := os.WriteFile(filepath.Join(verifyBin, name), []byte("stale"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	output, err := runBackendVerificationWithFakeGo(t, powerShell, sandbox, backendScript, fakeBin)
	if err == nil {
		t.Fatalf("backend verification must propagate the fake go failure\n%s", output)
	}
	if !strings.Contains(string(output), "exit code 23") {
		t.Fatalf("backend verification failed for the wrong reason: %v\n%s", err, output)
	}

	entries, err := os.ReadDir(verifyBin)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("verification output directory contains stale entries after early failure: %v", entries)
	}
}

func TestBackendVerificationRefusesReparseOutputDirectory(t *testing.T) {
	powerShell := requireWindowsPowerShell(t)
	root := backendRoot(t)
	sandbox := t.TempDir()
	backendScript := filepath.Join(sandbox, "scripts", "verify-backend.ps1")
	copyTestFile(t, filepath.Join(root, "scripts", "verify-backend.ps1"), backendScript)
	fakeBin := filepath.Join(sandbox, "fake-bin")
	writeFakeGoCommand(t, fakeBin)

	temporaryDirectory := filepath.Join(sandbox, ".tmp")
	if err := os.MkdirAll(temporaryDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(sandbox, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(outside, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(temporaryDirectory, "verify-bin")); err != nil {
		t.Skipf("cannot create directory symlink for reparse-point test: %v", err)
	}

	output, err := runBackendVerificationWithFakeGo(t, powerShell, sandbox, backendScript, fakeBin)
	if err == nil {
		t.Fatalf("backend verification must reject a reparse output directory\n%s", output)
	}
	if !strings.Contains(strings.ToLower(string(output)), "reparse point") {
		t.Fatalf("backend verification failed without identifying the reparse point: %v\n%s", err, output)
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("reparse target was modified: %v", err)
	}
}

func TestAsynqmonChecksumMatchesTransparencyLogAcceptsCRLF(t *testing.T) {
	root := t.TempDir()
	architectureDir := filepath.Join(root, "internal", "architecture")
	if err := os.MkdirAll(architectureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.sum"), []byte(asynqmonV072Sum+"\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	previousWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(architectureDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previousWorkingDirectory); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	TestAsynqmonChecksumMatchesTransparencyLog(t)
}

func TestContainsExactLineRejectsModifiedLines(t *testing.T) {
	for name, content := range map[string]string{
		"old checksum": "github.com/hibiken/asynqmon v0.7.2 h1:EfLRppj5GlklMPzdCjdonpXz/D23meW0Pk6NAtkOPhw=\r\n",
		"prefix":       "prefix" + asynqmonV072Sum + "\r\n",
		"suffix":       asynqmonV072Sum + "suffix\n",
		"tampered":     strings.Replace(asynqmonV072Sum, "YohW", "YohX", 1) + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			if containsExactLine([]byte(content), asynqmonV072Sum) {
				t.Fatalf("modified line must not match: %q", content)
			}
		})
	}
}

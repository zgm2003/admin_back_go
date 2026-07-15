# Admin Foundation Security Baseline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the vulnerable Go 1.26.1 foundation with the approved stable Go 1.26.5 baseline and the minimum fixed `quic-go` and `x/image` versions before blocking CI is introduced.

**Architecture:** Treat the Go patch version as one repository-wide build contract shared by `go.mod`, Docker, CI, README, and active plans. Lock the contract with an architecture test, then apply only the module movements required to remove the reachable `quic-go` and `x/image` vulnerabilities. Local Go 1.27rc1 limitations remain visible; Task 6 CI on Go 1.26.5 provides the authoritative staticcheck and standard-library vulnerability proof.

**Tech Stack:** Go 1.26.5, PowerShell 5.1/7, Docker BuildKit, `govulncheck@v1.6.0`, `staticcheck@v0.7.0`.

---

## Target file map

- Modify `docs/superpowers/plans/2026-07-15-admin-foundation-verification-plan.md` — secure P01 toolchain and Task 6 setup-go version.
- Modify `docs/superpowers/plans/2026-07-15-admin-platform-super-refactor-execution-index.md` — program-wide Go baseline.
- Modify `docs/superpowers/plans/2026-07-15-admin-database-evolution-plan.md` — downstream Go baseline.
- Modify `docs/superpowers/plans/2026-07-15-admin-go-runtime-contracts-plan.md` — downstream Go baseline.
- Modify `docs/superpowers/plans/2026-07-15-admin-go-identity-routing-plan.md` — downstream Go baseline.
- Modify `docs/superpowers/plans/2026-07-15-admin-go-durable-work-realtime-plan.md` — downstream Go baseline.
- Modify `internal/architecture/dependency_integrity_test.go` — executable stable-version guard.
- Modify `go.mod` and `go.sum` — Go directive and minimum fixed modules.
- Modify `Dockerfile` — default official Go build image.
- Modify `deploy/docker-first/docker-compose.yml` — both mirrored Go build-image arguments.
- Modify `README.md` — supported backend language version.

### Task 1: Correct the approved program plans

**Files:**
- Modify: `docs/superpowers/plans/2026-07-15-admin-foundation-verification-plan.md`
- Modify: `docs/superpowers/plans/2026-07-15-admin-platform-super-refactor-execution-index.md`
- Modify: `docs/superpowers/plans/2026-07-15-admin-database-evolution-plan.md`
- Modify: `docs/superpowers/plans/2026-07-15-admin-go-runtime-contracts-plan.md`
- Modify: `docs/superpowers/plans/2026-07-15-admin-go-identity-routing-plan.md`
- Modify: `docs/superpowers/plans/2026-07-15-admin-go-durable-work-realtime-plan.md`

- [ ] **Step 1: Replace the obsolete program baseline**

Replace every active-plan declaration of `Go 1.26.1` with `Go 1.26.5`. In the P01 workflow example, replace:

```yaml
go-version: 1.26.1
```

with:

```yaml
go-version: 1.26.5
```

Do not rewrite historical current-version evidence in the approved design specification; the design intentionally records the transition from 1.26.1.

- [ ] **Step 2: Verify plan consistency**

Run:

```powershell
$plans = @(
  'docs/superpowers/plans/2026-07-15-admin-foundation-verification-plan.md',
  'docs/superpowers/plans/2026-07-15-admin-platform-super-refactor-execution-index.md',
  'docs/superpowers/plans/2026-07-15-admin-database-evolution-plan.md',
  'docs/superpowers/plans/2026-07-15-admin-go-runtime-contracts-plan.md',
  'docs/superpowers/plans/2026-07-15-admin-go-identity-routing-plan.md',
  'docs/superpowers/plans/2026-07-15-admin-go-durable-work-realtime-plan.md'
)
foreach ($plan in $plans) {
  $text = [IO.File]::ReadAllText((Resolve-Path -LiteralPath $plan))
  if ($text.Contains('Go 1.26.1') -or $text.Contains('go-version: 1.26.1')) {
    throw "active plan still contains the obsolete Go baseline: $plan"
  }
  if (!$text.Contains('1.26.5')) {
    throw "active plan is missing the secure Go baseline: $plan"
  }
}
```

Expected: all six files pass; the P01 plan contains both its corrected Tech Stack declaration and `go-version: 1.26.5`.

- [ ] **Step 3: Commit the plan correction**

```powershell
git add -- docs/superpowers/plans/2026-07-15-admin-foundation-verification-plan.md docs/superpowers/plans/2026-07-15-admin-platform-super-refactor-execution-index.md docs/superpowers/plans/2026-07-15-admin-database-evolution-plan.md docs/superpowers/plans/2026-07-15-admin-go-runtime-contracts-plan.md docs/superpowers/plans/2026-07-15-admin-go-identity-routing-plan.md docs/superpowers/plans/2026-07-15-admin-go-durable-work-realtime-plan.md
git diff --cached --check
git commit -m "docs(plan): align secure Go foundation baseline"
```

### Task 2: Add a failing stable-version guard

**Files:**
- Modify: `internal/architecture/dependency_integrity_test.go`

- [ ] **Step 1: Add go.mod and build-surface helpers**

Add helpers that parse whitespace-delimited `go.mod` lines without adding a parser dependency:

```go
func goModValue(t *testing.T, data []byte, key string) string {
	t.Helper()
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == key {
			return fields[1]
		}
	}
	t.Fatalf("go.mod value %q not found", key)
	return ""
}

func requireFileContainsCount(t *testing.T, root, rel, value string, count int) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(data), value); got != count {
		t.Fatalf("%s contains %q %d times, want %d", rel, value, got, count)
	}
}
```

- [ ] **Step 2: Add the secure baseline test**

```go
func TestSecureGoFoundationVersions(t *testing.T) {
	root := backendRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}

	for key, want := range map[string]string{
		"go":                           "1.26.5",
		"github.com/quic-go/quic-go": "v0.59.1",
		"golang.org/x/image":          "v0.43.0",
	} {
		if got := goModValue(t, data, key); got != want {
			t.Errorf("go.mod %s=%s, want %s", key, got, want)
		}
	}

	requireFileContainsCount(t, root, "Dockerfile", "golang:1.26.5-bookworm", 1)
	requireFileContainsCount(t, root, "deploy/docker-first/docker-compose.yml", "golang:1.26.5-bookworm", 2)
	requireFileContainsCount(t, root, "README.md", "Go `1.26.5`", 1)
}
```

- [ ] **Step 3: Run the guard and confirm the obsolete baseline**

Run:

```powershell
go test ./internal/architecture -run '^TestSecureGoFoundationVersions$' -count=1
```

Expected: FAIL reporting Go `1.26.1`, `quic-go v0.59.0`, `x/image v0.25.0`, and the missing 1.26.5 build-surface strings.

### Task 3: Apply the minimum stable security update

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Modify: `Dockerfile`
- Modify: `deploy/docker-first/docker-compose.yml`
- Modify: `README.md`
- Modify: `internal/architecture/dependency_integrity_test.go`

- [ ] **Step 1: Update the Go directive and fixed modules**

Run:

```powershell
go mod edit -go=1.26.5
go get github.com/quic-go/quic-go@v0.59.1 golang.org/x/image@v0.43.0
go mod tidy -go=1.26.5
```

Expected version facts:

```text
go 1.26.5
github.com/quic-go/quic-go v0.59.1 // indirect
golang.org/x/image v0.43.0
```

`golang.org/x/text` may move to the minimum selected version required by `x/image v0.43.0`. No unrelated direct dependency may change.

- [ ] **Step 2: Update every executable build surface**

Apply these exact replacements:

```text
Dockerfile:
  golang:1.26.1-bookworm -> golang:1.26.5-bookworm

deploy/docker-first/docker-compose.yml (two occurrences):
  docker.m.daocloud.io/library/golang:1.26.1-bookworm
  -> docker.m.daocloud.io/library/golang:1.26.5-bookworm

README.md:
  Go `1.26.1` -> Go `1.26.5`
```

- [ ] **Step 3: Review dependency movement**

Run:

```powershell
git diff -- go.mod go.sum
go mod graph | Select-String -Pattern 'quic-go|golang.org/x/image|golang.org/x/text'
```

Expected: the two security upgrades and their required minimum-version effects only. Stop and investigate any unrelated direct-module change before continuing.

- [ ] **Step 4: Run the guard and focused module checks**

```powershell
go test ./internal/architecture -run 'TestSecureGoFoundationVersions|TestAsynqmon' -count=1
go mod verify
```

Expected: tests pass and `go mod verify` prints `all modules verified`.

### Task 4: Verify and commit the secure baseline

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Modify: `Dockerfile`
- Modify: `deploy/docker-first/docker-compose.yml`
- Modify: `README.md`
- Modify: `internal/architecture/dependency_integrity_test.go`

- [ ] **Step 1: Run repository verification that the workstation supports**

```powershell
go test ./... -count=1
go vet ./...
go build -trimpath -o $env:TEMP\admin-api-secure-baseline.exe ./cmd/admin-api
go build -trimpath -o $env:TEMP\admin-worker-secure-baseline.exe ./cmd/admin-worker
```

Expected: all four commands exit 0.

- [ ] **Step 2: Prove the module findings are gone**

Run:

```powershell
$report = (& go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./... 2>&1 | Out-String)
foreach ($removed in @('GO-2026-5676', 'GO-2026-5061', 'GO-2026-4961')) {
  if ($report.Contains($removed)) { throw "module vulnerability remains: $removed" }
}
if (!$report.Contains('GO-2026-5856')) {
  throw 'expected local Go 1.27rc1 toolchain finding was not reported'
}
```

Expected: the three module vulnerability IDs are absent. `GO-2026-5856` remains because the workstation itself runs Go 1.27rc1; Task 6 CI on Go 1.26.5 must remove it.

- [ ] **Step 3: Record the local staticcheck limitation without bypassing it**

Run:

```powershell
go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...
```

Expected on this workstation: nonzero with Go 1.27rc1 export-data incompatibility. Do not change the pinned version and do not add a skip; Task 6 CI runs the command under Go 1.26.5.

- [ ] **Step 4: Commit the implementation**

```powershell
git add -- go.mod go.sum Dockerfile deploy/docker-first/docker-compose.yml README.md internal/architecture/dependency_integrity_test.go
git diff --cached --check
git commit -m "fix(build): update secure Go foundation baseline"
```

- [ ] **Step 5: Review gate**

Require both reviews before Task 6:

1. specification review against `docs/superpowers/specs/2026-07-16-admin-foundation-security-baseline-design.md` and this plan;
2. code-quality/security review of the version guard, dependency diff, Docker/Compose consistency, and verification evidence.

Any Critical or Important finding returns to the implementer and is re-reviewed after amendment.

## Completion gate

Run:

```powershell
go test ./internal/architecture -run 'TestSecureGoFoundationVersions|TestAsynqmon' -count=1
go test ./... -count=1
go vet ./...
git diff --check HEAD^ HEAD
git status --short
```

Expected: all executable checks exit 0 and the worktree is clean. Local Go 1.27rc1 prevents authoritative `staticcheck` and standard-library `govulncheck` success; Task 6 CI on Go 1.26.5 owns those final proofs.

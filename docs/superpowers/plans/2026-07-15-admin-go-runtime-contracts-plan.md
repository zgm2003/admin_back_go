# Admin Go Runtime and Contracts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the oversized bootstrap with fail-fast API/Worker runtimes, deepen the Error and telemetry modules, establish one route-policy registry, and publish a deterministic Admin Contract Bundle.

**Architecture:** `internal/runtime` owns process lifecycle and cleanup; `internal/platform/admin` owns the Admin service graph; Capability Modules remain under `internal/module`. A route registry compiles access/audit metadata into runtime middleware and contract generation. The bundle is generated from the same registry and typed schema catalog, never from a second route list.

**Tech Stack:** Go 1.26.5, Gin, GORM, MySQL, Redis, Asynq, Prometheus client 1.23.2, JSON Schema 2020-12, OpenAPI 3.1.

---

## Target file map

- Create `internal/runtime/{runtime,cleanup,resources,api,worker,health}.go` and focused tests.
- Create `internal/platform/admin/graph.go` and `internal/platform/admin/build.go` for grouped Admin services.
- Create `internal/server/adminroute/{definition,registry,compile}.go` and tests.
- Modify `internal/server/router.go` and `internal/server/routes_*.go` to consume grouped graphs and compiled route policy.
- Deepen `internal/shared/apperror`; add `internal/middleware/error_reporter.go`.
- Create `internal/telemetry/{telemetry,redact,prometheus}.go` and HTTP/SQL/Redis/queue/provider/realtime instrumentation seams.
- Create `cmd/admin-contract/main.go`, `internal/admincontract/*`, and `contracts/admin/v1/*`.
- Modify `cmd/admin-api/main.go` and `cmd/admin-worker/main.go` to use signal-aware runtimes.
- Remove `internal/bootstrap/app.go`, `worker.go`, `resources.go`, and provider-builder code after their replacements pass; keep P04/P05-owned identity/durable behavior files untouched.

### Task 1: Deepen the Error Module and safe envelope

**Files:**
- Modify: `internal/shared/apperror/error.go`
- Modify: `internal/shared/apperror/error_test.go`
- Modify: `internal/shared/response/response.go`
- Modify: `internal/shared/response/response_test.go`
- Create: `internal/middleware/error_reporter.go`
- Create: `internal/middleware/error_reporter_test.go`

- [ ] **Step 1: Write classification, cause, and response-safety tests**

```go
func TestErrorPreservesCauseAndExposesSafeMetadata(t *testing.T) {
	cause := errors.New("dial mysql user:secret@tcp(private:3306)")
	err := Wrap("dependency.mysql", CategoryDependency, http.StatusServiceUnavailable, Retryable, "common.dependency_unavailable", nil, "服务暂不可用", cause)
	if !errors.Is(err, cause) || err.Code != "dependency.mysql" || !err.Retryable() {
		t.Fatalf("error=%+v", err)
	}
	body := marshalHTTPError(t, err, "req-1", "trace-1")
	if bytes.Contains(body, []byte("secret")) || bytes.Contains(body, []byte("private")) {
		t.Fatalf("cause leaked: %s", body)
	}
	for _, want := range []string{`"error"`, `"dependency.mysql"`, `"dependency"`, `"request_id":"req-1"`} {
		if !bytes.Contains(body, []byte(want)) {
			t.Fatalf("missing %s in %s", want, body)
		}
	}
}
```

- [ ] **Step 2: Prove failure**

Run: `go test ./internal/shared/apperror ./internal/shared/response ./internal/middleware -run 'TestError|TestResponse' -count=1`

Expected: FAIL because categories, retryability, and safe metadata do not exist.

- [ ] **Step 3: Implement the stable interface**

```go
type Category string
const (
	CategoryValidation Category = "validation"
	CategoryAuthentication Category = "authentication"
	CategoryAuthorization Category = "authorization"
	CategoryNotFound Category = "not_found"
	CategoryConflict Category = "conflict"
	CategoryRateLimit Category = "rate_limit"
	CategoryDependency Category = "dependency"
	CategoryTimeout Category = "timeout"
	CategoryInternal Category = "internal"
	CategoryCanceled Category = "canceled"
)
type RetryClass string
const (Permanent RetryClass = "permanent"; Retryable RetryClass = "retryable")
type Error struct {
	Code string
	LegacyCode int
	Category Category
	HTTPStatus int
	Retry RetryClass
	MessageID string
	TemplateData map[string]any
	Message string
	Cause error
	Operation string
}
```

Keep existing helper names so domain migration is incremental, but assign explicit stable defaults such as `request.invalid` and `internal.unknown`. Add `WithCode` and `WithOperation` methods that clone rather than mutate shared errors.

The HTTP body remains compatible at top level and adds:

```go
type ErrorMeta struct {
	Code string `json:"code"`
	Category apperror.Category `json:"category"`
	Retryable bool `json:"retryable"`
	RequestID string `json:"request_id,omitempty"`
	TraceID string `json:"trace_id,omitempty"`
}
type Body struct {
	Code int `json:"code"`
	Data any `json:"data"`
	Msg string `json:"msg"`
	Error *ErrorMeta `json:"error,omitempty"`
}
```

`response.Error` adds the internal error to Gin context. `ErrorReporter` logs a 5xx cause once after `c.Next()` with request/task/run/trace identifiers and redacts values for token, authorization, cookie, password, secret, certificate, prompt, and payload keys.

- [ ] **Step 4: Verify and commit**

```powershell
go test ./internal/shared/apperror ./internal/shared/response ./internal/middleware -count=1
git add -- internal/shared/apperror internal/shared/response internal/middleware/error_reporter.go internal/middleware/error_reporter_test.go
git commit -m "feat(error): add classified safe application failures"
```

### Task 2: Add lifecycle and cleanup primitives

**Files:**
- Create: `internal/runtime/runtime.go`
- Create: `internal/runtime/cleanup.go`
- Create: `internal/runtime/runtime_test.go`

- [ ] **Step 1: Test reverse cleanup and joined failures**

```go
func TestCleanupRunsOnceInReverseOrder(t *testing.T) {
	var got []string
	stack := NewCleanup()
	stack.Add("db", func(context.Context) error { got = append(got, "db"); return errors.New("db close") })
	stack.Add("redis", func(context.Context) error { got = append(got, "redis"); return nil })
	err := stack.Close(context.Background())
	_ = stack.Close(context.Background())
	if !reflect.DeepEqual(got, []string{"redis", "db"}) || !strings.Contains(err.Error(), "db close") {
		t.Fatalf("got=%v err=%v", got, err)
	}
}
```

- [ ] **Step 2: Implement the narrow runtime contract**

```go
type Report struct {
	Status string `json:"status"`
	Checks map[string]Check `json:"checks"`
}
type Runtime interface {
	Start(context.Context) error
	Shutdown(context.Context) error
	Health(context.Context) Report
}
type Cleanup struct {
	mu sync.Mutex
	closed bool
	entries []cleanupEntry
}
```

`Cleanup.Add` rejects an empty name or nil function. `Close` snapshots once, runs reverse order under the caller context, annotates each error with its resource name, and returns `errors.Join`.

- [ ] **Step 3: Verify and commit**

```powershell
go test ./internal/runtime -run 'TestCleanup|TestRuntime' -count=1
git add -- internal/runtime/runtime.go internal/runtime/cleanup.go internal/runtime/runtime_test.go
git commit -m "feat(runtime): add deterministic process lifecycle"
```

### Task 3: Make resource initialization all-or-nothing

**Files:**
- Create: `internal/runtime/resources.go`
- Create: `internal/runtime/resources_test.go`
- Modify: `internal/infra/database/database.go`
- Modify: `internal/infra/redisclient/redis.go`
- Delete: `internal/bootstrap/resources.go`
- Delete: `internal/bootstrap/resources_test.go`

- [ ] **Step 1: Test partial-open cleanup**

Use injected DB/Redis/queue open functions. Make DB open succeed, Redis open fail, then assert DB closes exactly once and the returned `Resources` is nil. Add API/Worker capability tables that prove each enabled resource is required and each disabled resource is reported as disabled.

- [ ] **Step 2: Implement process-owned resources**

```go
type Resources struct {
	DB *database.Client
	Redis *redisclient.Client
	TokenRedis *redisclient.Client
	QueueRedis *redisclient.Client
	cleanup *Cleanup
}
func OpenResources(ctx context.Context, process config.Process, cfg config.Config, open Openers) (*Resources, error)
func (r *Resources) Close(ctx context.Context) error
func (r *Resources) Health(ctx context.Context) Report
```

Open required dependencies in order, ping them before publishing the resource graph, and register cleanup immediately after each successful open. An enabled queue/realtime/scheduler capability with no Redis is a startup error. Do not return a partially populated resource value on failure.

- [ ] **Step 3: Verify and commit**

```powershell
go test ./internal/runtime ./internal/infra/database ./internal/infra/redisclient -count=1
git add -- internal/runtime/resources.go internal/runtime/resources_test.go internal/infra/database/database.go internal/infra/redisclient/redis.go
git rm -- internal/bootstrap/resources.go internal/bootstrap/resources_test.go
git commit -m "refactor(runtime): fail atomically when resources cannot open"
```

### Task 4: Build grouped Admin capability graphs

**Files:**
- Create: `internal/platform/admin/graph.go`
- Create: `internal/platform/admin/build.go`
- Create: `internal/platform/admin/build_test.go`
- Create: `internal/platform/retired/graph.go`
- Create: `internal/runtime/providers.go`
- Move provider factories from: `internal/bootstrap/app.go`
- Modify: `internal/server/router.go`
- Modify: `internal/server/routes_auth.go`
- Modify: `internal/server/routes_admin_ai.go`
- Modify: `internal/server/routes_admin_commerce_rbac.go`
- Modify: `internal/server/routes_admin_comms.go`
- Modify: `internal/server/routes_admin_foundation.go`
- Modify: `internal/server/routes_admin_user.go`
- Modify: `internal/server/routes_canvas.go`

- [ ] **Step 1: Test required graph completeness**

```go
func TestGraphValidateRejectsMissingRequiredCapability(t *testing.T) {
	graph := validGraphForTest()
	graph.Identity.Auth = nil
	err := graph.Validate()
	if err == nil || !strings.Contains(err.Error(), "identity.auth") {
		t.Fatalf("err=%v", err)
	}
}
```

- [ ] **Step 2: Define responsibility groups**

```go
type Graph struct {
	Identity IdentityGraph
	System SystemGraph
	Communications CommunicationsGraph
	Commerce CommerceGraph
	AI AIGraph
}
type IdentityGraph struct {
	Auth auth.SessionService
	Captcha auth.CaptchaHTTPService
	Users user.HTTPService
	Permissions permissionadmin.ManagementService
	Roles roleadmin.HTTPService
	AuthPlatforms authplatformadmin.HTTPService
	Sessions auth.SessionAdminHTTPService
	LoginLogs auth.LoginLogHTTPService
}
```

Define the remaining groups with the existing active Admin HTTP service interfaces. Grouping is composition only; it does not create `ServiceImpl`, Manager, or capability-wide catch-all interfaces.

`Build` receives validated config, resources, key ring, logger, telemetry, queue producer, and realtime publisher. Extract OpenAI-compatible, COS, mail/SMS, Alipay, and secretbox construction into focused functions in `internal/runtime/providers.go`. API and Worker call those same functions.

- [ ] **Step 3: Replace the 50-field router dependency**

`server.Dependencies` becomes:

```go
type Dependencies struct {
	Core CoreDependencies
	Admin admin.Graph
	Retired retired.Graph
}
```

`CoreDependencies` contains readiness, logger, CORS, authenticator, permission checker, compiled route registry, operation recorder, queue monitor UI, and realtime handler. `retired.Graph` is a temporary typed holder for the existing App/Canvas transport dependencies so P03 compiles without deepening them; Admin contract generation never reads it, and P09 deletes it together with the retired route registrations. Route aggregation reads the grouped graphs; it does not rebuild capabilities.

- [ ] **Step 4: Verify and commit**

```powershell
go test ./internal/platform/admin ./internal/server ./internal/runtime -count=1
git add -- internal/platform/admin/graph.go internal/platform/admin/build.go internal/platform/admin/build_test.go internal/platform/retired/graph.go internal/runtime/providers.go internal/server/router.go internal/server/routes_auth.go internal/server/routes_admin_ai.go internal/server/routes_admin_commerce_rbac.go internal/server/routes_admin_comms.go internal/server/routes_admin_foundation.go internal/server/routes_admin_user.go internal/server/routes_canvas.go
git diff --cached --check
git commit -m "refactor(runtime): group admin capability composition"
```

### Task 5: Implement signal-aware API and Worker runtimes

**Files:**
- Create: `internal/runtime/api.go`
- Create: `internal/runtime/api_test.go`
- Create: `internal/runtime/worker.go`
- Create: `internal/runtime/worker_test.go`
- Modify: `cmd/admin-api/main.go`
- Modify: `cmd/admin-worker/main.go`
- Modify: `internal/platform/admin/build.go`
- Modify: `internal/platform/retired/graph.go`
- Modify: `internal/server/router.go`
- Create: `scripts/tests/process-sigterm.tests.ps1`
- Delete: `internal/bootstrap/app.go`
- Delete: `internal/bootstrap/worker.go`
- Delete: `internal/bootstrap/worker_test.go`

- [ ] **Step 1: Write lifecycle-order tests**

API expected order:

```text
resources → providers → admin graph → router → realtime subscriber → HTTP listen
HTTP stop → realtime stop → queue producer close → resources close
```

Worker expected order:

```text
resources → providers → task handlers → queue server → scheduler reconciler
scheduler stop → queue drain → publisher close → resources close
```

Tests inject fakes, cancel context, and assert reverse shutdown plus cleanup after a failure at every boundary.

- [ ] **Step 2: Implement process runtimes**

```go
type APIRuntime struct {
	cfg config.Config
	server HTTPServer
	resources *Resources
	cleanup *Cleanup
	health atomic.Pointer[Report]
}
type WorkerRuntime struct {
	cfg config.Config
	queue QueueServer
	scheduler SchedulerRuntime
	resources *Resources
	cleanup *Cleanup
	health atomic.Pointer[Report]
}
```

`Start` is idempotent only before shutdown; a second concurrent start returns `runtime.already_started`. `Shutdown` accepts the caller deadline, stops admission first, waits for in-flight work, closes resources once, and joins errors.

- [ ] **Step 3: Make both mains own signal cancellation**

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()
if err := process.Start(ctx); err != nil {
	logger.Error("process failed", "process", processName, "error", err)
	os.Exit(1)
}
shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
defer cancel()
if err := process.Shutdown(shutdownCtx); err != nil {
	logger.Error("process shutdown failed", "process", processName, "error", err)
	os.Exit(1)
}
```

The API `Start` blocks on HTTP termination; Worker `Start` blocks on context cancellation or a queue/scheduler fatal error.

- [ ] **Step 4: Verify shutdown under real signals**

```powershell
go test ./internal/runtime -run 'TestAPI|TestWorker' -race -count=1
go build -o $env:TEMP\admin-api-runtime.exe ./cmd/admin-api
go build -o $env:TEMP\admin-worker-runtime.exe ./cmd/admin-worker
pwsh -NoProfile -File scripts/tests/process-sigterm.tests.ps1
```

The PowerShell test allocates loopback ports, starts each temporary binary with the P01 ignored test environment, waits for readiness, sends process termination, and fails if the child survives 15 seconds, exits nonzero, or logs more than one shutdown sequence. It redacts the child environment from failure output.

Expected: both processes become ready, receive termination, exit 0 within 15 seconds, and log one shutdown sequence.

- [ ] **Step 5: Commit**

```powershell
git add -- cmd/admin-api/main.go cmd/admin-worker/main.go internal/runtime/api.go internal/runtime/api_test.go internal/runtime/worker.go internal/runtime/worker_test.go internal/platform/admin/build.go internal/platform/retired/graph.go internal/server/router.go scripts/tests/process-sigterm.tests.ps1
git rm -- internal/bootstrap/app.go internal/bootstrap/worker.go internal/bootstrap/worker_test.go
git diff --cached --check
git commit -m "refactor(runtime): replace bootstrap with api and worker runtimes"
```

### Task 6: Establish one route-policy registry

**Files:**
- Create: `internal/server/adminroute/definition.go`
- Create: `internal/server/adminroute/registry.go`
- Create: `internal/server/adminroute/registry_test.go`
- Modify: `internal/server/router.go`
- Modify: `internal/bootstrap/route_meta.go` during migration
- Create: `internal/server/testdata/admin_route_policy_golden.json`

- [ ] **Step 1: Test duplicate and incomplete definitions**

```go
func TestRegistryRejectsUnclassifiedMutation(t *testing.T) {
	r := NewRegistry()
	err := r.Add(Definition{Method: http.MethodPost, Path: "/api/admin/v1/widgets", Access: Authenticated()})
	if !errors.Is(err, ErrAuditDecisionRequired) {
		t.Fatalf("err=%v", err)
	}
}
func TestRegistryRejectsUnknownPermissionAndDuplicateRoute(t *testing.T) {
	r := NewRegistry(WithPermissionCatalog(map[string]struct{}{"widget_create": {}}))
	mustAdd(t, r, Definition{Method: "GET", Path: "/api/admin/v1/widgets", Access: Permission("widget_read"), Audit: NoAudit("read-only")})
	if err := r.Compile(); !errors.Is(err, ErrUnknownPermission) {
		t.Fatalf("err=%v", err)
	}
}
```

- [ ] **Step 2: Implement closed policies**

```go
type AccessKind string
const (AccessPublic AccessKind = "public"; AccessAuthenticated AccessKind = "authenticated"; AccessPermission AccessKind = "permission")
type Access struct { Kind AccessKind; PermissionCode string }
type Audit struct { Enabled bool; Module string; Action string; Title string; Reason string }
type Definition struct {
	Method string
	Path string
	OperationID string
	Access Access
	Audit Audit
	Tags []string
	RequestSchema string
	ResponseSchema string
}
```

`Registry.Add` normalizes method/path, rejects duplicates, requires a permission code for `Permission`, requires `Audit` or a non-empty `NoAudit` reason for POST/PUT/PATCH/DELETE, and rejects audit on public provider callbacks.

- [ ] **Step 3: Compile current metadata through the registry**

Convert `permissionRouteRules` and `operationRouteRules` into definitions keyed by actual Gin routes. Public paths are explicit definitions. Authenticated read routes get `NoAudit("read-only")`; authenticated current-user mutations get a domain reason such as `NoAudit("self-service state; domain audit retained")`. Compilation fails on any Gin route without exactly one policy.

P04 moves policy calls next to each transport registration and deletes the old maps; this task supplies the single compiled runtime format and a complete golden snapshot first.

- [ ] **Step 4: Verify and commit**

```powershell
go test ./internal/server/adminroute ./internal/server ./internal/bootstrap -run 'TestRegistry|TestRoute|TestPermission|TestOperation' -count=1
git add -- internal/server/adminroute internal/server/router.go internal/bootstrap/route_meta.go internal/server/testdata/admin_route_policy_golden.json
git commit -m "feat(routing): compile access and audit policy in one registry"
```

### Task 7: Add redacted telemetry seams and a real metric adapter

**Files:**
- Create: `internal/telemetry/telemetry.go`
- Create: `internal/telemetry/redact.go`
- Create: `internal/telemetry/redact_test.go`
- Create: `internal/telemetry/prometheus.go`
- Create: `internal/telemetry/prometheus_test.go`
- Modify: `internal/middleware/access_log.go`
- Modify: `internal/infra/database/database.go`
- Modify: `internal/infra/redisclient/redis.go`
- Modify: `internal/infra/taskqueue/server.go`
- Modify: `go.mod` and `go.sum`

- [ ] **Step 1: Test secret redaction and bounded labels**

```go
func TestSanitizeAttributesDropsSensitiveAndHighCardinalityValues(t *testing.T) {
	got := SanitizeAttributes(map[string]any{
		"authorization": "Bearer secret",
		"prompt": "private prompt",
		"http.method": "GET",
		"http.route": "/api/admin/v1/users/:id",
		"user_id": int64(99),
	})
	if _, ok := got["authorization"]; ok { t.Fatal("authorization retained") }
	if _, ok := got["prompt"]; ok { t.Fatal("prompt retained") }
	if got["http.method"] != "GET" || got["http.route"] != "/api/admin/v1/users/:id" {
		t.Fatalf("safe attributes missing: %v", got)
	}
	if _, ok := got["user_id"]; ok { t.Fatal("high-cardinality user id retained as metric label") }
}
```

- [ ] **Step 2: Define the seam**

```go
type Attributes map[string]any
type Recorder interface {
	Count(name string, delta float64, attrs Attributes)
	Observe(name string, value float64, attrs Attributes)
	Start(ctx context.Context, name string, attrs Attributes) (context.Context, func(error))
}
```

Provide `Noop`, deterministic in-memory tests, and Prometheus counters/histograms. Pin `github.com/prometheus/client_golang v1.23.2`. Capability code depends only on `Recorder` where it owns a lifecycle; transport/infra adapters own protocol timings.

- [ ] **Step 3: Instrument required boundaries**

Use bounded attributes for:

- HTTP route/method/status/error code/duration;
- SQL operation/table/duration and slow digest hash;
- Redis operation/error/duration;
- queue type/lane/latency/retry/lease expiry/exhaustion;
- provider name/modality/status/first-byte/total/token totals;
- realtime connection/reconnect/drop/send-pressure;
- scheduler reconciliation and lease ownership.

Never record URL query, bodies, token/session/user IDs, credentials, certificates, prompts, provider payloads, or full SQL binds.

- [ ] **Step 4: Verify and commit**

```powershell
go test ./internal/telemetry ./internal/middleware ./internal/infra/database ./internal/infra/redisclient ./internal/infra/taskqueue -count=1
git add -- go.mod go.sum internal/telemetry internal/middleware/access_log.go internal/infra/database/database.go internal/infra/redisclient/redis.go internal/infra/taskqueue/server.go
git commit -m "feat(observability): instrument runtime boundaries safely"
```

### Task 8: Generate the Admin Contract Bundle

**Files:**
- Create: `cmd/admin-contract/main.go`
- Create: `internal/admincontract/bundle.go`
- Create: `internal/admincontract/bundle_test.go`
- Create: `internal/admincontract/openapi.go`
- Create: `internal/admincontract/openapi_test.go`
- Create: `internal/admincontract/permissions.go`
- Create: `internal/admincontract/permissions_test.go`
- Create: `internal/admincontract/views.go`
- Create: `internal/admincontract/views_test.go`
- Create: `internal/admincontract/realtime.go`
- Create: `internal/admincontract/realtime_test.go`
- Create: `internal/admincontract/manifest.go`
- Create: `internal/admincontract/manifest_test.go`
- Create: `contracts/admin/v1/openapi.json`
- Create: `contracts/admin/v1/permissions.json`
- Create: `contracts/admin/v1/views.json`
- Create: `contracts/admin/v1/realtime/envelope.schema.json`
- Create: `contracts/admin/v1/realtime/events.schema.json`
- Create: `contracts/admin/v1/manifest.json`
- Create: `scripts/generate-admin-contract.ps1`
- Create: `scripts/check-admin-contract.ps1`

- [ ] **Step 1: Write determinism and scope tests**

```go
func TestBundleIsDeterministicAndAdminOnly(t *testing.T) {
	first := buildBundleBytes(t)
	second := buildBundleBytes(t)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("bundle generation is not deterministic")
	}
	for name, data := range first {
		text := string(data)
		if strings.Contains(text, "/api/app/") || strings.Contains(text, "/api/canvas/") {
			t.Fatalf("%s contains retired operation", name)
		}
	}
}
func TestManifestHashesEveryArtifact(t *testing.T) {
	bundle := buildBundle(t)
	for name, data := range bundle.Artifacts {
		if bundle.Manifest.Artifacts[name].SHA256 != sha256Hex(data) {
			t.Fatalf("bad hash for %s", name)
		}
	}
}
```

- [ ] **Step 2: Define the manifest**

```go
type Manifest struct {
	BundleVersion string `json:"bundle_version"`
	OpenAPIVersion string `json:"openapi_version"`
	PermissionVersion string `json:"permission_version"`
	RealtimeVersion string `json:"realtime_version"`
	BackendCommit string `json:"backend_commit"`
	Artifacts map[string]Artifact `json:"artifacts"`
}
type Artifact struct {
	SHA256 string `json:"sha256"`
	SchemaVersion string `json:"schema_version"`
}
```

Bundle version is `admin-2026-07-15.1`. The commit is provided by `-commit` and never discovered differently in tests.

- [ ] **Step 3: Generate contracts from runtime truth**

`openapi.json` is OpenAPI 3.1 and contains every Admin route plus `/health`, `/ready`, and required payment callback routes. It uses the Error Module envelope and names every operation ID. `permissions.json` contains the permission-code catalog and policy per operation. `views.json` contains route/view/menu metadata returned by `users/me`. Realtime schemas close event names and payload shapes; there is no free-form string event escape.

`admin-contract generate --out contracts/admin/v1 --commit $BackendCommit` writes a temporary directory, sorts all paths/codes/events, hashes each artifact, writes manifest last, and atomically replaces output. `--check` generates elsewhere and byte-compares every file. `scripts/generate-admin-contract.ps1` sets `$BackendCommit` from a clean committed checkout and passes it explicitly.

- [ ] **Step 4: Generate, check, and commit**

```powershell
pwsh -NoProfile -File scripts/generate-admin-contract.ps1
pwsh -NoProfile -File scripts/check-admin-contract.ps1
go test ./internal/admincontract -count=1
git add -- cmd/admin-contract/main.go internal/admincontract/bundle.go internal/admincontract/bundle_test.go internal/admincontract/openapi.go internal/admincontract/openapi_test.go internal/admincontract/permissions.go internal/admincontract/permissions_test.go internal/admincontract/views.go internal/admincontract/views_test.go internal/admincontract/realtime.go internal/admincontract/realtime_test.go internal/admincontract/manifest.go internal/admincontract/manifest_test.go contracts/admin/v1/openapi.json contracts/admin/v1/permissions.json contracts/admin/v1/views.json contracts/admin/v1/realtime/envelope.schema.json contracts/admin/v1/realtime/events.schema.json contracts/admin/v1/manifest.json scripts/generate-admin-contract.ps1 scripts/check-admin-contract.ps1
git diff --cached --check
git commit -m "feat(contract): publish deterministic admin bundle"
```

Expected: check reports no diff; manifest hashes match; no App/Canvas operation exists.

### Task 9: Add architecture and runtime gates

**Files:**
- Create: `internal/architecture/runtime_contract_test.go`
- Create: `scripts/verify-runtime-contracts.ps1`
- Modify: `scripts/verify-backend.ps1`
- Modify: `scripts/verify-go-clean.ps1`
- Modify: `.github/workflows/verify-backend.yml`
- Modify: `docs/architecture.md`
- Modify: `CONTEXT.md`

- [ ] **Step 1: Guard the target shape**

The architecture test rejects:

- `bootstrap.New`, `bootstrap.NewWorker`, or a process composition root outside `internal/runtime`;
- more than ten top-level fields in `server.Dependencies`;
- App/Canvas operations in `contracts/admin`;
- direct Gin imports in Capability service/repository files;
- raw `apperror.Error` cause serialization;
- route-policy maps outside `adminroute`;
- a mutable config value after runtime construction.

- [ ] **Step 2: Implement the shared gate**

`verify-runtime-contracts.ps1` runs:

```powershell
go test ./internal/runtime ./internal/platform/admin ./internal/server/... ./internal/admincontract ./internal/telemetry -race -count=1
pwsh -NoProfile -File scripts/check-admin-contract.ps1
go test ./internal/architecture -run 'TestRuntime|TestAdminContract|TestRoutePolicy' -count=1
go build -trimpath -o $env:TEMP\admin-api-contract.exe ./cmd/admin-api
go build -trimpath -o $env:TEMP\admin-worker-contract.exe ./cmd/admin-worker
```

- [ ] **Step 3: Make it blocking and commit**

```powershell
pwsh -NoProfile -File scripts/verify-runtime-contracts.ps1
pwsh -NoProfile -File scripts/verify-backend.ps1
git add -- internal/architecture/runtime_contract_test.go scripts/verify-runtime-contracts.ps1 scripts/verify-backend.ps1 scripts/verify-go-clean.ps1 .github/workflows/verify-backend.yml docs/architecture.md CONTEXT.md
git commit -m "ci: enforce runtime and admin contract architecture"
```

Expected: both process binaries build, contract drift is empty, architecture gates pass, and CI runs the same script.

## Plan completion gate

```powershell
pwsh -NoProfile -File scripts/verify-runtime-contracts.ps1
pwsh -NoProfile -File scripts/verify-go-clean.ps1
go test ./internal/runtime ./internal/server/... -race -count=1
git status --short
```

Expected: clean status; both runtimes pass lifecycle/race tests; no partial resource graph can start; all routes are compiled into one policy registry; the deterministic Admin Contract Bundle is current and contains no retired operation.

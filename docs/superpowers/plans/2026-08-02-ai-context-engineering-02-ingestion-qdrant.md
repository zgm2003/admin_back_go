# AI 上下文工程摄取与 Qdrant Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 交付 Profile/Space/Document 的不可变摄取链、六种文本格式解析、结构化分块、Embedding、Qdrant Dense/Sparse 派生索引、重建和 source-aware readiness。

**Architecture:** MySQL Document Version 与 Chunk 始终是事实，Qdrant Point 只保存来源身份、过滤字段和向量。Worker 使用 Version lease fencing 与确定性任务键；新版本只有在 Chunk、向量和 Point 完整校验后才原子激活，失败永不覆盖旧活动版本。

**Tech Stack:** Go 1.26.5、Asynq、execution-tested Qdrant Server image、`github.com/qdrant/go-client v1.18.3`、Goldmark v1.8.5、ledongthuc/pdf、Excelize、标准库 CSV/ZIP/XML。

---

## Fixed Runtime Policy

```text
qdrant server candidate:       qdrant/qdrant:v1.18.3 (candidate only; not a lock)
qdrant Go client:              github.com/qdrant/go-client v1.18.3
qdrant host debug ports:       127.0.0.1:${ADMIN_QDRANT_HTTP_HOST_PORT:-36333}:6333
                               127.0.0.1:${ADMIN_QDRANT_GRPC_HOST_PORT:-36334}:6334
container address:             qdrant:6334
collection alias:              admin_context_profile_<profile_id>
physical collection:           admin_context_profile_<profile_id>_g<generation>
parser policy:                 context_parser_v1
chunker policy:                context_chunker_v1
structural chunk target:       min(800, profile embedding max input tokens)
oversize token overlap:        min(80, target/10)
source bytes:                  50 MiB
expanded archive bytes:        200 MiB
extracted text bytes:          100 MiB
PDF pages:                     1000
XLSX sheets/cells:             100 / 1,000,000
parser wall time:              2 minutes
```

These are versioned service policy values, not user fields. Changing parsing, chunking, Sparse normalization or Point identity requires a new policy/version and a new Context Profile or Version as specified below.

### Task 1: Prove a Qdrant candidate and pin the tested RepoDigest

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Create: `internal/infra/contextindex/types.go`
- Create: `internal/infra/contextindex/qdrant/client.go`
- Create: `internal/infra/contextindex/qdrant/contract_integration_test.go`
- Create: `scripts/context/verify-qdrant-candidate.ps1`
- Modify: `deploy/docker-state/docker-compose.yml`
- Create: `deploy/docker-state/qdrant-image.env`
- Modify: `scripts/docker-platform.ps1`
- Modify: `scripts/tests/docker-stability.tests.ps1`
- Modify: `internal/config/docker_compose_test.go`

- [ ] **Step 1: Add the neutral type scaffold and candidate capability test**

Create the shared index protocol types in `internal/infra/contextindex/types.go`; neither this parent package nor `qdrant` may import `contextengine`:

```go
type SourceKind string

const (
	SourceKindDocumentChunk    SourceKind = "document_chunk"
	SourceKindConversationTurn SourceKind = "conversation_turn"
)

type PointRef struct {
	ID              uuid.UUID
	ProfileID       uint64
	IndexGeneration uint64
	SourceKind      SourceKind
	SourceID        uint64
	SourceSHA256    [32]byte
}

type ConversationScope struct {
	UserID         uint64
	ConversationID uint64
}

type ScopeFilter struct {
	ProfileID       uint64
	IndexGeneration uint64
	Platform        string
	SpaceIDs        []uint64
	Conversation    *ConversationScope
}

type SparseVector struct {
	Indices []uint32
	Values  []float32
}
```

Then create an integration test guarded by `//go:build integration` that connects only to `QDRANT_INTEGRATION_ADDR`, creates a uniquely named disposable collection, and proves all required server behaviors. Its test helper constructs the already-pinned official client directly, so this RED step compiles before the runtime wrapper in Step 4 exists:

```go
func TestServerSupportsContextQueryContract(t *testing.T) {
	client := mustIntegrationClient(t)
	collection := uniqueCollection(t)
	createDenseSparseIDFCollection(t, client, collection, 4)
	upsertContractPoints(t, client, collection)

	result := queryBatchDenseSparseAndRRF(t, client, collection, contextindex.ScopeFilter{
		ProfileID: 7, IndexGeneration: 1, Platform: "admin", SpaceIDs: []uint64{11},
	})
	assertIndependentBranchRanks(t, result)
	assertOfficialRRFOrder(t, result)
	assertFilterExcludesOtherSpace(t, result)
}
```

The test must use the official QueryBatch RPC, one Dense branch, one Sparse branch with IDF modifier, and one official RRF Query with identical Prefetch branches. It must not implement RRF in Go to make an unsupported server pass. Cleanup deletes only the unique collection created by the test.

- [ ] **Step 2: Add the fixed dependencies required to compile the RED gate**

Run:

```powershell
go get github.com/qdrant/go-client@v1.18.3
go get github.com/google/uuid@v1.6.0
go get github.com/ledongthuc/pdf@v0.0.0-20250511090121-5959a4027728
go get github.com/yuin/goldmark@v1.8.5
go mod tidy
```

Expected: `go.mod` records those exact versions; Excelize remains at the repository's existing version. The test already exists, but dependencies are installed before executing it so RED reports the intended missing server rather than an unresolvable import.

- [ ] **Step 3: Run the test without a server and verify RED**

Run: `$env:QDRANT_INTEGRATION_ADDR='127.0.0.1:36334'; go test -tags=integration ./internal/infra/contextindex/qdrant -run ServerSupportsContextQueryContract -count=1`

Expected: FAIL with a connection error because dependencies compile and the candidate container is not running.

- [ ] **Step 4: Implement the provider-neutral index contract and runtime client**

Add constructors/validators for the Step 1 types: require positive Profile/generation, normalized platform, sorted unique positive Space IDs, paired positive Conversation scope, and at least one Space or Conversation branch. `SparseVector` requires equal lengths; both empty slices are the explicit “no lexical token” value, otherwise indices are strictly ascending/unique and values are finite/positive. Empty Sparse omits that vector/branch while Dense remains required; it is not converted to a fake token. `PointRef` must match the closed Source Kind and a UUIDv8 identity recomputed by Context Engine. Implement the Qdrant client against only these neutral contracts and the official client; collection/point methods never receive a repository, user object or domain `Candidate`.

- [ ] **Step 5: Implement the isolated candidate verifier and run it**

`verify-qdrant-candidate.ps1` must require the approved
`-CandidateImage qdrant/qdrant:v1.18.3`, use the fixed container name
`admin-context-qdrant-contract`, fixed loopback ports `36335/36336`, and a
`try/finally` that removes only that exact container. The script rejects every
other tag and digest-only input: `v1.18.3` is a candidate, while the tested
human-readable tag plus immutable digest is the release evidence. It emits a
closed JSON result so the proven reference can be consumed by the pinning step
without manual copy:

```powershell
param(
  [Parameter(Mandatory)]
  [ValidateSet('qdrant/qdrant:v1.18.3')]
  [string] $CandidateImage
)

$healthCommand = 'bash -c ''exec 3<>/dev/tcp/127.0.0.1/6333 && printf "GET /readyz HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n" >&3 && grep -q "200 OK" <&3'''
docker pull $CandidateImage
docker run --rm --entrypoint bash $CandidateImage --version
docker run --detach --name admin-context-qdrant-contract `
  --publish 127.0.0.1:36335:6333 --publish 127.0.0.1:36336:6334 `
  --health-cmd $healthCommand --health-interval 2s --health-timeout 2s `
  --health-retries 30 --health-start-period 5s $CandidateImage

$deadline = [DateTimeOffset]::UtcNow.AddSeconds(90)
do {
  $health = docker inspect admin-context-qdrant-contract --format '{{.State.Health.Status}}'
  if ($health -eq 'unhealthy') { throw 'candidate failed the exact Compose healthcheck' }
  if ($health -eq 'healthy') { break }
  Start-Sleep -Seconds 1
} while ([DateTimeOffset]::UtcNow -lt $deadline)
if ($health -ne 'healthy') { throw "candidate health timeout: $health" }

$env:QDRANT_INTEGRATION_ADDR = '127.0.0.1:36336'
go test -tags=integration ./internal/infra/contextindex/qdrant -run ServerSupportsContextQueryContract -count=1
$repoDigest = (docker image inspect $CandidateImage --format '{{index .RepoDigests 0}}').Trim()
if ($repoDigest -notmatch '^qdrant/qdrant@sha256:([0-9a-f]{64})$') { throw "invalid RepoDigest: $repoDigest" }
$testedImage = "$CandidateImage@sha256:$($Matches[1])"
[pscustomobject]@{ candidate_image = $CandidateImage; tested_image = $testedImage } |
  ConvertTo-Json -Compress
```

The actual script wraps all container work in `try/finally`, removes only `admin-context-qdrant-contract`, and fails if `bash` or the exact `/readyz` healthcheck is absent. Expected: container `healthy`, integration PASS, and one JSON object whose `tested_image` matches `^qdrant/qdrant:v1\.18\.3@sha256:[0-9a-f]{64}$`. Registry metadata alone is not acceptance evidence.

- [ ] **Step 6: Pin Compose to the proven digest**

After Step 5 succeeds, add service `qdrant`, volume `qdrant-data`, `platform` network, loopback-only HTTP/gRPC ports and the byte-identical readiness healthcheck exercised by the verifier:

```yaml
  qdrant:
    image: ${QDRANT_SERVER_IMAGE:?QDRANT_SERVER_IMAGE must be a tested tag@sha256 digest}
    restart: unless-stopped
    ports:
      - "127.0.0.1:${ADMIN_QDRANT_HTTP_HOST_PORT:-36333}:6333"
      - "127.0.0.1:${ADMIN_QDRANT_GRPC_HOST_PORT:-36334}:6334"
    volumes:
      - qdrant-data:/qdrant/storage
    networks:
      - platform
    healthcheck:
      test: ["CMD-SHELL", "bash -c 'exec 3<>/dev/tcp/127.0.0.1/6333 && printf \"GET /readyz HTTP/1.1\\r\\nHost: localhost\\r\\nConnection: close\\r\\n\\r\\n\" >&3 && grep -q \"200 OK\" <&3'"]
      interval: 5s
      timeout: 3s
      retries: 20
      start_period: 10s
```

Add a mandatory `-PinEnv deploy/docker-state/qdrant-image.env` argument to the
verifier. Only after container health and the real capability test pass, create
the absent lock atomically with one UTF-8 line
`QDRANT_SERVER_IMAGE=<tested_image>`. If the lock already exists, require its
entire byte content to equal that canonical line and leave it unchanged; a
different digest aborts instead of repinning a mutable tag. A capability
failure, malformed digest or existing-lock mismatch must leave the lock
byte-for-byte unchanged. Changing the approved candidate or digest requires an
explicit reviewed plan change, not a verifier flag. `scripts/docker-platform.ps1`
must pass this file with `docker compose --env-file` for every state-project
command; its tests prove `up`, `stop`, `status` and recovery paths cannot bypass
the lock.

Run: `pwsh -NoProfile -File scripts/context/verify-qdrant-candidate.ps1 -CandidateImage qdrant/qdrant:v1.18.3 -PinEnv deploy/docker-state/qdrant-image.env`

Expected: PASS; after loading `qdrant-image.env`, `docker compose config --images` returns the same literal `qdrant/qdrant:v1.18.3@sha256:digest` as `tested_image`. `internal/config/docker_compose_test.go` rejects a missing lock file, tag-only, all-zero and malformed values. The equal client/server version numbers are an approved candidate choice, not evidence of Server capability; only this real test authorizes the digest lock.

- [ ] **Step 7: Verify and commit the dependency boundary**

Run: `docker compose --env-file deploy/docker-state/qdrant-image.env -f deploy/docker-state/docker-compose.yml config --quiet`

Run: `go test ./internal/config ./internal/infra/contextindex/... -count=1`

Expected: PASS. Unit tests use a fake client; only the explicit integration command uses a real Qdrant server.

```bash
git add -- go.mod go.sum internal/infra/contextindex/types.go internal/infra/contextindex/qdrant/client.go internal/infra/contextindex/qdrant/contract_integration_test.go scripts/context/verify-qdrant-candidate.ps1 deploy/docker-state/docker-compose.yml deploy/docker-state/qdrant-image.env scripts/docker-platform.ps1 scripts/tests/docker-stability.tests.ps1 internal/config/docker_compose_test.go
git commit -m "feat(ai): add proven qdrant index dependency"
```

### Task 2: Add Qdrant configuration, resource lifecycle and source-aware readiness

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/config/runtime.go`
- Modify: `internal/config/runtime_test.go`
- Modify: `deploy/docker-first/admin-go.env.example`
- Modify: `deploy/docker-first/docker-compose.yml`
- Modify: `internal/runtime/resources.go`
- Modify: `internal/runtime/resources_test.go`
- Create: `internal/runtime/context_readiness.go`
- Create: `internal/runtime/context_readiness_test.go`
- Modify: `internal/runtime/api.go`
- Modify: `internal/runtime/worker.go`
- Modify: `internal/readiness/readiness.go`
- Modify: `internal/readiness/readiness_test.go`

- [ ] **Step 1: Write failing config and readiness tests**

Cover four non-secret environment keys and conditional API readiness:

```go
func TestAPIReadinessRequiresQdrantOnlyWhenContextSourcesExist(t *testing.T) {
	checker := NewContextReadiness(fakeIndex{err: errors.New("down")}, fakeSources{active: false})
	if got := checker.Check(t.Context()); got.Status != readiness.StatusDegraded {
		t.Fatalf("pure chat should expose degraded qdrant, got %#v", got)
	}
	checker = NewContextReadiness(fakeIndex{err: errors.New("down")}, fakeSources{active: true})
	if got := checker.Check(t.Context()); got.Status != readiness.StatusDown {
		t.Fatalf("active sources require qdrant, got %#v", got)
	}
}
```

Add `readiness.NewReport` tests proving `degraded` remains overall `ready`, while any `down` component makes the report `not_ready`. At this checkpoint Worker readiness covers MySQL, Redis/Asynq and Qdrant capability only; no Context handler exists yet, so this Task must not assert future task registration. Add a leak test proving readiness messages do not contain Qdrant API keys or addresses with credentials.

- [ ] **Step 2: Run focused tests and verify RED**

Run: `go test ./internal/config ./internal/readiness ./internal/runtime -run 'Qdrant|ContextReadiness|ReadinessReport|Worker' -count=1`

Expected: FAIL because config/resources/checker do not exist.

- [ ] **Step 3: Add typed config and environment contract**

Define:

```go
type QdrantConfig struct {
	Addr             string
	CollectionPrefix string
	TLS              bool
	APIKey           string
}
```

Load only `QDRANT_ADDR`, `QDRANT_COLLECTION_PREFIX`, `QDRANT_TLS`, `QDRANT_API_KEY`. Validate host:port, prefix syntax and production TLS policy; never log `APIKey`. Add to `admin-go.env.example`:

```text
QDRANT_ADDR=qdrant:6334
QDRANT_COLLECTION_PREFIX=admin_context
QDRANT_TLS=false
QDRANT_API_KEY=
```

Do not add credentials to Compose. Both API and Worker containers already share `admin-platform`; keep their topology unchanged.

- [ ] **Step 4: Add resource and semantic readiness**

Add `StatusDegraded = "degraded"` to `internal/readiness`; `NewReport` treats it as visible non-blocking degradation and continues to treat only `down` as `not_ready`. Open one Qdrant client in runtime resources and close it during shutdown. `ContextReadiness` checks server version, collection schema, Dense/Sparse IDF, QueryBatch/RRF capability, every MySQL-referenced active physical collection and alias/generation agreement. It performs no collection writes.

API result rules:

```text
no enabled ready Space Document Version:
  Qdrant failure -> degraded component, overall pure-chat API remains ready
at least one enabled ready Space Document Version:
  Qdrant/schema/generation failure -> overall not_ready
unused failed Profile:
  management diagnostic only
```

Worker readiness always requires Qdrant. Task 6 extends it to the first implemented Context handler, `document-index`; Task 7 extends it to exactly three Plan 02 handlers. Plan 04 expands source-aware API readiness to ready Conversation Documents and indexable complete Turns, and expands Worker readiness to five handlers only after those handlers exist.

- [ ] **Step 5: Run tests and commit**

Run: `go test ./internal/config ./internal/readiness ./internal/runtime -run 'Qdrant|ContextReadiness|ReadinessReport|Worker' -count=1`

Expected: PASS.

```bash
git add -- internal/config deploy/docker-first/admin-go.env.example deploy/docker-first/docker-compose.yml internal/readiness internal/runtime/resources.go internal/runtime/resources_test.go internal/runtime/context_readiness.go internal/runtime/context_readiness_test.go internal/runtime/api.go internal/runtime/worker.go
git commit -m "feat(ai): wire qdrant readiness"
```

### Task 3: Parse supported documents into bounded structural blocks

**Files:**
- Create: `internal/infra/documentparser/types.go`
- Create: `internal/infra/documentparser/registry.go`
- Create: `internal/infra/documentparser/limits.go`
- Create: `internal/infra/documentparser/txt.go`
- Create: `internal/infra/documentparser/markdown.go`
- Create: `internal/infra/documentparser/pdf.go`
- Create: `internal/infra/documentparser/docx.go`
- Create: `internal/infra/documentparser/csv.go`
- Create: `internal/infra/documentparser/xlsx.go`
- Create: `internal/infra/documentparser/parser_test.go`
- Create: `internal/infra/documentparser/testdata/txt_utf8.txt`
- Create: `internal/infra/documentparser/testdata/txt_utf16le.txt`
- Create: `internal/infra/documentparser/testdata/markdown_structured.md`
- Create: `internal/infra/documentparser/testdata/pdf_text_layer.pdf`
- Create: `internal/infra/documentparser/testdata/pdf_scanned.pdf`
- Create: `internal/infra/documentparser/testdata/pdf_encrypted.pdf`
- Create: `internal/infra/documentparser/testdata/docx_structured.docx`
- Create: `internal/infra/documentparser/testdata/docx_macro.docx`
- Create: `internal/infra/documentparser/testdata/docx_external_relationship.docx`
- Create: `internal/infra/documentparser/testdata/docx_expansion_limit.docx`
- Create: `internal/infra/documentparser/testdata/csv_rows.csv`
- Create: `internal/infra/documentparser/testdata/csv_invalid.csv`
- Create: `internal/infra/documentparser/testdata/xlsx_sheets.xlsx`
- Create: `internal/infra/documentparser/testdata/xlsx_corrupt.xlsx`
- Create: `internal/infra/documentparser/testdata/binary_disguised_as_text.txt`

- [ ] **Step 1: Add six-format golden tests and hostile-input tests**

Use small committed fixtures with known content and locations. The shared result is:

```go
type Block struct {
	Ordinal     uint32
	Text        string
	HeadingPath []string
	Locator     ContextLocatorV1
}

type Parser interface {
	Name() string
	Version() string
	Parse(context.Context, Source, Limits) ([]Block, error)
}
```

Golden tests assert exact text, heading path and locator for TXT lines, Markdown block ordinal, PDF page, DOCX paragraph, CSV row range and XLSX sheet/cell range. Negative fixtures cover invalid encoding, binary-as-text, scanned/encrypted PDF, macro/external DOCX, ZIP expansion ratio, corrupt CSV/XLSX, page/sheet/cell/text limits and deadline cancellation.

- [ ] **Step 2: Run parser tests and verify RED**

Run: `go test ./internal/infra/documentparser -count=1`

Expected: FAIL because the parser package is absent.

- [ ] **Step 3: Implement one registry and bounded readers**

Registry selection uses normalized MIME plus extension and rejects mismatches that indicate binary disguise. TXT accepts UTF-8 and BOM-declared UTF-16LE/BE only. Markdown uses Goldmark AST and preserves headings, paragraphs, lists, tables and code blocks. PDF reads text layer page by page; empty text layer returns `ai.context.document_parse_failed` with an unsupported-scanned-PDF cause. DOCX reads only bounded `word/document.xml` from ZIP, rejects macros and external relationships. CSV uses `encoding/csv`; XLSX uses existing Excelize in read-only streaming mode.

Every parser checks `context.Context`, `Limits`, expanded bytes and extracted text while reading; no implementation uses `io.ReadAll` on an unbounded stream.

- [ ] **Step 4: Run tests and commit**

Run: `go test ./internal/infra/documentparser -count=1`

Expected: PASS with deterministic golden output on repeated runs.

```bash
git add -- internal/infra/documentparser
git commit -m "feat(ai): add bounded context document parsers"
```

### Task 4: Add structural Chunking, shared Sparse encoding, Embedding and Point identity

**Files:**
- Create: `internal/module/ai/contextengine/chunker.go`
- Create: `internal/module/ai/contextengine/chunker_test.go`
- Create: `internal/module/ai/contextengine/point.go`
- Create: `internal/module/ai/contextengine/point_test.go`
- Create: `internal/module/ai/contextengine/sparse.go`
- Create: `internal/module/ai/contextengine/sparse_test.go`
- Create: `internal/infra/ai/embedding.go`
- Create: `internal/infra/ai/openaicompat/embedding.go`
- Create: `internal/infra/ai/openaicompat/embedding_test.go`
- Modify: `internal/runtime/providers.go`
- Modify: `internal/runtime/providers_test.go`

- [ ] **Step 1: Write failing structure/hash/capability tests**

Test that structural blocks stay whole below the limit, oversized blocks split only on token windows, overlap follows the fixed policy, repeated input yields identical ordinals/hashes, and a retry with different output at the same ordinal is rejected.

Point identity test uses the exact preimage and UUIDv8 bits:

```go
func TestPointIDIsStableUUIDv8(t *testing.T) {
	id := PointID(7, contextindex.SourceKindDocumentChunk, 91, sha256.Sum256([]byte("facts")))
	if id.Version() != 8 || id.Variant() != uuid.RFC4122 { t.Fatalf("id=%s", id) }
	if id != PointID(7, contextindex.SourceKindDocumentChunk, 91, sha256.Sum256([]byte("facts"))) {
		t.Fatal("same source must yield same point")
	}
}
```

Sparse golden tests lock `unicode_lexical_v1`:

```go
func TestUnicodeLexicalV1Golden(t *testing.T) {
	got := EncodeSparse("Go语言 GO 语言123")
	want := contextindex.SparseVector{
		Indices: []uint32{701548806, 1708916009, 2415828576, 2669990252, 4154103862},
		Values:  []float32{1.6931472, 1.6931472, 1.6931472, 1.6931472, 1},
	}
	if diff := cmp.Diff(want, got); diff != "" { t.Fatal(diff) }
}
```

Add a same-package constructive test that feeds two different tokens through an injected test indexer returning the same `uint32` and proves their weights are summed into one coordinate. Embedding adapter tests validate response count, dimensions, finite numbers, model identity, usage and declared max inputs/tokens.

- [ ] **Step 2: Run focused tests and verify RED**

Run: `go test ./internal/module/ai/contextengine ./internal/infra/ai ./internal/infra/ai/openaicompat ./internal/runtime -run 'Chunk|PointID|UnicodeLexical|Embedding' -count=1`

Expected: FAIL because Chunker, Point identity, Sparse encoder and Embedding interfaces are absent.

- [ ] **Step 3: Implement the fixed algorithms**

Canonical Index Text is heading path joined with ` > `, one newline, then content. `content_sha256` hashes content only; `chunk_facts_sha256` hashes `context_chunk_facts_v1`, heading path, Content Hash and canonical locator JSON. Chunker uses the Profile's immutable embedding counter and rejects a single token unit that cannot fit.

Implement `unicode_lexical_v1` once in `contextengine/sparse.go` and return `contextindex.SparseVector`: Unicode NFKC then case folding; Latin letters/digits form continuous tokens; each Han sequence emits unigrams and adjacent bigrams; punctuation/space is a boundary; weight is `1 + ln(term_frequency)`. Sparse Index is the first four bytes, big-endian, of `sha256("unicode-lexical-v1\0" + token)`. Aggregate repeated/colliding indices, reject non-finite weights and sort unique indices ascending. Both document ingestion in Task 6 and query retrieval in Plan 03 must call this exported encoder; no adapter-local tokenizer is permitted.

Point ID preimage is exactly:

```text
admin-context-point-v1\0<profile_id>\0<source_kind>\0<source_id>\0<lowercase source sha256>
```

Take the first 16 SHA-256 bytes and set RFC 9562 UUIDv8 version/variant bits. Payload contains only the closed fields in design section 9.1 and never content, filename, query, URL or credentials.

Define provider-neutral `EmbeddingClient`/`EmbeddingFactory`; OpenAI-compatible implementation calls `/embeddings`. Resolver requires `model_kind=embedding` and explicit capabilities. Unsupported engine types return `ai.context.embedding_failed`; they do not select another Provider/Model.

- [ ] **Step 4: Run tests and commit**

Run: `go test ./internal/module/ai/contextengine ./internal/infra/ai ./internal/infra/ai/openaicompat ./internal/runtime -run 'Chunk|PointID|UnicodeLexical|Embedding' -count=1`

Expected: PASS.

```bash
git add -- internal/module/ai/contextengine/chunker.go internal/module/ai/contextengine/chunker_test.go internal/module/ai/contextengine/point.go internal/module/ai/contextengine/point_test.go internal/module/ai/contextengine/sparse.go internal/module/ai/contextengine/sparse_test.go internal/infra/ai/embedding.go internal/infra/ai/openaicompat/embedding.go internal/infra/ai/openaicompat/embedding_test.go internal/runtime/providers.go internal/runtime/providers_test.go
git commit -m "feat(ai): add context chunks and vectors"
```

### Task 5: Implement Profile, Space and Document admin capabilities

**Files:**
- Modify: `internal/module/ai/contextengine/repository.go`
- Modify: `internal/module/ai/contextengine/repository_test.go`
- Modify: `internal/shared/enum/upload.go`
- Modify: `internal/shared/enum/upload_test.go`
- Create: `internal/infra/storage/conditional_object.go`
- Create: `internal/infra/storage/conditional_object_test.go`
- Create: `internal/infra/storage/cos/conditional_object_reader.go`
- Create: `internal/infra/storage/cos/conditional_object_reader_test.go`
- Modify: `internal/infra/storage/cos/object_stream.go`
- Modify: `internal/infra/storage/cos/object_stream_test.go`
- Create: `internal/module/ai/contextengine/admin_dto.go`
- Create: `internal/module/ai/contextengine/admin_service.go`
- Create: `internal/module/ai/contextengine/admin_service_test.go`
- Create: `internal/module/ai/contextengine/transport/admin/request.go`
- Create: `internal/module/ai/contextengine/transport/admin/handler.go`
- Create: `internal/module/ai/contextengine/transport/admin/handler_test.go`
- Create: `internal/module/ai/contextengine/transport/admin/route.go`
- Modify: `internal/server/router_test.go`
- Modify: `internal/platform/admin/build.go`
- Modify: `internal/platform/admin/build_test.go`

- [ ] **Step 1: Write failing state and authorization tests**

Test Profile creation against actual Provider Model kind/capability, immutable policy after first reference, retired Profile rules, Space profile-change restriction, soft-delete visibility, conversation-vs-space document XOR, attachment identity, and reindex creating a new Version. Handler tests assert platform comes from the trusted Admin route group and cannot be overridden by body/query/header. Add a router negative test in `internal/server` proving none of the new Context mutation paths is installed before the permission migration in Plan 05.

- [ ] **Step 2: Run capability tests and verify RED**

Run: `go test ./internal/module/ai/contextengine/... ./internal/shared/enum ./internal/infra/storage/... ./internal/server ./internal/platform/admin -run 'Profile|Space|Document|ContextRoute|UploadFolder|ConditionalObject' -count=1`

Expected: FAIL because admin service and routes do not exist.

- [ ] **Step 3: Implement typed services and narrow repositories**

Profile create validates embedding model kind, enabled channel, dimensions, max input, counter and distance; Reranker ID/threshold are paired; Memory model is Chat kind with trusted window/output/counter. Profile update exposes name and retirement only; generation/state changes use dedicated CAS methods.

Add `ai_context_documents` to the closed `enum.UploadFolders` list and its validator/contract tests. The existing `/api/admin/v1/upload-tokens` capability remains the only browser upload-token endpoint; it issues a key below that folder and still does not own a Document business row.

Introduce a business-neutral storage contract, independent of `infraai`:

```go
type ConditionalObjectInput struct {
	StorageProvider string
	ObjectKey       string
	ETag            string
	Size            int64
}

type ConditionalObjectMetadata struct {
	ETag     string
	Size     int64
	MIMEType string
}

type ConditionalObjectReader interface {
	Head(context.Context, ConditionalObjectInput) (ConditionalObjectMetadata, error)
	Open(context.Context, ConditionalObjectInput) (io.ReadCloser, ConditionalObjectMetadata, error)
}
```

The COS implementation reuses `uploadtoken.ObjectConfigProvider`, sends `If-Match` for both HEAD and GET, checks ETag/size on both responses, and maps 404/412 to stable unavailable/version-changed errors. Keep the current Chat behavior through a thin `infraai.PreparedFileOpener` adapter that additionally applies `TrustedAIChatObjectKey`; do not make Context import `infraai.PreparedFileOpenInput` and do not weaken current chat-prefix checks.

Space create/edit/delete enforces platform and Profile. Document create accepts the closed browser object reference, requires the `ai_context_documents/` prefix, conditionally HEADs it through `ConditionalObjectReader`, and persists the verified provider/key/ETag/size/MIME/filename facts. It writes Document plus `queued` Version in one transaction and returns the committed resource even if post-commit enqueue fails. Reindex creates a new immutable Version using current parser/chunker versions; it never rewinds an old Version.

Implement closed admin DTOs, service methods, handlers and `RegisterRoutes`, and test that function on an isolated Gin engine. Do **not** call `RegisterRoutes` from `internal/server/routes_admin_ai.go` in this Slice. Plan 05 installs all Context routes in the same change that adds `ai_context_*` route metadata and permission rows; generic Admin authentication alone is not sufficient authorization for these mutations.

- [ ] **Step 4: Wire the Agent profile resolver from Plan 01**

Implement `RequireAssignable` and `RequireAgentProfileChangeAllowed` using Context repositories. Profile initial assignment accepts only enabled/ready and enqueues bounded historical backfill after commit; clearing/changing obeys the fixed conflict conditions. The Agent transaction never infers a Profile from Space bindings.

- [ ] **Step 5: Run tests and commit**

Run: `go test ./internal/module/ai/contextengine/... ./internal/module/ai/agent ./internal/shared/enum ./internal/infra/storage/... ./internal/server ./internal/platform/admin -run 'Profile|Space|Document|ContextRoute|UploadFolder|ConditionalObject' -count=1`

Expected: PASS.

```bash
git add -- internal/module/ai/contextengine internal/module/ai/agent internal/shared/enum/upload.go internal/shared/enum/upload_test.go internal/infra/storage/conditional_object.go internal/infra/storage/conditional_object_test.go internal/infra/storage/cos/conditional_object_reader.go internal/infra/storage/cos/conditional_object_reader_test.go internal/infra/storage/cos/object_stream.go internal/infra/storage/cos/object_stream_test.go internal/server/router_test.go internal/platform/admin/build.go internal/platform/admin/build_test.go
git commit -m "feat(ai): add context administration services"
```

### Task 6: Build the fenced document-index Worker

**Files:**
- Create: `internal/jobs/ai_context.go`
- Create: `internal/jobs/ai_context_test.go`
- Create: `internal/module/ai/contextengine/ingestion.go`
- Create: `internal/module/ai/contextengine/ingestion_test.go`
- Create: `internal/module/ai/contextengine/jobs.go`
- Create: `internal/module/ai/contextengine/jobs_test.go`
- Create: `internal/module/ai/contextengine/reconciler.go`
- Create: `internal/module/ai/contextengine/reconciler_test.go`
- Modify: `internal/runtime/worker.go`
- Modify: `internal/runtime/worker_test.go`
- Modify: `internal/infra/taskqueue/registry.go`
- Modify: `internal/infra/taskqueue/registry_test.go`

- [ ] **Step 1: Write crash-window and lease tests**

Cover crashes after MySQL Chunk write, after Qdrant Upsert and before activation; duplicate delivery; expired/stale lease; transient retry; permanent parse/dimension/resource error; two Versions finishing out of order; and a failed newest Version preserving the prior active Version.

```go
func TestActivationRejectsOlderReadyVersion(t *testing.T) {
	repository := newIngestionRepositoryFixture(t)
	activateReadyVersion(t, repository, 12)
	activateReadyVersion(t, repository, 11)
	document := mustLoadDocument(t, repository, 7)
	if document.ActiveVersionID == nil || *document.ActiveVersionID != 12 {
		t.Fatalf("active_version_id=%v, want 12", document.ActiveVersionID)
	}
}
```

Add explicit registry tests for a retryable failure at `Retry < MaxRetry`, a retryable failure at `Retry == MaxRetry`, a permanent error, and a crash during exhausted finalization. The first stays `processing`; the latter three eventually CAS the Version to `failed`, while the crash case is completed by Reconciler without another external parse/Embedding call.

- [ ] **Step 2: Run Worker tests and verify RED**

Run: `go test ./internal/jobs ./internal/infra/taskqueue ./internal/module/ai/contextengine ./internal/runtime -run 'DocumentIndex|VersionLease|Activation|Reconcile|ContextTaskRegistration|WorkerReadiness' -count=1`

Expected: FAIL because task DTO, state machine and handler are absent.

- [ ] **Step 3: Define the versioned task and exact idempotency key**

```go
const TaskContextDocumentIndexV1 = "ai:context-document-index:v1"

type ContextDocumentIndexV1 struct {
	DocumentVersionID uint64 `json:"document_version_id"`
}
```

The Redis payload carries no Profile, source hash, parser or chunker facts. Producer and Handler load the immutable Version plus Profile from MySQL and compute the full application idempotency key exactly as `task version + version_id + profile_id + source_facts_sha256 + parser_version + chunker_version`. Duplicate delivery is harmless because the Version lease and immutable Chunk/Point identities enforce that key; Redis is never a second source of business truth.

Register fixed low queue, timeout and max retry policy. Extend `taskqueue.Definition` with an optional exhausted-attempt finalizer:

```go
type ExhaustedFinalizer func(context.Context, any, *apperror.Error) *apperror.Error

type Definition struct {
	// existing fields remain unchanged
	FinalizeExhausted ExhaustedFinalizer
}
```

`Registry.Handle` calls `FinalizeExhausted` only when a decoded handler error is retryable and the `taskqueue.Task` supplied by Mux has `Retry >= MaxRetry`; successful finalization joins `asynq.SkipRetry` so no hidden extra attempt occurs. Existing definitions without the hook keep current behavior. Permanent Context failures CAS Version to `failed` inside the business handler before returning `asynq.SkipRetry`. The exhausted hook locks Version and CASes the same lease/attempt to `failed` with a sanitized stable code. Reconciler also finalizes an expired `processing` Version whose persisted `attempt_count` has reached the registered bound, covering a crash between the last adapter failure and the hook commit; it must not issue another external call.

- [ ] **Step 4: Implement the eight-stage ingestion pipeline**

Follow design section 10.4 exactly: load immutable Version/Profile facts, conditional object read through `storage.ConditionalObjectReader` and stream the source hash, parse, chunk/bounds, immutable MySQL Chunk Upsert, batched Dense plus Sparse vectors, deterministic Qdrant Upsert, full count/hash/dimension verification, then one fenced activation transaction. Every stage verifies the current lease token. Same ordinal with different immutable facts is a deterministic failure.

The Reconciler scans `queued` rows without a valid lease and expired `processing` rows in stable ID batches and re-enqueues the same task identity. It stores no cursor table; every pass is restartable.

Register only `TaskContextDocumentIndexV1` in the Worker at this checkpoint. Extend Worker readiness to require exactly that one delivered Context task in addition to its existing non-Context handlers; cleanup and rebuild are not required until Task 7 creates them.

- [ ] **Step 5: Run tests and commit**

Run: `go test ./internal/jobs ./internal/infra/taskqueue ./internal/module/ai/contextengine ./internal/runtime -run 'DocumentIndex|VersionLease|Activation|Reconcile|ContextTaskRegistration|WorkerReadiness' -count=1`

Expected: PASS.

```bash
git add -- internal/jobs/ai_context.go internal/jobs/ai_context_test.go internal/module/ai/contextengine/ingestion.go internal/module/ai/contextengine/ingestion_test.go internal/module/ai/contextengine/jobs.go internal/module/ai/contextengine/jobs_test.go internal/module/ai/contextengine/reconciler.go internal/module/ai/contextengine/reconciler_test.go internal/runtime/worker.go internal/runtime/worker_test.go internal/infra/taskqueue/registry.go internal/infra/taskqueue/registry_test.go
git commit -m "feat(ai): index context documents durably"
```

### Task 7: Implement generation-fenced rebuild, cleanup and consistency repair

**Files:**
- Create: `internal/module/ai/contextengine/rebuild.go`
- Create: `internal/module/ai/contextengine/rebuild_test.go`
- Create: `internal/module/ai/contextengine/cleanup.go`
- Create: `internal/module/ai/contextengine/cleanup_test.go`
- Modify: `internal/jobs/ai_context.go`
- Modify: `internal/jobs/ai_context_test.go`
- Modify: `internal/module/ai/contextengine/jobs.go`
- Modify: `internal/module/ai/contextengine/jobs_test.go`
- Modify: `internal/module/ai/contextengine/reconciler.go`
- Modify: `internal/module/ai/contextengine/reconciler_test.go`
- Modify: `internal/runtime/context_readiness.go`
- Modify: `internal/runtime/context_readiness_test.go`
- Modify: `internal/runtime/worker.go`
- Modify: `internal/runtime/worker_test.go`

- [ ] **Step 1: Write failing generation and cleanup-window tests**

Test all legal CAS paths, Alias switched/MySQL old, MySQL new/Alias old, healthy rebuild failure, missing active collection, Document Point missing, retirement grace period and cleanup visibility checks. Document Point inconsistency fails the Profile. Conversation Point repair is deliberately absent until Plan 04 creates the canonical Turn and Conversation Index task.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/jobs ./internal/module/ai/contextengine ./internal/runtime -run 'Rebuild|Generation|Alias|Cleanup|Consistency|ContextTaskRegistration|WorkerReadiness' -count=1`

Expected: FAIL because generation/rebuild handlers are absent.

- [ ] **Step 3: Implement the two task contracts**

```go
const (
	TaskContextProfileRebuildV1 = "ai:context-profile-rebuild:v1"
	TaskContextIndexCleanupV1   = "ai:context-index-cleanup:v1"
)

type CleanupKind string
const (
	CleanupDocumentVersionPoints CleanupKind = "document_version_points"
	CleanupRetiredCollection     CleanupKind = "retired_collection"
)
```

Cleanup DTO is one closed union with Profile, source identity and generation fence. At this checkpoint its legal kinds are Document Version Points and retired collections. Plan 04 extends the same union with Conversation Points only after that source exists. Handler rechecks MySQL visibility, Active/Target/Alias and grace deadline before deletion. It does not duplicate state machines.

- [ ] **Step 4: Implement rebuild order and runtime truth**

Acquire Profile fencing, wait bounded active writers, set Target, build/validate new physical collection, atomically switch Alias, then CAS MySQL Target to Active. Runtime reads the physical collection named from the MySQL generation snapshot, never Alias. Retain old collection beyond maximum BuildPlan plus adapter retry duration; cleanup then rechecks all three pointers.

Healthy rebuild failure deletes Target and returns to `ready` with old Active. A missing/corrupt Active enters `failed` until a new generation validates. Reconciler repairs Alias/MySQL mismatch and exposes readiness failure while inconsistent.

Register `TaskContextIndexCleanupV1` and `TaskContextProfileRebuildV1`, then extend Worker readiness to require exactly the three Context handlers delivered by Plan 02: `document-index`, `index-cleanup`, and `profile-rebuild`.

- [ ] **Step 5: Run tests and commit**

Run: `go test ./internal/jobs ./internal/module/ai/contextengine ./internal/runtime -run 'Rebuild|Generation|Alias|Cleanup|Consistency|ContextTaskRegistration|WorkerReadiness' -count=1`

Expected: PASS.

```bash
git add -- internal/jobs/ai_context.go internal/jobs/ai_context_test.go internal/module/ai/contextengine/rebuild.go internal/module/ai/contextengine/rebuild_test.go internal/module/ai/contextengine/cleanup.go internal/module/ai/contextengine/cleanup_test.go internal/module/ai/contextengine/jobs.go internal/module/ai/contextengine/jobs_test.go internal/module/ai/contextengine/reconciler.go internal/module/ai/contextengine/reconciler_test.go internal/runtime/context_readiness.go internal/runtime/context_readiness_test.go internal/runtime/worker.go internal/runtime/worker_test.go
git commit -m "feat(ai): rebuild context indexes safely"
```

### Task 8: Verify ingestion and indexing without touching the user's services

**Files:**
- Modify: `docs/architecture.md`
- Create: `docs/runbooks/ai-context-index-rebuild.md`

- [ ] **Step 1: Document operational truth**

Document MySQL-vs-Qdrant ownership, deterministic rebuild, source-aware API readiness, strict Worker readiness, candidate digest evidence, profile generation states, cleanup grace and recovery commands. State that deleting Qdrant data is recoverable but does not authorize deleting MySQL/object storage.

- [ ] **Step 2: Run focused unit gates**

Run: `gofmt -w internal/infra/contextindex internal/infra/documentparser internal/infra/storage internal/infra/ai internal/infra/taskqueue internal/module/ai/contextengine internal/module/ai/agent internal/shared/enum internal/jobs internal/config internal/readiness internal/runtime internal/server internal/platform/admin`

Run: `go test ./internal/infra/contextindex/... ./internal/infra/documentparser ./internal/infra/storage/... ./internal/infra/ai/... ./internal/infra/taskqueue ./internal/module/ai/contextengine/... ./internal/module/ai/agent ./internal/shared/enum ./internal/jobs ./internal/config ./internal/readiness ./internal/runtime ./internal/server ./internal/platform/admin -count=1`

Expected: PASS.

- [ ] **Step 3: Run the explicit real-Qdrant gate**

Run: `pwsh -NoProfile -File scripts/context/verify-qdrant-candidate.ps1 -CandidateImage qdrant/qdrant:v1.18.3 -PinEnv deploy/docker-state/qdrant-image.env`

Expected: PASS for Sparse IDF, QueryBatch, official RRF and Filter, then print the same RepoDigest pinned in Compose. The script accepts only the approved `qdrant/qdrant:v1.18.3` candidate, requires the existing pin to remain byte-identical, and removes only its own disposable container.

- [ ] **Step 4: Run static safety checks**

Run: `rg -n 'content|filename|query|url|api_key' internal/infra/contextindex/qdrant --glob '*.go'`

Expected: matches are limited to explicit rejection tests and config field handling; Point payload serialization contains none of those business content fields.

Run: `git diff --check`

Expected: no whitespace errors.

- [ ] **Step 5: Commit docs and record checkpoint**

```bash
git add -- docs/architecture.md docs/runbooks/ai-context-index-rebuild.md
git commit -m "docs(ai): add context index operations"
```

Run: `git status --short`

Expected: clean. Do not run Compose against the user's state project, do not restart `admin-dev`, and do not apply `202608020101` to a live database.

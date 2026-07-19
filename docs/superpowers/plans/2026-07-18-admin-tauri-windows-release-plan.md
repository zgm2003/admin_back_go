# [CANCELLED] Admin Tauri Windows Candidate Release Implementation Plan

> **Execution status:** CANCELLED by the approved `../specs/2026-07-19-admin-browser-only-tauri-retirement-design.md`. Do not execute any task or checkbox in this file.
>
> This document is retained only as an audit record of the abandoned P08.5 proposal. No Tauri tag, GitHub Workflow, Windows runner, signing input, NSIS artifact, COS candidate, candidate-import API/UI, updater manifest, or promotion mechanism may be created. The active replacement is `2026-07-19-admin-browser-only-tauri-retirement-plan.md`.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Use subagents only when the user explicitly requests delegation. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn a reviewed `tauri-vX.Y.Z` tag into a signed Windows NSIS candidate in COS, import that candidate through the documented Admin contract, and leave latest/force-update promotion under explicit user control.

**Architecture:** The frontend repository owns deterministic version synchronization, Windows signing, candidate manifest generation, and immutable COS upload. The backend owns candidate discovery, validation, import, database state, and publication of the live updater manifest. GitHub Actions never receives an Admin token or database credential, and a tag never promotes itself to latest.

**Tech Stack:** Windows Server 2025 GitHub runner, Node 22.23.1, Tauri 2, Rust 1.97.0, NSIS, PowerShell 7, `cos-nodejs-sdk-v5` 3.0.0, Go 1.26.5, Tencent COS.

---

## Fixed execution policy

- Execute backend and frontend tasks serially in the two existing `master` checkouts; never create a Git worktree or `.worktrees` directory.
- Web/API/worker/state runtime remains Docker-only. The Windows runner performs native compilation only and never deploys Web or backend services.
- `.github/workflows/release-tauri.yml` is the only allowed GitHub Workflow. It builds/signs/uploads a Tauri candidate and cannot deploy Web, API, worker, or database.
- The Workflow triggers only from `tauri-v*` tags and validates the exact `tauri-vX.Y.Z` grammar itself.
- Playwright is absent. Use it only if the user explicitly requests it in a later task.
- COS is the only distribution origin. GitHub Releases are not created.
- Promotion to latest and force-update selection remain manual Admin actions.

## Candidate object contract

Each tag writes immutable objects below:

```text
tauri_candidates/windows-x86_64/<version>/CloudAdmin_<version>_x64-setup.exe
tauri_candidates/windows-x86_64/<version>/CloudAdmin_<version>_x64-setup.nsis.zip
tauri_candidates/windows-x86_64/<version>/CloudAdmin_<version>_x64-setup.nsis.zip.sig
tauri_candidates/windows-x86_64/<version>/candidate.json
```

`candidate.json` is strict JSON:

```json
{
  "schema_version": 1,
  "version": "1.0.8",
  "platform": "windows-x86_64",
  "commit": "0123456789abcdef0123456789abcdef01234567",
  "tag": "tauri-v1.0.8",
  "installer_object_key": "tauri_candidates/windows-x86_64/1.0.8/CloudAdmin_1.0.8_x64-setup.exe",
  "installer_url": "https://cos.zgm2003.cn/tauri_candidates/windows-x86_64/1.0.8/CloudAdmin_1.0.8_x64-setup.exe",
  "installer_sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "installer_file_size": 12345678,
  "updater_object_key": "tauri_candidates/windows-x86_64/1.0.8/CloudAdmin_1.0.8_x64-setup.nsis.zip",
  "updater_url": "https://cos.zgm2003.cn/tauri_candidates/windows-x86_64/1.0.8/CloudAdmin_1.0.8_x64-setup.nsis.zip",
  "updater_signature": "tauri-signature-content",
  "updater_sha256": "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
  "updater_file_size": 12000000,
  "created_at": "2026-07-18T12:00:00Z"
}
```

Unknown fields, non-SemVer versions, non-Windows platforms, non-lowercase hashes, tag/version mismatch, paths outside the version prefix, wrong `.exe`/`.nsis.zip` suffixes, non-HTTPS URLs, and zero/negative sizes are contract errors. The existing `client_versions.file_url`, `signature`, and `file_size` fields store the updater bundle values; the raw installer remains an immutable candidate download and is not substituted for the updater bundle.

### Task 1: Publish the candidate and Admin API contracts before runtime code

**Backend files:**
- Modify: `docs/architecture.md`

- [ ] **Step 1: Document the exact candidate and API contracts**

Declare these exact Admin operations:

```text
GET  /api/admin/v1/client-versions/candidates?platform=windows-x86_64
POST /api/admin/v1/client-versions/import-candidate
```

The GET response data is `{ "list": Candidate[] }` using only the candidate fields above. The POST request is exactly:

```json
{"version":"1.0.8","platform":"windows-x86_64"}
```

The POST response data is exactly `{ "id": 8 }`. GET requires authenticated Admin access and is read-only/no-audit. POST reuses `system_clientVersion_add` and records an operation audit action `import_candidate`.

Add the candidate JSON schema, API request/response, permissions, empty response (`{"list":[]}`), conflict behavior (`clientversion.candidate_already_imported`), missing object, invalid manifest, COS unavailable, and audit behavior to `docs/architecture.md`. Document that import always creates `is_latest=NO` and `force_update=NO`; no endpoint auto-promotes.

- [ ] **Step 2: Review the document against the fixed contract**

Use a review fixture where version, tag, commit, object key, URL, signature, hash, size, and timestamp are all distinct values. Confirm every field, method, path, query, body, response, empty state, and error is defined exactly once and no alias or fallback is documented.

- [ ] **Step 3: Verify and commit the contract document**

```powershell
go test ./internal/architecture -count=1
git add -- docs/architecture.md
git diff --cached --check
git commit -m "docs(client-version): define Tauri candidate contract"
```

### Task 2: Make repository versions deterministic

**Frontend files:**
- Create: `scripts/tauri/set-version.mjs`
- Create: `scripts/tauri/check-version.mjs`
- Create: `tests/shared/tauri/version-sync.test.ts`
- Modify: `package.json`
- Modify when releasing: `package-lock.json`
- Modify when releasing: `src-tauri/Cargo.toml`
- Modify when releasing: `src-tauri/Cargo.lock`
- Modify when releasing: `src-tauri/tauri.conf.json`

- [ ] **Step 1: Write failing synchronization tests**

Use temporary copies with intentionally different versions. Require `set-version.mjs 1.0.8` to update `package.json`, both root version locations in `package-lock.json`, the `cloudadmin` package in `Cargo.toml`/`Cargo.lock`, and `tauri.conf.json`. Require strict stable SemVer and reject prefixes, prereleases, build metadata, leading zeroes, or partial versions.

Require `check-version.mjs --tag tauri-v1.0.8` to compare the tag with all five files, print no file contents, and fail on any mismatch.

```powershell
npm test -- tests/shared/tauri/version-sync.test.ts
```

Expected: FAIL because the scripts do not exist.

- [ ] **Step 2: Implement one version command**

Add exact scripts:

```json
{
  "tauri:version": "node scripts/tauri/set-version.mjs",
  "tauri:version:check": "node scripts/tauri/check-version.mjs"
}
```

The setter writes atomically, preserves final newlines, runs `cargo metadata --locked --no-deps` after the edit, and rolls back all temporary files if any validation fails. It never commits or creates a tag.

- [ ] **Step 3: Verify and commit tooling**

```powershell
npm test -- tests/shared/tauri/version-sync.test.ts
npm run tauri:version:check -- --tag "tauri-v$((Get-Content package.json -Raw | ConvertFrom-Json).version)"
git add -- scripts/tauri/set-version.mjs scripts/tauri/check-version.mjs tests/shared/tauri/version-sync.test.ts package.json package-lock.json
git diff --cached --check
git commit -m "build(tauri): synchronize release versions"
```

### Task 3: Build and upload immutable COS candidates

**Frontend files:**
- Create: `scripts/tauri/candidate-contract.mjs`
- Create: `scripts/tauri/build-candidate.mjs`
- Create: `scripts/tauri/upload-candidate.mjs`
- Create: `tests/shared/tauri/candidate-contract.test.ts`
- Create: `tests/shared/tauri/candidate-upload.test.ts`
- Modify: `package.json`
- Modify: `package-lock.json`

- [ ] **Step 1: Write failing manifest and upload tests**

Test strict candidate parsing, exact installer/updater object keys, both SHA-256 calculations, updater signature-file reading, URL construction from one configured HTTPS base, and atomic upload order. Inject a fake COS client and assert this sequence:

```text
put installer -> put updater bundle -> put updater signature
-> head/get installer -> verify installer sha256
-> head/get updater bundle -> verify updater sha256
-> put candidate.json
```

Reject an existing immutable key, mismatched `Content-Length`, mismatched downloaded SHA-256 for either artifact, missing updater signature, response outside the configured bucket/region, or any attempt to write `tauri_updater/windows-x86_64.json`.

- [ ] **Step 2: Run RED tests**

```powershell
npm test -- tests/shared/tauri/candidate-contract.test.ts tests/shared/tauri/candidate-upload.test.ts
```

Expected: FAIL because candidate tooling does not exist.

- [ ] **Step 3: Implement strict candidate tooling**

Pin `cos-nodejs-sdk-v5` to exact dev version `3.0.0`. Read only these environment variables:

```text
TENCENT_COS_SECRET_ID
TENCENT_COS_SECRET_KEY
TENCENT_COS_BUCKET
TENCENT_COS_REGION
TAURI_COS_PUBLIC_BASE_URL
```

The scripts must never print secret values. They may print version, tag, commit, object keys, SHA-256, byte count, and pass/fail. `candidate.json` is uploaded last and uses `application/json; charset=utf-8`; both installer and updater bundle are uploaded with metadata `sha256=<lowercase hash>`.

- [ ] **Step 4: Verify and commit**

```powershell
npm test -- tests/shared/tauri/candidate-contract.test.ts tests/shared/tauri/candidate-upload.test.ts
git add -- scripts/tauri/candidate-contract.mjs scripts/tauri/build-candidate.mjs scripts/tauri/upload-candidate.mjs tests/shared/tauri/candidate-contract.test.ts tests/shared/tauri/candidate-upload.test.ts package.json package-lock.json
git diff --cached --check
git commit -m "build(tauri): publish immutable COS candidates"
```

### Task 4: Add the tag-triggered Windows candidate Workflow

**Frontend files:**
- Create: `.github/workflows/release-tauri.yml`
- Create: `tests/shared/deployment/tauri-workflow.test.ts`
- Modify: `scripts/verify-tauri.ps1`

- [ ] **Step 1: Write the failing Workflow contract test**

Require one workflow with:

```yaml
on:
  push:
    tags:
      - 'tauri-v*'
permissions:
  contents: read
```

Require `windows-2025`, protected environment `tauri-production`, exact action SHAs, version/tag validation before dependency installation, `git merge-base --is-ancestor "$env:GITHUB_SHA" "origin/master"`, full frontend/Tauri gates, signed NSIS build, candidate construction, COS upload, and post-upload verification. Reject `workflow_dispatch`, branch pushes, Web/backend deployment, database access, Admin token, GitHub Release creation, secret echo, mutable action tags, and browser automation.

- [ ] **Step 2: Pin all reusable actions**

Use these reviewed commits:

```text
actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683
actions/setup-node@49933ea5288caeca8642d1e84afbd3f7d6820020
actions/cache@5a3ec84eff668545956fd18022155c47e93e2684
dtolnay/rust-toolchain@2c7215f132e9ebf062739d9130488b56d53c060c
```

The environment exposes Tauri signing secrets as `TAURI_SIGNING_PRIVATE_KEY` and `TAURI_SIGNING_PRIVATE_KEY_PASSWORD`, plus only the five COS variables declared in Task 3.

- [ ] **Step 3: Implement fail-closed build/sign/upload order**

The job checks out the tagged commit with full history, fetches `origin/master`, validates ancestry/version, runs `npm ci`, frontend gates, Rust gates, `npm run tauri:build`, identifies exactly one NSIS installer (`.exe`), one updater bundle (`.nsis.zip`), and one updater signature (`.nsis.zip.sig`), builds the candidate manifest, uploads it, and writes a secret-free GitHub step summary. Any ambiguous/missing artifact fails before COS mutation.

- [ ] **Step 4: Verify and commit**

```powershell
npm test -- tests/shared/deployment/tauri-workflow.test.ts
pwsh -NoProfile -File scripts/tests/tauri-security-source.tests.ps1
git add -- .github/workflows/release-tauri.yml tests/shared/deployment/tauri-workflow.test.ts scripts/verify-tauri.ps1
git diff --cached --check
git commit -m "ci(tauri): publish signed Windows candidates"
```

### Task 5: Read and validate candidates from configured COS

**Backend files:**
- Modify: `internal/infra/storage/cos/object_reader.go`
- Modify: `internal/infra/storage/cos/object_reader_test.go`
- Create: `internal/module/clientversion/candidate.go`
- Create: `internal/module/clientversion/candidate_store.go`
- Create: `internal/module/clientversion/candidate_test.go`
- Modify: `internal/module/clientversion/dto.go`
- Modify: `internal/module/clientversion/service.go`
- Modify: `internal/module/clientversion/service_test.go`
- Modify: `internal/platform/admin/build.go`

- [ ] **Step 1: Write failing storage/service tests**

Add bounded COS `List` and `Head` operations. List accepts only prefix `tauri_candidates/windows-x86_64/`, caps results at 100, and never returns object bodies. Head returns content length and normalized `x-cos-meta-sha256`.

Service tests cover empty candidates, strict manifest decode, malformed/oversized JSON, version/tag/key mismatch, unknown fields, foreign URL origin, missing installer/updater/signature, metadata size/hash mismatch for both artifacts, duplicate platform/version import, COS timeout, and successful import. Successful import maps only `updater_url`, `updater_signature`, and `updater_file_size` to the existing version record and creates it with `IsLatest=CommonNo` and `ForceUpdate=CommonNo`.

- [ ] **Step 2: Run RED tests**

```powershell
go test ./internal/infra/storage/cos ./internal/module/clientversion -count=1
```

Expected: FAIL because candidate storage/service behavior is missing.

- [ ] **Step 3: Implement bounded candidate discovery and import**

Use the existing enabled COS upload configuration and secretbox. Construct keys from validated platform/version; never accept an arbitrary object key or URL from the request. Decode candidate JSON with `DisallowUnknownFields`, limit it to 32 KiB, validate all fields, and compare artifact HEAD metadata before returning/importing it. Do not download the installer into API memory.

- [ ] **Step 4: Verify and commit**

```powershell
go test ./internal/infra/storage/cos ./internal/module/clientversion -count=1
git add -- internal/infra/storage/cos/object_reader.go internal/infra/storage/cos/object_reader_test.go internal/module/clientversion/candidate.go internal/module/clientversion/candidate_store.go internal/module/clientversion/candidate_test.go internal/module/clientversion/dto.go internal/module/clientversion/service.go internal/module/clientversion/service_test.go internal/platform/admin/build.go
git diff --cached --check
git commit -m "feat(client-version): import verified COS candidates"
```

### Task 6: Expose the exact Admin candidate transport and bundle

**Backend files:**
- Modify: `internal/module/clientversion/transport/admin/request.go`
- Modify: `internal/module/clientversion/transport/admin/handler.go`
- Modify: `internal/module/clientversion/transport/admin/handler_test.go`
- Modify: `internal/module/clientversion/transport/admin/route.go`
- Modify: `internal/server/router_test.go`
- Modify: `internal/server/testdata/admin_routes_golden.txt`
- Modify: `internal/server/testdata/admin_route_policy_golden.json`
- Modify: generated files under `contracts/admin/v1/`

- [ ] **Step 1: Write failing transport, policy, and bundle tests**

Use a manifest fixture where version, tag, commit, object key, URL, signature, hash, size, and timestamp are all distinct. Assert the exact GET query, POST body, success/empty/error envelopes, permission, and audit action. Assert that no alias field or credential parameter is generated.

```powershell
go test ./internal/module/clientversion/transport/admin ./internal/server ./internal/admincontract -count=1
```

Expected: FAIL because requests, handlers, and routes do not exist.

- [ ] **Step 2: Implement requests and handlers exactly as documented**

`CandidateListRequest` accepts optional `platform` and defaults only as documented to `windows-x86_64`. `ImportCandidateRequest` requires exact `version` and `platform`. Transport maps fields one-to-one and returns service errors unchanged through the standard Admin envelope.

- [ ] **Step 3: Register policy and audit behavior**

GET is authenticated/no-audit. POST requires `system_clientVersion_add` and audit action `import_candidate`. No route may be public or accept a credential in query/body.

- [ ] **Step 4: Generate and verify the Admin bundle**

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/generate-admin-contract.ps1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/check-admin-contract.ps1
go test ./internal/module/clientversion/transport/admin ./internal/server ./internal/admincontract -count=1
```

Expected: all new tests pass and the bundle exposes only documented candidate fields.

- [ ] **Step 5: Commit transport and contract**

```powershell
git add -- internal/module/clientversion/transport/admin/request.go internal/module/clientversion/transport/admin/handler.go internal/module/clientversion/transport/admin/handler_test.go internal/module/clientversion/transport/admin/route.go internal/server/router_test.go internal/server/testdata/admin_routes_golden.txt internal/server/testdata/admin_route_policy_golden.json contracts/admin/v1
git diff --cached --check
git commit -m "feat(contract): publish Tauri candidate operations"
```

### Task 7: Add the Admin candidate import UI

**Frontend files:**
- Sync: `contracts/backend/admin/v1/`
- Sync: `contracts/backend/admin/lock.json`
- Modify: `src/api/system/clientVersion.ts`
- Create: `src/views/Main/system/clientVersion/components/CandidateImportDialog.vue`
- Modify: `src/views/Main/system/clientVersion/index.vue`
- Modify: `src/i18n/locales/en-US.ts`
- Modify: `src/i18n/locales/zh-CN.ts`
- Create: `tests/component/client-version/CandidateImportDialog.test.ts`
- Create: `tests/shared/system/client-version-api.test.ts`

- [ ] **Step 1: Sync and list the exact mapping before implementation**

```powershell
npm run contract:sync -- --backend E:/admin/admin_back_go
npm run contract:generate
npm run contract:check
```

Record the one-to-one mapping from every documented candidate field to the dialog view. Do not add aliases, default business values, or runtime mock data.

- [ ] **Step 2: Write failing API/component tests**

Assert exact candidate GET query, exact import POST body, empty list, contract error, COS error, duplicate conflict, selection, import confirmation, refresh after success, and that no import automatically calls latest/force-update operations.

```powershell
npm test -- tests/shared/system/client-version-api.test.ts tests/component/client-version/CandidateImportDialog.test.ts
```

Expected: FAIL because the API methods and dialog do not exist.

- [ ] **Step 3: Implement the import-only dialog**

Add “Import COS candidate” beside the existing Add button. Display version, tag, short commit, installer URL/hash/size, updater URL/hash/size, and created time. Import uses only the selected exact version/platform. On success close, refresh the ordinary version list, and leave latest/force-update unchanged for separate existing actions.

- [ ] **Step 4: Verify and commit**

```powershell
npm run contract:check
npm test -- tests/shared/system/client-version-api.test.ts tests/component/client-version/CandidateImportDialog.test.ts
npm run build:check
git add -- contracts/backend/admin/v1 contracts/backend/admin/lock.json src/api/system/clientVersion.ts src/views/Main/system/clientVersion/components/CandidateImportDialog.vue src/views/Main/system/clientVersion/index.vue src/i18n/locales/en-US.ts src/i18n/locales/zh-CN.ts tests/component/client-version/CandidateImportDialog.test.ts tests/shared/system/client-version-api.test.ts
git diff --cached --check
git commit -m "feat(client-version): import Tauri COS candidates"
```

### Task 8: Rehearse tag-to-candidate and user promotion

**Backend files:**
- Create: `scripts/verify-tauri-candidate.ps1`
- Create: `scripts/tests/tauri-candidate-release.tests.ps1`
- Create: `docs/runbooks/tauri-windows-release.md`
- Modify: `docs/architecture.md`

**Frontend files:**
- Create: `docs/acceptance/p08.5-tauri-release-manual.md`

- [ ] **Step 1: Test the release verifier without production secrets**

Use fake COS HTTP fixtures to prove immutable ordering, read-back verification, redaction, malformed candidate denial, duplicate import conflict, and no latest mutation. The verifier accepts tag/version/commit plus public candidate URL, checks exact fields and updater signature presence, and never prints a secret or artifact body.

- [ ] **Step 2: Write exact operator steps and stop conditions**

The runbook sequence is:

```text
run tauri:version -> review version diff -> commit on master -> create tauri-vX.Y.Z tag
-> push tag -> inspect successful release-tauri job -> verify candidate in COS
-> open Admin candidate dialog -> import -> review ordinary version row
-> user separately chooses notes, latest, and force-update
```

Stop on tag/version mismatch, non-master commit, failed Workflow, missing signature, hash/size mismatch, foreign COS origin, duplicate version, backend import error, or user rejection. Never edit live updater JSON by hand.

- [ ] **Step 3: Run a real candidate release**

The user selects the actual next stable version. After the tag job succeeds, read the committed version and run:

```powershell
$approvedVersion = (Get-Content E:/admin/admin_front_ts/package.json -Raw | ConvertFrom-Json).version
pwsh -NoProfile -File E:/admin/admin_back_go/scripts/verify-tauri-candidate.ps1 -Version $approvedVersion
```

Then import it in Admin. The Agent records evidence but does not click or claim latest/force-update approval for the user.

- [ ] **Step 4: Commit rehearsal tooling and evidence template**

```powershell
cd E:/admin/admin_back_go
pwsh -NoProfile -File scripts/tests/tauri-candidate-release.tests.ps1
git add -- scripts/verify-tauri-candidate.ps1 scripts/tests/tauri-candidate-release.tests.ps1 docs/runbooks/tauri-windows-release.md docs/architecture.md
git diff --cached --check
git commit -m "docs(tauri): add Windows candidate release runbook"

cd E:/admin/admin_front_ts
git add -- docs/acceptance/p08.5-tauri-release-manual.md
git diff --cached --check
git commit -m "docs(tauri): record candidate promotion acceptance"
```

## Plan completion gate

```powershell
cd E:/admin/admin_back_go
go test ./...
powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/check-admin-contract.ps1
pwsh -NoProfile -File scripts/tests/tauri-candidate-release.tests.ps1

cd E:/admin/admin_front_ts
npm run contract:check
pwsh -NoProfile -File scripts/verify-frontend.ps1
pwsh -NoProfile -File scripts/verify-tauri.ps1
npm test -- tests/shared/tauri tests/shared/deployment/tauri-workflow.test.ts tests/shared/system/client-version-api.test.ts tests/component/client-version/CandidateImportDialog.test.ts
git grep -n -i playwright -- package.json package-lock.json src tests scripts .github

git -C E:/admin/admin_back_go status --short
git -C E:/admin/admin_front_ts status --short
```

Expected: all commands exit 0; only `.github/workflows/release-tauri.yml` exists; tag/version/commit match; the signed immutable candidate is read back from COS; Admin imports it with latest and force-update both disabled; browser-tool and both status searches produce no output. P08.5 is accepted only after the user confirms the candidate and any later promotion action.

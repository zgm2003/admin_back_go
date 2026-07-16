# P02 Docker Pause Handoff

Snapshot date: 2026-07-16 (Asia/Shanghai)

## Repository and operator constraints

- Repository: `E:\admin\admin_back_go`
- Branch: `master`
- Work directly in this checkout. The operator explicitly does not want a Git worktree.
- Do not modify either frontend repository while executing the backend P02 plan.
- Do not push. Local task commits are allowed and expected by the plan.
- Never print, stage, or commit `deploy/docker-first/admin-go.env`, a DSN, `APP_SECRET`, database passwords, decrypted credentials, MySQL option files, or database dumps.
- Preserve unrelated operator changes if the worktree is not clean.

## Program progress

The program contains P01 through P09.

- P01 foundation is complete. Its final commit is `65d46fc fix(foundation): close production safety gaps`.
- P02 database evolution is active.
- P02 Task 1 is complete in commit `7767150 chore(database): quarantine legacy migrations and pin atlas`.
- P02 Task 2 is paused at a tested but uncommitted checkpoint.
- P02 Tasks 3 through 11 have not started.
- P03 through P09 have not started in this continuation.

Authoritative inputs:

- Approved design: `docs/superpowers/specs/2026-07-15-admin-foundation-database-design.md`
- P02 plan: `docs/superpowers/plans/2026-07-15-admin-database-evolution-plan.md`
- Program index: `docs/superpowers/plans/2026-07-15-admin-platform-super-refactor-execution-index.md`

## P02 Task 1 completed state

Task 1 made these durable changes:

- moved all 48 historical SQL files from `database/migrations/` to `database/legacy-migrations/` without changing their contents;
- created the inert Atlas baseline `database/migrations/202607150001_baseline.sql`;
- created the digest-pinned offline wrapper `scripts/database/atlas.ps1`;
- created `database/README.md` and updated root/architecture documentation;
- updated existing tests that inspect historical SQL so they read `legacy-migrations`;
- added `internal/architecture/database_layout_test.go`.

Verification performed before commit:

```text
go test ./internal/architecture -count=1   PASS
go test ./...                             PASS
legacy SQL count                          48
active Atlas SQL count                    1
```

The Atlas container validation was not run because Docker was not installed. The wrapper was executed only far enough to confirm that it fails closed with the expected Docker-required error.

## P02 Task 2 uncommitted checkpoint

The following untracked files intentionally contain the current Task 2 work:

```text
internal/databaseevolution/fingerprint.go
internal/databaseevolution/fingerprint_test.go
```

Implemented and tested:

- fingerprint model types for tables, columns, indexes, foreign keys, checks, triggers, routines, and events;
- deterministic canonical JSON ordering;
- exclusion of volatile auto-increment values from canonical JSON;
- secret-safe `MYSQL_DSN` schema validation;
- read-only `information_schema` capture with bound schema arguments;
- sqlmock coverage for the stable capture fields.

Current focused verification:

```text
go test ./internal/databaseevolution -count=1   PASS
```

Task 2 is not complete. The next agent must review the uncommitted code and then add, test-first:

1. the fingerprint document containing the Git commit and `schema_sha256`;
2. SHA-256 calculation over canonical JSON;
3. temporary-file plus rename output writing and cleanup tests;
4. `cmd/admin-db/main.go` with the `fingerprint` subcommand;
5. secret-safe CLI error handling and exact argument validation;
6. two live captures against `admin`, proving identical schema hashes;
7. full `go test ./...`, diff review, and the planned Task 2 commit.

Do not commit Task 2 merely because its focused tests pass; the CLI and live determinism gate are still missing.

## Local toolchain and services

At the snapshot:

```text
Go:                go1.27rc1 windows/amd64
CGO_ENABLED:       0
PowerShell:        Windows PowerShell 5.1
pwsh:              unavailable
Docker:            unavailable (operator is installing it before resume)
MySQL:             8.4 on 127.0.0.1:3306
Redis:             8.8 on 127.0.0.1:6379
Runtime env:       deploy/docker-first/admin-go.env (ignored, owner-only)
```

The real runtime env contains the local project database credential and stable `APP_SECRET`. Read it into a process without echoing values. Do not replace it or regenerate its secret.

Native processes were running at the snapshot, but PIDs are ephemeral:

```text
admin-api:     127.0.0.1:8080
admin-worker:  running
admin frontend: http://127.0.0.1:5174
```

Port 5173 was occupied by an unrelated project under `D:\work\hqd-ai-new`, so this Admin frontend uses 5174. Native backend launch also requires `ZONEINFO` to point to the installed Go `lib/time/zoneinfo.zip`, and it overrides container paths such as `/app` with Windows repository paths.

## Current local database facts

The operator refreshed the local `admin` database immediately before this pause. The last read-only audit found:

```text
tables:                         59
required CAPTCHA/upload TTLs:  present and enabled
active non-COS upload drivers: 0
open payment orders:           0
API /ready:                    ready
CAPTCHA generation:            working
```

Secretbox compatibility with the current `APP_SECRET` at the last audit:

- COS upload configuration: decryptable;
- mail configuration: decryptable;
- SMS configuration: disabled and empty;
- Alipay sandbox private key: not decryptable and must be re-entered;
- AI provider API keys for `本地`, `灵算`, `方中杰`, and `鲨鱼辣椒`: not decryptable and must be re-entered.

All three Alipay certificate files existed after the database refresh. These facts can change if the operator edits configuration; re-audit rather than assuming they remain current.

The public certificate served by `cos.zgm2003.cn` expired on 2026-06-21. The tested avatar object itself returned HTTP 200 when TLS validation was bypassed. Certificate renewal is a Tencent COS custom-domain operation, not a backend-code issue.

## Docker acceptance before resuming P02

After Docker installation, confirm Linux-container operation and the two pinned images needed by P02:

```powershell
docker version
docker info
docker run --rm hello-world
docker pull arigaio/atlas:0.38.0@sha256:9883fdf5290020022ad0ac91fe20b846d32f93c19f68dfd3cf3b327c3e1b7e1a
docker pull mysql:8.4.10
powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/database/atlas.ps1 migrate validate --dir file://database/migrations
```

Do not loosen the Atlas digest, enable network access in the Atlas wrapper, or substitute an unpinned image. A Docker failure is an external gate, not permission to claim Atlas validation passed.

## Exact resume sequence

1. Read `AGENTS.md`, the approved design, the full P02 plan, and this handoff.
2. Confirm `git status --short --branch`. Expect the handoff commit plus the two untracked Task 2 files unless the operator changed them.
3. Verify Docker with the acceptance commands above.
4. Run `go test ./internal/databaseevolution -count=1` before editing.
5. Review the Task 2 checkpoint for MySQL 8.4 compatibility; do not discard it blindly.
6. Continue Task 2 with strict red-green-refactor cycles.
7. Import the ignored runtime env without printing it, then capture the live `admin` fingerprint twice and compare only `schema_sha256`.
8. Run `go test ./...` before the Task 2 commit.
9. Continue P02 one task and one planned commit at a time.

Do not apply reconciliation or destructive SQL to the live `admin` database. P02 first creates and verifies a recovery artifact and uses disposable restored databases for expand/backfill/verify work.

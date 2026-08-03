# AI Context Engineering Cutover

This runbook is for an approved deployment window. It does not authorize a local or production migration by itself. Keep the old images and the verified recovery artifact until browser acceptance is complete.

## Release Inputs

Record these before stopping anything:

```text
backend image digest:
frontend image digest:
backend Git SHA:
frontend Git SHA:
Admin Contract SHA-256:
database target:
operator:
window:
```

The frontend Contract manifest must name the same backend source commit and artifact hashes as the backend Bundle. Image labels must match the recorded Git SHAs. A mismatch is a hard stop.

Migration SHA-256 values:

```text
202608020101_ai_context_expand.sql      2c98b934d469c9e1d512b6fa249cdf1eae24dc0ea2f7981a4b0e77754b58fb37
202608020102_ai_context_permissions.sql 8b719142fc3f8624a193068ac03cf1e5cedf13678578ab81234fac5270a9a844
202608020103_ai_context_contract.sql    6f0cdb3074358225babc39a62be772f3ad0fabd5eb90a6ce231d50e659165133
```

Verify them from the release checkout:

```powershell
Get-FileHash database/migrations/202608020101_ai_context_expand.sql,database/migrations/202608020102_ai_context_permissions.sql,database/migrations/202608020103_ai_context_contract.sql -Algorithm SHA256
pwsh -NoProfile -File scripts/database/atlas.ps1 migrate validate --dir file://database/migrations
```

## Atomic Cutover

Run Compose commands from `deploy/docker-first`. Replace image tags only through the approved release manifest; do not rebuild during the window.

1. Stop ingress and the old API first. Keep the old Worker alive.

```powershell
docker compose stop frontend admin-api
docker compose ps --all frontend admin-api admin-worker
```

`frontend` and `admin-api` must be exited. `admin-worker` must still be running on the old image.

2. Observe the old Worker until Reply Commands, Chat Attempts, finalizers, and reconcilers have drained. Do not infer drain from quiet logs. Query the authoritative state through the approved operational SQL/evidence channel and record counts and active IDs.

```text
Reply Commands in claimed/running/outcome_unknown: 0
Chat Attempts in prepared/dispatched/outcome_unknown: 0
```

3. Stop the old Worker and prove no old process can write.

```powershell
docker compose stop admin-worker
docker compose ps --all frontend admin-api admin-worker
docker inspect admin-app-admin-api-1 admin-app-admin-worker-1 --format '{{.Name}} {{.State.Status}} {{.Image}}'
```

Both application containers must be exited. If the deployment uses different container names, resolve them with `docker compose ps --format json`; do not guess.

4. Run the new image's read-only preflight against the target database while all application writers remain stopped.

```powershell
docker run --rm --network admin-platform --env-file .\admin-go.env <approved-backend-image-digest> /app/ai-context-preflight
```

Expected result: exit code 0; zero active Reply Commands; zero active Chat Attempts; all six legacy table counts are zero; every enabled Chat Agent has a valid Provider, chat model, protocol, context window, output limit, and token counter. The preflight must not log prompts, messages, object keys, signed URLs, or API keys.

5. Create and restore-verify the locked recovery artifact outside both repositories.

```powershell
pwsh -NoProfile -File scripts/database/new-recovery-artifact.ps1 -Database admin -BackupRoot $env:ADMIN_BACKUP_ROOT
```

Require a non-empty dump, SHA-256, successful disposable restore, and identical critical table counts. Store the artifact manifest with the release evidence.

6. Apply migrations exactly in Atlas order. The protected release controller must provide `ADMIN_ATLAS_URL` from its secret store; never print it or save it in either repository.

```powershell
docker run --rm --network admin-platform --volume "${PWD}/../..:/workspace:ro" --workdir /workspace arigaio/atlas:0.38.0@sha256:9883fdf5290020022ad0ac91fe20b846d32f93c19f68dfd3cf3b327c3e1b7e1a migrate status --dir file://database/migrations --url $env:ADMIN_ATLAS_URL
docker run --rm --network admin-platform --volume "${PWD}/../..:/workspace:ro" --workdir /workspace arigaio/atlas:0.38.0@sha256:9883fdf5290020022ad0ac91fe20b846d32f93c19f68dfd3cf3b327c3e1b7e1a migrate apply --dir file://database/migrations --url $env:ADMIN_ATLAS_URL
docker run --rm --network admin-platform --volume "${PWD}/../..:/workspace:ro" --workdir /workspace arigaio/atlas:0.38.0@sha256:9883fdf5290020022ad0ac91fe20b846d32f93c19f68dfd3cf3b327c3e1b7e1a migrate status --dir file://database/migrations --url $env:ADMIN_ATLAS_URL
```

The applied sequence must be `202608020101`, `202608020102`, then `202608020103`. The final status must be clean. Never start a new binary against a partially migrated schema.

7. Start only the synchronized images, then verify readiness and registration.

```powershell
docker compose up -d --no-build admin-api admin-worker frontend
docker compose ps
Invoke-RestMethod http://127.0.0.1:8080/health
Invoke-RestMethod http://127.0.0.1:8080/ready
```

Require healthy API/Worker/Frontend, the recorded Contract SHA, menu authorization, and all five Context task registrations. `/health` alone is insufficient.

## Hard Stops

Stop immediately when any of these is true:

- an old API or Worker process can still write;
- preflight reports an active command/attempt, invalid enabled Agent, or a non-zero legacy table;
- the recovery artifact is missing, empty, checksum-invalid, or cannot be restored;
- migration checksum, Atlas status, schema fingerprint, image SHA, or Contract SHA differs;
- the Contract migration names a legacy table/count failure before its first `DROP`;
- any migration is partially applied or the new readiness check fails.

Do not manually delete legacy rows, bypass the Contract guard, edit migration history, or start only one new application process. Before the Contract migration commits, rollback means stop the new processes and restart the recorded old images against the unchanged schema. After the Contract migration commits, database rollback requires the verified recovery artifact and the matching old images; reverse DDL is not a rollback.

## User Browser Acceptance

The user performs this checklist after readiness passes. No Playwright run is part of this procedure.

- `/ai/context` shows Spaces, Documents, Index Profiles, and Evaluation.
- Agent Context saves Profile `NULL`, a ready Profile with zero Spaces, and compatible Spaces.
- TXT, Markdown, PDF, DOCX, CSV, and XLSX versions show truthful queued/processing/ready/failed ingestion state.
- A valid Chat citation opens its persisted source drawer after a full page refresh.
- An invalid citation remains plain response text and has no source mapping.
- Run detail shows budget proof, metrics/stages, selected/excluded items, and failure outcome.
- Menu and search contain Context Engineering and no retired entry.
- Chat settings contain no history-count control.

Record each item as pass/fail with the browser, user, timestamp, conversation/run IDs, and screenshots where useful. Do not call the live cutover complete until every item passes or an explicit rollback decision is recorded.

# Admin-only release observability

Status: production operator runbook

Observe the synchronized frontend, API, Worker, database, Redis, storage, and
provider system by release ID and request ID.

## Deployment signals

- Frontend `/healthz`: HTTP 200 from the promoted immutable frontend image.
- API `/health`: process liveness.
- API `/ready`: database, Redis, queue, and required dependency readiness.
- OCI revision labels: exact frontend/backend Git commits from the manifest.
- Compose state: API and Worker use the same backend image ID.

## Runtime signals

- **WebSocket**: connect failures, origin rejection, resume cursor, retention
  watermark, reconnect count, duplicate terminal events, and slow consumers.
- **queue**: pending/active/retry/dead counts, enqueue latency, Worker heartbeat,
  lease recovery, cancellation, and graceful termination.
- **scheduler**: elected owner, duplicate execution count, missed runs, and
  execution duration.
- **provider**: request count, timeout/rate-limit/error class, duration, token or
  media usage totals, and circuit state. Never log prompt/body/key content.
- Storage: signed-upload failures, object verification, and retained COS-key
  reachability. Never log signed URLs or object content.

## Log handling

Require structured timestamps, release ID, request ID, operation, safe status,
and duration. The release verifier hashes command output and stores no rows or
payloads. Run a **redaction** review for DSNs, Authorization/Cookie values,
session credentials, secrets, prompts, dumps, certificates, signed URLs, and
provider bodies before exporting logs.

## Alert and incident escalation

Open **incident escalation** immediately for readiness failure after promotion,
repeated Worker loss, durable-work duplication/loss, WebSocket replay gaps,
scheduler double ownership, schema drift, or provider/storage credential
exposure. Record release ID, image IDs, manifest/proof hashes, start time,
impact, current RTO/RPO estimate, and rollback decision. Do not include secret
values or live data.

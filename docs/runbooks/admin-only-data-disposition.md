# Admin-only data disposition

## Scope and evidence boundary

This runbook is the P09 classification source for product-platform rows. It is
not permission to execute destructive SQL. Counts and hashes are frozen by
`release/admin-only/input-lock.json`; external recovery, fingerprint, query,
and COS evidence stays outside both repositories and is referenced only by
SHA-256.

The locked P08R recovery artifact was restored and verified before P09. Its
source database is `admin`, its dump SHA-256 is recorded in the P08R cutover
runbook, and source/restore counts match. P09 must stop again for **fresh
destructive approval** before the first contract DDL execution, including a
disposable-restore rehearsal that drops `client_versions`.

No field is reassigned by inference. A row is retained, renamed, revoked, or
removed only by the explicit rules below.

## Identity and RBAC

### App/Canvas sessions and login attempts

- Preserve the locked recovery counts and platform breakdown as audit evidence.
- Active App/Canvas sessions must already be zero before contract work. If a
  newly active row appears, stop; do not silently expire or delete it.
- During the approved row contract, delete App/Canvas session rows and login
  attempts only after their counts match the locked evidence.
- Admin sessions and Admin login history remain unchanged.

### Permissions, role grants, and auth policy

- Build the exact retired permission ID set from `platform IN ('app','canvas')`.
- Remove active `role_permissions` for that set before removing the permission
  rows.
- One pre-existing active orphan is separately locked: role-permission ID 723,
  role ID 1, missing permission ID 539. The approved row contract removes only
  that grant; it does not invent or attach a replacement permission.
- Remove App/Canvas `auth_platforms` rows only after the single enabled Admin
  policy is proven present.
- Never assign an App/Canvas grant to an Admin permission merely because names
  or numeric IDs look similar.

### `users_quick_entry`

`users_quick_entry` is already absent before P09. Historical P02 evidence
recorded 107 rows and 3 active rows before its earlier reviewed retirement.
P09 verifies absence; it does not recreate the table and does not claim to drop
those rows again.

## Notifications and exports

### `notification_task` and notifications

- Rows whose documented audience is `all` become `admin` because Admin is now
  the sole product audience.
- App/Canvas-only task and notification rows are removed after exact counts are
  captured.
- Unknown audiences stop the migration. They are not treated as `admin`.

### `export_tasks`

- Admin exports remain.
- Non-Admin `export_tasks` are removed only after their task count and hashed
  object-reference disposition are present in the locked evidence.
- A missing object is an error for retained Admin data; it is not converted to
  an empty download or a mock file.

## AI capabilities and storage

### Product rows

- Canvas `ai_runs`, `ai_text_tasks`, and `ai_image_tasks` are retired.
- Delete `ai_image_files`, run events, tool calls, and knowledge retrieval
  dependents before their owning task/run rows.
- Orphan image-file rows are retired only by their explicit orphan
  classification; no owner is invented.
- `canvas_video_tasks` currently contains zero rows. It is dropped only after
  the empty count is rechecked and `ai_video_tasks` is proven present.
- Provider, agent, model, prompt, generic asset, upload, and COS configuration
  survive unless a separate explicit rule names the row.

### Prompt catalog reset

The user explicitly classified the existing Canvas-era prompt catalog as old
product data. The locked source contains exactly 1,356 prompt rows, all active,
five `ai_prompt_*` permissions, and ten role grants for those permissions.
There are no foreign keys referencing `ai_prompts`.

The approved row contract deletes the ten role grants, then the five
permissions, then all 1,356 prompt rows. Task 7 removes the Admin prompt routes,
menu, API client, and page from the published contract and frontend. The
contract must preserve the empty ai_prompts table and the transport-neutral
Prompt model/repository/service so a future formally contracted platform can
load new data. It must not replace the deleted catalog with mock/default rows.

### Scene rename

The only approved scene mappings are:

| Historical value | Admin capability value |
| --- | --- |
| `canvas_text_generate` | `text_generate` |
| `canvas_image_generate` | `image_generate` |
| `canvas_video_generate` | `video_generate` |
| `canvas_audio_generate` | `audio_generate` |

Any other unknown value stops the contract. Existing `chat` and
`agent_generate` values remain unchanged.

Legacy `ai_billing_rules` and `ai_billing_records` tables are already absent.
P09 verifies that fact and does not reference or recreate them.

### COS disposition

The fresh P09 preflight (not the older P02 network snapshot) observed these
hashed-reference classes:

- one active Admin AI image metadata row points to an already missing object;
- 426 Canvas AI image references are retired-row references: one object is
  reachable and 425 are already missing;
- 11 orphan AI image references are retired-row references;
- 437 of 438 AI image references were already `not_found`;
- all eight frozen `client_versions.file_url` references returned HTTP 404.

The evidence file contains only SHA-256 reference identities, source class,
platform class, reachability status, and disposition. Raw object keys, URLs,
credentials, and signed query values are not stored in the lock.

**No COS delete operation** is authorized by P09. Retired database rows may be
removed after approval, but any object that still exists is preserved. The
client-version objects are already missing; P09 records that fact and performs
no remote delete call. The Admin image task metadata is preserved with an
explicit missing-object disposition; P09 neither invents content nor silently
turns the missing object into a successful asset. Any reference classified as
an actually retained object must be reachable at every release proof; the
current evidence has zero such object references.

## Frozen client-version history

`client_versions` remains frozen at 8 rows with deterministic row SHA-256
`ca574b6ce101d92b05cc3571e7e138aa9bf2bc5096c04357c8d39792ba806661`.

Before physical removal, all of the following must be true:

1. the live count/hash still matches P08R;
2. no active route, task, menu, role grant, package, or generated Admin
   contract references the table;
3. all eight historical URL hashes have the explicit
   `record_missing_no_object_delete` disposition;
4. the recovery artifact and complete rollback rehearsal remain valid;
5. the user gives fresh destructive approval.

## Already-absent compatibility tables

The current schema already excludes `canvas_prompts`, `canvas_assets`,
`users_quick_entry`, `ai_billing_rules`, and `ai_billing_records`. P09 treats
their reappearance as a violation. The contract migration does not include a
fake second drop for any of them.

## Stop conditions

Stop immediately on any count/hash mismatch, active retired session,
unclassified platform/scene, retained missing object, non-terminal durable AI
work, orphan not covered by an explicit rule, contract-lock mismatch, recovery
verification failure, secondary worktree, dirty unrelated path, or absent
fresh destructive approval.

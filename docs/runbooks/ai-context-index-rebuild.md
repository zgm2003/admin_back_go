# AI Context index rebuild

Status: implementation checkpoint; real Qdrant candidate verification deferred

## Ownership

- MySQL owns Profiles, Spaces, Documents, immutable Versions, Chunks, bindings,
  Plans, and generation state.
- Object storage owns immutable source bytes referenced by Version facts.
- Qdrant owns only rebuildable Dense/Sparse points and physical collections.
- Redis/Asynq owns delivery only. Queue state never proves business completion.
- Deleting Qdrant data is recoverable. It does not authorize deleting MySQL
  rows or source objects.

Collection names are deterministic:

```text
alias:    <prefix>_profile_<profile_id>
physical: <prefix>_profile_<profile_id>_g<generation>
```

Runtime retrieval reads the physical collection named by the MySQL Active
Generation. It never follows the Alias to choose business data.

## Generation states

```text
provisioning(NULL,target) -> ready(active,NULL)
ready(active,NULL) -> rebuilding(active,target) -> ready(target,NULL)
failed(active?,target?) -> rebuilding(active?,new_target) -> ready(new_target,NULL)
```

A healthy target failure deletes the target and returns to the old Active
Generation with a safe error code. A missing/corrupt Active collection or a
missing promised Document Point moves the Profile to `failed`. New generation
numbers are monotonic and are never reused.

## Rebuild order

1. CAS the Profile into `provisioning` or `rebuilding` with a Target Generation.
2. Create the target physical collection with the Profile schema.
3. Rebuild active MySQL Document Versions and their immutable Chunks.
4. Re-read active Version IDs and reject a changed snapshot.
5. Verify collection schema, expected Point IDs/hashes, and vector dimensions.
6. Atomically switch the stable Alias to the target collection.
7. CAS MySQL Target to Active and clear Target.
8. Retain the old collection through the configured operational grace window.
9. Cleanup rechecks MySQL Active, Target, Alias, and the grace deadline before
   deleting the retired collection.

The reconciler repairs an Alias already switched while MySQL is old by
finishing the MySQL CAS. If MySQL is new while the Alias is old, it verifies the
new Active collection before restoring the Alias.

## Readiness

API readiness is source-aware. No active Context source yields degraded Qdrant
status without breaking pure chat. An enabled source requires a readable
`ready` or `rebuilding` Active Generation, matching Alias, valid collection
schema, and the required Qdrant query capabilities. Worker readiness always
requires Qdrant and exactly these Plan 02 handlers:

```text
ai:context-document-index:v1
ai:context-index-cleanup:v1
ai:context-profile-rebuild:v1
```

## Candidate verification

The approved candidate is `qdrant/qdrant:v1.18.3`, but a tag is not deployment
evidence. Before adding Qdrant to state Compose, run the disposable capability
gate and pin the exact reported RepoDigest:

```powershell
pwsh -NoProfile -File scripts/context/verify-qdrant-candidate.ps1 `
  -CandidateImage qdrant/qdrant:v1.18.3 `
  -PinEnv deploy/docker-state/qdrant-image.env
```

The gate must pass Sparse IDF, QueryBatch, official RRF, Filter, and payload
schema checks. Do not create `qdrant-image.env` manually and do not accept a
tag-only Compose image.

## Recovery

1. Stop mutation of the affected Profile; do not delete MySQL or source data.
2. Inspect Profile `index_state`, Active/Target generations, and the Alias.
3. Let the reconciler repair a verified two-system pointer mismatch.
4. For `failed`, enqueue the versioned Profile rebuild task through the normal
   Worker registry. Do not mutate generation columns by hand.
5. Confirm API/Worker `/ready` and inspect only safe IDs, stages, and error codes.
6. Enqueue retired collection cleanup only after the grace deadline.

Never rebuild in place, dual-write generations, bypass MySQL CAS, log document
content/API keys/signed URLs, or treat a successful queue delivery as a ready
Version.

## Deferred checkpoint gates

The current implementation pass intentionally does not run Docker, the real
Qdrant candidate, migrations, or broad package scripts. Those gates remain
required before deployment and will be executed during the final coordinated
test pass.

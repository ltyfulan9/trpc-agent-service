# Online Data Migration

`cmd/data-migrate` operates the durable online migration coordinator. The
production Worker installs Session, Knowledge and Artifact decorators;
Summary Worker installs the same Session decorator. `cmd/migrate` remains
the separate PostgreSQL schema migration command.

## Supported Domains

| Domain | Source and target | Copied state |
| --- | --- | --- |
| `session` | Redis / PostgreSQL, including a different instance of the same backend | Official SDK Session-owned state, ordered Events and Tracks |
| `knowledge` | Qdrant to another endpoint or collection | Scoped documents, vectors, content and metadata; compatible embedding definition and dimension required |
| `artifact` | S3/MinIO to another endpoint or bucket | Exact artifact versions, bytes and version metadata, including tombstones |

Memory backend selection is supported by the platform, but online Memory
migration is not implemented. Platform Summary checkpoints already belong
to PostgreSQL and do not move with the SDK Session store. Session migration
rejects App/User shared state, native SDK summaries, TTL and inventories or
histories reaching the configured safety limit. It fails instead of dropping
those fields. The target tenant namespace must be empty at creation. Changing
only a profile alias is rejected when both profiles identify the same store.

## Deployment Contract

1. Apply schema migrations through `045`, then deploy this revision of every
   Worker and Summary Worker. Older writers do not participate in the gate.
2. Publish both source and target operator-owned profiles to the migration
   command and all writers before creating a migration. Use the same immutable
   profile definitions everywhere. Keep both available throughout the rollback
   window. Profile tenant allowlists still apply.
3. Run the command with `DATABASE_URL`, `STORAGE_BACKEND_PROFILES` and
   `DATA_PLANE_PROFILES`, plus only their referenced credential environment
   variables. Production database URLs require TLS. The control database pool
   must allow at least three connections; the command reserves up to ten.
4. Ensure all domain mutations go through the platform decorators. Direct SDK
   clients, maintenance scripts and external writers bypass the journal and
   are outside this protocol. Do not edit storage profiles or tenant storage
   settings through another path during migration.

The Compose `operations` profile includes the command image. It uses the
isolated stack's local Redis and PostgreSQL profiles and receives data-plane
credentials without IM or chat-model credentials:

```bash
docker compose -f deploy/docker-compose.yml --profile operations build data-migrate
docker compose -f deploy/docker-compose.yml --profile operations run --rm data-migrate \
  create --id tenant-a-session-1 --tenant tenant-a --domain session \
  --source-profile local-redis --target-profile local-postgres \
  --source-backend redis --target-backend postgres --config-version 7 \
  --actor operator-name --reason "replace session backend"
docker compose -f deploy/docker-compose.yml --profile operations run --rm data-migrate \
  run --id tenant-a-session-1 --batch-size 100 --timeout 30m
docker compose -f deploy/docker-compose.yml --profile operations run --rm data-migrate \
  status --id tenant-a-session-1
```

Replace the tenant, ID and expected `config_version` with actual values. The
create transaction compares the current version, backend and profile; stale
configuration is rejected. `run` stops at `ROLLBACK_WINDOW`. It never calls
`complete` automatically. `step` performs one state-machine step and is useful
for controlled maintenance. Batch size is `1..1000`; long validation scans
are bounded by the command deadline, not by one copy batch.

For Kubernetes, build `deploy/Dockerfile.data-migrate`, pin the resulting image
digest, and run its `/app/data-migrate` entrypoint in an operator-controlled Job
with the same profile ConfigMap and narrowly scoped storage Secrets. Set an
explicit namespace, `restartPolicy: Never`, `backoffLimit: 0`, a deadline,
non-root user, read-only root filesystem, disabled service-account token
mounting, and reviewed egress to the control database and both stores. This
operational Job is separate from the schema Job and the application release
bundle allowlist. A retry of `run` with the same ID resumes durable state.

## Runtime Protocol

Creation installs a tenant/domain route and enables write capture before
inventory copy begins. Every decorated operation acquires the shared
PostgreSQL advisory gate and reads this route. While mirroring is active it
reacquires the exclusive gate and rereads the route. Other tenants and domains
remain independent.

For a mutation, the coordinator drains previous unresolved writes, commits
all affected record identities as intents, writes the source, reads canonical
source state, appends immutable ordered journal records, applies the target,
and verifies target payload/hash before recording `projected_at`. A deletion
is a tombstone. Intents survive cancellation, process failure and ambiguous
source responses; the next operation or coordinator step reconciles them.
New mutations cannot overtake an unresolved delete and recreation sequence.

Snapshot discovers pre-existing data from native stores. Copy cursor and
watermark advance only after projection. Later writes use the same journal
during copy, catch-up, validation and the rollback window. `VALIDATE` and
`READ_SHADOW` compare canonical records from both stores, including inventories
and deleted keys. This is full record shadow verification, not sampling of
live similarity-search rankings or user responses.

Cutover holds the exclusive gate, drains pending writes and rechecks both
stores. The tenant configuration CAS, durable read/write route, phase,
lease/fence check and audit are committed in one transaction. During
`ROLLBACK_WINDOW`, reads use the target while writes continue to the source
and mirror to the target. Cached Worker clients read the route on every call,
so an already-created client follows the new destination.

Active migration serializes operations for that tenant/domain. Full shadow
verification and final cutover can block its operations for the duration of
the scan. Synchronous mirroring adds target latency and makes target failure
visible to callers; the source may already have committed when an error is
returned. Size the maintenance window and retry policy accordingly. This is
not a zero-latency or unlimited-scale migration guarantee.

## Operator Recovery

All mutating operator commands require `--actor` and `--reason`, except the
leased `step`/`run` progression that uses the creation audit identity.

```bash
data-migrate pause --id tenant-a-session-1 --actor operator-name --reason "inspect lag"
data-migrate resume --id tenant-a-session-1 --actor operator-name --reason "backend recovered"
data-migrate abort --id tenant-a-session-1 --actor operator-name --reason "cancel before cutover"
data-migrate rollback --id tenant-a-session-1 --actor operator-name --reason "target degraded"
data-migrate complete --id tenant-a-session-1 --actor operator-name --reason "observation accepted"
```

Use `abort` only before cutover and `rollback` only in the rollback window.
Pause stops coordinator progression; write capture continues so source changes
remain recoverable. After a transient backend failure, repair the backend and
rerun the same ID. Inspect `status`, `last_error`, pending intents and pending
journal rows; do not manually mark a row projected or advance its cursor.

`complete` performs final verification and makes both reads and writes use
the target. The terminal route is retained for cached clients. Neither
completion, abort nor rollback deletes the source store. Cleanup of old data
and journal retention is a separately scheduled operator action after backup
and retention requirements are met. After completion, a reverse transfer
requires a new migration into an empty destination; the previous rollback
window has ended.

```sql
SELECT migration_id, count(*) AS unresolved_intents
FROM data_migration_live_intents GROUP BY migration_id;
SELECT migration_id, count(*) AS pending_records, min(created_at) AS oldest_pending
FROM data_migration_live_journal WHERE projected_at IS NULL GROUP BY migration_id;
SELECT id, tenant_id, domain, phase, paused, applied_watermark, last_error
FROM data_migrations ORDER BY updated_at DESC;
```

The integration entrypoints are `online_session_migration_test.go` and
`online_dataplane_migration_test.go` under `test/integration`. They exercise
production decorators and coordinator hooks against PostgreSQL, Redis,
Qdrant and MinIO. Current execution evidence and environment limits belong
in [ACCEPTANCE_EVIDENCE.md](ACCEPTANCE_EVIDENCE.md).

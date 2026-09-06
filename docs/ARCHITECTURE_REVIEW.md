# Enterprise Multi-Tenant Agent Platform Architecture Decisions

## 1. Design Focus

The platform separates durable coordination from stateless Agent execution:

```text
Gateway -> Inbox -> Consumer -> Worker -> Runner -> Session/Memory -> Outbox -> Delivery
```

Each module owns an enforceable invariant. PostgreSQL owns queue state,
execution fences, immutable versions and audit records; the selected tRPC
backend owns Session/Memory; Qdrant owns Knowledge vectors; S3/MinIO owns
Artifact content. The control plane publishes references and immutable
configuration instead of duplicating data-plane state.

This review explains those ownership decisions and their trade-offs. The
system sequence is documented in [ARCHITECTURE.md](ARCHITECTURE.md), validation
results in [ACCEPTANCE_EVIDENCE.md](ACCEPTANCE_EVIDENCE.md), and deployment
prerequisites in [EXTERNAL_ACCEPTANCE_RUNBOOK.md](EXTERNAL_ACCEPTANCE_RUNBOOK.md).

## 2. Module Decisions

| Module | Responsibility and invariant | Design trade-off |
| --- | --- | --- |
| Gateway | Verify provider identity, normalize routing, enforce size/rate limits, and acknowledge only after Inbox commit. Duplicate source identities retain their original authoritative route. | Durable acknowledgement adds one database transaction to ingress latency; in return, acknowledged work survives process loss. |
| Inbox | Own idempotency, per-session FIFO, lease/fence, retry, DLQ and replay. Complete Inbox and create its unique Outbox in one transaction. | A failed or uncertain predecessor pauses its session to preserve causality; unrelated sessions continue. |
| Queue scheduling | Use weighted virtual runtime, transactional `max_inflight` checks and atomic `max_queued` admission. Maintain expiry in a bounded `SKIP LOCKED` loop outside Claim. | Fair scheduling adds schedule-row contention. Its rollout requires the same schema/capability on every Gateway and Consumer, and capacity tests must include hot tenants. |
| Consumer | Claim durable work, validate its authoritative identity, invoke Worker over signed transport, classify the outcome, and commit completion. | Keeping execution and provider delivery outside Consumer preserves independent fault and scaling boundaries. |
| Worker | Resolve immutable versions, hold a session lease, enforce an execution deadline, reuse bounded Runner instances, and persist result/execution records. | A timeout before Runner starts is retry-safe; after execution starts it becomes reconciliation work because external effects may already exist. |
| Runtime registry | Bind LLM, Chain, Graph, Parallel and Cycle factories to capability identities; validate and freeze the installed registry before serving. | Publication and execution require matching capabilities. This rejects incompatible deployments before they can silently run a different implementation. |
| Runner governance | Intercept actual Tool execution through the Runner Plugin for allowlists, audit, content policy, masking, token budgets and durable approval. | One enforcement point keeps approval and audit semantics consistent across built-in and MCP tools. |
| Session and Memory | Use tenant-scoped backend profiles, canonical Session identity, group owner scope and actor-scoped Memory. | Shared backends remove sticky-session requirements; their availability becomes an execution prerequisite. |
| Summary | Read authoritative committed events, generate under the pinned Agent version, publish a fenced checkpoint, and overlay it into the next Runner session. | Asynchronous generation avoids blocking the normal response; sequence and cutoff checks keep delayed work from regressing context. |
| Knowledge and Artifact | Use tenant-scoped Qdrant IDs and immutable SQL/S3 artifact versions with hash checks and tombstones. | SQL, vector and object stores have different commit boundaries; projection journals, idempotency and validation provide recoverable convergence. |
| Delivery | Persist `DISPATCH_STARTED` before each provider call, track a segmented cursor, classify provider errors, and reconcile uncertain sends. | Delivery is at-least-once. Explicit resume can repeat an uncertain fragment when the provider has no idempotency support. |
| Admin | Enforce server-side Principal, role and tenant scope; publish immutable snapshots; apply configuration CAS and transactional audit. | Authentication can be supplied by an operator-owned verifier while authorization remains inside the platform. |

## 3. Consistency and Recovery Decisions

### Authoritative Time and Ownership

Lease eligibility uses PostgreSQL `clock_timestamp()` when a decision follows
a possible row-lock wait. Ordinary audit timestamps can retain transaction
time. Every completion, renewal and retry checks status, owner, monotonically
increasing fence and expiry together.

The Redis session lease serializes the full Runner lifecycle and cancels
execution when ownership is lost. Its UUID represents ownership, while
PostgreSQL `lease_version` supplies the durable monotonic fence. These are
separate contracts rather than interchangeable tokens.

### Unknown Outcomes

The Consumer conservatively marks a request as possibly dispatched when
`httptrace.GotConn` acquires a connection. DNS or connection-setup failures can
retry; a transport failure after connection acquisition moves work to
reconciliation. `WroteRequest` can arrive after `Do` returns, so the absence of
that callback does not prove that the Worker did not execute. Malformed
execution proof, a post-Runner timeout or an execution heartbeat loss also
requires reconciliation. Delivery uses the same connection evidence together
with its durable pre-dispatch marker and cursor commit.

Result caching avoids a repeated model call after a committed response.
Business tools still require target-system idempotency keys, because a local
result record cannot undo an already accepted external effect.

### Bounded Maintenance and Observability

Consumer and Delivery run one expiry-maintenance loop per process, with
bounded batches and dedicated partial indexes. Queue inspection publishes
depth and oldest-age metrics for automatic states; a failed inspection keeps
the last valid sample and increments its failure counter. Dead-letter metrics
count successful state transitions rather than attempted mutations.

### Migration and Cutover

Snapshot and catch-up use persisted cursors, monotonic watermarks, target-side
idempotent projection and an owner fence. Cursor reset is valid only at the
defined snapshot/dual-write/catch-up phase boundaries. Projection markers
include migration identity, so a later migration cannot reuse another
migration's completion proof. Cutover requires validation and a configuration
CAS; the rollback window retains the source and incremental journal.

Session migration uses connection-level PostgreSQL advisory locks. Supported
connections are direct PostgreSQL or PgBouncer session pooling; transaction
and statement pooling do not preserve that ownership contract.

## 4. Security and Configuration Decisions

- AES-GCM `enc:v2` envelopes authenticate both tenant identity and stable
  credential field. Unversioned and `enc:v1` envelopes remain readable for
  stored-data compatibility; writes and rewraps use `enc:v2`.
- Secret references are scoped by tenant, purpose, provider and model. The
  environment resolver accepts only `env://TRPC_SECRET_*` and emits value-free
  errors. Gateway/Delivery resolve only their selected Channel binding;
  Worker/Summary Worker receive only their required model/data-plane secrets.
- Tenant configuration stores operator-owned profile IDs instead of DSNs or
  arbitrary provider URLs. Production PostgreSQL profiles require
  `sslmode=verify-full`.
- Admin derives authorization and audit actors from server-side Principals.
  An injected OIDC/IAP/mTLS verifier must validate its identity proof before
  returning a Principal; client identity headers are not trusted.
- Approval grants bind the exact tenant, invocation, actor, owner, session,
  tool and canonical arguments. Queue resume consumes the durable grant once;
  HTTP responses expose challenge identifiers instead of capability tokens.
- Consumer production transport requires HTTPS. Development HTTP and mesh
  application hops have explicit modes; mesh mode requires verified peer
  authentication in the deployment.
- Attachment handling passes validated references to the model provider and
  does not download them inside Worker. MCP addresses and headers are
  operator-owned, with precise Tool allowlists and governed execution.
- Prometheus labels use bounded allowlists. Durable trace context links the
  request stages without exposing raw payloads, secrets or user identities.

## 5. Compatibility and Operational Constraints

The durable FIFO key is `(tenant, agent app, session_id)`. Gateway generates
canonical session IDs containing the direct-message subject or group
conversation. In-process compatibility callers that supply the same session
ID for different subjects can cause head-of-line blocking; they must preserve
the same identity rule. Changing this partition key requires a coordinated
schema and group-ordering migration.

Custom runtime factories use `RegisterWithCapability` to supply a stable
implementation identity. Type-only registration is restricted to compatible
in-process use; strict Admin/Worker publication does not accept an unidentified
custom implementation. A process-local key-ring operation is likewise distinct
from a coordinated multi-replica key rotation: deployment operators need key
epochs, staged rollout and resumable rewrapping.

Tools must cooperate with cancellation. Untrusted or cancellation-ignoring
tools require a killable process/container execution boundary. Capacity sizing
must account for process connection pools, HPA expansion, model latency,
provider quotas and hot-session serialization.

These operational constraints are captured as concrete failure modes and
owners in [RISK_REGISTER.md](RISK_REGISTER.md). Environment-specific identity,
HA/DR, network, capacity and provider checks follow the linked deployment
runbook.

# Production Architecture Review

> Status: historical review record. The current implementation and acceptance
> state are authoritative in `docs/ACCEPTANCE_EVIDENCE_V14.md`,
> `docs/PROJECT_SUMMARY_V14_20260905.md` and `docs/ARCHITECTURE.md`. Statements
> below describing missing adapters or unexecuted scenarios refer to the
> review checkpoint at the time this file was written and must not override
> newer evidence.

## Scope and evidence

This review follows one durable request path:

```text
Gateway -> Inbox -> Consumer -> Worker -> Runner -> Session/Memory -> Outbox -> Delivery
```

It reviews the source snapshot, unit and race tests, and configuration
manifests. It does not treat an interface, a manifest, or a design diagram as
proof that an external system is deployed. In particular, no conclusion here
claims a live PostgreSQL/Redis failover, a real IM sandbox, KMS/Vault,
service-mesh mTLS, Kubernetes rollout, or capacity test unless that scenario
has separately been executed and recorded.

The decision labels have a precise meaning:

- **Retain**: the module owns a real invariant and has a focused test surface.
- **Simplify**: remove a misleading behavior or move periodic work out of a
  request hot path; do not replace it with another pass-through layer.
- **Do not add**: a proposed abstraction has no concrete adapter or cannot
  preserve the stated invariant yet.
- **Missing**: the requirement is valid, but this source snapshot has no
  end-to-end evidence for it.

## Current implementation reconciliation

This document is a design review, not a historical implementation snapshot.
The current source has since closed the Summary, Knowledge, Artifact, and
projection paths described below: the corresponding PostgreSQL/Redis/Qdrant/
MinIO vertical slices are `LOCAL_VERIFIED` in the acceptance matrix. The
remaining gates are deployment-specific (external IM sandboxes, production
KMS/Vault, HA/DR, capacity, and service-mesh rollout) and must not be confused
with missing source modules.

## Layer-by-layer decision record

| Layer | Retain | Simplify or remove | Missing / production acceptance gate |
| --- | --- | --- | --- |
| Gateway | Provider verification before durable acceptance, canonical routing, payload hash idempotency, body/depth limits, and fail-closed Redis admission. | Do not silently truncate user text. Oversized extracted content is rejected with 413 before Inbox. Rate-limit authenticated sources before authorization so rejected identities cannot amplify audit writes. | A trusted ingress identity policy and pre-auth edge/WAF budget are still required. `RemoteAddr` alone is not a safe client identity behind an arbitrary proxy. |
| Inbox | PostgreSQL is authoritative for idempotency, per-session ordering, leases, fences, DLQ, and replay. `CompleteInbox` plus unique Outbox insertion remains one transaction. A bounded `ReapExpired` CTE with partial indexes and `SKIP LOCKED` now terminalizes final attempts and approval expiry outside Claim. Queue inspection uses dedicated partial `(created_at, id)` indexes for automatic states. Optional migration 035 adds operator-owned schedule rows, atomic `max_queued` ingress admission, and a transactional weighted virtual-runtime fair claimer with `MaxInflight`; expired processing leases are excluded from the active count, and deleting an override resets the default rather than removing the schedule row. | Do not restore global expiry updates to Claim. Consumer and Delivery each run one maintenance loop per process, not one loop per claim worker. Keep fair scheduling opt-in until every Consumer/Gateway has migration 035 and a capability rollout. | `QueueInspector` remains global by design. Built-in Postgres/Memory admission is covered by unit/sqlmock tests; real multi-replica PostgreSQL fairness, lock contention, and quota-storm evidence remain required. |
| Consumer | Keep it as orchestration only: claim, validate authoritative Inbox identity, call Worker, then complete Inbox. Retain HMAC/nonce protection and error classification. | Do not place Agent runtime, routing authority, or direct provider delivery here. | Consumer-to-Worker needs a verified confidential transport in the real deployment. HMAC authenticates bytes but does not encrypt `http://` traffic. Startup must also fail before claims if the Worker endpoint is malformed or unavailable. |
| Worker | Immutable agent/deployment resolution, idempotency result storage, execution fencing, tenant storage checks, group Session owner versus actor Memory scope, bounded Runner caching, and a finite execution deadline are all justified. The deadline distinguishes retry-safe pre-Runner admission from a post-`Runner.Run` outcome that may have side effects. | Reject a runtime type at version admission when the Admin/Worker composition lacks its factory. Do not publish `graph`, `chain`, `parallel`, or `cycle` and hope a later Worker error explains it. | A Go Tool that ignores cancellation additionally needs a killable process/container boundary; a context alone cannot force-stop it. Production still needs kill/recovery evidence that the deadline releases leases and routes post-Runner timeouts to reconciliation. |
| Runner and governance | Native tRPC Runner with injected shared Session/Memory services, Plugin interception, audit, whitelist, content policy, masking, budget reservation, and durable approval are correct seams. Summary now has a real Session-event reader, sequence-aware enqueue, fenced checkpoint, and Runner-visible overlay in the local vertical slice. | Keep the Plugin as the only tool-enforcement seam. Delete inactive static-tool wrappers: they are not on the Runner path and can drift from approval/audit semantics. | Production deployment still needs external model/SLO evidence; this is an acceptance gate, not an absent Summary implementation. |
| Session and Memory | Tenant-scoped app names, group Session owner, actor-scoped Memory, backend-profile ownership, Redis coordination, and PostgreSQL execution fences should remain. Artifact and Knowledge have concrete PostgreSQL/MinIO/Qdrant adapters with tenant scope and hash/tombstone checks; Redis-to-PostgreSQL Session migration and projection cutover are covered by local vertical tests. | Tenant JSON must not carry DSNs or provider URLs. Keep it to operator-owned profile identifiers. | HA migration, cloud IAM, and production data-volume rehearsals remain external acceptance gates. |
| Outbox and Delivery | Retain atomic Inbox-to-Outbox creation, at-least-once semantics, cursor-based segmented replies, provider retry classification, pre-dispatch `DISPATCH_STARTED` fence, fenced state changes, DLQ, audited replay, and the shared bounded expiry reaper. Success mutations now reject `DELIVERING` and require `DISPATCH_STARTED` in both built-in stores. | Keep the contract honest: a provider-success/cursor-commit failure now parks the row for reconciliation; an explicit audited resume can still duplicate without provider idempotency. | Add provider sandbox contract tests and tenant backlog admission before declaring delivery SLOs. Fair Inbox claiming does not itself bound Outbox delivery or provider quota. Dead-letter metrics count only successful state transitions; expiry reaper distinguishes final-attempt and dispatch-unknown outcomes. |

## Cross-cutting security and operations decisions

### Retain

- **Database clock for lease authority**: PostgreSQL `now()` is fixed at
  transaction start, so it is not a safe authority for a decision made after a
  row-lock wait. All Inbox, Outbox, execution, result-commit, Summary and
  migration lease claims, renewals, expiry selections, retry deadlines and
  fenced mutations use `clock_timestamp()` where wall-clock freshness changes
  eligibility. Transaction timestamps remain appropriate for ordinary audit
  fields. SQL-shape regression tests cover this rule; a real PostgreSQL
  lock-wait scenario remains an integration acceptance gate.
- Tenant credentials are encrypted at rest and are now authenticated with
  AES-GCM associated data bound to `tenant_id` and a stable credential field.
  A ciphertext cannot be copied to another tenant or credential field and
  remain valid.
- The key-ring loader has a prefix-scoped `SecretResolver` seam. The built-in
  environment resolver accepts only `env://TRPC_SECRET_*` references and
  returns stable, value-free errors; this is a composition boundary, not a
  claim that an external KMS/Vault client is present.
- Legacy unversioned and `enc:v1` credential envelopes remain readable during
  migration; new tenant credentials and rewrapped credentials use `enc:v2`.
- Production PostgreSQL profiles require `sslmode=verify-full`; encrypted
  transport without server identity verification is not adequate.
- Admin authorization maps tokens to server-side Principals; client supplied
  identity headers must not become authorization or audit identity.
- Metrics use bounded tenant labels, and request/queue spans propagate a W3C
  trace context across durable boundaries.
- Worker attachment admission rejects userinfo, fragments, localhost, and
  literal non-routable IP targets. Worker does not download the URL; this is
  not an SSRF-safe downloader. A future download-capable adapter still needs
  DNS pinning, per-hop redirect checks, bounded responses/timeouts, and an
  operator-owned egress allowlist.

### Simplify or remove

- The online `RotateEncryptionKey` method must not be described as a
  multi-replica key rotation protocol. It mutates a process-local active ring
  before all rows are durable. Replace it with a coordinated KMS/Vault key
  epoch and a resumable migration worker before exposing it in a multi-node
  control plane.
- The web Admin approval-grant response should not return a raw capability
  token when the durable queue resume path consumes the server-side grant by
  exact scope. Keeping a secret that no normal queue client needs enlarges the
  disclosure surface.
- Each workload must receive a least-privilege database role and Secret. A
  shared `db-credentials/url` is not tenant isolation or blast-radius
  isolation.

### Missing acceptance evidence

- mTLS or an equivalent confidential service identity between Consumer and
  Worker, with a policy that rejects plaintext in production.
- The Consumer source now enforces that boundary before claiming Inbox work:
  production requires `https://`; local HTTP requires explicit development
  mode; mesh mode requires an explicit operator mTLS assertion. This is a
  source-level fail-closed gate, not evidence that the current Worker serves
  TLS or that a mesh is installed.
- Real PostgreSQL and Redis multi-replica failure injection: lease expiry,
  stale commit rejection, process termination during Inbox completion and
  Outbox cursor persistence, and recovery after transient datastore loss.
- Telegram and WeCom sandbox contract tests for signatures, retry behavior,
  reply limits, permanent/transient error classification, and verification that
  provider request logging never records the WeCom `corpsecret` query.
- Connection-pool sizing evidence. Current manifests can allocate roughly a
  thousand PostgreSQL connections before storage adapters and HPA expansion;
  set one shared process pool and role-specific maxima from measured load.
- Capacity data with workload, environment, command, p50/p95/p99, queue lag,
  DB/Redis QPS, connection wait, CPU/memory, error rate, and token cost.

The Admin authentication package also exposes an explicit
`PrincipalResolver` seam for an operator-owned OIDC/IAP/mTLS verifier. The
built-in bootstrap/scoped bearer credentials remain intentionally available
for local and emergency operation; production deployments should inject a
verifier through this seam and disable long-lived bootstrap access after
provisioning. The resolver must validate signature/issuer/audience/expiry and
the mTLS identity before returning a server-side Principal. No client-supplied
identity header is trusted by the platform.

## Changes made in this review pass

1. **Credential binding**: new `enc:v2` envelopes bind AES-GCM authentication
   to tenant and stable credential identity. Regression tests reject
   cross-tenant and cross-field ciphertext transplantation while retaining
   v1/legacy reads.
2. **Verified database TLS**: a non-development PostgreSQL backend profile
   only accepts `sslmode=verify-full`.
3. **Gateway correctness and abuse resistance**: oversize text is rejected,
   authenticated unauthorized sources are budgeted before authorization, and
   duplicate denial audits are coalesced by provider event identity.
4. **DLQ observability**: Consumer and Delivery dead-letter counters increment
   only after the fenced store transition succeeds.
5. **Runtime admission truthfulness**: version admission checks the installed
   runtime registry. The default composition contains only the built-in `llm`
   factory, so unavailable runtime types fail before they can enter Inbox work.
6. **Bounded expiry maintenance**: final Inbox/Outbox lease expiry and approval
   expiry are no longer global updates inside Claim. Both durable adapters
   expose the optional `ExpiredWorkReaper` seam; PostgreSQL uses bounded
   `SKIP LOCKED` CTEs and MemoryStore keeps matching state-machine semantics.
   Consumer and Delivery start one loop per process, with an explicit batch
   and interval in Compose/Kubernetes. Unit/race coverage exists; the real
   PostgreSQL integration scenario is compiled but not executed in this
   environment because Docker is unavailable.
7. **Outbox pre-dispatch fence**: Delivery now durably transitions a claimed
   row to `DISPATCH_STARTED` before invoking a provider. Pre-dispatch expired
   claims remain reclaimable; marked expired claims move to
   `WAITING_RECONCILIATION` and require audited replay. PostgreSQL and Memory
   state machines share the same transitions and metrics.
8. **Execution deadline semantics**: a timeout before `Runner.Run` now marks
   the durable attempt safe to retry and returns retryable 503; once Runner
   may have started a model or Tool, timeout remains non-retry-safe and maps
   to 423 reconciliation. The Worker also stops waiting for an event stream
   that ignores cancellation and releases its invocation lease before return.
9. **Protocol outcome safety**: a budget-aware Consumer treats a successful
   Worker response whose execution proof or response body is missing or
   malformed as an unknown outcome and blocks the Inbox for reconciliation.
   Post-Runner errors and execution-lease heartbeat loss return 423 rather than
   retryable 500, so the HTTP classifier agrees with the durable
   `retry_safe=false` record. Production budget-client constructors also reject
   a nil service signer at startup.
10. **Governance seam truthfulness**: removed the inactive static
   `AgentWithGovernance` wrapper and its self-referential tests. The Runner
   Plugin is the sole production Tool interception seam and owns approval,
   audit and masking semantics.
11. **Migration checkpoint integrity**: CatchUp now persists and validates its
   own cursor instead of passing an empty cursor on every step. Memory and
   PostgreSQL migration stores reject watermark regressions and cursor resets
   outside the two explicit phase boundaries. A repeated non-terminal source
   batch now fails closed instead of being upserted indefinitely.
12. **Authoritative retry deadlines**: Consumer and Delivery no longer fall
   back to a node wall clock when a custom Store lacks `RetryAfterStore`.
   Retrying with an untrusted local deadline would make replicas disagree;
   such a Store now parks work for reconciliation. Built-in Memory and
   PostgreSQL stores implement the authoritative-clock contract.
13. **Tenant ingress budget**: Gateway's atomic Redis admission now enforces a
   tenant-wide fixed-window ceiling in addition to the per-user ceiling.
   Provider redeliveries still use the existing source marker and do not spend
   another tenant token. This is an ingress guard only; it does not replace a
   WAF/global edge budget or durable queue backlog admission.
14. **Operator-owned key references**: key-ring startup accepts mutually
    exclusive direct values or prefix-scoped `SecretRef` values, normalizes
    resolver failures, and clears resolver-owned byte buffers after copying.
    Existing `MASTER_KEY_RING`/`MASTER_KEY` deployments remain compatible.
15. **Scoped channel credentials**: Telegram/WeCom channel `TokenRef`,
    `SecretRef`, and `EncodingAESKeyRef` are resolved only on the selected
    Gateway/Delivery binding. Ingress and egress reject tenant services that do
    not expose the scoped reader capability instead of falling back to a broad
    credential-bearing read.
16. **Closed channel configuration**: installed adapters currently consume only
    `account_id`, `corp_id`, and `encoding_aes_key`. Unknown `Channel.Config`
    keys are rejected before persistence; Admin responses also mask historical
    secret-like keys and preserve them on redacted PUT updates.
17. **Migration phase invariant**: an empty cursor may be introduced only on
     `SNAPSHOT_COPY -> DUAL_WRITE` or `DUAL_WRITE -> CATCH_UP`. A legal
     `CATCH_UP -> CATCH_UP` heartbeat cannot reset the cursor and rescan the
     change stream.
18. **Runtime capability binding**: Admin admission records a stable fingerprint
    of the installed runtime capability set in new immutable snapshots. Worker
    construction rejects a non-empty snapshot fingerprint that differs from its
    registry. Admin and strict production Worker admission reject a type-only
    registration for non-built-in runtimes; custom factories must provide an
    operator-owned implementation or build identity through
    `RegisterWithCapability`. The compatibility `Register` form remains for
    legacy in-process callers and historical built-in fingerprints only.

## Production exit criteria

The source-level platform foundation is implemented and locally verified. It is
not production-certified until the deployment owner supplies reproducible
evidence for the remaining external gates above: confidential service
identity, HA/DR and capacity, KMS/Vault workload identity, and real IM
sandboxes. Those gates validate environment and operations; they do not imply
that the Summary, Artifact, Knowledge, or migration source paths are absent.

The Fable design was used as a review input, not as implementation evidence.
Its Organization/Project hierarchy, external SecretRef resolver, replica-aware
router, Pub/Sub invalidator, concrete Redis-to-PostgreSQL adapter, physical
tenant pools, and TLS OTLP collector remain design or deployment-owner work
until a real adapter and reproducible acceptance test exist. In particular,
the current tenant model uses application-layer AES-GCM envelopes; this is not
an external KMS/Vault SecretRef integration and must not be presented as one.

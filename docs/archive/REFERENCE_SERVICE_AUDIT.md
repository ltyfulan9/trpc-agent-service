# Reference service audit

Audit date: 2026-08-24 (Asia/Shanghai)

This note compares the first-party reference repository
[`liuzengh/trpc-agent-service`](https://github.com/liuzengh/trpc-agent-service)
with this archive. The reference was read at `HEAD`
`aa000c8407dcd6ea7788fcdccde9574b44bbe2d2` (resolved with `git ls-remote`),
and the local comparison is against the source tree in this directory. No
container, external service, or credential was used.

## Executive finding

The reference repository is a requirements document and an intentionally
minimal scaffold, not an implemented multi-tenant platform. Its README says
the task is primarily architecture design and that a complete system is not
required (`README.md:13-18`); it also calls the tree below an illustrative
directory
(`README.md:108-110`). The only executable entry point prints metadata and
returns. There is no tRPC-Agent-Go dependency, listener, Runner, storage,
channel callback, control plane, telemetry pipeline, migration, deployment
manifest, or test implementation in that repository.

This archive therefore should be evaluated against the reference's acceptance
requirements, not against a competing production implementation. The archive
already goes materially beyond the reference in runtime behavior, while the
remaining gaps below are real boundaries and should not be filled with
placeholder APIs.

## First-party reference facts

| Claim | Evidence at reference HEAD |
|---|---|
| Scope is an architecture exercise; full implementation is explicitly not required | `README.md:13-18` |
| The proposed `cmd`/`trpcservice` layout is illustrative, not a required implementation tree | `README.md:108-110` |
| The README lists tenant, backend, IM, governance, observability, recovery, and deployment requirements | `README.md:21-74` and acceptance checklist `README.md:85-105` |
| Module has no framework dependency | `go.mod:1-3` contains only the module path and `go 1.21`; there is no `require` block and no `trpc-agent-go` import |
| The only command is a metadata CLI | `cmd/trpc-service/main.go:3-16`; `main` prints `trpcservice.Version` and a description, optionally prints usage, then returns |
| The advertised packages are package comments only | `trpcservice/agent/agent.go:1-3`, `channels/channels.go:1-3`, `config/config.go:1-2`, `log/log.go:1-2`, `metrics/metrics.go:1-2`, `skill/skill.go:1-2`, `tenant/tenant.go:1-2`, `tool/tool.go:1-2`, `web/web.go:1-2`, `workspace/workspace.go:1-2` |
| Version package contains no runtime behavior | `trpcservice/version.go:1-5` |
| Build/start scripts do not create a service | `build.sh:4-9` only builds the metadata CLI; `start.sh:7-20` runs it with `nohup`, so the process exits immediately; `stop.sh:4-19` only removes the PID file |
| No implementation/deployment artifacts are present | The complete tree at this commit has no Dockerfile, Compose, Kubernetes/Helm, migration, CI, or Go test file. `docs/README.md:1-8` only asks for future design artifacts |

The reference's README statements about existing tRPC-Agent-Go capabilities
(`README.md:5-7`, `README.md:31-32`, `README.md:44-56`) are requirements and
reuse guidance. They are not evidence that those capabilities are implemented
in the reference repository.

## Capability comparison

| Requirement area | Reference repository | This archive: source evidence | Assessment |
|---|---|---|---|
| Tenant model and isolation | README/schema guidance only | `pkg/tenant/tenant.go:29-58` models tenant, agents, models, policies, channels, storage and budgets; `pkg/tenant/service.go:278-377` encrypts configured secrets; `docs/ARCHITECTURE.md:94-107` describes scope checks and redaction | Implemented platform layer; KMS/Vault remains an external production dependency |
| Agent execution | No Runner or tRPC dependency | `pkg/worker/worker.go:124-196` creates an agent, injects shared Session/Memory services, and installs the governance plugin; `cmd/worker/main.go:208-377` exposes the authenticated process path | Real execution path exists; current model factory/provider scope is intentionally limited |
| Shared Session/Memory backends | No interfaces or adapters | `pkg/storage/backend_factory.go:27-188` constructs tRPC InMemory, Redis, and PostgreSQL Session/Memory services; `pkg/storage/adapter_impl.go:41-57,192-227` routes and owns them | Real tenant-selected adapters for the installed backends; not a generic Redis/SQL/vector/object-store matrix |
| Durable ingress and idempotency | No webhook or queue | `pkg/gateway/server.go:64-245` verifies/parses callbacks and persists Inbox; `pkg/reliable/postgres.go:45-298` implements unique payload-hash checks, leases, retries, DLQ and replay | Strong differentiator; PostgreSQL is the durable boundary |
| Worker concurrency/recovery | No worker | `pkg/reliable/postgres.go:99-183,303-385` uses lease versions and expiry; `pkg/worker/worker.go:359-432` coordinates the full session invocation lease; `docs/ARCHITECTURE.md:74-92` records exact recovery semantics | Implemented for the covered path; external tool side effects still need their own idempotency keys |
| Outbox delivery | No adapter or delivery process | `pkg/pipeline/delivery.go:31-203` claims, sends, classifies errors, advances segmented cursors and marks delivery; `pkg/reliable/postgres.go:388-511` fences state and audits replay | Implemented at-least-once delivery; exactly-once provider behavior is not claimed |
| IM channels | Package comment only | `pkg/channel/adapter.go:21-54` defines the adapter contract and includes WeCom/Telegram types; concrete adapters are wired in `cmd/gateway/main.go:77-80` and `cmd/delivery/main.go:42-45`; session IDs are hashed in `pkg/channel/adapter.go:104-129` | Two real primary channels; WeChat service/MP constants do not imply adapters |
| Governance and audit | Package comment only | `pkg/governance/plugin.go` hooks Runner Before/AfterTool; `pkg/governance/approval_postgres.go` provides durable challenge/grant/consume; `pkg/telemetry/telemetry.go` defines non-sensitive audit records; `pkg/auth/service.go` verifies body-bound HMAC and one-time Redis nonces | Implemented fail-closed controls for the covered hooks and PostgreSQL approval workflow; KMS/Vault remains an external production dependency |
| Control plane and rollout | No API or state | `pkg/controlplane/service.go:40-220` performs transactional app/version/deployment changes; `pkg/controlplane/resolver.go:105-247` resolves stable/canary and pins idempotency keys | Implemented stable/canary/pinning path |
| Observability | No metrics/tracing code | `pkg/telemetry/telemetry.go:27-60` exposes tenant metrics; `pkg/telemetry/otel.go:18-87` configures W3C/OTLP propagation; `docs/VERIFICATION.md:35-36` maps async trace propagation | Wiring exists; exporter/backend availability must be verified in deployment |
| Deployment | No manifests | `deploy/docker-compose.yml` and `deploy/kubernetes/*` provide Compose/Kubernetes artifacts; `scripts/validate.sh` defines local build, vet, tests and deployment checks | Artifacts exist, but this source-only package has no repository CI policy; local verification explicitly says Docker/real DB integration were not run (`docs/VERIFICATION.md:17-20,65-73`) |

## High-value, bounded next work

These are the most valuable gaps that follow directly from the requirements and
current source boundaries. They are deliberately narrower than adding every
possible backend or IM provider.

1. **Run the existing infrastructure gates in CI or a disposable environment.**
   The source-level and unit evidence is useful, but Compose health, image
   builds, Prometheus rule validation, and PostgreSQL integration remain
   unverified (`docs/VERIFICATION.md:48-73`; `scripts/validate.sh`).
   Record command output and versions rather than claiming success from static
   files.
2. **Complete the deployment hardening check.** At the audit snapshot,
   `deploy/docker-compose.yml:18-26` started Redis without ACL/TLS and published
   `6379:6379` (`:21-22`). Redis stores nonce, lease, rate-limit and budget
   control state (`pkg/auth/service.go:35-46`, `pkg/gateway/server.go:258-290`).
   The default Compose profile should keep Redis on the internal network (or
   require explicit authentication and restricted binding); retain a regression
   assertion so a future edit cannot re-expose it.
3. **Bind the Summary coordinator to a production Session generator.**
   This archive now has a bounded `pkg/summary` Processor, PostgreSQL job store,
   lease renewal, retry limits and CAS checkpoint sink with stale-job tests.
   It intentionally leaves the event snapshot/model adapter explicit: wiring a
   concrete tRPC Session backend and summarizer still requires a contract test,
   and a schema-only deployment must not be advertised as generated summaries.
4. **Keep expanding migration adapters only with live evidence.** The archive
   now has a bounded copy/dual-write state machine with checkpoint, fencing,
   pause/resume, validation, shadow-read and operator-controlled rollback
   hooks (`pkg/datamigration`, `docs/ARCHITECTURE.md:133-154`). Concrete Redis,
   SQL, vector and object-store adapters still require their own contract and
   integration tests; the generic executor must not be treated as proof that
   every backend is supported.
5. **Keep Knowledge/Artifact/vector/object-store work behind explicit adapters.**
   The current factory intentionally supports only InMemory/Redis/PostgreSQL
   Session and Memory (`pkg/storage/backend_factory.go:54-188`; `go.mod:18-23`).
   Do not add enum values that silently route to an unimplemented backend.
6. **Keep provider scope honest.** The README records that the current model
   factory is OpenAI-only and custom endpoints are rejected (`README.md:141-152`).
   A new provider should arrive with transport policy, credential handling,
   timeout/cancellation behavior and contract tests, not just a switch case.

## Non-goals and evidence rules

- Do not copy the reference package comments as implementation.
- Do not call the archive production-ready solely because the reference is a
  scaffold, or because a local unit test run is green.
- Do not describe WeChat service/MP, vector stores, object storage, external
  Memory, KMS/Vault, mTLS, capacity limits, or concrete migration backends as
  implemented without a running-path test and post-state evidence.
- Preserve the explicit at-least-once delivery and tool-level idempotency
  boundaries documented in `docs/ARCHITECTURE.md:82-92`.

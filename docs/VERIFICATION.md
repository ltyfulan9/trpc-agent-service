# Verification Baseline

Updated: 2026-09-06 (Asia/Shanghai)

This is the current verification baseline for Enterprise Multi-Tenant Agent Platform.
The item-by-item matrix is authoritative in [ACCEPTANCE_EVIDENCE.md](ACCEPTANCE_EVIDENCE.md);
architecture contracts are in [ARCHITECTURE.md](ARCHITECTURE.md) and [DATA_MODEL.md](DATA_MODEL.md).
Historical machine-repair notes and intermediate runs are retained only under archive/.

## Evidence vocabulary

| Status | Meaning |
|---|---|
| `LOCAL_VERIFIED` | Current source or a reproducible local fixture passed with exit code and post-state. |
| `IMPLEMENTED` | Production path and automated regression exist; target account/infrastructure is still needed. |
| `EXTERNAL_REQUIRED` | Requires target identity, network, provider, or operator action; tests and fakes cannot upgrade it. |

Every result must be scoped by date, command, environment, and evidence source. A later
code change requires rerunning the affected gate.

## Current source gate

Run from a fresh checkout or extracted `platform-source` directory. The source archive
does not include Git metadata, so VCS stamping is disabled.

```bash
go version                         # production baseline: go1.26.7
go mod verify
test -z "$(gofmt -l cmd pkg migrations test)"
go build -buildvcs=false -p 1 ./cmd/...
go vet -p 1 ./...
go test -buildvcs=false -count=1 -p 1 ./...
go test -buildvcs=false -race -count=1 -p 1 ./...
go test -buildvcs=false -tags=integration -count=1 -p 1 ./test/integration
bash ./scripts/static_verify.sh
docker compose -f deploy/docker-compose.yml config
```

`scripts/validate.sh` is the aggregate gate when Go, Bash, and Docker are available.
The module compatibility floor is Go 1.25.14; production build/security gates use Go
1.26.7. Record each stage independently; a shell parse is not a runtime validation.

## Windows C-drive path

```powershell
Set-Location <source-root>
.\scripts\run_c_local_stack.ps1 -ProjectName agent-platform-c-local -Build
bash ./scripts/validate.sh
.\scripts\run_c_local_stack.ps1 -ProjectName agent-platform-c-local -Down
```

The helper resolves the root from `$PSScriptRoot`, injects one-time process values and
uses isolated ports. The aggregate Bash gate requires a working Bash and Docker
installation; when Bash is unavailable, run the PowerShell-equivalent static checks
and the Go gates separately, and record that limitation. The helper does not read or write E:.
Compose without an operator `.env`
must fail closed. Real `deploy/.env.wecom.local` and provider credentials are excluded.

## Functional contracts

- Gateway verifies and normalizes WeCom/Telegram callbacks, commits a tenant-scoped
  Inbox before acknowledgement, and fixes reply routing at ingress.
- PostgreSQL owns Inbox/Outbox idempotency, payload conflict detection, session FIFO,
  leases, monotonic fences, retries, DLQ, and audited replay.
- Worker resolves immutable AgentVersion/deployment, holds a renewable Session lease,
  and persists results under tenant/owner scope. Unknown side effects enter reconciliation.
- Admin RBAC, tenant allowlists, SecretRef purpose/provider/model bindings, and
  Worker-only credential resolution fail closed.
- Built-in `llm`, `chain`, `graph`, `parallel`, and `cycle` factories validate
  topology, budgets, allowlists, and bounded iteration. Custom factories bind capability
  fingerprints to immutable versions.
- Summary freezes an exact event boundary and publishes a fenced checkpoint. If an upstream
  sliding window cannot prove absolute sequence, it returns `ErrTranscriptIncomplete`.
- Qdrant, S3/MinIO, Redis-to-PostgreSQL Session projection, and migration markers are
  tenant/app scoped; projection markers include migration identity.
- Governance Plugin is the single Tool admission point for approval, budget, redaction,
  and audit. HMAC binds method/path/body/trace context and consumes one-time nonces.
- Compose/Kubernetes use non-root, read-only roots, dropped capabilities, default-deny
  policy, and digest-pinned rendered releases; `releaseverify` rejects mutable images.

## Local backend evidence

`test/integration` uses disposable PostgreSQL, Redis, Qdrant, and MinIO and a local
OpenAI-compatible model fixture. It proves queue fencing, Session projection, Knowledge,
Artifact, Summary-to-Runner, and MCP governance for that fixture. It does not prove a
cloud provider, real IM account, HA failover, or production capacity.

C-local Compose additionally checks migration completion, health probes, Prometheus
targets/rules, and restart state when Docker is available. Save command output and
container post-state with a dated evidence log; historical logs apply only to the exact
source and environment recorded.

## External acceptance gates

1. WeCom/Telegram URL verification, encrypted callback, duplicate delivery, retries, and real reply.
2. Production OIDC/IAP/mTLS, KMS/Vault identity, least privilege, rotation, and audit sink.
3. Kubernetes/service-mesh rollout, rollback, strict peer identity, certificates, and private egress.
4. PostgreSQL/Redis failover, PITR/DR, reconciliation, migration rollback window, and RPO/RTO.
5. Real payload capacity, provider latency, throughput, queue lag, resources, and cost.
6. Business MCP authentication, allowlist, quota, idempotency, timeout, and SLA.

Use [EXTERNAL_ACCEPTANCE_RUNBOOK.md](EXTERNAL_ACCEPTANCE_RUNBOOK.md) in its specified
order. Evidence may contain timestamps, exit codes, digests, trace IDs, and redacted status
only; never store secrets, cookies, database URLs, JWTs, or message bodies.

## Delivery reproducibility

Run `scripts/package_all_materials.ps1` after the final commit. It recomputes inventory
and SHA-256, excludes archive/runtime data/credentials/binaries/nested archives, then
performs fresh extraction and secret-signature checks. Package counts are inventory metadata,
not acceptance evidence. The package inventory, SHA-256 list, and build result must agree.

The public repository must expose the same final commit as the package. Record
`git rev-parse HEAD` from a fresh clone; a stale default branch is a reproducibility failure.

## Historical records

Earlier dated logs under archive/ may mention older Go versions, Docker repairs, temporary
registry names, K3d/Vault experiments, or incomplete source states. They must not override
the current matrix or be used to claim external or production acceptance.

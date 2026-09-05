# tRPC-Agent Enterprise Handoff V13

> 历史归档：本文件不代表 V14 当前状态。请从 `README.md` 与 `TRPC_AGENT_ENTERPRISE_HANDOFF_V14_FINAL.md` 开始评审。

Snapshot date: 2026-08-29  
Source tree: `trpc-agent-enterprise-v13`  
Baseline: V12 combined source, preserved separately  
Nature: source-level enterprise platform candidate; **not** a production certification or ranking guarantee

## 1. Why V13 exists

V12 already had a strong reliable Inbox/Outbox, multi-tenant control plane, shared tRPC Session/Memory, governance Plugin, WeCom/Telegram adapters, deployment templates and extensive tests. V13 focuses on the issues a top architecture review would challenge immediately:

1. local validation could run on an unsafe Go patch version even though CI/images were pinned;
2. the Redis→PostgreSQL integration test started `miniredis` despite CI exposing a real Redis service;
3. multi-replica crash takeover was described by lower-level tests but lacked one end-to-end PostgreSQL scenario with two independent pools, stale completion rejection and exactly one Outbox;
4. the submission lacked a short judge-facing architecture, exact evidence matrix, formal security report and owned risk register.

## 2. V13 code changes

- Added `scripts/require_secure_go.sh`. Production validation requires stable Go 1.26.7 or newer. The module retains Go 1.25.14 as a separate source-compatibility contract; compatibility is not a production security claim.
- `scripts/validate.sh` is now 10 gates and starts isolated PostgreSQL **and Redis** containers. It passes `TEST_DATABASE_URL` and `TEST_REDIS_URL`, runs the full integration tag, and reports real backends explicitly.
- CI exports `TEST_REDIS_URL` and executes the secure Go gate in both the main and vulnerability jobs.
- `test/integration/datamigration_test.go` now requires a reachable real Redis server, verifies it with `PING`, uses a UUID prefix and cleans only that prefix. It never calls `FLUSHDB`.
- Added `TestPostgresReplicaTakeoverFencesCrashedWorkerAndCreatesOneOutbox`: Worker A claims with a short DB-clock lease and disappears; Worker B uses a separate `sql.DB`, reclaims with a higher fence, A's late completion is rejected, and B creates exactly one authoritative Outbox.
- Deployment contract tests now pin the new CI/toolchain requirements.

## 3. V13 submission documents

- `docs/COMPETITION_SUBMISSION_V13.md`: standalone architecture submission with architecture and sequence diagrams.
- `docs/ACCEPTANCE_EVIDENCE_V13.md`: every prompt requirement mapped to code, test and `VERIFIED/IMPLEMENTED/DESIGNED/EXTERNAL` status.
- `docs/RISK_REGISTER_V13.md`: 23 production risks with impact/probability, leading indicator, mitigation, owner and status.
- `docs/SECURITY_REVIEW_V13.md`: structured security findings with IDs, severity, locations, evidence, impact, remediation and false-positive notes.
- `docs/DATA_MODEL.md`: authoritative platform tables plus logical Session/Memory/Knowledge/Artifact contract.

## 4. Current reproducible verification

Executed on Windows/amd64 with `GOTOOLCHAIN=go1.26.7`, `GOMAXPROCS=1` and serial package scheduling where applicable:

```text
go mod verify                                                   PASS
gofmt -l cmd pkg migrations test deploy                         CLEAN
go build -buildvcs=false -p 1 ./cmd/...                         PASS
go vet -p 1 ./...                                               PASS
go test -buildvcs=false -count=1 -p 1 ./...                     PASS
go test -buildvcs=false -race -count=1 -p 1 ./...               PASS
go test -tags=integration -count=5 -p 1 ./test/integration      PASS (real PostgreSQL 15.8 + Redis 7.4)
bash ./scripts/static_verify.sh (inside bash:5.2 container)      PASS
promtool check rules deploy/prometheus-rules.yml                PASS (12 rules)
docker compose config + six application image builds            PASS
docker compose up --wait (11 services / 37 migrations)          PASS
govulncheck v1.7.0 ./...                                        No vulnerabilities found
```

The same source on the host-default Go 1.26.2 produced 14 reachable standard-library vulnerability findings. The new gate rejects that toolchain; switching to Go 1.26.7 removes those findings. This distinction is important: the production CI and builder images were already pinned to 1.26.7, while the missing local gate was the defect.

## 5. Infrastructure verification follow-up

Docker Desktop 4.71.0 initially failed before its engine became reachable:

```text
initializing Inference manager: listening on ...\Docker\run\dockerInference:
remove ...\dockerInference: The file cannot be accessed by the system
```

The immediate recovery did not delete any image, container, volume, WSL disk or project data. Docker was stopped and only the unrecoverable ephemeral parent directories containing `dockerInference` and `engine.sock` were renamed aside and recreated. A native `docker desktop restart` reproduced the same failure. The official `docker desktop disable model-runner` command and `EnableDockerAI=false` did not stop the manager from creating the socket. This matches Docker's still-open Windows reports [desktop-feedback#342](https://github.com/docker/desktop-feedback/issues/342) and [desktop-feedback#531](https://github.com/docker/desktop-feedback/issues/531).

Docker Desktop was upgraded in place to 4.88.1 build 237512; the installer recorded `Installation succeeded`, preserved the WSL 2 data backend, and installed Engine 29.7.2 plus Compose 5.4.0. The first 4.88.1 launch nevertheless reproduced the same stale-socket crash, so the upgrade is a security/maintenance improvement, not evidence that the upstream defect is fixed. The host now uses a narrow safe-start workaround outside this source package:

- Model Runner is disabled through the official CLI, `EnableDockerAI=false` remains set, and the working `ProxyHTTPMode=system` setting remains in force;
- `%LOCALAPPDATA%\Docker\safe-start\Start-DockerDesktopSafe.ps1` uses a per-user mutex and a three-second engine probe; it stops only Docker Desktop processes, renames only the two documented ephemeral runtime directories, recreates them, starts the signed Desktop executable and waits for `docker info`;
- the current-user `Docker Desktop` login entry points to the safe-start script, and a `Docker Desktop Safe Restart` Start-menu shortcut invokes it with `-Restart`;
- the script recovered 4.88.1 from the crash and then completed three consecutive safe restarts. The final restart preserved a purpose-built Redis AOF value and the PostgreSQL control-plane data, restored all ten long-running containers and produced no fatal socket match in the current backend log.

The login entry was inspected but a full Windows sign-out/reboot was not performed. Native Docker Desktop restart remains unsafe on this host until Docker fixes the open issue; the safe-start script is a repeatable workaround, not a vendor root-cause fix. Quarantined runtime directories remain recoverable under `AppData\Local\Docker` and `AppData\Local` and are not part of the source package.

The independent registry failure came from Docker Desktop being pinned to a stale manual proxy at `127.0.0.1:7897`. It was changed to `ProxyHTTPMode=system`; the engine then pulled the fixed official images successfully. All external runtime images are pinned in Compose by tag **and** observed Docker Hub digest.

Live acceptance then completed:

- six application images built with the digest-pinned Go 1.26.7 builder and Alpine 3.24.1 runtime;
- the integration suite passed once and then five consecutive times against isolated real PostgreSQL 15.8 and Redis 7.4 containers;
- the full 11-container stack became ready, migration 001 through 037 applied, and Gateway/Admin/Prometheus/Grafana host health returned HTTP 200;
- Admin failed closed with HTTP 401, created a tenant with all model/channel secrets redacted, then created an Agent App, immutable Version, publication and stable Deployment; PostgreSQL showed `1|1|1|1|5` tenant/app/version/deployment/audit rows;
- Redis AOF data and PostgreSQL tenant/deployment data survived container restarts; a dedicated Redis value also survived a complete safe Desktop restart and was then removed; Worker recovered healthy after a forced restart;
- Prometheus reported 5/5 configured application targets `up`; Grafana 11.1.4 reported `database=ok`;
- application and migration containers were recreated with UID/GID `65532:65532`, read-only root filesystems, `cap_drop: ALL`, `no-new-privileges`, non-privileged mode and a bounded `/tmp` tmpfs; data and health remained intact;
- all ten long-running Compose services use `restart: unless-stopped`; after the final safe Desktop restart, 10/10 were running, the five health-checked Go/data services were healthy, migration remained Exited(0), four host endpoints returned 200 and Prometheus remained 5/5.

Grafana emits two non-fatal startup lines because its bundled `xychart` plugin is registered twice. Health, its embedded database and all Prometheus targets remain good; this warning is retained as evidence rather than relabeled as a clean log scan.

## 6. Exact implemented boundary

- Bundled runtime: real `llm` → tRPC `LLMAgent`/`runner.Runner`.
- `chain`, `graph`, `parallel`, `cycle`: validated type names and fail-closed `RuntimeAgentRegistry` extension seam only. A concrete factory with capability fingerprint must be operator-installed; there is no silent LLMAgent fallback.
- Session/Memory: real tenant-routed Redis/PostgreSQL adapters; InMemory is test/explicit local only and rejected by production admission.
- Migration: concrete versioned opaque Redis source → fenced PostgreSQL target. Provider-specific Session/Memory projection, vector-store and object-store migration remain open.
- Knowledge/Artifact: architecture and logical contracts exist; no production data-plane consumer in this source package.
- IM: WeCom and Telegram adapters have extensive deterministic tests; no real provider sandbox run in this continuation.
- Local K3d/Linkerd/Vault-dev/OTLP-TLS/capacity mechanisms are now verified in section 8. Production HA KMS/Vault, vendor-supported mesh/Kubernetes, database least-privilege roles and failover, target workload capacity and disaster recovery remain target-environment acceptance items.

## 7. Recommended next sequence

1. Execute `docs/EXTERNAL_ACCEPTANCE_RUNBOOK.md`: target infrastructure and zero-call preflight first, then Telegram and WeCom sandbox callbacks/replies within the attempt budgets.
2. Repeat the locally proven digest rollout/rollback and mesh identity allow/deny on a vendor-supported target Kubernetes/Linkerd combination.
3. Replace the local Vault-dev seam with production workload identity and rotate a dual-key envelope without downtime.
4. Implement one real Session/Memory provider projection and one vector/object migration only after their actual runtime consumers are installed.
5. Repeat the capacity/fault matrix with business payload/model latency and publish p50/p95/p99, queue lag, DB/Redis QPS, cost and error-budget evidence; do not extrapolate the local 2,200-message baseline.
6. Until Docker closes the AF_UNIX issue, use `Docker Desktop Safe Restart` rather than native restart. Validate the configured login hook during the next planned Windows reboot; keep the quarantined runtime-only directories until a future Docker version passes native restart repeatedly, then revert the Run entry and retire the workaround deliberately.

Do not push or present this source as GitHub-published unless a remote, commit and CI run are separately created and verified.

## 8. 2026-08-30 local production-like continuation

The earlier target-environment list above is now partially closed by a local
three-node lab. Treat this section as newer evidence while retaining the exact
production boundary:

- K3d/K3s: one server and two agents Ready; compatible migration, immutable
  digest rollout and Gateway undo completed.
- Linkerd CNI/native sidecars: meshed same-request path reached application
  authentication (401); unmeshed/no-identity path was denied by Linkerd (403).
  NetworkPolicy now explicitly permits transparent proxy port 4143 and the
  required control-plane path.
- HPA uses `ContainerResource`; Gateway and Worker report
  `ScalingActive=True/ValidMetricFound` instead of sidecar-induced unknown
  Pod metrics.
- Vault chart 0.34.1/Vault 2.0.4 dev mode: scoped Kubernetes ServiceAccount
  read succeeded, default ServiceAccount read was denied. This is not HA
  Vault, auto-unseal, production KMS or key-rotation evidence.
- Correct OTLP CA succeeded and a rogue CA failed.
- A clean isolated PostgreSQL 15.8 + Redis 7.4 full integration run passed in
  176.984 seconds. Default unit, full race, vet, build and module verification
  passed; Go 1.27.0 also passed the full default suite without modifying
  `go.mod` or `go.sum`. Production images remain pinned to Go 1.26.7.
- The capacity harness found a real stale-candidate race in
  `ClaimInboxFair`. The final SQL mutation now rechecks current status and
  eligibility before updating, with a red/green regression test. A rebuilt
  Consumer was rolled out at digest
  `sha256:1d7758cf8261681b2701fff78d6e6af746a474d4126b77094920533138f71208`,
  2/2 Ready, zero restarts, fair queue enabled and concurrency 4.
- Clean final capacity run `cap-1788060387113445463`: 2,200 enqueued,
  2,200 completed Inbox, 2,200 Outbox, zero errors; 176.6 enqueue ops/s and
  15.7 claim/complete ops/s. This is a laptop/K3d/single-PostgreSQL baseline,
  not a target capacity promise.

New operational artifacts:

- `docs/LOCAL_PRODUCTION_VALIDATION_20260830.md`
- `docs/EXTERNAL_ACCEPTANCE_RUNBOOK.md`
- `scripts/external_acceptance_preflight.ps1` (offline credential-shape and
  callback/TLS preflight; it makes zero Provider calls unless the optional
  public callback probe is requested, and even that sends no route key)

Still external: real WeCom/Telegram sandboxes, vendor-supported production
Kubernetes/Linkerd combination, production KMS/Vault HA and dual-key rotation,
PostgreSQL/Redis multi-replica failover, cloud vector/object backends, DR and
target workload/cost capacity. Execute them in the order and attempt budgets
documented by the external acceptance runbook.

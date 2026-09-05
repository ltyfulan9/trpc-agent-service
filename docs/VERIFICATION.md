# 验证报告与证据边界

> 本文件保留 V12/V13 历史验证记录，不代表当前 V14 的完整验收状态。
> V14 的逐项结论与可复现命令以
> [`ACCEPTANCE_EVIDENCE_V14.md`](ACCEPTANCE_EVIDENCE_V14.md) 为准。

## 2026-09-05 V14 latest continuation

This section is the latest source-and-runtime evidence for V14. Older sections
below are retained as dated historical records and must not override this
section when they describe an earlier Docker-unavailable or source-only round.

### Path diagnosis and reproducible startup

The authoritative source was run from the C drive. A recursive path scan found
no hard-coded `E:\` reference. The ignored `deploy/.env.wecom.local` exists only
in the authoritative working tree and was not read or printed. A fresh archive
does not contain `.env` by design; therefore a direct `docker compose` command
without an operator-created env file returns the required-variable error. This
is fail-closed secret handling, not an E-drive dependency.

For Windows validation, `scripts/run_c_local_stack.ps1 -Build` resolves the
repository from `$PSScriptRoot`, sets disposable values only in the process,
uses the isolated overlay ports, and leaves no env file behind. It was tested
from the C-drive source copy.

### C-local evidence

```text
go mod verify                                      PASS
gofmt -l cmd pkg migrations test                   CLEAN
go build -buildvcs=false -p 1 ./cmd/...            PASS
go vet -p 1 ./...                                  PASS
go test -buildvcs=false -count=1 -p 1 ./...        PASS
go test -buildvcs=false -race -count=1 -p 1 ./...  PASS
go test -buildvcs=false -tags=integration -count=1 -p 1 ./test/integration  PASS (9.775s)
bash:5.2 scripts/static_verify.sh                   PASS
docker compose config                               PASS
docker compose build                                PASS (7 application images)
```

The latest continuation reran the complete `scripts/validate.sh` gate after
the runtime/release changes. It passed all 10 stages with serial per-service
BuildKit targets and a clean isolated backend run; the integration stage took
10.862 seconds. The separate evidence scripts also passed: K3d bootstrap and
compatible migration, Linkerd protected-route 401/403 identity distinction,
Gateway digest rollback/restore, local Vault dev identity allow/deny, and the
isolated 2,200-message fair-queue capacity baseline.

The source gate used Go 1.26.7 auto toolchain, `GOMAXPROCS=1` and serial
package scheduling; the complete host round took about 199.82 seconds. The
isolated Compose project `trpc-v14-c-local-20260905` contained 12 containers:
11 services plus the one-shot migration. Migration exited `0`; Gateway and
Admin `/health`, Prometheus `/-/healthy`, and Grafana `/api/health` returned
HTTP 200. Prometheus reported 6/6 active targets `up` and 15 rules with
`health=ok`. All V14 temporary containers had restart count `0`, and a bounded
log scan found no `panic` or `fatal` line.

The Admin vertical slice passed unauthenticated `401`, tenant creation and
credential-redacted reads, Agent App/version creation, publish, stable
Deployment creation, list visibility and deletion. The external acceptance
preflight used shape-valid placeholder values and reported
`provider_calls=0`; it did not call a real provider. The integration test used
real PostgreSQL 15.8, Redis 7.4, Qdrant 1.16.3 and MinIO containers and a local
model stub.

## 2026-08-29 V13 continuation

V13 separates the production security toolchain from source compatibility,
replaces the migration integration's `miniredis` with a mandatory real Redis
endpoint, and adds a two-pool PostgreSQL Worker-takeover test. The original
continuation was blocked by Docker Desktop before live infrastructure could run;
the same 2026-08-29 follow-up repaired Docker and closed that evidence gap.
Current Windows/amd64 evidence used `GOTOOLCHAIN=go1.26.7`, `GOMAXPROCS=1` and
serial package scheduling where applicable:

```text
go mod verify                                                   PASS
gofmt -l cmd pkg migrations test deploy                         CLEAN
go build -buildvcs=false -p 1 ./cmd/...                         PASS
go vet -p 1 ./...                                               PASS
go test -buildvcs=false -count=1 -p 1 ./...                     PASS
go test -buildvcs=false -race -count=1 -p 1 ./...               PASS
go test -tags=integration -count=1 -p 1 ./test/integration      PASS (real PostgreSQL 15.8 + Redis 7.4)
go test -tags=integration -count=5 -p 1 ./test/integration      PASS (five consecutive live runs)
bash ./scripts/static_verify.sh (bash:5.2 container)             PASS
promtool check rules deploy/prometheus-rules.yml                PASS (12 rules)
docker compose config + six application image builds            PASS
docker compose up --wait + runtime acceptance                    PASS
govulncheck v1.7.0 ./...                                        No vulnerabilities found
```

The host-default Go 1.26.2 produced 14 reachable standard-library findings;
the new production gate rejects it. Production CI and Docker builders were
already pinned to Go 1.26.7, so this closes a local-validation bypass rather
than changing the module's Go 1.25.14 source-compatibility floor.

Docker Desktop 4.71.0 initially failed while initializing its Inference manager
because of stale AF_UNIX runtime reparse points. Docker processes were stopped,
the exact ephemeral runtime directories were quarantined, and clean directories
were created. No image, container, volume, WSL disk or project data was deleted.
A native restart reproduced the fault, and disabling Model Runner did not prevent
the manager from creating the socket. Docker Desktop was upgraded in place to
4.88.1 build 237512 (installer result: `Installation succeeded`), Engine 29.7.2
and Compose 5.4.0; its first launch still reproduced the same fault. This is the
same open Windows failure class tracked by Docker in
[desktop-feedback#342](https://github.com/docker/desktop-feedback/issues/342)
and [desktop-feedback#531](https://github.com/docker/desktop-feedback/issues/531).

The host therefore uses an external, recoverable safe-start wrapper. It applies a
bounded engine probe, stops only Docker Desktop processes, renames only the two
documented ephemeral runtime parents, recreates them, starts Desktop and waits for
`docker info`. It recovered the upgraded installation and completed three
consecutive safe restarts. The current-user login Run entry and a Start-menu safe
restart shortcut point to the wrapper. A real Windows reboot/logon was not
executed, and native restart remains a known-broken path on this host; this is a
workaround rather than a vendor root-cause fix.

The independent registry failure was a stale manual proxy at `127.0.0.1:7897`.
Docker Desktop was moved to the working system proxy and successfully pulled every
fixed official image. The quarantined runtime-only directories remain recoverable
outside the source tree.

Live infrastructure evidence after repair:

- six application images built using Go 1.26.7 and Alpine 3.24.1 digest pins;
- PostgreSQL 15.8, Redis 7.4, OTel Collector 0.108.0, Prometheus 2.54.1 and
  Grafana 11.1.4 were pulled from Docker Hub and their observed digests were
  pinned into Compose;
- all 37 migrations applied and the 11-service Compose topology reached its
  expected state (ten long-running services plus a zero-exit migration job);
- Gateway, Admin, Prometheus and Grafana host endpoints returned HTTP 200;
- Admin returned 401 without credentials, then completed tenant creation,
  secret redaction, Agent App creation, Version publication and stable
  Deployment with an authenticated bootstrap principal;
- Redis AOF data, PostgreSQL tenant/deployment data and Worker health survived
  forced container restarts; a purpose-built Redis value also survived a full
  safe Desktop restart and was deleted after verification;
- Prometheus reported 5/5 application targets `up`; Grafana reported
  `database=ok`;
- migration and all five Go services run as `65532:65532`, non-privileged, with
  read-only root filesystems, `cap_drop: ALL`, `no-new-privileges` and bounded
  `/tmp` tmpfs mounts;
- all ten long-running services use `restart: unless-stopped`; after the final
  safe Desktop restart 10/10 recovered, migration remained Exited(0), four host
  endpoints returned HTTP 200, PostgreSQL counts remained `1|1|1|1|5`, and the
  current backend log tail contained no fatal socket failure.

The only structured error-level log lines after startup were two non-fatal
Grafana messages for duplicate registration of its bundled `xychart` plugin.
They did not affect Grafana health, its database, or Prometheus scraping and
remain an explicit observability-image warning. Historical V12 evidence below
remains historical; the bullets above are the distinct V13 live run. See the
canonical `TRPC_AGENT_ENTERPRISE_HANDOFF_V13.md` and
`docs/ACCEPTANCE_EVIDENCE_V13.md`.

## 2026-08-28 租户公平 Inbox 调度切片

- migration 035 新增 operator-owned `tenant_queue_schedule` 及 fair-claim
  partial index；PostgreSQL 和 MemoryStore 实现 `FairInboxClaimer`，按
  weighted virtual-runtime 调度选择每个租户的 session head，并在同一
  claim 临界区检查 `max_inflight`。
- 034/035 的事务内索引迁移均由 Runner 设置 5 秒 `lock_timeout` 和 30 分钟
  `statement_timeout`；大表仍必须在维护窗口执行，不能把普通事务索引构建
  当作零停机证据。
- Consumer 仅在 `FAIR_QUEUE_ENABLED=true` 时启用该能力；若 Store 未实现
  `FairInboxClaimer`，启动直接 fail-closed。默认仍使用兼容的全局 FIFO
  claim，避免滚动升级时出现隐式降级或饥饿。
- 内置 Gateway/Consumer 在 fair 模式启动时还验证 migration 035 的
  `tenant_queue_schedule` 表和 `idx_inbox_fair_tenant_head` 索引；缺失时在
  首次 claim 前 fail-closed，而不是等运行期暴露数据库缺表错误。
- 当前测试覆盖 Memory 的权重/MaxInflight/MaxQueued/策略校验、Consumer
  capability gate、PostgreSQL queue-admission SQL 和 migration up/down 契约。
  migration 035 会回填历史租户 schedule，admission 入队会按租户单行补齐
  新租户；fair claim 的 PostgreSQL SQL 契约同时要求只统计未过期的
  `PROCESSING` lease，避免 `MaxInflight` 把过期任务永久挡住。
  尚未执行多副本 PostgreSQL 竞争、真实 quota storm 或容量压测；fair 部署
  仍需所有 Gateway/Consumer 副本完成能力门禁。

## 2026-08-28 QueueInspector 运维切片

- Pipeline 的 expiry reaper 在每次有界维护后读取可选 `QueueInspector`，发布不含租户标签的 Inbox/Outbox depth 与 oldest-age gauges；Store 提供 `ObservedAt` 权威时钟以避免跨节点偏移，检查失败有独立计数器且不会把错误快照写成零值。
- 迁移 034 为自动队列状态增加 `(created_at, id)` partial indexes，并提供 up/down 配对回归测试；该迁移只优化观测读取，不改变 claim、FIFO、lease 或状态转换语义。Runner 在事务内为该迁移设置 5 秒 `lock_timeout` 和 30 分钟 `statement_timeout`，大表部署需维护窗口；不能改用 `CREATE INDEX CONCURRENTLY`，因为 Runner 将 schema 变更与 checksum 记录原子提交在同一事务中。
- `deploy/prometheus-rules.yml` 新增 Inbox/Outbox oldest-age page 规则，阈值与现有 queue-lag SLO 对齐；规则语法仍需目标环境的 `promtool`/Compose 门禁验证。
- `go test -count=1 ./pkg/pipeline ./migrations` 通过；后续完整门禁仍需在本轮收尾命令中重新执行。

## 2026-08-28 最新企业化硬化门禁

本轮使用可用的生产工具链 `go1.26.7`，在 Windows/amd64、`GOMAXPROCS=1`、`-p 1` 下串行完成源码级门禁：

- `go test -buildvcs=false -count=1 -p 1 ./...` 通过。
- `go test -buildvcs=false -race -count=1 -p 1 ./...` 通过。
- `go vet -p 1 ./...`、`go build -buildvcs=false -p 1 ./...`、`go mod verify` 通过。
- `go test -buildvcs=false -tags=integration -run '^$' -count=1 -p 1 ./test/integration` 通过；仅证明集成测试可编译，未连接真实数据库。
- `gofmt -l .` 无输出。
- `go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...` 返回 `No vulnerabilities found`。
- Windows 等价 PowerShell 静态门禁通过：入口、迁移 up/down 配对、默认拒绝 NetworkPolicy、关键生产 wiring 和生产 Go 源码禁用标记检查。仓库 Bash wrapper 因本机 WSL 缺少 `/bin/bash` 未执行，不能把等价检查写成 Bash 语法验证。

本轮新增回归与边界修复：

- `ChannelBinding` 的 Telegram/WeCom 凭据支持受限 `env://TRPC_SECRET_*` SecretRef；Gateway/Delivery 只解密和解析选中的 channel，resolver 错误不回显引用或秘密。
- Admin 更新在明文凭据和 SecretRef 两种形态下都保留“遮盖/省略即沿用”的语义；Channel.Config 未知键被拒绝，历史 secret-like 键在响应中统一掩码，避免 `api_key`/`access_token` 明文落库或泄漏。
- Gateway/Delivery 缺少 scoped tenant reader 时 fail-closed，不再回退到完整租户读取。
- Migration `Advance` 只允许真实阶段边界清空 cursor；`CATCH_UP -> CATCH_UP` 自转换不能触发重扫。
- Admin admission 将 runtime registry fingerprint 写入新版本快照；Worker 在构造前校验指纹。Admin 与 strict production Worker 对非内置 runtime 拒绝 type-only 注册；自定义 runtime 的 capability identity 必须通过 `RegisterWithCapability` 显式提供，且 capability 变化有回归测试；历史内置 `llm` 空指纹快照保留兼容窗口。
- Migration CatchUp 对非终态批次要求 applied watermark 严格推进；即使 opaque cursor 改变而 watermark 不变，也会返回 `ErrCursorStalled`，避免重复扫描循环。

上述结果只证明源码、单元/race、SQL mock 和静态契约；没有启动 PostgreSQL/Redis 多副本故障注入、真实 Telegram/企业微信 sandbox、Kubernetes rollout、OTLP TLS、KMS/Vault 或容量压测。

## 2026-08-28 runtime 与迁移收口门禁

本轮使用可用的 Go `1.26.7`（Windows/amd64，`GOMAXPROCS=1`、`-p 1`）重新执行：

- `go test -buildvcs=false -count=1 -p 1 ./...` 通过。
- `go test -buildvcs=false -race -count=1 -p 1 ./...` 通过。
- `go vet -p 1 ./...`、`go build -buildvcs=false -p 1 ./...`、`go mod verify` 通过。
- `go test -buildvcs=false -tags=integration -run '^$' -count=1 -p 1 ./test/integration` 通过；仅证明集成测试可编译，未连接真实数据库。
- `gofmt -l .` 无输出；`go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...` 返回 `No vulnerabilities found`。
- Windows 等价 PowerShell 静态门禁通过；仓库 Bash wrapper 仍因本机 WSL 缺少 `/bin/bash` 未直接执行，`bash -n` 仍属于 Linux/CI 验收项。

本轮新增证据：

- `RuntimeAgentRegistry` 的 fingerprint 具有确定性；自定义 runtime 通过 `RegisterWithCapability` 绑定稳定实现/构建标识，capability 变化会改变 fingerprint。显式入口拒绝把 capability 重复填写为 runtime type；旧 `Register` 仅为源码兼容，不能替代版本化 capability。
- `VersionSnapshot.Validate` 拒绝非 64 位十六进制 fingerprint；Admin admission 会写入当前 registry fingerprint；Worker 在模型/存储初始化前拒绝不匹配的 fingerprint。
- CatchUp 对非终态变更批次要求 applied watermark 严格推进；仅返回新 opaque cursor 而不推进水位时返回 `ErrCursorStalled`，不会重复扫描。

上述源码门禁不替代真实 PostgreSQL/Redis 多副本、IM sandbox、Kubernetes、OTLP TLS、KMS/Vault 或容量验收。

## 2026-08-28 本地追加门禁

本轮在 Windows/amd64、默认工具链 `go1.26.2` 下，使用 `GOMAXPROCS=1` 和 `-p 1` 串行执行。源码快照不在 Git 工作树中，因此 build/test 均显式使用 `-buildvcs=false`；不带该参数的裸 `go build` 只会因为无法读取 VCS stamping 失败，不代表 Go 编译失败。

- 定向回归：`go test -buildvcs=false -count=1 -p 1 ./pkg/reliable ./pkg/pipeline ./migrations` 通过。
- 全量普通测试：`go test -buildvcs=false -count=1 -p 1 ./...` 通过。
- 全量 race：`go test -buildvcs=false -race -count=1 -p 1 ./...` 通过。
- Tagged integration 编译门：`go test -buildvcs=false -tags=integration -run '^$' -count=1 -p 1 ./test/integration` 通过；未启动真实 PostgreSQL。
- 静态门禁：`go vet -p 1 ./...`、`go build -buildvcs=false -p 1 ./...`、`go mod verify` 和 `gofmt -l .` 均通过。
- 本轮覆盖了 `DISPATCH_STARTED` fence、过期 dispatch 进入 `WAITING_RECONCILIATION`、Memory/PostgreSQL 状态机一致性、非法 lease owner（UTF-8/控制字符/字节边界）及派生 worker owner 校验。
- 追加回归覆盖：Memory/PostgreSQL 的 `MarkDelivered` 与 `AdvanceOutbox` 拒绝未经过 `DISPATCH_STARTED` 的 `DELIVERING` lease；状态转换表同步移除该绕过路径，PostgreSQL SQL-shape 测试锁定该谓词。旧测试调用已显式执行 dispatch fence。

以上只证明源码、单元测试、竞态测试和 SQL mock/迁移静态行为；不等同于真实 Redis/PostgreSQL 多节点故障演练、IM sandbox、Kubernetes、KMS/Vault、OTel Collector 或容量验收。

## 本轮 Worker 传输结果边界复核（2026-08-27）

- 预算感知 `pkg/worker.HTTPClient` 使用 `net/http/httptrace` 记录请求是否越过写入边界。连接在写入前失败时仍按传输故障重试；写入回调已经发生后再断连，则返回 `ErrWorkerExecutionOutcomeUnknown`，由 Consumer 将 Inbox 转为 `WAITING_RECONCILIATION`，不自动重跑可能已到达 Worker 的模型或 Tool 调用。
- 该分类只依赖本地 transport 的写入证据，不解析或记录底层网络错误文本；它不能证明 provider 或业务 Tool 的 exactly-once。外部副作用仍需要 provider/Tool 自身的幂等键和真实故障注入验收。
- 回归测试：`pkg/worker/client_test.go` 的 `TestBudgetClientClassifiesTransportErrorAfterWriteAsUnknown` 覆盖写入前/写入后两条路径；串行执行 `go test -buildvcs=false -count=1 -p 1 ./pkg/worker ./pkg/gateway ./pkg/pipeline` 通过。

本次 Summary/迁移收尾核验日期：2026-08-25（Asia/Shanghai）。所有本地命令均使用 `GOMAXPROCS=1`、`-p 1` 串行执行。

> 2026-08-26 安全基线变更：`govulncheck` 识别出当前可达链路中的 Go 标准库、gRPC、pgx、OpenTelemetry、`x/net` 与 `x/text` 漏洞。相应修复版本最低要求 Go 1.25，因此模块最低版本已从 Go 1.21 提升到 Go 1.25；生产 CI 和构建镜像已改为官方 Go 1.26.7，兼容门使用 Go 1.25.14。下方所有 Go 1.21 记录仅保留为当时快照的历史证据，不能作为当前版本的兼容或安全声明。

## 当前安全与源码验证（2026-08-26）

- 最低兼容工具链 `go1.25.14`：`go test -buildvcs=false -count=1 -p 1 ./...` 通过。
- 生产工具链 `go1.26.7`：`go test -buildvcs=false -count=1 -p 1 ./...`、`go test -buildvcs=false -race -count=1 -p 1 ./...`、`go vet -buildvcs=false ./...`、`go build -buildvcs=false ./...`、`go mod verify`、全量 `gofmt -l` 和 integration 标签编译门均通过。
- `go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 -show verbose ./...` 在 `go1.26.7` 下报告 `No vulnerabilities found`；依赖树不再残留符号、包或模块级告警。
- Telemetry 对全局传播器执行 10 次随机顺序 `-race` 回归，验证只传播 W3C `traceparent`，不会提取或注入 baggage。
- 本轮同样未启动 Docker、PostgreSQL、Redis、真实 IM、KMS/Vault 或 OTel Collector；因此这些仍是目标环境的单独验收项，不能由上述源码门禁替代。

## 历史工具链快照（不可作为当前兼容声明）

- 在安全基线提升前，曾使用临时隔离的 Go 1.21.13 工具链真实执行（2026-08-25，全部退出码 0）。该结果仅用于历史追溯，不能作为当前兼容或安全声明：

```text
GOTOOLCHAIN=go1.21.13 GOMAXPROCS=1
gofmt -l <all Go files>                         # 无输出
go mod verify                                   # all modules verified
go build -buildvcs=false -p 1 ./...
go vet -p 1 ./...
go test -count=1 -p 1 ./...
go test -race -count=1 -p 1 ./...
go test -tags=integration -run '^$' -count=1 -p 1 ./test/integration
                                                    # 仅证明 tagged tests 可编译
```

- 静态记录覆盖：入口/27 组迁移配对、本地 replace 检查、生产 Go 源码 TODO 检查、`bash -n`、部署/Kubernetes Go 结构测试、默认拒绝 NetworkPolicy 断言、Summary migration CAS/lease/不变量字段断言、静态主链路追踪。
- 同一历史轮次在 Go 1.21.13 下再次通过 Summary lease/fence/CAS、并发入队和迁移约束门禁：`go build -buildvcs=false -p 1 ./...`、`go vet -p 1 ./...`、`go test -count=1 -p 1 ./...`、`go test -race -count=1 -p 1 ./...` 以及 integration 标签编译。Windows 当前没有可用 WSL `/bin/bash`，因此 Bash wrapper 未在该轮执行；等价 PowerShell 静态断言和 12 个部署 YAML 解析已通过。
- 本轮额外验证了迁移有效 lease 不可同 owner 重领、heartbeat 失权取消、迁移错误脱敏/控制字符与 4 KiB 上限、PostgreSQL `Claim/Advance` 使用数据库时钟并在最终 SQL 写入再次校验 owner/version/expiry、Session/Memory fencing 在后端 panic 时释放连接，以及 025 migration 的 up/down 配对。
- 未执行：`docker compose config/up`、镜像构建、迁移实际执行与真实 PostgreSQL/Redis 集成；本轮按低负载约束没有启动任何基础设施服务。

## 历史企业边界硬化复核（2026-08-25，低于当前安全基线）

在上述基线之后，本轮新增了严格 Fenced Session/Memory 作用域校验、生产 token 的 canonical app scope、健康探针转发、Worker 租户生命周期与直接 Go 请求边界校验、nil Client/Worker 防护，以及对应回归测试。针对这些新增代码，按串行低负载方式实际执行：

- 当时本机工具链 `go1.26.2`：`go test -count=1 -p 1 ./...`、`go vet -p 1 ./...` 通过；`go test -race -count=1 -p 1 ./pkg/fence ./pkg/storage ./pkg/worker` 通过。
- 当时的历史兼容快照 `go1.21.13`：`go test -count=1 -p 1 ./pkg/fence ./pkg/storage ./pkg/worker ./cmd/worker` 通过；这不是当前兼容承诺。

这些命令仍未启动 PostgreSQL、Redis、Docker、真实 IM、KMS/Vault 或 OTel Collector；因此不构成真实基础设施、外部 Provider、容量或生产就绪证明。

因此，本包可以声明“当前 Go 源码编译、单元测试和竞态测试已通过”，但不能声明 Compose、真实数据库集成、IM sandbox、容量或生产就绪已经验证。

## 本轮追加硬化（2026-08-25）

- 控制面 `CreateVersion` 在写库前同时校验 `VersionSnapshot.Agent.Name == appName`；这条约束覆盖原始输入和 `VersionSnapshotPreparer` 改写后的结果，防止错误 Agent 绑定快照进入数据库。
- Worker HTTP 请求和直接 Go 调用共用导出的 `worker.ValidateRequest`；控制面绑定、执行记录创建之前统一拒绝非法 metadata、非法 `traceparent` 和不完整请求身份。
- 严格 Worker 要求 `Options.AppName == AgentConfig.Name`，避免应用缓存键与 immutable Agent 配置分叉。
- Storage adapter 增加 `ServiceLeaseAdapter` 生命周期 seam 的文档约束；生产 worker 通过 `AcquireServices` 持有引用，避免空闲淘汰在 Runner 使用期间关闭 Session/Memory 客户端。
- 生产 worker 启用 `RequireBackendHealthProbe`。当 Redis/PostgreSQL tRPC 服务未暴露 `HealthCheck`/`PingContext` 时，adapter 通过 operator-owned `BackendProfileHealthChecker` 做短生命周期真实 Ping；没有探针能力则 fail-closed。InMemory 不需要远程探针。探针错误不回显连接字符串。
- 新增回归覆盖：控制面 Agent 身份重绑定、远程 profile 无探针拒绝、远程 profile 探针转发、探针错误脱敏、取消的 readiness 不污染缓存，以及以上边界与原有 Worker/Storage 生命周期测试的串行组合。

本追加轮次的历史门禁结果（Windows/amd64）：当时默认工具链 `go1.26.2` 下 `gofmt -l .` 无输出，`go build -buildvcs=false -p 1 ./...`、`go vet -p 1 ./...`、`go test -count=1 -p 1 ./...` 和 `go test -race -count=1 -p 1 ./pkg/fence ./pkg/storage ./pkg/worker` 通过；`go1.21.13` 下 `go test -count=1 -p 1 ./pkg/fence ./pkg/storage ./pkg/worker ./cmd/worker` 与 `go build -buildvcs=false -p 1 ./...` 通过。当前版本应以顶部的 Go 1.25.14/1.26.7 门禁为准。

本追加轮次没有启动 PostgreSQL、Redis、Docker、真实 IM、KMS/Vault 或 OTel Collector，也没有修改上游 `trpc-agent-go` 源码；因此远程 Ping、第三方驱动、真实租户控制面和容量指标仍须在具备基础设施的环境单独验收。

## 静态核对的生产 wiring

| 能力 | 入口 → 实现 → 存储证据 |
|---|---|
| Durable ingress | `cmd/gateway` → `NewDurableServer` → `reliable.EnqueueInbox` |
| Consumer | `cmd/consumer` → `pipeline.Consumer` → Worker HTTP → `CompleteInbox` |
| Delivery | `cmd/delivery` → `pipeline.Delivery` → Channel Adapter → `MarkDelivered` |
| Shared Session | `cmd/worker` → StorageAdapter → `runner.WithSessionService` |
| Shared Memory | StorageAdapter → `runner.WithMemoryService` + bounded preload → governed `memory.Service.Tools()` |
| Governance | Worker 输入策略 → `runner.WithPlugins(governance.Plugin)` → 最终文本脱敏 |
| Agent version | Worker → `PostgresResolver.ResolvePinned` → invocation binding → immutable snapshot → execution record |
| Runner lifecycle | exact tenant/config/app/version/deployment key → bounded reference-counted cache → idle TTL/shutdown close |
| Execution recovery | Worker background reconciler → bounded `SKIP LOCKED` → old RUNNING only → ABANDONED |
| Trace | Gateway middleware → Inbox traceparent → Consumer span → Worker → Outbox → Delivery |
| Service auth | Consumer Signer → Worker Verifier → Redis nonce SETNX |
| Migration runner | `cmd/migrate` → advisory lock + embedded checksum → schema_migrations（含 022 data migration、023 Summary job/checkpoint、024 additive invariants、025 migration error bound、026 route-key backfill、027 outbox reconciliation wait、028 durable tool approvals、029 approval invariants） |
| DLQ replay | `cmd/replay` → fenced store replay → replay audit |
| Segmented delivery | Adapter single chunk → fenced `delivery_cursor` → next claim / REPLIED |
| Tenant CAS | Admin → `Tenant.ConfigVersion` → PostgreSQL compare-and-swap update |
| Tool catalog | immutable version tools → tenant whitelist → operator `platformtool.Catalog` |
| Control audit | Admin actor → tenant/Agent repository transaction → `control_plane_audit` |
| Admin identity | bearer credential → hashed Principal → route permission + tenant allowlist → context-derived actor |
| Tool audit | Runner BeforeTool authorization → audit → execution → AfterTool result/latency audit |
| Telegram thread mapping | `update_id` durable dedup → trusted `Inbox.ReplyToID` → per-chat `message_id` reply |
| Session FIFO | transactional `session_sequence` → predecessor exclusion → DLQ pause/audited replay |
| Reply route isolation | verified Gateway mapping → authoritative Inbox route → narrow `OutboxReply` completion input |
| Network isolation | default-deny → explicit Gateway/Consumer/Worker/Delivery/Admin ingress+egress policies |
| Summary coordination | `pkg/summary.Processor` → fenced Store claim/renew/fail/complete → Generator → CAS Sink; Memory/PostgreSQL tests |

## 必须在有工具的机器执行

```bash
./scripts/validate.sh 2>&1 | tee validation.log
```

`validate.sh` 会强制要求 Go 与 Docker，构建所有命令和镜像，并启动隔离 PostgreSQL 容器执行 integration tag；工具缺失会返回非零，不会输出笼统“通过”。进一步的真实基础设施验收：

1. `docker compose up --build -d`，等待全部 health。
2. 创建 Tenant、AgentApp、Version、发布 stable deployment；验证缺失 deployment 时 Worker fail-closed。
3. 发送有真实 Telegram update_id 的签名 webhook，验证 Inbox→result→Outbox→Adapter。
4. 在以下点分别 `kill -9`：Inbox COMMIT 前后、Worker result 前后、Outbox COMMIT 前后、provider success 后/REPLIED 前。
5. 同时运行两个 Consumer/Delivery，验证同 ID 只有一个有效 fence；同 session 后序不能越过 PROCESSING/RETRY_WAIT/DEAD_LETTERED 前序，不同 session 仍可领取。
6. 重放相同 payload、不同 payload 同 ID、过期 HMAC、重复 nonce、跨租户 Agent version；切换 deployment 后重试同 Inbox，确认仍使用首次绑定版本。
7. `go test -race -count=20 ./...`；集成测试不得因无 DB 环境而静默 skip 后仍标绿。
8. 记录 `docker compose logs otel-collector` 中同一 trace 跨 gateway/consumer/worker/delivery 的 span。

## 尚未完成的基础设施证据

- 未执行 Docker 镜像构建、Compose health、`promtool` 与 integration tag PostgreSQL 测试；FIFO/replay/权威路由的 PostgreSQL 测试源码已加入但本轮只编译。
- CI 已配置 pinned `govulncheck`，但本环境未执行联网漏洞库扫描，不能写成通过。
- 无真实企业微信/Telegram sandbox 凭据的契约测试。
- 无正式容量基准原始结果。
- 无 KMS/Vault、mTLS/service mesh 与云对象/向量库实测。
- Summary job coordination、内存/PostgreSQL CAS sink 和通用 Processor 已有单元/race 覆盖；仍无绑定某个 tRPC Session 物理 backend 的生产 Generator/摘要模型适配器。Knowledge/Artifact 数据面和真实 Redis/SQL/向量/对象存储迁移仍未在基础设施中执行。

这些条目已在架构中给出接口和落地约束，但不会被写成“已实现”。

## 本轮生命周期与依赖边界复核（2026-08-26）

- `health.Coordinator` 的并发 Shutdown、Storage/Worker 关闭排空、第三方 health probe panic 隔离和取消传播均有串行单元/race 覆盖；关闭期间不会继续接纳新请求，清理错误不会泄漏 provider 返回值。
- Worker 的生产构造仍要求租户路由的共享 Session/Memory、Redis invocation coordination 和 backend-native fencing；兼容构造函数只服务本地/单元组合，缺少协调依赖时 Process 明确 fail-closed。
- 这些源码级结果不等同于 Docker、PostgreSQL、Redis、Kubernetes 或外部 provider 已完成验收，后者继续列在本报告的未验证边界中。

## 本轮指标访问边界硬化（2026-08-26）

- 五个 HTTP 进程的 `/metrics` 统一经过 `telemetry.MetricsHandlerFromEnv`：默认未配置时返回 404；配置 `METRICS_AUTH_TOKEN` 后要求且只接受单个 bearer token；重复 Authorization 头、错误 scheme 或错误 token 均返回 401。`METRICS_ALLOW_UNAUTHENTICATED=true` 仅作为显式本地开发开关。
- 新增 `pkg/telemetry/metrics_test.go` 覆盖默认拒绝、开发模式和认证边界。Compose 示例明确标注未认证指标只适用于宿主机绑定的开发网络；生产部署必须改为 Secret 注入 token 和受限 Prometheus 抓取。

## 本轮部署 wiring 与 Worker 错误分类复核（2026-08-26）

- Compose 的 Prometheus 现在接收同一个 `METRICS_AUTH_TOKEN`，启动时以 `umask 077` 写入容器内临时 token 文件；五个 scrape job 都通过 `bearer_token_file` 发送认证。这样在设置应用 token 后不会出现监控端持续 401。未设置 token 时仍保持本地 Compose 的显式匿名开发开关；这不是生产安全配置。
- Kubernetes 的五个 HTTP Deployment 都从必需的 `metrics-auth/token` Secret 注入 token；`scripts/k8s_apply.sh` 在迁移前预检该 Secret。新增的 `deploy/kubernetes/monitoring-example.yaml` 仅是 Prometheus Operator 的可选 `PodMonitor` 示例，不由 rollout 脚本自动应用，也未在真实集群执行。
- Worker 在缺少 Redis session invocation coordination 时仍 fail-closed，但现在返回可由 `errors.Is` 识别的 `ErrDistributedSessionCoordinationRequired`，并有回归测试；没有把本地兼容构造函数误当作多副本生产执行模式。
- 本轮新增部署配置、Kubernetes wiring、metrics 认证和 Worker 错误分类测试；默认 Go 工具链下 `go vet -p 1 ./...`、`go test -count=1 -p 1 ./...`、`go test -race -count=1 -p 1 ./...`、integration tag 编译门禁均通过，全部按 `GOMAXPROCS=1`、`-p 1` 串行执行。
- 本轮仍未执行 Docker Compose 配置/镜像构建、Prometheus 运行时抓取、Prometheus Operator、Kubernetes 集群、PostgreSQL/Redis、真实 IM 或 OTel Collector 验收。当前环境的 `bash` 仅指向缺少 `/bin/bash` 的 WSL stub，也没有可用 Git Bash，因此 `bash -n`/`scripts/static_verify.sh` 未执行；等价静态断言已用 PowerShell 完成。

本轮保持单机、串行、无外部基础设施运行，针对企业运行时最容易造成级联故障的生命周期边界做了最小修改和回归覆盖：

- `health.Coordinator` 的并发 `Shutdown` 现在等待首个调用真正完成，并返回相同的清理错误；等待者可用自己的 context 超时，不会永久阻塞。
- Shutdown 在 admission 同一把锁下发布 draining 状态，消除“已经开始关闭但仍短暂接纳新请求”的窗口；单个清理 hook 的错误或 panic 不会阻断后续 hook，panic 值不会进入错误文本。
- Storage backend 的 builder、health service、profile resolver 和 `Close` 都在边界处隔离第三方 panic；Session/Memory 关闭会继续执行并聚合稳定错误，不保留 provider 返回值或凭据。
- 配置租户 readiness 使用 context-aware single-flight：同一时刻只做一次冷启动后端遍历，等待者可独立超时；取消、租户列表 panic 和探针 panic 都不会把 in-flight 状态或 readiness 缓存锁死。
- Redis/PostgreSQL profile live probe 增加两秒内部上限，并继续沿用连接材料脱敏和调用方取消传播。

历史轮次实际门禁（Windows/amd64，Go 1.26.2；当前版本以顶部门禁为准；未启动 PostgreSQL、Redis、Docker、真实 IM、KMS/Vault 或 OTel Collector）：

```text
gofmt -l .                                      # 无输出
go mod verify                                   # all modules verified
go build -buildvcs=false -p 1 ./...             # 通过
go vet -p 1 ./...                               # 通过
go test -count=1 -p 1 ./...                     # 全包通过
go test -race -count=1 -p 1 ./pkg/fence ./pkg/health ./pkg/storage ./pkg/worker  # 通过
```

这些结果证明源码构建、静态检查、单元测试以及关键生命周期包的竞态测试通过；不构成真实数据库/Redis 连通性、IM provider 契约、容量或生产就绪证明。

## 本轮高风险包竞态复核（2026-08-26）

在不启动外部基础设施、保持 `-p 1` 串行执行的条件下，历史环境使用 Windows/amd64 默认 Go 1.26.2 完成以下命令，退出码均为 0：

```text
go test -race -count=1 -p 1 ./pkg/channel ./pkg/gateway ./pkg/reliable ./pkg/pipeline ./pkg/auth ./pkg/worker ./pkg/storage
gofmt -l .                                      # 无输出
go vet -p 1 ./...                               # 无输出
```

本轮 race 覆盖 Channel Adapter、Gateway、可靠队列、Pipeline、认证、Worker 和 Storage 的并发边界。未执行 Docker/Compose、PostgreSQL、Redis、Kubernetes、真实 IM、KMS/Vault、OTel Collector 或容量测试；上述源码级门禁不能替代这些环境验收。

审计补充：Consumer 对 typed-nil `HTTPStatusError` 的错误分类路径已增加 nil guard，避免 `errors.As` 成功但目标指针为空时发生解引用 panic；该回归包含在本轮全包普通测试、全包 race、build、vet 和格式检查中。

## 本轮 Kubernetes OTLP 信任链复核（2026-08-26）

- 企业审计发现并修复一个真实配置缺口：五个 HTTP Deployment 原先要求 `OTEL_EXPORTER_OTLP_INSECURE=false`，但没有向进程注入 Collector CA，私有 CA 或自签证书场景下 OTLP 连接无法可靠建立。
- 现在 Gateway、Worker、Admin、Consumer、Delivery 的清单均设置 `OTEL_EXPORTER_OTLP_CERTIFICATE=/var/run/secrets/otel/ca.crt`，以只读方式挂载 `otel-collector-tls` Secret 的 `ca.crt`；`secrets-example.yaml`、`scripts/k8s_apply.sh`、Kubernetes README 和 YAML 契约测试已同步更新。
- Collector 必须由部署方提供启用 TLS 的 OTLP receiver，并使用与服务 DNS 名称匹配的 SAN；本快照不包含真实集群中的 CA 签发、Secret 创建、Collector TLS 运行或网络连通性验证。
- 修复后在 Windows/amd64、`GOMAXPROCS=1`、`-p 1` 下使用 Go `1.21.13` 串行执行并通过：`go test -count=1 -p 1 ./...`、`go test -race -count=1 -p 1 ./...`、`go build -buildvcs=false -p 1 ./...`、`go vet -p 1 ./...`、全部 Go 文件 `gofmt -l`（无输出）以及 `go mod verify`（`all modules verified`）。另外 `go test -count=1 -p 1 ./deploy/...` 的部署契约测试通过。
- 未执行 Docker/Compose、PostgreSQL、Redis、Kubernetes 集群、OTel Collector、真实 IM、KMS/Vault 或容量测试；以上结果仍然是源码和清单级证据，不构成生产就绪证明。

## 本轮 Runner 与日志边界复核（2026-08-26）

- `collectRunnerResponse` 现在显式拒绝 nil event stream；Runner 构造异常不再让 Worker 在无事件通道上永久阻塞并持有 session lease。新增回归测试覆盖该路径。
- Worker、Consumer、Delivery、Gateway、Admin/Control Plane、迁移、重放和 Result Cache 的运行日志不再格式化第三方/数据库错误原文，统一记录稳定错误码；错误文本可能包含 DSN、token、provider body 或用户输入，详细诊断仍只允许进入受控脱敏 sink。
- 历史轮次实际门禁（Windows/amd64，Go 1.26.2，全部 `-p 1` 串行）退出码均为 0：

```text
gofmt -l .
go mod verify
go build -buildvcs=false -p 1 ./...
go vet -p 1 ./...
go test -count=1 -p 1 ./...
go test -race -count=1 -p 1 ./...
```

仍未执行 Docker/Compose、PostgreSQL、Redis、Kubernetes、真实 IM、KMS/Vault、OTel Collector 或容量测试；源码门禁不能替代这些环境验收，也不构成生产就绪证明。

## 本轮装配与健康边界复核（2026-08-26）

- Worker/Storage/Tool Catalog 对 typed-nil `StorageAdapter`、Session/Memory service、Tool/ToolResolver 做 fail-closed 校验；工具 `Declaration()` panic 也会被视为无效工具，不会在启动或首次请求时传播 provider panic。
- `HealthChecker` nil receiver 现在返回 `health_checker=unhealthy` 和 HTTP 503，而不是在 `/health` 路径直接 panic。
- 对应回归已纳入本轮全包普通测试和全包 race；随后 `go build -buildvcs=false -p 1 ./...`、`go vet -p 1 ./...`、`gofmt -l .`（无输出）和 `go mod verify`（all modules verified）均通过。

上述改动只加强进程内边界和错误处理，仍不等同于真实数据库/Redis、第三方 IM、KMS/Vault、OTel Collector、Kubernetes 或容量环境验收。

## 审批闭环追加复核（2026-08-26）

- 新增 `tool_approvals` 迁移与 `PostgresApprovalStore`：challenge 复用、operator grant、token hash、tenant/actor/session-owner/session/tool/args/invocation 全字段绑定，以及 SQL 条件更新的一次性消费。后续 029 迁移以追加约束保证 `consumed_at` 只能在已 grant 后出现，不修改已发布 028 checksum。迁移 030 新增有界 `WAITING_APPROVAL` Inbox 状态和 `approval_deadline`，审批等待不消耗普通 attempt；过期行由 Claim 原子转 DLQ，旧 Store 能力缺失时转 reconciliation。
- Admin 新增带 tenant scope 的 challenge 查询与 grant 路由；响应不返回参数原文、token hash 或 raw approval token，并标记 `no-store`。
- Worker 的 428 approval response 只携带 challenge ID/过期时间；Consumer 校验有界 expiry 后将 428 持久化为 `WAITING_APPROVAL`，不消耗普通 Inbox attempt，下一次同 invocation 在数据库中消费已授予 grant，不把 raw token 运输到 Inbox/Session/模型。
- 同步 Runner 错误、事件流错误和重复/过期/错误 scope 均保持 typed approval 或 fail-closed 语义；没有 durable ApprovalStore 时危险工具仍拒绝执行。
- 历史串行回归已执行：`GOTOOLCHAIN=go1.21.13 GOMAXPROCS=1 go test -count=1 -p 1 ./pkg/governance ./cmd/admin ./migrations`、对应 `-race` 和 `go vet`，退出码均为 0。PostgreSQL SQL 路径使用 sqlmock 验证了原子 consume、同事务审计及审计失败回滚；Memory 路径验证并发消费恰好成功一次、重复 JSON key 和非法 UTF-8 拒绝。尚未启动真实 PostgreSQL 或执行 Compose/infrastructure 验收。

## 审批 admission 再审计（2026-08-27）

- `ApprovalResumeStateInspector` 将 active challenge 与 `granted_at` 从同一存储一致性边界返回；PostgreSQL 使用单条有界查询，Memory 使用同一互斥区。HTTP `/v1/process` 通过 admission gate，在未授权轮询时不会调用 `StartWithRequest`，并在授权状态上记录内部 challenge fence。
- Worker 在 fence 对应的 challenge 被其他副本消费、替换、过期或与 Session 最后一个待执行 tool call 不匹配时返回 `ErrApprovalResumeUnsafe`，进入 423 reconciliation，不会把审批重试当成新的 user turn。challenge 响应统一 `Cache-Control: no-store`，且对 ID、scope、hash、expiry 做结构校验。
- 新增回归覆盖：重复 pending poll 的零 execution admission、授权 marker、原子 Memory/PostgreSQL inspection、取消 context、缺少 atomic inspector、损坏 challenge、并发消费后恢复和孤立 pending transcript。串行 `go test -count=1 -p 1 ./...` 在 Go 1.26.7 下通过；真实 PostgreSQL 多连接竞争仍需 CI/基础设施验收。

## 最终源码门禁复核（2026-08-27）

- 审批等待的 deadline 现在由 Store 的权威时钟和固定 24 小时上限验证；LocalClient 的审批暂停与 HTTP 428 走同一 `WAITING_APPROVAL` 路径，`ErrApprovalResumeUnsafe` 与 HTTP 423 同样进入人工 reconciliation，不消耗自动重试预算。
- Redis token reservation 再次在运行时拒绝超出 Lua 精确整数范围的预算，避免宿主代码绕过 Tenant 配置校验后产生不精确的账本运算。
- Windows/amd64、`GOMAXPROCS=1` 下，使用 `go1.26.7` 实际通过：`go mod verify`、全量 `gofmt -l`、`go build -buildvcs=false -p 1 ./...`、`go vet -p 1 ./...`、`go test -count=1 -p 1 ./...`、`go test -race -count=1 -p 1 ./...` 和 `go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...`（`No vulnerabilities found`）。
- 兼容门 `go1.25.14 go test -count=1 -p 1 ./...` 与 integration-tag 编译门 `go1.26.7 go test -tags=integration -run '^$' -count=1 -p 1 ./test/integration` 也通过。PowerShell 静态契约门确认：无 `go.mod replace`、所有 up/down migration 成对、关键 NetworkPolicy/入口 wiring 存在，且生产 Go 源码不存在 TODO/FIXME/not-implemented 标记。
- 使用进程内临时的非生产占位值执行 `docker compose -f deploy/docker-compose.yml config --quiet` 通过；本机 Docker daemon 未运行，所以未构建镜像或启动 Compose。
- 本轮未启动真实 PostgreSQL/Redis、企业微信/Telegram sandbox、Kubernetes、KMS/Vault、OTel Collector 或容量压测；上述源码与编译证据不能替代这些基础设施验收。

## Worker deadline and governance seam review (2026-08-27)

- Removed the inactive `AgentWithGovernance` static wrapper. Production tool
  policy is enforced only by the Runner Plugin, which is the path that also
  owns durable approval, audit, and masking.
- Added a side-effect boundary for deadlines: a timeout before `Runner.Run`
  is recorded `SafeToRetry=true` and returned as retryable 503; a timeout once
  Runner may have started model or Tool work remains non-retry-safe and enters
  423 reconciliation. A non-cooperative event stream is bounded by the Worker
  context and the regression proves the Session lease is released before
  return.
- Fresh targeted source gate on Windows/amd64, run serially with the local Go
  toolchain: `GOMAXPROCS=1 go test -count=1 -p 1 ./pkg/worker ./cmd/worker`.
  It passed. This is an incremental package-level check only; the earlier
   whole-module, race, infrastructure, and provider acceptance statements must
   be rerun after the complete review set is finalized.
- The Consumer-to-Worker budget is now a per-request, HMAC-bound
  `ExecutionContract`. Budget-aware Consumers call `/v1/process`; the
  unversioned `/process` route is deliberately fail-closed with 503 so an old
  authenticated Consumer cannot bypass the contract during rollout. A version
  response header distinguishes an old Worker route miss from a real application
  404/405. The Worker validates the contract before tenant lookup, and the same
  absolute deadline covers deployment resolution, result-cache lookup, approval
  admission, Worker construction and Runner execution. A final pre-Runner check
  rejects an already-expired context as retry-safe preflight; once Runner may
  have started, timeout remains non-retry-safe and enters reconciliation.
- Fresh targeted gates after this protocol change were run serially on
  Windows/amd64: `go1.26.2 go test -count=1 -p 1 ./pkg/worker ./cmd/worker
  ./cmd/consumer ./pkg/pipeline`, the same command with `-race`, `go vet -p 1`
  for those packages, and `GOTOOLCHAIN=go1.25.14 go test -count=1 -p 1` for the
  same package set. All passed. These tests include old-route fail-closed
  behavior, signed-contract tamper rejection, business-404 classification,
  route reachability, preflight timeout classification, and no deadline
  extension by nested Worker contexts.

## Retry-safety boundary review (2026-08-27)

- A budget-aware HTTP client now treats a successful response with missing,
  mismatched or malformed execution proof, including an invalid JSON response,
  as `ErrWorkerExecutionOutcomeUnknown`. The Consumer moves the claimed Inbox
  row to `WAITING_RECONCILIATION`; it never blindly re-runs a request whose
  model or Tool side effect may already have occurred.
- Worker errors after `Runner.Run` and execution-lease heartbeat loss now
  return HTTP 423 with `retry_safe=false` semantics. Pre-Runner admission
  failures remain retryable 503 responses. This keeps HTTP classification and
  the durable execution record on the same side-effect boundary.
- Production HTTPS budget-client constructors fail closed when the service
  signer is nil. The generic test/local constructor remains available for
  explicit non-production compositions.
- Added regression coverage for unknown response outcomes, malformed success
  responses, Consumer reconciliation blocking, post-Runner classification and
  signer validation. Serial package tests passed after the changes; the full
  module and infrastructure gates must be rerun after this review set.

## Lease-clock authority review (2026-08-27)

- PostgreSQL lease authority no longer relies on `now()`, whose value is fixed
  at transaction start. Inbox/Outbox claim eligibility and newly issued lease
  deadlines, fenced mutations, retry-after deadlines, execution fences,
  Summary leases, migration leases, and result-cache completion use
  `clock_timestamp()` wherever a row-lock wait could otherwise accept stale
  ownership or shorten a fresh lease.
- Added SQL-shape regression coverage for every reliable Inbox/Outbox fenced
  mutation and expiry-selection path, the result-cache producer commit fence,
  and the Inbox/Outbox claim lease writes. These tests prove generated SQL, not
  real PostgreSQL lock-wait behavior.
- Fresh local targeted checks were run serially on Windows/amd64 with local Go
  `go1.26.2`: `go test -count=1 -p 1 ./pkg/reliable`,
  `go test -race -count=1 -p 1 ./pkg/reliable`, and
  `go test -count=1 -p 1 ./pkg/resultcache ./pkg/controlplane ./pkg/summary ./pkg/datamigration`.
  All passed.
- Docker, PostgreSQL, Redis, IM sandboxes, Kubernetes, KMS/Vault, OTel
  Collector, and capacity workloads were not started in this pass. A real
  PostgreSQL test must hold a target-row lock, let the lease expire, release
  the lock, and demonstrate rejection of stale completion before this becomes
  infrastructure-level evidence.

## Fable design adversarial review (2026-08-28)

- The Fable architecture and production-risk notes were reviewed section by
  section against the source contracts. The proposed replica router,
  Redis-Pub/Sub invalidator, online Redis `SCAN` migration, PostgreSQL
  advisory-lock fallback, sample-only migration validation, and in-memory
  Inbox fallback were rejected as either non-compiling pseudocode or as
  violations of read-after-write, durable Inbox, or lease-fence invariants.
- Migration CatchUp now persists its own cursor and passes it back to
  `Source.Changes`; a repeated non-terminal batch is rejected with
  `ErrCursorStalled`. Both MemoryStore and PostgreSQL Store reject snapshot or
  applied watermark regressions and cursor clearing outside the explicit phase
  boundaries. Regression coverage is in `pkg/datamigration`.
- Consumer and Delivery no longer use a node wall clock when a custom Store
  lacks `RetryAfterStore`; the work is parked for reconciliation. Gateway now
  applies an atomic tenant-wide fixed-window ingress cap in addition to the
  per-user cap, while source markers keep provider redeliveries free.
- Fresh targeted checks passed after these changes:

```text
go test -buildvcs=false -count=1 -p 1 ./pkg/datamigration ./pkg/pipeline ./pkg/gateway
```

The key-ring loader now has a tested, prefix-scoped `SecretResolver` boundary
for operator-owned `env://TRPC_SECRET_*` references. This proves reference
validation and error redaction only; it is not an external KMS/Vault client.
The following remain explicit acceptance gates rather than claims: external
KMS/Vault SecretRef resolution, least-privilege production database roles,
TLS-configured OTLP Collector, Redis/PostgreSQL multi-replica failure
injection, real IM sandbox contracts, concrete backend migration adapters,
Kubernetes rollout, and capacity measurements.

## Enterprise hardening gate (2026-08-28)

The SecretRef loader change was verified with the complete source gate on
Windows/amd64 using the available Go `go1.26.2` (the module minimum remains
Go `1.25.14`; CI/container production toolchain remains `1.26.7`):

```text
go test -buildvcs=false -count=1 -p 1 ./...                 PASS
go test -buildvcs=false -race -count=1 -p 1 ./...           PASS
go vet -p 1 ./...                                          PASS
go build -buildvcs=false -p 1 ./...                        PASS
go mod verify                                               PASS
go test -buildvcs=false -tags=integration -run '^$' ./test/integration PASS
gofmt -l .                                                  CLEAN
```

The repository's `scripts/static_verify.sh` could not be launched directly
because this host's `/bin/bash` is an unavailable WSL relay. Its file, command,
migration-pair, NetworkPolicy, forbidden-marker, and required-entrypoint checks
were run through an equivalent PowerShell gate; shell syntax parsing remains a
Linux/CI acceptance step.

## 2026-08-28 Fair-queue correctness closure

The follow-up hardening pass fixed four cross-backend/deployment edges:

- PostgreSQL fair `MaxInflight` counts only non-expired `PROCESSING` leases;
  expired work can be reclaimed instead of self-blocking a tenant.
- Memory fair claim rejects expired `WAITING_APPROVAL` rows with the same
  deadline semantics as PostgreSQL.
- Queue-full admission checks the provider idempotency key before returning
  `ErrTenantQueueFull`, so duplicate redelivery remains a canonical no-op.
- Deleting an operator queue override resets the unlimited weight-one default
  and keeps the schedule row, preserving existing backlog eligibility.

Migration 035 now seeds existing tenants once, and migration 036 adds the
tenant/status partial index used by the admission count. Admission creates a schedule row
for tenants created later, removing the fair-claim queue-wide `DISTINCT` scan.
Built-in Gateway and Consumer verify the schedule table and fair-head index at
startup when `FAIR_QUEUE_ENABLED=true`, with a bounded 10-second readiness
deadline so a half-open database cannot stall process startup indefinitely.

With the downloaded production toolchain `go1.26.7 windows/amd64`, the updated
snapshot passed:

```text
go test -buildvcs=false -count=1 -p 1 ./...                 PASS
go test -buildvcs=false -race -count=1 -p 1 ./...           PASS
go vet -p 1 ./...                                          PASS
go build -buildvcs=false -p 1 ./...                        PASS
go mod verify                                               PASS
go test -buildvcs=false -tags=integration -run '^$' -p 1 ./test/integration PASS
go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...        No vulnerabilities found
gofmt -l .                                                  CLEAN
PowerShell static structure/migration-pair gate                  PASS
```

That earlier source-only pass did not execute Docker-backed gates. The later
2026-08-28 closure section below records the subsequent Compose image,
`promtool`, and live PostgreSQL evidence. Redis multi-replica, external
sandbox, Kubernetes, KMS/Vault, OTel and capacity gates remain unexecuted.
## 2026-08-28 PostgreSQL fair-queue and lease-lock closure

本轮针对企业级并发审查发现的问题补充了真实数据库回归，并完成完整门禁：

- `RetryInbox`/`RetryInboxAfter` 先在同一事务中对目标 Inbox 行执行 `SELECT ... FOR UPDATE`，拿到锁后再用 PostgreSQL `clock_timestamp()` 做 owner/fence/status/lease fenced update。这样锁等待跨过 lease 到期时不会错误接受旧执行者。
- 新增真实 PostgreSQL 锁等待测试：事务 A 持有 processing 行锁并让 lease 过期，事务 B 等待后重试；释放锁后 B 返回 `ErrStaleLease`，原行仍为 `PROCESSING`。
- 新增真实 PostgreSQL fair-queue 测试：`max_inflight=1` 时过期 lease 可被接管；删除租户 override 后 backlog 仍可领取，默认 schedule 行不会丢失。
- 新增 SQL-shape 回归，锁定 `RetryInbox` 的 `$4::timestamptz` 显式类型转换。

完整门禁在 Windows/amd64、`GOMAXPROCS=1`、`-p 1` 下通过：

```text
./scripts/validate.sh                                      PASS (9/9)
go test -tags=integration -count=5 -p 1 ./test/integration PASS
docker golang:1.26.7-alpine3.24 govulncheck ./...          No vulnerabilities found
```

容器证据使用 `postgres:15.8-alpine`（digest `sha256:8b963ea3038c3b32182ee7f592ccde21242fa7c5fd9d1b72aa333c27f1bfc809`）；六个应用镜像真实构建，Prometheus rules `12` 条由 `promtool` 检查通过。

上述结果仍不等同于生产环境验收。Redis/PostgreSQL 多副本故障注入、quota storm/容量压测、Telegram/企业微信 sandbox、Kubernetes rollout、OTLP TLS、KMS/Vault workload identity、服务间 mTLS 及 concrete backend migration adapters 仍需在目标基础设施中执行。

- Go `1.25.14` minimum-version compatibility was re-run on the current source after the lease-lock changes: `go test -buildvcs=false -count=1 -p 1 ./...` passed.

## 2026-08-28 Lease-fence completion (current snapshot)

The lock-wait audit was extended from retry paths to every PostgreSQL
lease-sensitive mutation. `RenewInbox`, `CompleteInbox`, approval/block/dead-letter
Inbox transitions, and all Outbox renew/dispatch/delivery/cursor/retry/block/dead-letter
transitions now acquire the target row with `SELECT ... FOR UPDATE` in the same
transaction before evaluating the owner/fence/status/lease predicate with
`clock_timestamp()`. This closes the PostgreSQL `UPDATE` wait race in which a
lease could expire while a worker was blocked on a row lock but the pre-wait
predicate was still accepted.

Evidence added in this pass:

- SQL-shape tests require `BEGIN -> SELECT ... FOR UPDATE -> fenced mutation`
  for every mutation and verify rollback on stale ownership.
- `TestPostgresRetryInboxRejectsLeaseExpiredWhileWaitingForRowLock` proves the
  Inbox retry path returns `ErrStaleLease` and preserves `PROCESSING` state.
- `TestPostgresMarkDeliveredRejectsLeaseExpiredWhileWaitingForRowLock` proves
  the Outbox provider-side-effect fence returns `ErrStaleLease` and preserves
  `DISPATCH_STARTED` without setting `delivered_at`.
- `TestPostgresCompleteInboxRejectsLeaseExpiredWhileWaitingForRowLock` proves
  a stale completion leaves the Inbox in `PROCESSING` and creates no Outbox
  reply.
- Real PostgreSQL integration passed five consecutive serial runs after the
  Outbox regression, and three consecutive runs after the completion-path
  regression was added.

The complete local gate also passed after this completion:

```text
./scripts/validate.sh                                      PASS (9/9)
go test -tags=integration -count=5 -p 1 ./test/integration PASS
```

The gate ran with Go `1.26.7`, module minimum `1.25.14`, `GOMAXPROCS=1`, and
`-p 1`; it built all six application images, checked 12 Prometheus rules, and
used a disposable `postgres:15.8-alpine` container. This remains source and
single-database evidence, not proof of Redis/PostgreSQL multi-replica failure
injection, IM provider sandbox behavior, Kubernetes rollout, OTLP TLS,
KMS/Vault workload identity, service mTLS, capacity limits, or concrete
backend migration adapters.

## 2026-08-28 Redis-to-PostgreSQL migration vertical slice

This continuation adds one bounded concrete backend path instead of claiming
that the generic migration protocol is itself an adapter:

- `RedisSource` reads a version-watermark snapshot and change stream from a
  tenant/domain-scoped version index. Immutable versioned payload keys make
  the payload read stable even when the index advances concurrently. It rejects missing payloads, duplicate
  versions across a page boundary, non-integral or non-exact Redis scores,
  invalid cursors and non-monotonic batches.
- `PostgresTarget` validates the authoritative `data_migrations` lease at the
  start and at the commit boundary, and writes migration 037's opaque records
  in the same transaction. Inserts use `ON CONFLICT DO NOTHING`; conflicts are
  row-locked and resolved by monotonic version, while equal-version hash
  conflicts fail closed.
- Redis payloads are read from immutable version-suffixed keys. Deletions are
  represented by a separate retained `:deleted=1` marker and persisted as
  `deleted=true`;
  an unconditional migration-037 drop is blocked while target rows exist.
- A disposable `postgres:15.8-alpine` container ran the full integration suite
  once and the new vertical-slice test three additional consecutive times.
  The test applies all embedded migrations, runs the Executor through snapshot,
  CatchUp, rollback window and explicit completion, then verifies two target
  records.
- Fresh source gates after this slice passed on local Go `1.26.2`: full unit
  tests, full race tests, vet, build and module verification. The minimum Go
  `1.25.14` full test gate passed, and Go `1.26.7` `govulncheck` reported
  `No vulnerabilities found`. The native static script also passed in a Linux
  Bash container after its non-portable/fail-open marker scan was corrected.

This evidence does not include a real Redis server, Redis/PostgreSQL replica
failover, production Session/Memory schema projection, a capacity result or a
cutover against live provider traffic. Vector and object-store adapters remain
unimplemented.

## Final continuation gate

After the scope-binding, payload-bound, tombstone, rollback-guard and
fail-closed static-scan fixes, the current source passed:

```text
go test -buildvcs=false -count=1 -p 1 ./...                 PASS
go test -buildvcs=false -race -count=1 -p 1 ./pkg/datamigration ./migrations ./pkg/reliable ./pkg/storage ./pkg/worker PASS
go vet -buildvcs=false -p 1 ./...                          PASS
go build -buildvcs=false -p 1 ./...                        PASS
go mod verify                                               PASS
go test -buildvcs=false -tags=integration -count=3 -p 1 ./test/integration PASS
docker bash:5.2 scripts/static_verify.sh                  PASS
go1.26.7 govulncheck ./...                                  No vulnerabilities found
```

The integration run used a disposable PostgreSQL 15.8 container and included
the Redis-to-PostgreSQL vertical slice three consecutive times. The race run
covered the changed migration package and all lease/storage/worker packages;
the earlier full-repository race gate also passed before this final narrow
continuation. Docker Compose image rebuild, provider sandboxes, replica
failover and production rollout remain separate acceptance work.

## 2026-08-30 local production-like continuation

The previous paragraphs are historical snapshots. This continuation started
the infrastructure that those snapshots explicitly left open; exact scope and
non-production boundaries are recorded in
历史容量验证材料不包含在当前精简交付包中。

Fresh source gates after the fair-queue and deployment fixes:

```text
go mod verify                                               PASS
gofmt -l .                                                  PASS (no output)
go build -buildvcs=false -p 1 ./cmd/...                     PASS
go vet -p 1 ./...                                           PASS
go test -buildvcs=false -count=1 -p 1 ./...                 PASS
go test -buildvcs=false -race -count=1 -p 1 ./...           PASS
go1.27.0 go mod verify                                      PASS
go1.27.0 go test -buildvcs=false -count=1 -p 1 ./...        PASS
go test -tags=integration -count=1 -p 1 ./test/integration  PASS (176.984s)
releaseverify --schema-class compatible ...                 PASS
GOTOOLCHAIN=go1.26.7 ./scripts/validate.sh                   PASS (10/10)
go1.26.7 govulncheck v1.7.0 ./...                            No vulnerabilities found
```

The Go 1.27.0 compatibility run left both `go.mod` and `go.sum` byte hashes
unchanged. Production image builds remain pinned to Go 1.26.7; the module's
`go 1.25.14` directive remains a separate minimum source contract.
The final 10/10 validation rebuilt all six images, checked 12 Prometheus rules,
ran the isolated real PostgreSQL/Redis integration suite in 7.286 seconds and
completed the static gate. The separate Windows-to-K3d port-forward integration
run took 176.984 seconds; both passed, but they exercise different network
paths.

Infrastructure evidence added in this continuation:

- A three-node K3d/K3s lab completed a compatible migration, immutable-digest
  rollout and Gateway rollback. Worker/Admin readiness is now awaited before
  Consumer/Delivery admission.
- Linkerd CNI/native sidecars enforced the same-request identity distinction:
  a meshed client reached application authentication (401), while an
  unmeshed/no-identity client was rejected by Linkerd (403). NetworkPolicy
  tests now require transparent-proxy port 4143 and control-plane egress.
- Gateway/Worker HPAs use `ContainerResource`, with post-rollout
  `ScalingActive=True/ValidMetricFound`; this avoids sidecar request gaps that
  made the earlier Pod Resource metric unknown.
- Correct OTLP CA trust succeeded and a rogue CA failed.
- Official Vault chart 0.34.1/Vault 2.0.4 in dev mode validated a scoped
  Kubernetes identity and denied the default ServiceAccount. This is not a
  claim of HA Vault, auto-unseal or cloud KMS.
- Real PostgreSQL 15.8 and Redis 7.4 integration ran in an isolated database.
  The first attempt incorrectly reused the populated capacity database and
  therefore claimed unrelated global queue rows; it was rejected as invalid
  evidence. The clean isolated rerun passed every test.

Capacity testing exposed a real PostgreSQL fair-claim race: a candidate CTE
could wait while another transaction completed the row, after which the old
final `UPDATE ... WHERE id=$1` resurrected the terminal Inbox to PROCESSING.
The final mutation now rechecks the current state and eligibility timestamps;
a changed candidate returns `ErrNoWork`, rolling back the schedule increment.
`TestPostgresFairClaimDoesNotResurrectCompletedStaleCandidate` first failed
against the old SQL and passes after the fix. The rebuilt Consumer was pushed
and rolled out at:

```text
trpc-v13-registry:5000/v13/consumer@sha256:1d7758cf8261681b2701fff78d6e6af746a474d4126b77094920533138f71208
```

Two replicas were Ready with zero application/proxy restarts and explicit
`FAIR_QUEUE_ENABLED=true`, `CONCURRENCY=4`. The clean final capacity run was:

```text
CAPACITY_RUN=cap-1788060387113445463
ENQUEUE total=2200 workers=16 elapsed=12.456120016s throughput=176.6_ops_s p50=89.020022ms p95=99.016557ms p99=129.954639ms
FAIRNESS sample=200 quiet_claims=100 first_quiet_position=2 max_consecutive_noisy=1
CLAIM_COMPLETE total=2200 workers=8 elapsed=2m7.233270184s throughput=15.7_ops_s p50=101.184408ms p95=195.918054ms p99=1.050497368s
DURABILITY completed_inbox=2200 outbox=2200 errors=0
```

These numbers are a laptop/K3d/single-PostgreSQL-Pod baseline, not a target
capacity promise. Real WeCom/Telegram sandboxes, a vendor-supported production
Kubernetes/Linkerd combination, production KMS/Vault rotation, PostgreSQL and
Redis HA failover, cloud vector/object backends, DR and target workload/cost
measurements remain external gates.

# Enterprise Multi-Tenant Agent Platform Acceptance Evidence

更新日期：2026-09-05（Asia/Shanghai）

评委快速入口：先看 [评委快速摘要与验收执行清单](JUDGE_QUICKSTART_V14.md)，再按本表核对命令、容器后状态和外部验收边界。

## 状态定义

- `LOCAL_VERIFIED`：本机对当前 V14 源码执行过对应命令或真实容器链路，退出码/后状态通过。
- `IMPLEMENTED`：生产路径已有实现与自动化回归，但需要目标账号或目标基础设施才能完成最终验收。
- `EXTERNAL_REQUIRED`：配置/Runbook 已提供，当前本机没有足够的外部授权或真实环境证据。
- `DESIGNED`：明确保留的扩展面，不宣称已实现。

## 需求覆盖

| 验收项 | 实现/证据入口 | 当前证据 | 状态 |
|---|---|---|---|
| tenant/app/model/tool/channel/storage/audit 模型 | `pkg/tenant`, `pkg/controlplane`, migrations 001–042 | validation、RBAC、optimistic-lock、immutable version tests | LOCAL_VERIFIED |
| 无状态多节点与 Session 路由 | `pkg/pipeline`, `pkg/worker`, `pkg/storage` | session FIFO、Redis lease、PostgreSQL takeover/fence 集成 | LOCAL_VERIFIED |
| Inbox/Outbox 幂等与乱序 | `pkg/reliable`, migrations 002/013–016/033–036 | 重复、payload conflict、分段 cursor、过期接管、fair queue 测试 | LOCAL_VERIFIED |
| 企业微信 | `pkg/channel/wework_adapter.go`、Gateway/Delivery composition | URL verify、签名、AES、CorpID、分段契约自动化 | IMPLEMENTED |
| Telegram | `pkg/channel/telegram_adapter.go`、Gateway/Delivery composition | secret header、消息规范化、429/retry 契约自动化 | IMPLEMENTED |
| 真实 IM sandbox | `docs/EXTERNAL_ACCEPTANCE_RUNBOOK.md` | 需讲师/目标账号 webhook、token 与回调网络 | EXTERNAL_REQUIRED |
| PostgreSQL 控制面/队列 | migrations、`pkg/reliable/postgres*` | 本地 PostgreSQL 15.8 全量 integration | LOCAL_VERIFIED |
| Redis Session/Memory/coordination | `pkg/storage`, budget/nonce/session lease | 本地 Redis 7.4 integration；无 InMemory 生产回退 | LOCAL_VERIFIED |
| 生产 Summary Generator | `pkg/summary`, `pkg/summaryruntime`, `cmd/summary-worker`, migration 042 | 真实 PG/Redis + 捕获模型：enqueue→freeze→generate→fenced checkpoint→下一次 Runner 请求；断言摘要/新消息存在且旧历史被裁剪 | LOCAL_VERIFIED |
| Summary 取消与 goroutine 排空 | `pkg/summary/poller.go`, `processor.go` | 父取消停止 claim、活跃 job 排空；timeout 后 FAILED 状态持久化 | LOCAL_VERIFIED |
| Knowledge 真实数据面 | `pkg/runtimeplane`, `pkg/knowledgeplane`, `pkg/platformtool` | 真实 Qdrant：seed/search、tenant/app scope、framework Knowledge 注入 | LOCAL_VERIFIED |
| Artifact 真实数据面 | `pkg/runtimeplane`, `pkg/artifactplane`, migration 039 | 真实 PostgreSQL+MinIO：save/load/list/delete、版本/hash/幂等 release | LOCAL_VERIFIED |
| Redis→SQL Session migration | `pkg/datamigration`、`pkg/dataprojection/session.go`、migration 037 | 官方 Redis Session→规范化 State/Event/Track→官方 PostgreSQL Session；snapshot/catch-up/shadow/CAS cutover 集成 | LOCAL_VERIFIED |
| Session/vector/object projection | `pkg/dataprojection`, migration 040 | fenced record→PostgreSQL Session/Qdrant/MinIO、tombstone、重放、失败 marker/final fence tests | LOCAL_VERIFIED |
| 迁移 cutover/rollback | `pkg/datamigration` operator hooks | 状态机与 CAS/lease 回归；正式数据量演练待目标环境 | IMPLEMENTED |
| Tool/Guardrail/预算/审批 | `pkg/governance`, `pkg/approval`, `pkg/budget` | Runner Plugin、Redis Lua、durable challenge 单元/SQL 集成 | LOCAL_VERIFIED |
| MCP 运行时 | `pkg/platformtool/mcp.go`, `pkg/worker/mcp_runtime_integration_test.go` | official MCP ToolSet + Streamable HTTP：Worker→Runner→治理→远端 Tool→模型回合；Header SecretRef、超时、精确工具过滤、错误脱敏和进程关闭 | LOCAL_VERIFIED |
| Trace/metrics/audit | `pkg/telemetry`, `pkg/audit`, Prometheus rules | W3C trace propagation、指标鉴权/基数、15 条规则 promtool 解析 | LOCAL_VERIFIED |
| Summary 告警 | `cmd/summary-worker`, `deploy/prometheus-rules.yml`, `docs/SLO.md` | 30/60/120/300s buckets，失败突增/失败率/高延迟规则 | LOCAL_VERIFIED |
| Compose 最小部署 | `deploy/docker-compose.yml`, isolated overlay | V14 隔离栈应用健康、migrate=0、公开探针 200、restart=0 | LOCAL_VERIFIED |
| Kubernetes 模板/发布门 | `deploy/kubernetes`, `pkg/releaseverify`, `scripts/k8s_apply.sh` | Secret 最小暴露、profile ConfigMap、egress policy、digest/rollout tests | LOCAL_VERIFIED |
| 正式 Kubernetes/mesh rollout | 外部验收 Runbook | 需目标集群、正式 CA、供应商支持版本 | EXTERNAL_REQUIRED |
| KMS/Vault workload identity | `tenant.SecretResolver` seam、K8s Secret 示例 | 本地只验证 resolver/fail-closed，不是正式 KMS/Vault | EXTERNAL_REQUIRED |
| HA chaos、容量、PITR/DR | SLO/Runbook/容量模型 | 尚需目标规格与测试窗口 | EXTERNAL_REQUIRED |
| Graph/Chain/Parallel/Cycle runtime | `pkg/worker/runtime_factory.go`、runtime factory/composition tests | 四种内置上游 Agent 构造、实际拓扑执行、Worker composition；节点预算/工具/提示词、DAG/可达性和有限循环校验 | LOCAL_VERIFIED |

## 后端支持范围与证据边界

`pkg/tenant/tenant.go:224-235` 的 `StorageConfig` 当前生产支持范围是：Session/Memory 使用 operator-owned Redis 或 PostgreSQL profile；Knowledge 使用 Qdrant profile；Artifact 使用 S3-compatible（本地验收为 MinIO）profile。`Summary` 的 job/checkpoint 与 `Audit` 的平台审计表固定由平台 PostgreSQL 持有，当前没有租户可选的 Summary/Audit backend/profile，也没有把固定 PostgreSQL 设计包装成“任意后端可切换”。因此本矩阵中的“多后端”是跨数据域的已安装组合，而不是六类数据全部各自拥有可插拔后端。

`inmemory` Session/Memory 仅用于单元测试和显式本地 composition；生产 Worker/Admin 在 `ValidateDistributedStorage` 路径拒绝进程内状态，不能据此宣称多副本生产后端覆盖。当前已实测的迁移闭环是 Redis→PostgreSQL Session，以及带 fence 的 Session/Knowledge/Artifact projection；Summary checkpoint 和 Audit 不在该迁移范围内。

Knowledge 的真实数据面测试使用真实 Qdrant/MinIO，但 embedding 请求由本地 httptest OpenAI-compatible 服务响应，Summary 使用本地捕获模型。它们证明协议、租户隔离和数据流，不证明外部模型/embedding provider 的额度、质量、限流、TLS、SLA 或版本兼容。

## 2026-09-05 接续基础设施证据

| 检查 | 实际证据 | 边界 |
|---|---|---|
| C 盘宿主 | Docker 29.7.2 恢复；safe-start 仅移动损坏 runtime 目录到可恢复备份，未删除 image/volume/database；C 盘仍约 35 GB 可用 | 仍需把 safe-start 纳入宿主机开机/重启运维流程 |
| C-local Compose | `scripts/validate.sh` 10/10 全绿；7 个应用镜像逐个构建；真实 PG/Redis/Qdrant/MinIO integration 10.862s；Gateway/Admin 200、Prometheus 6/6、15 rules | 本地隔离验证，不是生产认证 |
| K3d V14 bootstrap/compatible | `agent-platform-v14` migration complete；17 个应用 Pod 及 PostgreSQL/Redis Ready；compatible migration 重放成功；3-node HPA metrics valid | k3d-trpc-v13 lab，不是目标集群 |
| Linkerd identity | 带 identity 的 Consumer-labelled probe 到 Worker protected route 为应用 401；无 identity 的同请求为 Linkerd 403 | 本地 all-authenticated policy；不等于正式 strict mTLS/证书轮换 |
| Gateway rollback | V14 digest → V13 digest → 原 V14 digest 真实 rollout/restore 成功，3 replicas Ready | 只覆盖 Gateway image rollback，不覆盖全平台 breaking schema rollback |
| Vault workload identity | 本地 dev Vault：绑定 `vault-client` 的 projected JWT 可读允许路径并被拒绝 forbidden path；错误 ServiceAccount 返回 403 | 不等于 HA/auto-unseal/cloud KMS |
| Fair queue capacity | 干净隔离 PostgreSQL：2200 Inbox/Outbox 完成，errors=0，quiet claim first position=2，max consecutive noisy=1 | 单机实验室基线，不是目标容量承诺 |
| External IM | callback public health 200；无 route key 的 `/webhook` 返回 400；企微/Telegram 凭据均未配置 | 真实控制台登录、URL verify、消息回路仍 EXTERNAL_REQUIRED |

容量基线的入口是 `scripts/test_local_capacity.ps1 -HarnessPath <Go harness>`；脚本负责创建一次性 C 盘/loopback PostgreSQL、迁移 schema、执行外部 harness 并清理容器。历史容量结果不随当前精简交付包提供，不能宣称从全新归档目录直接重放 benchmark。该脚本和结果均不覆盖模型吞吐、端到端 IM、HA 故障、生产 sizing、成本或 DR。

## 当前源码门禁

生产工具链以 `GOTOOLCHAIN=go1.26.7+auto`、`GOMAXPROCS=1` 和 `-p 1` 运行：

```powershell
go mod verify
gofmt -l cmd pkg migrations test
go build -buildvcs=false -p 1 ./cmd/...
go vet -p 1 ./...
go test -buildvcs=false -count=1 -p 1 ./...
go test -buildvcs=false -race -count=1 -p 1 ./...
go test -buildvcs=false -tags=integration -count=1 -p 1 ./test/integration
```

以上源码门禁在本次 C 盘复核均通过（完整串行轮次约 199.82 秒）。真实 integration 使用隔离端口的 PostgreSQL、Redis、Qdrant 和 MinIO；本次 C-local 轮次用时 9.775 秒。MinIO 凭据从测试容器环境只在进程内读取，未写入报告。`TestExecutionReconcilerOnlyAbandonsStaleRunningRecords` 曾暴露测试 cleanup 的外键删除顺序错误；修复为先删 reconciliation/guard/result/binding 再删 execution/app/tenant，并连续执行两次通过，运行后 `reconcile-%` 租户与执行记录均为 0。该问题属于测试隔离，生产 reconciler 的全局扫描语义没有改成按测试租户过滤。

`TestSummaryRuntimePostgresRedisEndToEnd` 在补充 Runner 请求捕获后先按预期失败：checkpoint 已在 `Session.Summaries[""]`，但上游 LLMAgent 默认 `BranchFilterModePrefix` 按 Agent 分支取摘要，导致全会话 checkpoint 被忽略。生产 Worker 现仅在启用平台 Summary data plane 时同时设置 `WithAddSessionSummary(true)` 与 `BranchFilterModeAll`。修复后该测试使用真实 Redis Session、真实 PostgreSQL checkpoint 和本地捕获模型通过，并同时断言 cutoff 前 Event 不再进入请求；这条测试不调用外部 LLM、不会消耗 Provider 次数。

`TestTRPCSessionRedisToPostgresMigrationVerticalSlice` 使用 tRPC-Agent-Go v1.11.x 的真实 Redis/PostgreSQL Session 模块：snapshot 后在 Redis 继续追加 Event 并修改 State，迁移再 catch-up；目标从官方 PostgreSQL Service 读回最终 State、两条 Event 与 Track，之后租户配置以 CAS 从 version 1 切到 version 2。重复/分叉/上限/私有 summary 的单元契约在 `pkg/dataprojection/session_test.go`。第一次真实运行还暴露上游初始化器对 PostgreSQL 63 字节 identifier 截断的 fail-closed 行为；测试和生产 profile 均要求短、固定 schema 名。

`TestMCPProfileWorkerRunnerGovernanceVerticalSlice` 启动本地 Streamable HTTP MCP server，使用框架官方 MCP ToolSet 完成 `Worker.Process → Runner/LLMAgent → BeforeTool/AfterTool 治理 → MCP call → 模型最终回复`。测试断言 Header SecretRef 被注入、远端名只能通过 operator 前缀暴露、审计包含 allow/success 且不含凭据或用户正文；这证明本地协议纵切，不代表任一真实业务 MCP server 已验收。

## Docker Desktop 修复证据

Docker Desktop 因 `Docker\run\sailor-ingest.sock`、`dockerInference` 等损坏 AF_UNIX reparse point 启动失败。故障期间没有 factory reset，也没有删除 Docker data/volume/image。停掉卡死进程后，把两个运行时目录移动为可恢复备份并重启：

- `C:\Users\admin\AppData\Local\Docker\run.stale-20260903-234447`
- `C:\Users\admin\AppData\Local\docker-secrets-engine.stale-20260903-234447`

后状态为 Docker client/server 29.7.2 可响应，V13 容器仍健康，V14 PostgreSQL/Redis/Qdrant/MinIO 仍可访问。备份目录尚未删除。

## 2026-09-05 C 盘隔离栈复核

本次从 C 盘源码副本重新执行，确认运行链路不依赖 E 盘：

| 检查 | 实际结果 |
|---|---|
| Compose 项目 | `trpc-v14-c-local-20260905`；配置来自 C 盘源码和 `docker-compose.isolated.yml` |
| 容器后状态 | 12 个容器；11 个服务运行且健康，one-shot `migrate` 退出码 `0` |
| HTTP 探针 | Gateway/Admin `/health` 均 HTTP `200`；Grafana `/api/health` HTTP `200`；Prometheus `/-/healthy` HTTP `200` |
| Prometheus | 6/6 active targets 为 `up`；规则 API 返回 15 条，全部 `health=ok` |
| 稳定性 | 本轮所有 V14 临时容器 restart count `0`；容器日志未发现 `panic` 或 `fatal` |
| Admin 纵切 | 未认证 `401`；创建租户、脱敏 GET、Agent App、Version、Publish、stable Deployment、列表和删除全部通过 |
| 外部 preflight | 合法形状的假值运行，`provider_calls=0`；不发送真实 Provider 请求 |
| 集成 | `go test -buildvcs=false -tags=integration -count=1 -p 1 ./test/integration` PASS（9.775 秒） |
| 镜像 | Compose 配置 PASS；7 个应用镜像构建 PASS |

直接在解压目录运行 `docker compose` 若没有 `.env` 会返回 `set POSTGRES_PASSWORD in .env` 等必需变量错误，这是源码不携带秘密的 fail-closed 设计，不是 E 盘故障。`scripts/run_c_local_stack.ps1 -Build` 从脚本位置定位源码，在进程内注入一次性验证值并使用隔离端口；本次已验证该路径。权威树的真实 `deploy/.env.wecom.local` 只做存在性记录，未读取、未输出、未进入归档。

## 不得扩大解释的边界

1. 本地假 embedding/summary 模型证明协议与 Runner 数据流，不证明外部 Provider 额度、质量、限流或 SLA。
2. Compose/Kubernetes YAML 和 releaseverify 不等于目标集群 rollout 已成功。
3. 本地 MinIO/Qdrant 验证不等于云 S3/COS 或托管向量库的 IAM、网络与灾备验收。
4. 单机多容器接管测试不等于 PostgreSQL/Redis HA 故障注入。
5. source/race 测试不能替代真实企业微信和 Telegram sandbox。
6. 本地 MCP server 不能替代目标 MCP 服务的认证、DNS/egress、配额、幂等和 SLA 验收。

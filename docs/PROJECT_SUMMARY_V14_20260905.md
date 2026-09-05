# tRPC-Agent Enterprise V14 项目总结

更新日期：2026-09-05（Asia/Shanghai）  
项目：基于 tRPC-Agent-Go 的多租户节点化 Agent 平台  
实现框架：tRPC-Agent-Go v1.11.2  
模块最低版本：Go 1.25.14  
生产构建工具链：Go 1.26.7

## 1. 一句话结论

V14 是一套以 PostgreSQL 可靠队列和控制面为权威、以 Redis/PostgreSQL Session/Memory 为共享运行态、以无状态 Worker 执行 tRPC Runner、并把 Summary、Knowledge、Artifact、MCP、治理、审计和部署安全边界接入生产组合根的候选生产实现。源码、自动化回归和本地后端纵切已形成闭环；真实企业微信/Telegram 账号、目标集群、正式 KMS/Vault、HA/灾备和业务 MCP 仍必须在目标环境完成外部验收，不能把本包称为生产认证。

## 2. 当前源码范围

当前权威目录是本目录。源码快照（不含被忽略的真实环境文件）目前包含：

- 交付清单会在打包时重新计算文件数和字节数；当前脱敏快照为 432 个可交付文件、3,128,319 字节，其中 Go 278、SQL 84、Markdown 28、YAML/YML 17、Shell 5、PowerShell 6，另含安全模板 `.env.example`。权威目录另有一个被忽略的真实 `deploy/.env.wecom.local`，只记录存在性，不计入交付；最终以包内清单为准。
- 9 个服务/作业入口：`gateway`、`consumer`、`worker`、`summary-worker`、`delivery`、`admin`、`migrate`、`replay`、`releaseverify`。
- 42 个版本化数据库迁移；每个 `.up.sql` 都有对应 `.down.sql`。
- `cmd/` 入口与测试、`pkg/` 平台实现与回归、`migrations/` schema、`deploy/` Compose/Kubernetes/监控、`scripts/` 验证和外部验收向导、`test/integration/` 真实后端纵切、`.github/workflows/` CI 门禁。
- 顶层入口文档：`README.md`、`PACKAGE_MANIFEST.md`、`CODE_PACKAGE_CONTENTS.txt`、`HANDOFF.md`、`TRPC_AGENT_ENTERPRISE_HANDOFF_V14_FINAL.md`。

## 3. 架构和职责

1. **入口层**：企业微信和 Telegram Adapter 负责验签、解密/规范化、身份和回复目标固化；Gateway 只在 Inbox durable commit 后确认回调。
2. **可靠流水线**：Consumer 进行租户公平调度、同 Session FIFO、lease/fence 和 Consumer→Worker HMAC；Delivery 负责 Outbox 分段、cursor、限流、未知结果 reconciliation 和 DLQ。
3. **执行层**：Worker 绑定 immutable AgentVersion，持有整次 Session lease，构造 tRPC-Agent-Go Runner，连接共享 Session/Memory、治理 Plugin、Knowledge、Artifact 和 MCP。
4. **Summary**：独立 `summary-worker` 领取固定 AgentVersion 的任务，冻结事件边界，生成并预算结算，在 PostgreSQL 做 fenced CAS；下一轮 Worker 用 Session overlay 注入摘要并裁剪已覆盖历史。
5. **控制面**：Admin 管理 Tenant、App、不可变 Version、Deployment、发布/回滚和审计；版本重试不会漂移到另一套配置。
6. **数据面**：PostgreSQL 是控制面、Inbox/Outbox、执行 guard/fence、迁移协调、审计和 Artifact 元数据的权威；Redis/PostgreSQL 提供 Session/Memory/租约/预算；Qdrant 提供 Knowledge；S3/MinIO 保存 Artifact 正文。
7. **安全和观测**：租户作用域、RBAC、SecretRef、预算/审批、HMAC、默认拒绝网络策略、镜像 digest、Prometheus、OpenTelemetry 和脱敏审计贯穿各进程。

## 4. 已实现能力和证据

以下状态沿用 `docs/ACCEPTANCE_EVIDENCE_V14.md` 的定义：`LOCAL_VERIFIED` 表示有本机命令或真实容器链路证据，`IMPLEMENTED` 表示源码和自动化回归已具备但还需要目标环境，`EXTERNAL_REQUIRED` 表示必须由外部账号/基础设施完成。

| 能力 | 当前状态 | 主要入口 |
|---|---|---|
| Tenant/App/Model/Tool/Channel/Storage/Audit 模型 | LOCAL_VERIFIED | `pkg/tenant`、`pkg/controlplane`、migrations 001–042 |
| Inbox/Outbox 幂等、顺序、租约、fence、重放 | LOCAL_VERIFIED | `pkg/reliable`、`pkg/pipeline` |
| 无 sticky session 的多节点 Worker | LOCAL_VERIFIED | `pkg/storage`、`pkg/worker`、Session FIFO/Redis lease |
| 企业微信加密文本 1:1 回调和主动回复 | IMPLEMENTED | `pkg/channel/wework_adapter.go` |
| Telegram webhook、429/retry 和分段回复 | IMPLEMENTED | `pkg/channel/telegram_adapter.go` |
| Summary 生成、预算、取消排空和 Runner overlay | LOCAL_VERIFIED | `pkg/summary`、`pkg/summaryruntime`、`cmd/summary-worker`、migration 042 |
| Qdrant Knowledge 租户/App 隔离 | LOCAL_VERIFIED | `pkg/knowledgeplane`、`pkg/platformtool` |
| S3/MinIO Artifact 不可变版本、hash、tombstone | LOCAL_VERIFIED | `pkg/artifactplane`、migration 039 |
| Redis→PostgreSQL Session 迁移和 Session/Vector/Object projection | LOCAL_VERIFIED | `pkg/datamigration`、`pkg/dataprojection`、migration 037/040 |
| 工具白名单、危险操作审批、预算和结果脱敏 | LOCAL_VERIFIED | `pkg/governance`、`pkg/approval`、`pkg/budget` |
| MCP Streamable HTTP/SSE 运行时纵切 | LOCAL_VERIFIED | `pkg/platformtool/mcp.go`、`pkg/worker/mcp_runtime_integration_test.go` |
| Trace、Metrics、Audit 和 Summary 告警 | LOCAL_VERIFIED | `pkg/telemetry`、`deploy/prometheus-rules.yml` |
| Compose/Kubernetes 模板和 releaseverify | LOCAL_VERIFIED | `deploy/`、`pkg/releaseverify`、`scripts/k8s_apply.sh` |
| 真实 IM sandbox | EXTERNAL_REQUIRED | `docs/EXTERNAL_ACCEPTANCE_RUNBOOK.md` |
| 正式 Kubernetes/mesh、KMS/Vault、云 IAM、HA/DR、容量 | EXTERNAL_REQUIRED | `docs/EXTERNAL_ACCEPTANCE_RUNBOOK.md`、`docs/RISK_REGISTER_V14.md` |
| Graph/Chain/Parallel/Cycle concrete runtime | LOCAL_VERIFIED | 内置上游 Agent factory、实际拓扑执行、Worker composition；自定义 runtime 仍要求稳定 capability identity |

## 5. 重要一致性和安全决策

- Gateway 先写 Inbox 再 ack；数据库失败不能伪造成功。
- PostgreSQL lease-sensitive mutation 在同一事务内先锁行，再按 `clock_timestamp()` 重检 owner/fence/expiry，避免等待行锁期间租约过期仍被接受。
- Provider/Tool/模型响应在副作用边界后丢失时进入 reconciliation，不自动重跑可能已经产生副作用的请求；系统明确是 at-least-once，不宣传 exactly-once。
- Tenant、App、Session owner、Channel account、AgentVersion 和 reply target 均在持久化边界固化；Worker 不能覆盖 Gateway 选择的投递路由。
- 租户 JSON 只能保存加密值、profile ID 或 operator-owned `SecretRef`；MCP URL/Header 由运维预注册，Worker 才解析 Header Secret；模型密钥只进入 Worker/Summary Worker，Channel 密钥只进入 Gateway/Delivery。
- Qdrant 物理 ID、Artifact object key 和 Session schema/prefix 都绑定租户/App 作用域；迁移 projection 只有目标副作用成功且最终 fence 仍有效时才写 marker。
- 日志、trace 和审计不保存 token、API key、DSN、Authorization、完整用户正文或原始用户标识；用户标识使用租户 HMAC 假名。
- Docker/Kubernetes 运行时采用 non-root、只读根文件系统、`cap_drop: ALL`、`no-new-privileges`、seccomp、默认拒绝 NetworkPolicy 和不可变镜像 digest 门禁。

## 6. 现有验证记录

`docs/ACCEPTANCE_EVIDENCE_V14.md` 和 `docs/VERIFICATION.md` 记录了此前及本次复核的验证结果，包括：

- `go mod verify`、gofmt、build、vet、全量 unit test、全量 race test 和 integration-tag 编译门。
- 真实 PostgreSQL、Redis、Qdrant、MinIO 纵切；Summary→Runner 请求捕获；Session migration；Knowledge/Artifact projection；MCP 本地 Streamable HTTP 纵切。
- Compose 隔离栈、12 容器健康、公开 Gateway 探针、Prometheus 目标、Grafana health、零重启/无 panic-fatal 的记录。
- 三节点 K3d/Linkerd、镜像 digest rollout/rollback、HPA、OTLP TLS、Vault dev workload identity 和 2,200 条公平队列容量基线的记录。

这些记录都保留了环境和边界说明。它们证明源码和指定本地实验场景，不等于目标生产集群、真实 IM、正式密钥系统或 HA/灾备认证。

## 7. 本次接续复核状态（2026-09-05）

- 已确认权威源码目录、V14 最终交接文档、V14 验收/安全/风险/竞赛材料和历史 V13 材料均在工作区；源码树无 Git 元数据，构建使用 `-buildvcs=false`。
- C 盘权威树与本线程 C 盘副本除真实 `deploy/.env.wecom.local` 外一致；没有任何硬编码 `E:\` 路径。该环境文件含本地企微/运行时秘密，永不进入交付包。
- 直接在新归档目录执行 Compose 时若没有 `.env` 会按设计返回必需变量错误；这不是 E 盘依赖。新增 `scripts/run_c_local_stack.ps1` 从 `$PSScriptRoot` 定位源码，用进程内一次性验证值和隔离端口启动，已用 C 盘副本验证配置与 7 个应用镜像构建。
- 本次 C-local 运行项目为 `trpc-v14-c-local-20260905`：12 个容器（11 个服务加 one-shot migration），migration 退出码 0，Gateway/Admin `/health` 返回 200，所有应用健康，Prometheus 6/6 targets `up`、15 条规则健康，全部 restart count 为 0，日志未发现 panic/fatal。
- C-local 真实后端集成 `go test -tags=integration -count=1 -p 1 ./test/integration` 通过（9.775 秒）；Admin 纵切 401 → tenant → 脱敏读取 → Agent App → Version → Publish → stable Deployment → list → delete 通过；外部验收 preflight 使用假值时 `provider_calls=0`。
- 本次源码门禁在 Go 1.26.7 自动工具链、`GOMAXPROCS=1`、`-p 1` 下通过：module verify、gofmt、build、vet、全量 unit、全量 race（约 199.82 秒）。
- 接续复核补充：`scripts/validate.sh` 10/10 通过（串行七镜像构建、15 条 Prometheus 规则、真实 PG/Redis/Qdrant/MinIO integration 10.862 秒）；K3d V14 bootstrap/compatible migration、Linkerd 401/403 identity probe、Gateway digest rollback、Vault dev workload identity、2200 条公平队列容量基线均有独立脱敏日志。
- 当前企微/Telegram 所需 route key、provider secret、CorpID/AgentID 均未配置；公网 tunnel `/health` 为 200、无 route key 的 `/webhook` 为 400，但真实 IM 回路仍为 `EXTERNAL_REQUIRED`，不能把企微登录过期页当作已登录证据。
- 打包后还会执行：精确文件清单、秘密模式扫描、归档成员复核、SHA-256、临时解包比对，以及只清理本轮两个临时 Compose 项目。

## 8. 交付和外部验收边界

可交付表述应为：**“V14 生产级候选实现，源码与本地实测证据随包提供。”**

在以下证据写入目标环境记录前，不得表述为“已生产上线/生产认证”：

1. 企业微信和 Telegram sandbox 的 URL verify、加密文本、重复回调、限流/失败恢复和真实出站回复。
2. 目标 Kubernetes/service-mesh 的 rollout、rollback、strict mTLS、证书轮换和节点故障。
3. KMS/Vault workload identity、最小权限、HA/auto-unseal、审计和无停机双 key 轮换。
4. PostgreSQL/Redis failover、PITR/异地恢复、RPO/RTO、云 S3/Qdrant IAM 和私网 egress。
5. 业务 MCP profile 的真实认证、出网 allowlist、幂等、配额、超时和 SLA。

## 9. 归档策略

本次总包覆盖当前 V14 源码树的全部可提交文件，并纳入 V13 源码、参考 `trpc-agent-service-wangzilong` 源码（去除 `.git`）、V13/V14 验收/竞赛/架构/风险/安全/原创性文本和交接材料。为避免把凭据扩散到压缩包，明确排除：真实 `.env`/`.env.*`（包括 `deploy/.env.wecom.local`）、Docker volume/image、运行时数据库、日志/缓存、二进制、临时工作目录、E 盘实验室缓存/数据库/证书/私钥/工具以及未审核的嵌套归档。排除项、来源路径、文件计数和 SHA-256 会在包内的 `PACKAGE_INVENTORY_20260905.md` 与 `SHA256SUMS_20260905.txt` 中逐项记录。

推荐阅读顺序：

1. 本文件
2. `README.md`
3. `docs/COMPETITION_SUBMISSION_V14.md`
4. `docs/ACCEPTANCE_EVIDENCE_V14.md`
5. `docs/DATA_MODEL.md`
6. `docs/SECURITY_REVIEW_V14.md`
7. `docs/RISK_REGISTER_V14.md`
8. `TRPC_AGENT_ENTERPRISE_HANDOFF_V14_FINAL.md`
9. `docs/EXTERNAL_ACCEPTANCE_RUNBOOK.md`

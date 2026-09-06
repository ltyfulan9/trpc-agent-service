# Enterprise Multi-Tenant Agent Platform

[![verify](https://github.com/ltyfulan9/trpc-agent-service/actions/workflows/verify.yml/badge.svg?branch=submission-online-migration-20260906)](https://github.com/ltyfulan9/trpc-agent-service/actions/workflows/verify.yml?query=branch%3Asubmission-online-migration-20260906)

面向企业场景的多租户 Agent 部署与运行平台，基于 tRPC-Agent-Go 构建，提供消息接入、可靠执行、租户隔离、版本发布、知识与对象数据面、治理审批和运行观测。

系统围绕三个核心约束组织：消息提交后确认、执行绑定不可变版本、故障恢复验证租约与 fence。企业微信和 Telegram 共用 Gateway → Inbox → Consumer → Worker → Outbox → Delivery 主链路。

## 阅读导航

| 关注内容 | 文档 |
|---|---|
| 项目概览与演示顺序 | [评审指南](docs/JUDGE_QUICKSTART.md) |
| 完整方案与框架复用边界 | [项目方案](docs/COMPETITION_SUBMISSION.md) |
| 模块边界与设计取舍 | [架构](docs/ARCHITECTURE.md)、[设计决策](docs/ARCHITECTURE_REVIEW.md) |
| 数据所有权与一致性 | [数据模型](docs/DATA_MODEL.md) |
| 租户选择不同数据后端 | [多后端适配方案](docs/MULTI_BACKEND_DESIGN.md)、[在线迁移](docs/ONLINE_MIGRATION.md) |
| 在线数据迁移与恢复 | [迁移运行指南](docs/ONLINE_MIGRATION.md) |
| 安全、故障和运行指标 | [安全设计](docs/SECURITY_REVIEW.md)、[风险登记册](docs/RISK_REGISTER.md)、[SLO](docs/SLO.md) |
| 运行及验证 | [交付指南](ENTERPRISE_PLATFORM_HANDOFF.md)、[验证方法](docs/VERIFICATION.md)、[验收矩阵](docs/ACCEPTANCE_EVIDENCE.md) |

## 功能与架构

| 能力 | 执行方式 |
|---|---|
| IM 接入 | 企业微信签名/AES/CorpID 校验、Telegram webhook secret；统一消息身份和回复路由 |
| 可靠消息 | PostgreSQL Inbox/Outbox、幂等键与 payload hash、同 Session FIFO、lease/fence、重试、DLQ 和审计重放 |
| 多租户 | 配置/RBAC、存储键、工具权限、SecretRef 和观测标签按租户作用域隔离 |
| 版本发布 | Agent App → immutable Version → stable/canary Deployment；Session 稳定分桶和请求重试版本绑定 |
| Agent 运行 | LLM、Chain、Graph、Parallel、Cycle；节点提示词、工具白名单、调用预算和拓扑校验 |
| Session / Memory | 共享 Redis/PostgreSQL 服务、完整调用租约、跨节点恢复的会话和长期记忆 |
| Summary | 独立任务、事件边界冻结、固定版本生成、fenced checkpoint、下一轮 Runner overlay |
| Knowledge / Artifact | Qdrant tenant/app 检索；PostgreSQL 元数据与 S3/MinIO 对象、版本、SHA-256 和 tombstone |
| 数据迁移 | Session/Knowledge/Artifact 生产写入捕获、持久化 journal、同步镜像、全量规范记录比对、配置 CAS 切换与回滚；独立 `cmd/data-migrate` 命令 |
| 工具与治理 | Runner Plugin、预算 reservation、危险操作审批、递归脱敏、审计及 MCP profile 白名单 |
| 运维 | 健康与排空、Prometheus、OpenTelemetry、Compose、Kubernetes 发布门禁和默认拒绝网络策略 |

```mermaid
flowchart LR
    INBOUND["企业微信 / Telegram<br/>入站消息"] --> GW["Gateway<br/>验签与租户路由"]
    GW -->|提交后确认| IN[(Inbox)]
    IN -->|FIFO / lease| C[Consumer]
    C <-->|HMAC 请求 / 结果回执| W["Worker<br/>tRPC Runner"]
    C -->|完成 Inbox 的同一事务| OUT[(Outbox)]
    OUT --> D["Delivery<br/>分段投递 / fence"]
    D --> REPLY["企业微信 / Telegram<br/>回复接口"]
```

图中 Inbox/Outbox 均由 PostgreSQL 持久化；Worker 返回执行结果后，由 Consumer 提交完成事务。入站消息与回复接口是同一 IM 平台的两个交互方向。

Admin 管理不可变版本与部署；Worker 进程内的 Storage Adapter 选择官方 Redis/PostgreSQL Session/Memory Service，独立的数据面 Resolver 注入 Qdrant Knowledge 和 PostgreSQL 元数据 + S3/MinIO 对象的 Artifact Service。Summary 由独立 Summary Worker 消费任务并发布 checkpoint，下一轮 Runner 读取摘要。接线与存储所有权见[架构分图](docs/ARCHITECTURE.md#21-session--memory-适配)。

PostgreSQL 另持有控制面、执行 guard、审计和迁移 fence。Runner 缓存按租户、配置、版本和部署标识绑定，并具有容量、TTL 和排空约束。

在线迁移从创建时捕获增量；回滚窗口读目标、写源并镜像目标，`complete` 后读写目标且保留旧数据。活跃迁移会串行化该租户数据域的操作，全量记录校验可能暂时阻塞请求；Memory 在线迁移、Session shared state/native summary/TTL 不在支持范围。前置条件、命令和验证边界见[迁移运行指南](docs/ONLINE_MIGRATION.md)。

各 `cmd/*` 是独立进程组合根，共享协议和领域规则位于 `pkg/*`。Admin/Worker 在 bootstrap、HTTP、policy 和 config 文件中完成装配、协议适配和策略解析；运行时注册表在启动时封存。

### 消息与故障处理

1. Gateway 验证消息、映射租户身份、执行限流，将规范化消息和回复路由提交到 Inbox 后返回确认。
2. Consumer 按持久化 Session 序号领取任务；owner、fence 和有效期共同约束状态提交。
3. Worker 解析首次绑定的 AgentVersion，装配共享数据面和治理插件，执行 Runner 并持久化结果。
4. Consumer 在同一事务中完成 Inbox、创建 Outbox，并按回执登记 Summary 任务。
5. Delivery 在调用 IM 前提交 `DISPATCH_STARTED`，按分段 cursor 推进投递。

重复 ID 同内容复用记录，内容冲突被拒绝。前序消息处于等待核对或死信时，同 Session 后续消息暂停，其他 Session 继续执行。模型/工具/IM 调用结果未知时进入 `WAITING_RECONCILIATION`，由操作者核对后审计恢复。投递采用 at-least-once；外部工具应使用业务幂等键。

## 本地运行

工具链：tRPC-Agent-Go v1.11.2；模块最低 Go 1.25.14，完整验证与容器构建使用 Go 1.26.7。Compose 栈需要运行中的 Docker Linux engine。

### 状态机演示

```powershell
go run -buildvcs=false ./cmd/demo
.\scripts\benchmark_local.ps1 -Count 5
```

演示使用 MemoryStore，展示 lease 接管、陈旧提交拒绝、Inbox/Outbox 状态转换和未知投递结果核对。基准输出该内存实现的 ns/op、B/op、allocs/op。详细步骤见 [演示说明](docs/DEMO.md) 和 [基准说明](docs/BENCHMARK.md)。

### 完整服务栈

在源码根目录执行：

```powershell
.\scripts\run_c_local_stack.ps1 -ProjectName agent-platform-review -Build
```

脚本通过进程环境注入本地验证配置，启动隔离 Compose 项目。默认端口为 Gateway `18080`、Admin `18081`、Prometheus `19095`、Grafana `13000`，均绑定 loopback。迁移任务在应用启动前执行；镜像构建建议保留至少 8 GB 磁盘空间。

```powershell
.\scripts\run_c_local_stack.ps1 -ProjectName agent-platform-review -Down
```

停止命令保留命名卷。直接使用 Compose 时，从 `deploy/.env.example` 配置本地环境，并为数据库、内部认证、审计和 Admin 设置独立随机密钥。

### Agent 配置与管理

管理 API 使用服务端 Principal 进行角色和租户授权。`ADMIN_API_TOKEN` 用于 bootstrap 管理，日常操作通过 `ADMIN_PRINCIPALS_JSON` 分配 `tenant_admin`、`release_manager`、`auditor` 及 tenant allowlist。

| 操作 | API |
|---|---|
| 创建租户 | `POST /api/v1/tenants` |
| 创建 Agent 应用 | `POST /api/v1/agent-apps` |
| 创建版本快照 | `POST /api/v1/agent-versions` |
| 发布版本 | `POST /api/v1/agent-versions/{version_id}/publish` |
| stable/canary 部署 | `POST /api/v1/deployments` |
| 查询工具审批 | `GET /api/v1/tool-approvals?tenantId={tenant_id}` |
| 授权审批 | `POST /api/v1/tool-approvals/{challenge_id}/grant?tenantId={tenant_id}` |

Worker 要求 Agent App 存在 active stable deployment。版本快照保存无密钥配置、模型目录信息和 runtime capability fingerprint；同一请求重试继续使用首次绑定版本。`canaryBps` 范围为 1–9999；不传 canary 时执行 stable 切换。

当前模型工厂支持 OpenAI，模型由 operator-approved catalog 准入。租户通过 operator-owned profile 和 SecretRef 选择数据面及凭据。Admin 响应对密钥脱敏；模型、通道和 MCP 密钥按进程职责注入。

企业微信沙箱配置依次使用 `scripts/wecom_sandbox_tunnel.ps1`、`scripts/wecom_sandbox_setup.ps1`、`scripts/wecom_sandbox_bootstrap.ps1`，参数和控制台步骤见 [接入与部署验收](docs/EXTERNAL_ACCEPTANCE_RUNBOOK.md)。

## 运行约束

- PostgreSQL 是共享协调依赖；执行 fencing 使用连接级 advisory lock，数据库连接使用直连或 PgBouncer session pooling。
- Consumer→Worker 生产连接使用验证证书的 HTTPS，或具有身份认证的 service mesh；HMAC 负责请求完整性和 nonce 防重放。
- `STORAGE_BACKEND_PROFILES` 与 `DATA_PLANE_PROFILES` 保存公开配置，SecretRef 绑定租户、用途、provider 和 model，实际秘密仅授予消费它的进程。
- 工具同时受版本与租户白名单约束；危险工具按 tenant/actor/session/tool/args/invocation 一次性审批。
- token 预算按 UTC 日账本原子预留；已 dispatch 但 usage 未知的调用保留预算占用。金额预算尚未接入，当前拒绝 `maxCostPerDay > 0` 的配置。
- 公平调度使用 `FAIR_QUEUE_ENABLED`、权重、`max_inflight` 与 `max_queued`。外部 Store 需提供公平领取、原子准入及 Outbox dispatch fence 能力。
- `/metrics` 使用 bearer 认证；tenant、agent 和 model 标签由有界 allowlist 管理。
- 重放要求 actor/reason 和可恢复状态。Outbox resume 保留已确认 cursor，restart 从第 0 段发送；操作前核对外部投递结果。

Kubernetes 的镜像 digest、Secret、网络策略、传输和迁移门禁见 [部署指南](deploy/kubernetes/README.md)。真实 IM、目标集群、外部密钥服务和 HA/容量验证统一记录在 [验收矩阵](docs/ACCEPTANCE_EVIDENCE.md)。

## 验证与交付

```powershell
go test -buildvcs=false -count=1 -p 1 ./...
go vet -p 1 ./...
go test -buildvcs=false -race -count=1 -p 1 ./...
```

完整门禁在 Git Bash 或 Linux 环境执行：

```bash
bash ./scripts/static_verify.sh
bash ./scripts/validate.sh
```

完整门禁包含构建、测试、race、容器和真实后端集成，运行条件见 [验证方法](docs/VERIFICATION.md)。交付包包含当前源码、测试、文档、验证记录和文件校验清单，内容索引见 [Package Manifest](PACKAGE_MANIFEST.md)。

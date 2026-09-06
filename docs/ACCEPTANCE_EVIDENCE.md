# 验收证据

Enterprise Multi-Tenant Agent Platform 的验收分为源码回归、后端集成和目标环境三个层次。本文件将功能要求映射到测试入口与验收断言；执行方法见 [验证指南](VERIFICATION.md)。

## 1. 提交包验证记录

验收状态使用统一含义：`LOCAL_VERIFIED` 为记录环境中的实际通过结果，`IMPLEMENTED` 为已实现并提供测试入口的能力，`EXTERNAL_REQUIRED` 为需要目标账号或基础设施执行的验收项。

交付包的 `verification-evidence/current-validation.log` 记录源码快照、工具链、执行时间、命令输出及退出码，覆盖以下检查：

```powershell
go test -buildvcs=false -count=1 -p 1 ./...
go vet -p 1 ./...
go test -buildvcs=false -race -count=1 -p 1 ./...
go run -buildvcs=false ./cmd/demo
```

全量常规测试覆盖未使用 `integration` 构建标签的测试；其中的本地模型、HTTP/MCP 服务和内存后端由测试创建。故障演示使用 `MemoryStore` 验证状态转换。真实后端集成、镜像构建和目标部署分别执行，结果单独记录。

公开 CI 对提交分支执行源码、race、四后端集成、镜像及漏洞检查；Windows job 构建 ZIP，Linux job 解压该 ZIP，校验 SHA-256、Shell 执行位和 LF 行尾，再运行包内静态检查与全量常规测试。`verification-evidence/public-ci.json` 保存运行地址、提交 SHA 和各 job 的结论；本地日志中的 `source_revision` 与 CI 的 `headSha` 标识同一提交。

## 2. 功能与回归断言

| 功能 | 实现与测试入口 | 验收断言 |
|---|---|---|
| 多租户控制面 | `pkg/tenant`、`pkg/controlplane`、`cmd/admin` | Tenant/App/Version/Deployment 按租户授权；更新使用乐观锁；发布版本不可变 |
| 企业微信适配 | `pkg/channel/wework_adapter.go` 及适配器测试 | URL challenge、签名、AES 解密、CorpID、文本规范化和回复分段符合协议 |
| Telegram 适配 | `pkg/channel/telegram_adapter.go` 及适配器测试 | webhook secret 校验、会话标识、分段、429 和 Retry-After 处理正确 |
| 可靠消息 | `pkg/reliable`、`pkg/pipeline` | 重复入站幂等；不同 payload 冲突；同 Session FIFO；过期租约接管后拒绝旧 fence |
| 公平调度 | `pkg/reliable`、`test/integration` | 新租户加入、空闲恢复和策略重置保持当前活跃租户公平性；权重与并发配额生效 |
| 原子完成与投递 | `pkg/reliable`、`pkg/pipeline/delivery.go` | Inbox 完成与 Outbox 创建同事务；分段 cursor 可恢复；发送结果未知进入 reconciliation |
| Worker 执行 | `pkg/worker`、`cmd/worker` | 请求固定 AgentVersion、租户和会话身份；持有 Session lease；执行结果可重用 |
| Runtime 组合 | `pkg/worker/runtime_factory.go` 及组合测试 | LLM/Chain/Graph/Parallel/Cycle 拓扑、节点预算、工具范围和有限循环受校验；运行时注册表启动后封存 |
| SecretRef 授权 | `pkg/tenant`、`pkg/worker`、`cmd/admin` | 引用绑定 tenant、purpose、provider 和 model；发布与执行均检查作用域 |
| 租户后端生命周期 | `pkg/storage` | 后端借用和释放受控；节点 readiness 检查公共依赖，租户后端按使用范围检查 |
| Summary | `pkg/summary`、`pkg/summaryruntime` | 固定事件边界、租约、fenced CAS、取消排空和预算结算；无法证明绝对序号时返回 `ErrTranscriptIncomplete` |
| Knowledge | `pkg/knowledgeplane`、`pkg/platformtool` | 查询和记录绑定 tenant/app；框架 Knowledge 通过运行时装配进入 Runner |
| Artifact | `pkg/artifactplane` | 元数据与正文分离；提交响应丢失不误删正文；删除失败后重试继续清理；不可变版本、SHA-256 与精确版本投影 |
| 数据迁移 | `pkg/datamigration`、`pkg/dataprojection` | snapshot/catch-up/shadow/CAS cutover 状态有序；projection marker 绑定 migration identity；最后写入检查 fence |
| 工具治理 | `pkg/governance/plugin.go`、`pkg/governance/approval_postgres.go`、`pkg/governance/budget.go` | 工具白名单、危险操作审批、预算 reservation/settlement、输出脱敏和审计贯穿执行 |
| MCP | `pkg/platformtool/mcp.go`、`pkg/worker/mcp_runtime_integration_test.go` | 本地 Streamable HTTP 服务完成 Worker→Runner→治理→Tool→回复；检查 Header SecretRef、工具过滤和资源关闭 |
| 内部通信 | `pkg/auth`、`pkg/worker` | HMAC 绑定请求内容和 trace context；nonce 单次使用；错误请求被拒绝 |
| 可观测性 | `pkg/telemetry`、`pkg/telemetry/audit_postgres.go` | trace 跨消息边界传播；指标鉴权和标签基数受控；审计保留决策与错误类 |
| 发布准入 | `pkg/releaseverify`、`deploy/kubernetes` | 镜像 digest、迁移类别、网络策略和 rollout 断言受校验 |

## 3. 真实后端集成

`test/integration` 提供 PostgreSQL、Redis、Qdrant 和 MinIO 的纵向测试，运行前按 [验证指南](VERIFICATION.md) 准备服务及测试环境变量。

| 场景 | 后端 | 关键断言 |
|---|---|---|
| 队列与执行持久化 | PostgreSQL | 事务原子性、lease/fence、重复投递、执行 reconciliation |
| Summary→Runner | PostgreSQL + Redis + 本地捕获模型 | checkpoint 的摘要进入下一次请求，已覆盖历史被裁剪，新消息保留 |
| Session 迁移 | Redis + PostgreSQL | State/Event/Track 从源复制，增量追平后在目标读回，配置通过 CAS 切换 |
| Knowledge 检索 | Qdrant + 本地 embedding 服务 | 写入、搜索、tenant/app 隔离及框架注入 |
| Artifact | PostgreSQL + MinIO | save/load/list/delete、版本、内容哈希和删除标记 |
| 数据投影 | PostgreSQL + Qdrant + MinIO | Session/vector/object 重放、tombstone、失败 marker 与最终 fence |

模型与 embedding 在这些测试中使用本地协议服务，测试对象是平台数据流和后端一致性。线上模型效果与服务额度在目标账号中验证。

## 4. 存储支持范围

| 数据域 | 可用后端 | 配置与所有权 |
|---|---|---|
| 控制面、Inbox/Outbox、执行记录、审计 | PostgreSQL | 平台统一管理 |
| Session/Memory | Redis 或 PostgreSQL | 运维注册 profile，租户选择已授权配置 |
| Summary job/checkpoint | PostgreSQL | 平台统一管理，Runner 读取 checkpoint overlay |
| Knowledge | Qdrant | 运维 profile，tenant/app 作用域 |
| Artifact | PostgreSQL 元数据 + S3-compatible 对象存储 | 运维 profile，本地集成使用 MinIO |

Session 迁移支持 Redis→PostgreSQL；投影覆盖 Session、Knowledge 和 Artifact。`MemoryStore` 用于测试和本地状态机演示，持久化服务使用 PostgreSQL 实现。

## 5. 目标环境验收

以下项目状态为 `EXTERNAL_REQUIRED`，统一按 [目标环境验收 Runbook](EXTERNAL_ACCEPTANCE_RUNBOOK.md) 执行：

| 验收项 | 所需条件 | 验收输出 |
|---|---|---|
| 企业微信 / Telegram 实际收发 | 测试账号、应用凭据、公网 HTTPS 回调 | 入站、执行、出站和 trace 对应记录 |
| 目标 Kubernetes / service mesh | 集群、镜像仓库、身份与网络配置 | rollout/rollback、Ready、镜像 digest、身份 allow/deny |
| 身份与密钥系统 | 目标 OIDC/IAP、KMS/Vault、ServiceAccount | 最小权限、错误身份拒绝、密钥轮换和审计 |
| HA、灾备与迁移切换 | 数据库/Redis 测试实例、备份与恢复窗口 | failover、PITR、RPO/RTO、backlog 恢复和回滚记录 |
| 业务容量与模型服务 | 目标规格、业务 payload、模型账号 | 吞吐、p50/p95/p99、queue lag、错误率、资源与成本 |
| 业务 MCP | 已授权 profile、认证和出网配置 | 工具授权、超时、幂等、配额和实际返回结果 |

记录只收集配置版本、时间、状态、脱敏标识和哈希；凭据、聊天正文、数据库 URL 与证书私钥不进入证据包。

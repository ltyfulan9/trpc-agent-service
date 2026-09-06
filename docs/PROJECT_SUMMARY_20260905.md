# Enterprise Multi-Tenant Agent Platform 项目总结

## 1. 项目定位

本项目是基于 tRPC-Agent-Go 的多租户节点化 Agent 部署平台，面向多个业务共用 Agent 基础设施时的接入、隔离、执行恢复、版本治理与运维需求。

项目采用“共享持久状态、无状态执行节点”的架构：PostgreSQL 记录消息和控制状态，Worker 按不可变版本执行 Agent，Summary、Knowledge、Artifact 和工具治理接入统一运行链路。遇到重复投递、节点接管或外部调用结果未知，系统通过幂等键、lease/fence 和 reconciliation 恢复处理。

实现框架为 tRPC-Agent-Go v1.11.2，模块兼容基线为 Go 1.25.14，构建与安全门禁使用 Go 1.26.7。

## 2. 功能执行链路

| 链路 | 执行过程 |
|---|---|
| IM 收发 | 企业微信/Telegram→Gateway 验签规范化→Inbox 提交→Consumer→Worker→Outbox→Delivery→渠道回复 |
| 多租户发布 | Admin 鉴权→Tenant/App 配置→Version 校验与发布→stable/canary Deployment→Worker 固定版本执行 |
| Agent 运行 | 请求身份验证→共享 Session lease→Runner→LLM/Chain/Graph/Parallel/Cycle→结果持久化 |
| 工具治理 | 工具白名单→预算预留→危险操作审批→工具执行→脱敏、预算结算与审计 |
| Summary | 入队→冻结事件边界→独立 Worker 生成→fenced checkpoint→下轮 Session overlay 与历史裁剪 |
| Knowledge | 运维 profile→tenant/app 作用域→Qdrant 检索→框架 Knowledge→Runner |
| Artifact | 授权→正文写入 S3-compatible 存储→PostgreSQL 元数据→版本/hash 校验→读取或 tombstone |
| MCP | 运维注册 profile→发布准入→Worker 解析 Header SecretRef→官方 MCP ToolSet→治理后调用 |
| 数据迁移 | PREPARE→SNAPSHOT_COPY→DUAL_WRITE→CATCH_UP→VALIDATE→READ_SHADOW→CUTOVER→ROLLBACK_WINDOW→COMPLETE |
| 运维观测 | trace 传播→有界 metrics→脱敏 audit→SLO 告警→核对、重放或回滚 |

## 3. 组件职责

`cmd/` 中的每个入口代表一个独立部署进程或运维命令：

- `gateway`：接收渠道请求，固定租户身份与回复目标，在 Inbox 持久化后确认回调。
- `consumer`：领取消息，进行租户公平调度、同 Session FIFO 和 lease/fence 协调。
- `worker`：装配 tRPC Runner、共享存储和治理插件，执行固定 AgentVersion。
- `summary-worker`：异步生成摘要，管理任务租约、checkpoint 和取消排空。
- `delivery`：按 Outbox 发送回复，维护分段 cursor、重试和结果核对状态。
- `admin`：管理租户、应用、版本、部署、审批与审计接口。
- `migrate`、`replay`、`releaseverify`：管理 schema、审计重放和发布门禁。
- `demo`：展示内存状态机的租约接管、旧写拒绝和未知结果处理。

入口负责配置与依赖装配；协议适配、进程策略与配置解析分别组织；可复用领域行为位于 `pkg/`。数据库迁移在 `migrations/`，部署和监控在 `deploy/`，集成测试在 `test/integration/`。

## 4. 核心设计约束

1. **状态所有权清晰**：平台 PostgreSQL 管理控制面、可靠队列、执行、审计和 Summary checkpoint；Redis/PostgreSQL 管理 Session/Memory；Qdrant 管理 Knowledge；对象存储管理 Artifact 正文。
2. **身份在持久边界固定**：tenant、app、session owner、AgentVersion、channel account 和回复目标随消息绑定，重试使用同一执行配置。
3. **旧节点不能提交**：对租约敏感的 PostgreSQL 修改在事务内锁行，并重检 owner、fence 与到期时间。
4. **副作用单独核对**：外部调用开始后结果未知时转入 reconciliation；经业务结果查询或人工确认后再审计处置。
5. **授权贯穿运行时**：SecretRef 绑定租户与用途；模型与工具受运维 catalog/profile 限定；MCP 凭据在 Worker 侧解析。
6. **共享后端保持作用域**：Session schema/prefix、Qdrant ID 与 Artifact key 绑定 tenant/app；投影 marker 同时绑定 migration identity。
7. **摘要边界可证明**：生成前冻结序号，发布采用 fenced CAS；无法证明完整事件窗口时返回 `ErrTranscriptIncomplete`。
8. **可观测性控制泄露与基数**：日志、trace 和审计保留决策与错误类；用户标识使用租户 HMAC 假名，指标标签由 allowlist 限定。

## 5. 验证与交付

源码包提供常规测试、race 门禁、PostgreSQL/Redis/Qdrant/MinIO 集成测试、故障演示、内存基准、Compose/Kubernetes 配置和 CI workflow。

交付包 `verification-evidence/current-validation.log` 保存本提交的常规全量测试、vet、race 与故障演示输出。功能断言和目标环境状态集中在 [验收证据](ACCEPTANCE_EVIDENCE.md)，执行步骤在 [验证指南](VERIFICATION.md)。实际 IM 收发、目标集群、密钥系统、HA/DR 与业务容量按 [目标环境 Runbook](EXTERNAL_ACCEPTANCE_RUNBOOK.md) 验收。

提交材料包括当前源码、架构与数据模型、竞赛方案、安全与风险说明、操作手册、测试证据和校验清单。凭据、运行时数据、缓存和构建产物不进入提交包。

## 6. 技术价值

平台把 Agent 从单进程交互提升为可部署、可治理、可恢复的多租户服务。主要价值不在于增加一种模型调用方式，而在于统一消息身份、执行版本、共享状态和外部副作用的处理规则，使业务能够在同一套基础设施中部署不同 Agent，并对失败的原因、影响范围和恢复路径进行追踪。

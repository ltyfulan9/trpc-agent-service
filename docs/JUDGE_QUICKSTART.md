# 评委快速入门

Enterprise Multi-Tenant Agent Platform 基于 tRPC-Agent-Go，将多租户 Agent 的接入、版本、执行、数据、治理和运维串成可恢复的服务链路。核心问题是：消息重复、节点中断和外部调用结果未知时，怎样保持执行身份稳定、阻止旧节点写入，并恢复后续处理。

## 1. 先看整体链路

```text
企业微信 / Telegram
  → Gateway 验签与规范化
  → PostgreSQL Inbox
  → Consumer 公平调度、Session FIFO、lease/fence
  → Worker 固定版本执行 tRPC Runner
  → Session / Memory / Knowledge / Artifact / Tool
  → Worker 返回结果；Consumer 事务完成 Inbox + 创建 Outbox/摘要任务
  → Delivery 分段发送、重试或结果核对
  → IM 回复
```

PostgreSQL 持有队列、版本、执行、审计和 fence；Session/Memory 使用共享 Redis 或 PostgreSQL；Knowledge 使用 Qdrant；Artifact 使用对象存储及 PostgreSQL 元数据。系统采用 at-least-once 处理，外部副作用结果未知时进入 reconciliation。

Summary Worker 独立领取摘要任务、读取已提交 Session 并发布 checkpoint，下一轮 Worker 再读取摘要；它不处于单次 IM 回复的同步路径。

## 2. 两分钟故障演示

安装 Go 后，在源码根目录运行：

```powershell
go run -buildvcs=false ./cmd/demo
```

该演示使用 `MemoryStore`，依次展示消息入队、租约过期接管、旧 fence 被拒绝、Inbox/Outbox 完成，以及发送结果未知后进入核对状态。演示无需账号或公网地址；PostgreSQL 持久化验证使用独立集成测试。

演示说明见 [DEMO.md](DEMO.md)。

## 3. 核验实现

```powershell
go test -buildvcs=false -count=1 -p 1 ./...
go vet -p 1 ./...
```

本提交的执行记录位于交付包 `verification-evidence/current-validation.log`。完整源码、race、真实后端、镜像和监控规则验证见 [VERIFICATION.md](VERIFICATION.md)。

建议重点检查以下断言：

| 关注点 | 预期行为 | 阅读入口 |
|---|---|---|
| 版本稳定性 | 重试继续使用已绑定 AgentVersion，运行时注册表启动后封存 | `pkg/controlplane`、`pkg/worker` |
| 租户隔离 | 入口身份、SecretRef、存储作用域和工具授权相互一致 | `pkg/tenant`、`pkg/governance` |
| 消息可靠性 | 同 Session FIFO；旧 lease/fence 不能提交；Inbox/Outbox 原子衔接 | `pkg/reliable`、`pkg/pipeline` |
| 外部副作用 | 已开始发送但结果未知时进入 reconciliation | `pkg/pipeline/delivery.go` |
| 长会话 | 冻结摘要边界、fenced checkpoint、下轮请求裁剪已覆盖历史 | `pkg/summary`、`pkg/summaryruntime` |
| 在线迁移与多后端投影 | 生产写入捕获、intent 恢复、snapshot/catch-up、完整记录比对、配置 CAS 和回滚；旧缓存随持久化路由切换 | `cmd/data-migrate`、`pkg/migrationruntime`、`pkg/datamigration`、`test/integration/online_session_migration_test.go`、`online_dataplane_migration_test.go` |

在线迁移通过 `cmd/data-migrate` 和 Worker/Summary Worker 的生产装饰器运行，`cmd/migrate` 单独负责 schema。新增集成测试使用实际生产协调器与后端适配；旧 `datamigration_test.go` 仍保留库级 hook 验证。能力状态为 `IMPLEMENTED`，实际通过范围以[验收证据](ACCEPTANCE_EVIDENCE.md)中的日志和提交身份为准。

评审迁移时应同时核对[运行边界](ONLINE_MIGRATION.md)：READ_SHADOW 是规范记录全量比对，尚无真实查询流量抽样；活跃迁移串行化租户数据域操作，回滚窗口源写目标读。Session shared state/native summary/TTL 与 Memory 在线迁移不在支持范围，完成迁移保留源数据。

## 4. 阅读顺序

1. [竞赛方案](COMPETITION_SUBMISSION.md)：题目映射、总体设计和技术取舍。
2. [架构](ARCHITECTURE.md) 与 [数据模型](DATA_MODEL.md)：组件边界与状态所有权。
3. [验收证据](ACCEPTANCE_EVIDENCE.md)：功能对应的测试和验收断言。
4. [安全设计](SECURITY_REVIEW.md) 与 [SLI/SLO](SLO.md)：授权、故障处置与服务目标。

## 5. 接入测试账号

企业微信和 Telegram 协议适配及本地回归随源码提供。实际账号收发需要应用凭据与公网 HTTPS 回调；目标 Kubernetes、密钥系统、HA/DR 和业务容量同样按部署环境验收，统一状态及步骤见 [验收证据](ACCEPTANCE_EVIDENCE.md#5-目标环境验收) 和 [目标环境验收 Runbook](EXTERNAL_ACCEPTANCE_RUNBOOK.md)。

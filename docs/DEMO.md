# 故障恢复演示

演示使用 `pkg/reliable` 中的 `MemoryStore`，在本地进程中展示可靠消息状态机的四个关键行为，无需模型账号、IM 凭据或公网回调。

## 运行

在源码根目录执行：

```powershell
go run -buildvcs=false ./cmd/demo
```

程序输出 JSON 事件；断言失败时返回非零退出码。

## 演示过程

| 步骤 | 操作 | 观察结果 |
|---|---|---|
| 消息入队 | 创建带租户、会话和消息标识的 Inbox | 分配消息 ID 与 Session sequence |
| 租约接管 | 首个 owner 的短租约到期，新 owner 重新领取 | 旧 owner 使用旧 fence 完成消息时得到 `ErrStaleLease` |
| 完成衔接 | 新 owner 完成 Inbox 并创建回复 | 生成关联 Outbox |
| 未知结果 | 标记发送开始后让租约过期，再执行 reap | Outbox 转入结果核对，停止自动重发 |

## 讲解重点

“这里展示的是故障发生后的状态如何收敛。节点失去租约后，即使恢复运行也不能用旧 fence 提交；发送一旦开始而结果丢失，系统将它与普通可重试失败区分，交给 reconciliation 核对。”

这是内存状态机演示。持久化、数据库事务和真实后端接管由 [集成验证](VERIFICATION.md#4-后端集成与部署检查) 覆盖；本演示不启动 PostgreSQL 或独立 Worker 进程。

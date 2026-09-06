# 本地可靠性基准

`BenchmarkMemoryStoreInboxOutbox` 测量 `MemoryStore` 的入队、领取及完成链路，用于同条件下比较实现开销。

```text
创建唯一消息和 Session → EnqueueInbox → ClaimInbox（lease/fence）
→ CompleteInbox + 创建 Outbox
```

## 运行

在源码根目录执行：

```powershell
.\scripts\benchmark_local.ps1 -Count 5
```

`Count` 指定 Go benchmark 的目标计时秒数。跨平台等价命令：

```bash
go test -buildvcs=false ./pkg/reliable -run '^$' -bench '^BenchmarkMemoryStoreInboxOutbox$' -benchtime=5s -benchmem
```

## 指标解释

| 输出 | 含义 |
|---|---|
| `ns/op` | 每次完整链路的平均耗时 |
| `B/op` | 每次链路分配的堆内存字节数 |
| `allocs/op` | 每次链路的堆分配次数 |
| `N` | 该轮校准后执行的操作次数 |

结果应同时保存源码快照、操作系统、CPU、内存、Go 版本和完整命令。测试在同一 Store 中保留消息和 Outbox，成本包含累计队列状态的影响，比较时应保持计时与运行条件一致。

## 测量范围

该基准使用内存后端，不包含数据库 I/O、网络、模型或 IM 延迟。正式容量验证采用目标部署和业务 payload，记录吞吐、p50/p95/p99、队列积压、资源与成本，步骤见 [目标环境验收 Runbook](EXTERNAL_ACCEPTANCE_RUNBOOK.md#8-故障回滚与容量)。

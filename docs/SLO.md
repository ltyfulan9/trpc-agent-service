# SLI、SLO、告警与 Runbook

这些目标是上线门槛，不是本压缩包已经测得的成绩。PromQL 必须在目标环境产生真实流量后验证。

## 服务目标

| SLI | 30 天目标 | PromQL/数据源 |
|---|---:|---|
| Gateway durable accept availability | 99.95% | `sum(rate(agent_gateway_webhooks_total{result="accepted"}[5m])) / sum(rate(agent_gateway_webhooks_total{result=~"accepted|persistence_error"}[5m]))`，仅在分母大于 0 时求值 |
| Consumer 单次处理成功率 | 99.0% | `sum(rate(agent_pipeline_messages_total{stage="consumer",result="success"}[10m])) / sum(rate(agent_pipeline_messages_total{stage="consumer"}[10m]))` |
| Delivery 单次投递成功率 | 99.0% | 同上，`stage="delivery",result=~"success|chunk_sent"`；分段成功不是失败 |
| Inbox claim→处理完成 p95 | < 30s（包含 Worker/模型/工具执行） | `histogram_quantile(.95,sum by(le)(rate(agent_pipeline_duration_seconds_bucket{stage="consumer"}[10m])))` |
| Outbox claim→发送 p95 | < 10s | `histogram_quantile(.95,sum by(le)(rate(agent_pipeline_duration_seconds_bucket{stage="delivery"}[10m])))` |
| Inbox queue lag p99 | < 120s | `histogram_quantile(.99,sum by(le)(rate(agent_pipeline_queue_lag_seconds_bucket{stage="consumer"}[10m])))` |
| Outbox queue lag p99 | < 60s | 同上，`stage="delivery"` |
| Automatic queue depth / oldest age | 按租户容量基线设阈值 | `agent_pipeline_queue_depth{queue=~"inbox|outbox"}` 与 `agent_pipeline_queue_oldest_age_seconds{queue=~"inbox|outbox"}`；由 Consumer/Delivery 的 QueueInspector 快照更新 |
| stale-fence commit | = 0 正常态 | `increase(agent_pipeline_fence_rejections_total[10m])` |
| Worker cache saturation | = 0 正常态 | `increase(agent_worker_cache_saturation_total[5m])` |
| execution reconciler errors | = 0 正常态 | `increase(agent_execution_reconcile_errors_total[10m])` |
| Summary attempt success rate | > 80%，且连续失败 < 5/10m | `1 - sum(increase(agent_summary_runs_total{result="failed"}[10m])) / clamp_min(sum(increase(agent_summary_runs_total[10m])),1)` |
| Summary generation p95 | < 60s | `histogram_quantile(.95,sum by(le)(rate(agent_summary_run_duration_seconds_bucket[10m])))` |

所有 tenant-scoped Prometheus 指标默认使用有界标签：非空但未列入 `METRICS_TENANT_ALLOWLIST` 的租户映射到 `__other__`，缺失值映射到 `__unknown__`。tenant allowlist 最多 100 个租户，且必须在每个 HTTP 进程启动时一致注入。`agent_name` 和 `model` 不会因为 tenant 被允许就直接保留；精确标签还必须分别列入 `METRICS_AGENT_ALLOWLIST` 和 `METRICS_MODEL_ALLOWLIST`，格式为 `tenant/name`，每类最多 200 对。非允许值聚合为 `__other__`，完整明细进入日志/成本仓库。这样运行期持续发布新版本或轮换名称也不会让进程内 Prometheus series 无界增长。

## Error budget

99.95% 月目标允许约 21.6 分钟不可用。Burn alert 对每个窗口都要求 eligible traffic 分母大于 0，空闲环境不会被 `clamp_min` 人为计算成 100% 错误率。规则：

- 1h burn rate > 14.4 且 5m > 14.4：Page，冻结发布。
- 6h > 6 且 30m > 6：Page，排查供应商/数据库。
- 3d > 1：Ticket，本周期只能做可靠性工作。
- 消耗 50%：停止非必要灰度；消耗 75%：自动回滚最新 DeploymentSet；100%：变更冻结并事故复盘。

## 已接入的最小告警规则

Inbox/Outbox lag、自动队列 oldest-age、queue inspection failure、租户队列容量拒绝、retry storm、fence rejection、Summary 失败/耗时以及 Gateway 5m/1h fast-burn、30m/6h slow-burn 规则已落在 `deploy/prometheus-rules.yml`，并由 `deploy/prometheus.yml` 的 `rule_files` 加载。pipeline 与 Summary duration bucket 显式覆盖 30/60/120/300 秒，否则 Prometheus 默认最大约 10 秒 bucket 无法量化这里的 Agent SLO。`validate.sh` 用 `promtool` 检查规则语法。压缩包没有内置 PagerDuty/企业微信等 Alertmanager receiver；目标环境必须配置路由后才能声称告警可达。

核心片段如下，完整规则以 `deploy/prometheus-rules.yml` 为准：

```yaml
groups:
- name: agent-platform
  rules:
  - alert: AgentInboxLagHigh
    expr: histogram_quantile(0.99, sum by(le)(rate(agent_pipeline_queue_lag_seconds_bucket{stage="consumer"}[10m]))) > 120
    for: 10m
    labels: {severity: page}
    annotations: {runbook: "docs/SLO.md#inbox-lag"}
  - alert: AgentDeliveryRetryStorm
    expr: sum(rate(agent_pipeline_retries_total{stage="delivery"}[5m])) > 0.2 * sum(rate(agent_pipeline_messages_total{stage="delivery"}[5m]))
    for: 10m
    labels: {severity: page}
  - alert: AgentFenceRejected
    expr: increase(agent_pipeline_fence_rejections_total[10m]) > 0
    labels: {severity: ticket}
```

DLQ 数量需使用 PostgreSQL exporter 的只读查询：

```sql
SELECT 'inbox' queue, count(*) FROM inbox_messages WHERE status IN ('DEAD_LETTERED','WAITING_RECONCILIATION')
UNION ALL
SELECT 'outbox', count(*) FROM outbox_messages WHERE status IN ('DEAD_LETTERED','WAITING_RECONCILIATION');
```

## Runbook

### Inbox lag

1. 看 Worker provider latency、Consumer replicas、DB connection wait 和 claim query latency。
2. 若模型变慢，先扩 Consumer 只会放大模型/DB 压力；按容量公式限制并发并联系 provider。
3. 若 DB checkpoint/I/O 饱和，降低 poll/concurrency，保留 Gateway 写入能力，必要时暂停非关键租户。
4. 恢复后观察 retry amplification 与 DLQ，禁止批量无审计重放。

### Delivery retry storm

1. 按 channel/account 聚合 429/5xx；确认 provider Retry-After。
2. 降低 Delivery 并发，增加退避上限；不要扩容制造更大 429。
3. Provider 恢复后逐租户重放 DLQ，填写 incident ID。

### Queue inspection

1. `AgentQueueInspectionFailing` 表示 Consumer/Delivery 无法读取可靠队列快照；先检查 PostgreSQL 连通性、只读权限、连接池和 `InspectQueue` 查询耗时。
2. 检查失败期间 `agent_pipeline_queue_depth` 与 `agent_pipeline_queue_oldest_age_seconds` 会保留最后一次成功快照，不应把它们解释为当前数据库状态；恢复检查并连续观察至少一个抓取周期后再判断积压是否解除。
3. 该告警只证明观测链路失败，不证明队列为空或已积压；需要结合数据库只读查询和 queue processing/retry 指标核对。Prometheus/Alertmanager 未配置接收器时，告警可见性仍是部署验收项。

### Queue admission

1. `AgentTenantQueueAdmissionRejecting` 表示至少一个租户触发了 operator-owned
   `max_queued`，Gateway 返回 429 并保留 provider 重试机会；这不是持久化故障。
2. 按租户容量基线检查自动 Inbox 行数、Consumer 吞吐和 `MaxInflight`，先扩展
   Worker/Consumer 或提高经过审计的配额，不要直接删除队列行绕过 admission。
3. 重投的相同 provider 消息按幂等键不会重复占用配额；若 429 持续超过容量窗口，
   应冻结该租户发布并检查 provider 回调峰值、DB 锁等待和 session 热点。

### Fence rejection

1. 查询相同 message ID 的 lease_owner/version/updated_at。
2. 检查进程暂停、超时配置、数据库时钟和模型调用是否超过 lease。
3. 不得绕过 fence 手工改 COMPLETED；先确认当前 owner，再决定 replay。

### Audit/result persistence failure

1. Worker 对 result cache 失败返回 503，检查 PostgreSQL。
2. 工具可能已产生外部副作用；重试前按 idempotency key 查询目标系统。
3. 对危险工具没有业务幂等证据时，转人工处置。

### Worker cache saturation

1. 检查 active request、Agent 版本/租户配置 churn 和模型客户端内存，而不是直接无限增大 cache。
2. 若全是长调用，扩 Worker 副本并限制单租户并发；若版本 churn，停止灰度发布并等待旧 key 空闲回收。
3. 只有在 RSS/FD/连接池基准有余量时才提高 `WORKER_CACHE_SIZE`。

### Execution reconciliation

1. 检查 PostgreSQL 可用性、`idx_execution_stale_running` 和 Worker 日志中的稳定错误类。
2. 查询 ABANDONED 对应的 invocation result；ABANDONED 表示执行终态未被可靠记录，不等价于模型一定失败。
3. 禁止把 ABANDONED 批量改成 SUCCEEDED；根据 result/tool 业务幂等证据逐条处置。

### Summary generation

1. 先区分失败突增、失败率和纯延迟：查看 `agent_summary_runs_total{result}` 与 `agent_summary_run_duration_seconds`，再关联同一时间窗的模型、Session 后端、PostgreSQL 和 Redis 延迟。
2. 只读查询 `summary_jobs` 的 PENDING/PROCESSING/FAILED 分布、`lease_until`、`attempts` 与 `last_error` 稳定错误类，并核对 `summary_checkpoints.max_event_sequence` 是否单调推进；不得在未确认 lease owner 时手改状态。
3. 若模型或 Session 后端变慢，降低 `SUMMARY_CONCURRENCY` 以保护数据库和供应商；扩副本前先确认限流、连接池和 token 预算有余量，避免把下游故障放大。
4. 单次超时会以独立的有界持久化上下文记录失败，父进程取消只停止领取新任务并排空已领取任务；因此先等 lease/retry 收敛，不要批量重置或重复生成。
5. 恢复后至少观察两个 10 分钟窗口，并抽样验证 checkpoint 序号、内容哈希和 Runner 可见摘要；Prometheus 告警恢复不等于历史摘要已补齐。

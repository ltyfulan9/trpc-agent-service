# V13 生产风险登记册

> 历史归档：当前权威风险登记册为 `RISK_REGISTER_V14.md`。

评分：影响/概率为 1–5；优先级 = 影响 × 概率。状态中的“代码闭环”不等于目标环境验收。

| ID | 风险 | 影响/概率 | 领先指标 | 缓解与恢复 | 所有者/状态 |
|---|---|---:|---|---|---|
| R-01 | Gateway 回 2xx 后消息尚未落库 | 5/2 | callback 2xx 与 Inbox 增量不一致 | 只在事务 COMMIT 后 2xx；DB 失败让 IM 重试 | Gateway；代码闭环 |
| R-02 | provider 重投或同 ID 不同内容 | 5/3 | duplicate/conflict rate | 复合唯一键 + payload SHA-256；相同内容幂等、冲突拒绝 | Reliable；代码闭环 |
| R-03 | 同 session 并发造成 Event/Tool 因果乱序 | 5/3 | session lock wait、FIFO blocked age | `session_sequence` 前序门禁 + 全 Runner 生命周期 lease | Consumer/Worker；本地真实 DB/FIFO 与容量已验，目标压测待验 |
| R-04 | 崩溃 Worker 复活提交旧结果 | 5/2 | stale fence rejection | owner + 单调 lease_version + DB clock；迟到写拒绝 | Reliable；集成测试已加入 |
| R-05 | IM 已发送但进程在确认前崩溃 | 5/3 | `WAITING_RECONCILIATION` 数量 | Outbox pre-dispatch fence、稳定 delivery ID、人工核对后 replay | Delivery；代码闭环，provider 语义待验 |
| R-06 | 模型/工具执行成功但 Consumer 未收到结果 | 5/3 | execution/result mismatch | invocation binding/result cache；进入 Runner 后未知错误不自动重试 | Worker；代码闭环 |
| R-07 | 危险 Tool 绕过审批或重复消费授权 | 5/2 | approval mismatch/reuse | Runner Plugin 唯一拦截面；challenge 精确绑定；token hash 一次消费 | Governance；代码闭环 |
| R-08 | 租户密钥、prompt 或身份泄漏到日志/trace | 5/2 | secret scanner、异常高字段长度 | AES-GCM AAD、SecretRef、opaque provider error、HMAC 身份、无正文审计 | Security；代码闭环，KMS 待验 |
| R-09 | 并发预算预检穿透或未知调用被错误退款 | 4/3 | uncertain reservation、budget reject | Redis Lua 原子预留；dispatch 后未知按最大 reservation 结算 | FinOps；代码闭环，Redis HA 待验 |
| R-10 | Redis/SQL 迁移漏数据、回滚后分叉 | 5/3 | watermark lag、shadow mismatch、DLQ | fenced state machine、双写、catch-up、hash validate、回滚窗口 | Storage；opaque slice 已实现，projection 待补 |
| R-11 | 旧 Summary 晚到覆盖新上下文 | 4/2 | checkpoint conflict | target sequence + content hash CAS；FencedSink 验证 job lease | Summary；协调器闭环，Generator 待接 |
| R-12 | Redis 故障时降级本地锁导致跨节点并发 | 5/2 | Redis health/session lease error | fail-closed；不允许 production InMemory；使用托管 HA 稳定端点 | SRE；目标环境待验 |
| R-13 | PostgreSQL 故障或连接池耗尽导致回调雪崩 | 5/3 | pool wait、Inbox oldest、callback 5xx | 有界 pool/timeout、入口限流、IM 重试、HA/PITR、queue-lag HPA | DBA/SRE；目标环境待验 |
| R-14 | Runner/Tool 忽略 context 造成 goroutine 泄漏 | 4/3 | goroutine、shutdown drain time | 固定 worker pool、有界 drain、context 合作；不合作工具进程隔离 | Runtime；代码闭环，第三方工具待验 |
| R-15 | Admin/Worker runtime capability 漂移 | 5/2 | capability fingerprint mismatch | immutable snapshot 记录 fingerprint；Worker fail-closed；digest 发布 | Release；代码闭环 |
| R-16 | 静态 Admin bearer token 被长期盗用 | 5/2 | 失败认证、异常 principal 行为 | 私网 Admin、独立 scoped token、轮换；生产换 OIDC/IAP 短期凭据 | IAM；开放项 |
| R-17 | Worker 应用跳明文或伪造 mesh 声明 | 5/2 | release verify failure | production 只收 HTTPS；mesh 必须断言 + evidence annotation + 外部策略验收 | Platform；外部 mTLS 待验 |
| R-18 | Prometheus 标签基数或审计量失控 | 3/3 | series 数、audit ingest lag | tenant 标签 allowlist；高基数进入日志仓；保留策略分级 | Observability；本地容量基线已验，目标保留/成本待验 |
| R-19 | 企业微信/Telegram 规则变化导致投递失败 | 4/3 | provider error/429、sandbox drift | typed adapter、Retry-After、contract sandbox、灰度 adapter 发布 | Channel；sandbox 待验 |
| R-20 | 本地容量被误外推导致目标峰值雪崩 | 5/3 | 目标环境无 p95/p99、queue lag/成本 | 已保留 2,200 条本地基线；目标业务 payload 重跑、1.5–2 倍 headroom、error-budget 发布门 | SRE/FinOps；本地闭环，目标开放 |
| R-21 | fair claim 的陈旧候选复活终态 Inbox | 5/2 | Outbox 已存在但 Inbox 回到 PROCESSING | 最终 UPDATE 重检状态/时间；并发回归；digest 发布；2,200 条 durability 对账 | Reliable；本地闭环 |
| R-22 | service mesh/Kubernetes 版本组合不在正式支持矩阵 | 5/3 | 只能使用 edge/alpha、proxy 注入异常 | 本地 edge 只做证据；生产选择受支持 K8s/Linkerd 组合并重跑 identity/NetworkPolicy/回滚 | Platform；目标环境阻断项 |
| R-23 | sidecar 使 Pod Resource HPA 指标为 unknown | 4/2 | HPA `ScalingActive=False`/missing request | Gateway/Worker 改用 `ContainerResource`；清单契约测试；发布后 ValidMetricFound | SRE；本地闭环 |

## 上线阻断条件

以下任一目标环境证据未完成则不允许把本包标记为 production certified：真实 IM sandbox；供应商支持版本上的 Kubernetes digest rollout/rollback；正式信任根下的严格 mTLS 与受控 egress；生产 KMS/Vault workload identity 和双 key 轮换；PostgreSQL/Redis HA 故障注入；生产 OTLP TLS 后端；业务负载容量、灾备和凭据轮换演练。本地 K3d/Linkerd/Vault/容量证据降低工程风险，但不能替代这些上线门。

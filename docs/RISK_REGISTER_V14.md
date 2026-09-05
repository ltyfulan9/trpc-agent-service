# Enterprise Multi-Tenant Agent Platform Risk Register

评分格式为影响/概率（1–5）。“源码闭环”表示已有控制和回归，不代表目标环境风险已经被接受。

| ID | 风险 | 评分 | 观测信号 | 缓解与恢复 | 状态/Owner |
|---|---|---:|---|---|---|
| R-01 | Gateway 未 durable commit 就向 IM 回 2xx，导致永久丢消息 | 5/2 | callback 2xx 与 Inbox 增量不一致 | 只在事务提交后 ack；DB 失败返回可重试错误 | 源码闭环 / Gateway |
| R-02 | IM 重复或同 ID 不同 payload | 4/4 | duplicate/conflict 计数 | tenant+channel+account+message unique key；payload hash 冲突拒绝 | 源码闭环 / Gateway |
| R-03 | 同 Session 多 Worker 并发导致历史与工具乱序 | 5/3 | fence rejection、sequence lag | durable FIFO + 整次 Runner Redis lease + SQL generation fence | 源码闭环 / Consumer/Worker |
| R-04 | Worker 崩溃后旧实例晚提交覆盖新 owner | 5/2 | stale-owner rejection | 单调 lease_version，事务内 row lock 后再检查 owner/fence/expiry | 源码闭环 / Reliable Store |
| R-05 | 模型/Tool 已产生副作用但网络响应丢失，被自动重放 | 5/3 | WAITING_RECONCILIATION 增长 | httptrace 写入边界；未知结果不自动重跑；Tool 业务幂等键+人工核对 | 源码闭环，业务工具待验收 / Worker |
| R-06 | IM 发送成功但 cursor 提交失败，产生重复消息 | 4/3 | DISPATCH_STARTED 过期 | Provider 前 fence；未知结果进 reconciliation；审计 replay | 源码闭环 / Delivery |
| R-07 | 租户配置或 SQL 查询漏 tenant scope | 5/2 | scope violation、跨租户测试 | scoped reader、复合唯一键、Qdrant 物理 ID、Artifact key 全部绑定 tenant | 源码闭环 / Platform |
| R-08 | 秘密进入租户 JSON、日志、trace 或 ConfigMap | 5/2 | secret scanner、异常字段 | profile 只引用 env 名；Worker-only secret；unknown field/raw secret fail-closed；脱敏 | 源码闭环；正式 KMS 待外部 / Security |
| R-09 | Redis 不可用时退回本地锁造成多副本分裂 | 5/2 | Redis error、lease acquire failure | 生产路径 fail-closed；不允许 InMemory Session/Memory | 源码闭环 / Worker |
| R-10 | PostgreSQL 连接池模式破坏 advisory lock | 5/2 | lock owner 异常、session overlap | 直连或 PgBouncer session pooling；部署前拒绝 transaction/statement pooling | 配置门禁+目标验收 / DBA |
| R-11 | 旧 Summary 晚到覆盖新上下文 | 4/2 | checkpoint CAS conflict | target sequence、cutoff_at、last_event_id、hash、job lease 的 fenced CAS | 源码闭环 / Summary |
| R-12 | Summary 请求取消后 job 永久 PROCESSING 或 goroutine 泄漏 | 4/2 | lease expiry、goroutine/latency | 停止新 claim但排空活跃 job；独立有界失败持久化；job timeout | 源码闭环 / Summary worker |
| R-13 | Summary 已生成但下一轮 Runner 未消费 | 4/2 | prompt 中无 summary、历史无限增长 | migration 042 精确边界；Session clone overlay；`WithAddSessionSummary(true)` + `BranchFilterModeAll`；捕获真实 Runner request | 源码闭环 / Worker |
| R-14 | 向量库数据串租户或保留 metadata 被覆盖 | 5/2 | scope violation、异常 hit | tenant+agent+logical ID 的 SHA-256 物理 ID；保留字段拒绝用户覆盖 | 源码闭环 / Knowledge |
| R-15 | Artifact 对象与 SQL 元数据不一致/损坏 | 4/3 | object 404、hash mismatch | 不可变版本、advisory xact lock、SHA-256 读校验、失败补偿、tombstone | 源码闭环 / Artifact |
| R-16 | 迁移 lease 过期后旧 projector 写目标并标记成功 | 5/2 | fence failure、marker drift | 副作用前后检查 fence；目标成功后才写 projected_at；幂等版本冲突 | 源码闭环 / Migration |
| R-17 | Redis→SQL、Qdrant/S3 大规模迁移追不上增量 | 4/3 | watermark lag、dual-write backlog | 分片/限速、checkpoint、shadow read、延后 cutover、回滚窗 | 小规模闭环；容量待外部 / Migration |
| R-18 | token 预算并发穿透或未知 usage 被少计 | 4/3 | pending/uncertain、provider 差异 | Redis Lua reservation；dispatch 一次性；usage 不明按完整预留；落账失败拒绝成功 | 源码闭环；真实 Provider 待验收 / Governance |
| R-19 | 恶意 Tool 参数或危险操作未经批准 | 5/2 | denied/challenge/audit | BeforeTool 唯一拦截；白名单；参数 canonical hash；持久审批一次性消费 | 源码闭环 / Governance |
| R-20 | 附件 URL SSRF/DNS rebinding | 5/3 | blocked URL、异常 egress | 当前只透传不下载；未来 downloader 必须 DNS pin、逐跳 redirect、大小/时限/allowlist | 当前禁用下载 / Security |
| R-21 | 指标 tenant/model/agent 标签爆炸 | 3/3 | series count、Prometheus 内存 | 默认 `__other__`；严格小型 allowlist；完整维度入日志/成本仓库 | 源码闭环 / Telemetry |
| R-22 | Trace/日志泄露用户身份或密钥 | 5/2 | DLP/secret scan | 租户 HMAC 假名、稳定错误类、禁止 payload/secret attribute、OTLP TLS | 源码闭环；OTLP TLS 待外部 / Telemetry |
| R-23 | 可变镜像或供应链被替换 | 5/2 | digest mismatch、SBOM 缺失 | releaseverify 强制 digest；non-root/read-only/seccomp；生产签名/SBOM admission | digest 门闭环；签名平台待外部 / Release |
| R-24 | breaking migration 与旧 Worker 协议并存 | 5/2 | schema/protocol mismatch | drain intake→停旧 Worker→migration→启新 Worker；校验 migration checksum | Runbook+门禁 / Release |
| R-25 | 企业微信/Telegram 配额、回调格式或网络规则与模拟不一致 | 4/4 | sandbox 4xx/429/timeout | 先 preflight，再各做 1 条文本/重复/限流/回包证据；保留原始 request_id，不记 secret | 外部必验 / Channel |
| R-26 | Docker Desktop AF_UNIX reparse point 再损坏 | 3/3 | dockerInference/sailor socket 1920 错误 | 停服务、移动运行时目录、保留 volumes/images、验证 API；不做 factory reset | 本机已恢复 / Developer env |
| R-27 | HA 切换、PITR 或备份无法恢复 | 5/2 | RPO/RTO 超标 | 定期 restore drill、WAL/Redis 策略、对象版本、运行手册与责任人 | 外部必验 / SRE |
| R-28 | 成本/吞吐估算低于真实长上下文峰值 | 4/3 | queue age、token/min、P95/P99 | 用真实 payload 阶梯压测；按限流/连接池/HPA 指标扩容；租户 quota | 外部必验 / SRE |
| R-29 | 过长 PostgreSQL Session schema/prefix 使索引名超过 63 字节并被截断 | 4/2 | Session Service 启动 schema verification 失败 | operator profile 限制短固定 schema/prefix；保留上游 fail-closed 校验；发布前真实初始化 | 本机已复现并规避 / Storage |
| R-30 | 租户借 MCP URL/Header 越权出网或一个 MCP 故障拖垮全体租户 | 5/3 | 非预期 egress、profile init error、Tool P95 | URL/Header/profile 仅 operator-owned；禁 stdio/危险 Header；精确 Tool allowlist；按使用 profile 延迟初始化；目标 NetworkPolicy/DNS/SLA 验收 | 源码闭环；目标 MCP 待外部 / Platform |

## 上线阻断条件

出现以下任一项不得宣称生产就绪：真实 IM callback 未通过；release bundle 含 tag-only 镜像；Worker Secret 泄露给非 Worker；数据库使用 transaction pooling；NetworkPolicy 需要临时全放通；Summary checkpoint 倒退；未知模型/Tool 结果被自动重放；KMS/OTLP TLS/告警接收端无目标环境证据；备份未做恢复演练。

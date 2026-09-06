# 生产风险与控制措施

评分格式为影响/概率（1–5），用于风险优先级排序。下表记录失效模式、观测信号、控制措施和责任模块；业务服务与基础设施的部署检查按 [EXTERNAL_ACCEPTANCE_RUNBOOK.md](EXTERNAL_ACCEPTANCE_RUNBOOK.md) 执行。

| ID | 风险 | 评分 | 观测信号 | 缓解与恢复 | 责任模块及部署检查 |
|---|---|---:|---|---|---|
| R-01 | Gateway 在持久化提交前向 IM 返回 2xx，导致消息丢失 | 5/2 | callback 2xx 与 Inbox 增量不一致 | 事务提交后确认回调；数据库失败返回可重试错误 | Gateway |
| R-02 | IM 重复投递或同 ID 对应不同 payload | 4/4 | duplicate/conflict 计数 | tenant+channel+account+message 唯一键；payload hash 冲突拒绝 | Gateway |
| R-03 | 同 Session 多 Worker 并发导致事件与工具调用乱序 | 5/3 | fence rejection、sequence lag | 持久化 FIFO、覆盖完整 Runner 生命周期的 Redis lease、SQL generation fence | Consumer / Worker |
| R-04 | Worker 崩溃后失效实例延迟提交覆盖新 owner | 5/2 | stale-owner rejection | 单调 lease_version，事务内取得行锁后检查 owner/fence/expiry | Reliable Store |
| R-05 | 模型/Tool 已产生副作用但网络响应丢失，被自动重放 | 5/3 | WAITING_RECONCILIATION 增长 | GotConn 后传输失败按可能已发送处理，不依赖 WroteRequest 是否及时回调；未知结果暂停重试；Tool 业务幂等键与结果核对 | Worker；业务工具幂等验收 |
| R-06 | IM 发送成功但 cursor 提交失败，产生重复消息 | 4/3 | DISPATCH_STARTED 过期 | 调用 Provider 前持久化 fence；未知结果进入 reconciliation；带审计的 replay | Delivery |
| R-07 | 租户配置或 SQL 查询缺失 tenant scope | 5/2 | scope violation、跨租户测试 | scoped reader、复合唯一键、Qdrant 物理 ID、Artifact key 绑定 tenant | Platform |
| R-08 | 明文凭据进入公开配置、日志、trace 或 ConfigMap | 5/2 | secret scanner、异常字段 | profile 引用环境变量名；按进程职责分配凭据；拒绝未知字段和原始秘密；输出脱敏 | Security；KMS 身份与轮换验收 |
| R-09 | Redis 不可用时使用本地锁造成多副本状态分歧 | 5/2 | Redis error、lease acquire failure | 生产路径 fail-closed；使用共享 Session/Memory 后端 | Worker |
| R-10 | PostgreSQL 连接池模式破坏 advisory lock | 5/2 | lock owner 异常、session overlap | 直连或 PgBouncer session pooling；部署前检查并排除 transaction/statement pooling | DBA；连接池模式验收 |
| R-11 | 过期 Summary 延迟提交覆盖新上下文，或会话重建读取旧摘要 | 4/2 | checkpoint CAS conflict、incarnation mismatch | Session 代次 UUID、target sequence、cutoff_at、last_event_id、hash、job lease 的 fenced CAS；新回执登记后续目标解析 | Summary |
| R-12 | Summary 请求取消后 job 停留在 PROCESSING 或 goroutine 泄漏 | 4/2 | lease expiry、goroutine/latency | 停止领取并排空活跃任务；独立且有界的失败持久化；任务超时 | Summary Worker |
| R-13 | Summary 已生成但下一轮 Runner 未消费 | 4/2 | prompt 中缺失 summary、上下文持续增长 | checkpoint 精确边界；Session clone overlay；`WithAddSessionSummary(true)` + `BranchFilterModeAll`；捕获 Runner 请求验证 | Worker |
| R-14 | 向量库跨租户访问或保留 metadata 被覆盖 | 5/2 | scope violation、异常 hit | tenant+agent app+logical ID 的 SHA-256 物理 ID；保留字段受平台管理 | Knowledge |
| R-15 | Artifact 对象与 SQL 元数据不一致或内容损坏 | 4/3 | object 404、hash mismatch | 独立写入标识、advisory xact lock、SHA-256、提交结果核对、tombstone 清理重试 | Artifact |
| R-16 | 迁移 lease 过期后失效 projector 写目标并标记成功 | 5/2 | fence failure、marker drift、pending journal | tenant/domain gate、副作用前后 fence 检查；目标 payload/hash 读回匹配后写 projected_at；路由/配置/阶段原子 CAS | Migration |
| R-17 | 在线迁移遗漏增量、目标失败或扫描长期阻塞租户 | 4/3 | unresolved intent、journal 水位、目标错误、gate 等待及请求延迟 | 创建时捕获，源写前提交 intent；先恢复未完成记录，再按版本同步镜像；全量记录比对后切换；回滚窗口保持源写、目标读 | Migration；目标负载与故障恢复验收 |
| R-18 | 并发预留超过 token 预算或未知 usage 被低估 | 4/3 | pending/uncertain、provider 差异 | Redis Lua reservation；dispatch 一次性授权；usage 不明按完整预留结算；结算失败返回错误 | Governance；Provider usage 验收 |
| R-19 | 恶意 Tool 参数或危险操作未经批准 | 5/2 | denied/challenge/audit | BeforeTool 统一授权；白名单；参数 canonical hash；持久审批一次性消费 | Governance |
| R-20 | 附件 URL SSRF/DNS rebinding | 5/3 | blocked URL、异常 egress | Worker 传递经验证的引用；实际出网侧配置域名/IP allowlist、DNS 与重定向校验 | Security；Provider 出网策略验收 |
| R-21 | 指标 tenant/model/agent 标签基数失控 | 3/3 | series count、Prometheus 内存 | 默认 `__other__`；有界 allowlist；详细维度进入受控日志与分析系统 | Telemetry |
| R-22 | Trace/日志泄露用户身份或密钥 | 5/2 | DLP/secret scan | 租户 HMAC 假名、稳定错误类、限制 payload/secret attribute、OTLP TLS | Telemetry；传输身份与 TLS 验收 |
| R-23 | 可变镜像或供应链被替换 | 5/2 | digest mismatch、SBOM 缺失 | releaseverify 强制 digest；non-root/read-only/seccomp；生产签名/SBOM admission | Release；签名与供应链准入验收 |
| R-24 | 不兼容 schema 迁移期间并存不同运行协议 | 5/2 | schema/protocol mismatch | 停止接入并排空→停止 Gateway/Consumer/Worker/Summary Worker/Delivery/Admin 全部写入副本→迁移→按依赖顺序恢复；校验 migration checksum | Release；协议兼容性准入 |
| R-25 | 企业微信/Telegram 配额、回调格式或网络规则与测试环境不同 | 4/4 | Provider 4xx/429/timeout | 接入预检后执行文本、重复投递、限流与回复检查；保留 request_id，排除 secret | Channel；目标账号协议验收 |
| R-26 | HA 切换、PITR 或备份无法恢复 | 5/2 | RPO/RTO 超标 | 定期恢复演练、WAL/Redis 持久化策略、对象版本、恢复手册与责任人 | SRE；恢复演练 |
| R-27 | 成本与吞吐估算低于实际长上下文峰值 | 4/3 | queue age、token/min、P95/P99 | 业务 payload 阶梯压测；按限流、连接池与 HPA 指标扩容；租户配额 | SRE；目标负载容量验收 |
| R-28 | 过长 PostgreSQL Session schema/prefix 使索引名超过 63 字节并被截断 | 4/2 | Session Service 启动 schema verification 失败 | operator profile 使用固定短 schema/prefix；上游 schema 校验；发布前实际初始化 | Storage；命名与初始化检查 |
| R-29 | 租户通过 MCP URL/Header 越权出网或 MCP 故障影响其他租户 | 5/3 | 非预期 egress、profile init error、Tool P95 | 运维管理 URL/Header/profile；限制传输与敏感 Header；精确 Tool allowlist；按使用 profile 延迟初始化 | Platform；MCP 网络策略、认证及 SLA 验收 |

## 发布准入

发布准入检查不可变镜像、进程级 Secret 范围、支持的数据库连接池模式、最小网络策略、单调 Summary checkpoint 和未知副作用恢复策略。真实 IM、KMS、OTLP TLS、告警接收端与备份恢复按部署验收运行手册保存证据；任一关键控制失败时暂停发布。

在线迁移入口为 `cmd/data-migrate`，Worker/Summary Worker 装饰器负责捕获和动态路由。迁移前统一写入副本版本与不可变 profile，所有写入经过平台入口；持久化存储身份与兼容性用于检测跨节点 profile 漂移。`cmd/migrate` 负责 schema 迁移。

活跃迁移串行化该租户数据域的操作，全量 inventory/规范记录比对和切换扫描可能暂时阻塞请求；同步镜像将目标延迟与故障带入调用路径，错误返回时源端可能已提交。READ_SHADOW 校验规范记录，不覆盖真实查询流量和检索排名抽样，检索质量需单独验收。Session shared state/native summary/TTL 及达到安全上限的数据不满足迁移准入，Memory 在线迁移尚未实现。回滚窗口保留源写、目标读，完成后停止镜像并保留源数据；准入条件、支持的数据域、恢复命令与保留策略见 [ONLINE_MIGRATION.md](ONLINE_MIGRATION.md)，执行记录见 [ACCEPTANCE_EVIDENCE.md](ACCEPTANCE_EVIDENCE.md)。

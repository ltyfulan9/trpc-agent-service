# tRPC-Agent Enterprise V14 最终交接

更新日期：2026-09-05（Asia/Shanghai）  
作者：王子龙

## 1. 权威入口

当前唯一权威源码目录是本目录。评审顺序：

1. `README.md`
2. `docs/COMPETITION_SUBMISSION_V14.md`
3. `docs/ACCEPTANCE_EVIDENCE_V14.md`
4. `docs/DATA_MODEL.md`
5. `docs/RISK_REGISTER_V14.md`
6. `docs/SECURITY_REVIEW_V14.md`

`TRPC_AGENT_ENTERPRISE_HANDOFF_V14_CURRENT.md` 与所有 V13 文件只保存历史检查点，不得用于判断当前实现状态。

## 2. 当前实现结论

生产主链路已经形成一条可运行闭环：企业微信/Telegram callback 经 Adapter 验签和规范化，Gateway 在 PostgreSQL Inbox durable commit 后应答，Consumer 以租户公平调度、Session FIFO、lease/fence 领取，Worker 按 immutable AgentVersion 构造 tRPC-Agent-Go Runner 并注入共享 Session/Memory、治理 Plugin、Knowledge 与 Artifact，执行结果经同事务 Inbox completion + Outbox 创建，Delivery 在 Provider 调用前写 dispatch fence 并分段投递。

PostgreSQL 是可靠队列、控制面、执行 guard/fence、迁移协调和审计的权威；Redis/PostgreSQL Session/Memory 是租户运行数据后端。后端可选不等于 PostgreSQL 可被移除。Worker 无本地权威 Session，因此无需 sticky session。

V14 相比历史检查点补齐：

- Summary：独立 summary-worker、固定版本生成、预算结算、精确 cutoff/last_event_id、fenced CAS、取消后失败落盘和 shutdown 排空；Worker 使用全会话 branch mode。真实 Redis/PostgreSQL 集成捕获下一轮 Runner 模型请求，证明摘要进入请求且被覆盖历史被裁剪。
- Knowledge：operator-owned Qdrant/embedding profile、Worker-only SecretRef、tenant/app 物理隔离、真实 seed/search 与框架 Knowledge 注入。
- Artifact：PostgreSQL 不可变版本元数据 + MinIO/S3 正文、SHA-256 校验、幂等版本、tombstone 和真实 save/load/list/delete。
- 迁移 projection：官方 Redis Session 的规范化 State/Event/Track 经版本化 journal、catch-up 与 fence 写入官方 PostgreSQL Session，并完成租户配置 CAS cutover；Qdrant/MinIO projection 同样包含重放、冲突、删除、目标失败与最终 fence 校验。
- MCP：operator-owned Streamable HTTP/SSE profile、Admin 无秘密准入、Worker 延迟连接官方 tRPC-Agent-Go MCP ToolSet、精确工具 allowlist、Header SecretRef 与进程关闭；本地真实 MCP server 纵切覆盖 Worker→Runner→治理→MCP→最终回复。
- 部署：Summary 进程、data-plane profiles、最小 Secret 暴露、NetworkPolicy、Prometheus Summary 规则和 releaseverify 契约。

## 3. 已取得的本机证据

当前生产工具链固定 `GOTOOLCHAIN=go1.26.7+auto`，框架固定 tRPC-Agent-Go v1.11.2。2026-09-05 已在 C 盘权威树重跑模块校验、gofmt、全部命令构建、vet、全量 unit/race（`GOMAXPROCS=1`、`-p 1`），以及真实 PostgreSQL/Redis/Qdrant/MinIO integration（9.775 秒）。Compose 配置和 7 个应用镜像构建、隔离栈启动、公开 `/health` 探针、restart count、Prometheus 6/6 targets 和 15 条规则解析均通过；Admin 纵切和零 Provider-call 外部 preflight 也通过。精确状态与不得扩大解释的边界见 `docs/ACCEPTANCE_EVIDENCE_V14.md`。

Docker Desktop 的损坏 AF_UNIX runtime 目录已通过可恢复移动修复，没有 factory reset，也没有删除 V13/V14 image、volume 或业务数据。历史备份目录仍保留，未经用户授权不要清理。E 盘的 `trpc-agent-v13-lab` 是 V13 历史实验室（含缓存、数据库 dump、证书/私钥和工具），不是 V14 运行依赖；本次 V14 复核从 C 盘完成，没有硬编码 `E:\` 路径。

直接从新归档目录运行 Compose 时没有 `.env` 会按设计拒绝启动；使用 `scripts/run_c_local_stack.ps1 -Build` 可从任意目录定位源码、注入进程内一次性验证值并使用隔离端口。该脚本不会读取或写入 E 盘，也不会覆盖已有项目。交付总包、清单、SHA-256 和解包复核结果位于本线程 `outputs` 目录，最终文件名以包内 `PACKAGE_INVENTORY_20260905.md` 为准。

## 4. 当前外部验收边界

以下项目不能由源码测试或本地模拟替代：

- 真实企业微信/Telegram sandbox；当前企业微信实现范围是“自建应用加密文本 1:1 callback + 主动回复”，不包含群机器人、微信客服、公众号、媒体下载或撤回。
- 真实业务 MCP server 的身份、出网 allowlist、配额、幂等与 SLA；本地 MCP server 只证明框架协议纵切。
- 正式 Kubernetes/service-mesh rollout/rollback、严格 mTLS 与证书轮换。
- KMS/Vault workload identity、云 S3/COS/Qdrant IAM 与私网策略。
- OTLP TLS、Alertmanager 实际接收端。
- PostgreSQL/Redis HA 故障注入、PITR/DR 恢复、目标 payload 容量和成本演练。

外部低次数执行顺序见 `docs/EXTERNAL_ACCEPTANCE_RUNBOOK.md`。任何外部凭据只应通过本机 Secret/env 或正式 Secret Manager 注入，不能写入源码、文档、聊天记录或压缩包。

本机已准备 `scripts/wecom_sandbox_tunnel.ps1` → `wecom_sandbox_setup.ps1` → `wecom_sandbox_bootstrap.ps1` 的闭环脚本，并为 Bash 环境提供经语法/ShellCheck 门禁的同类向导。脚本使用独立端口和 Compose project，不覆盖 V13；当前临时 Tunnel 可达只证明公网 TLS/转发服务已建立，在企业管理员登录、SecretRef 注入、控制台 URL verify 和真实成员消息完成前，企微状态仍是 `EXTERNAL_REQUIRED`。

## 5. 接手后的执行顺序

1. 先运行本地门禁，确认当前源码与归档哈希一致；Windows 可执行 `scripts/run_c_local_stack.ps1 -Build` 后再按 `docs/ACCEPTANCE_EVIDENCE_V14.md` 检查 `/health`、迁移和指标。
2. 由企业微信管理员创建/授权自建应用，提供 CorpID、AgentID、Secret、callback Token 与 EncodingAESKey 的本机注入；配置公网 HTTPS callback 后只做 URL verify、单条文本、重复回调与主动回复四项证据。
3. 在目标集群按 migration → control plane → workers 的顺序发布，验证 canary/rollback。
4. 再做 mTLS/KMS/OTLP/HA/容量/DR，逐项保存时间戳、trace_id、退出码和脱敏结果。

未取得上述目标环境证据前，可以声明“源码闭环且本机集成通过”，不能声明“已在正式生产环境上线”。

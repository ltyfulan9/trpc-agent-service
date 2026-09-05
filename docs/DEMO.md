# 面试演示脚本

## 先讲结论

“这个平台没有把 IM webhook 当普通 Chat HTTP。入口先做验签、租户与 Channel Account 解析，再把规范化消息写入 PostgreSQL Inbox，提交后才回 200。Consumer、Worker、Delivery 完全解耦，所以节点可以水平扩展；共享 Session/Memory 让 Worker 不依赖 sticky session。”

## 五分钟白板顺序

1. 画 `IM → Gateway → Inbox → Consumer → Worker → Outbox → Delivery → IM`。
2. 强调 Inbox 完成与 Outbox 创建同事务，否则会出现 Agent 成功但回复永久丢失。
3. 解释 `SKIP LOCKED` 解决并发领取，`lease_version` 解决旧 Worker 复活写；UUID owner 不是 fencing token，单调 version 才是。
4. 解释 at-least-once 边界：发送前先持久化 `DISPATCH_STARTED`；发送成功后崩溃会停在 `WAITING_RECONCILIATION`，必须核对 Provider 后审计 replay。exactly-once 仍必须有 provider/业务系统幂等支持。
5. 解释多租户隔离：配置、数据 namespace、Channel Account、Agent Version、工具策略、密钥、审计与预算，不只是 tenant_id。
6. 解释治理为什么放 Runner Plugin：只包静态 CallableTool 会漏掉 MCP、动态 ToolSet 和子 Agent，Agent wrapper 还可能被 setupInvocation 替换。
7. 解释灰度：不可变 snapshot、API Key 不进 snapshot、同 session 万分桶、ExecutionRecord 固定版本、DeploymentSet 回滚。
8. 最后主动说明验证边界：2026-09-05 已从 C 盘复核 Go build/vet/unit/race、7 个镜像、Compose、真实 PostgreSQL/Redis/Qdrant/MinIO integration 和 Admin 纵切；真实企业微信/Telegram、目标集群、正式 KMS/Vault、HA/DR 与容量仍按外部验收 Runbook 执行。源码包不带 `.env`，Windows 可用 `scripts/run_c_local_stack.ps1 -Build` 启动一次性隔离验证栈。

## 常见追问

**为什么不用 sticky session？** 共享 SessionService 承担状态，Worker 可以任意路由；sticky 只作为缓存优化，不能成为正确性前提。

**Redis 和 PostgreSQL怎么选？** 控制面与消息状态机需要事务、唯一约束和行锁，选 PostgreSQL；Redis 用于短租约、nonce、预算等低延迟协调；向量库/对象存储承担专用数据，不强行用同一一致性模型。

**数据库挂了 Gateway 为什么不先回 200？** 回 200 等于平台接管责任；未落 durable store 就确认会永久丢消息，因此返回 503 让 IM 重试。

**如何避免重复模型调用？** Inbox 单 owner + fence 限制并发，Worker success response 写 result cache。外部 Tool 仍必须自己幂等，因为模型结果缓存无法回滚付款等副作用。

**摘要怎么防乱序？** Summary job 只能在 Event/State commit 后创建，重新读主存储并携带 max_event_sequence，CAS 只允许新版本覆盖旧版本。

**这个实现还缺什么？** Summary Generator、下一轮 Runner history overlay、Qdrant Knowledge、PostgreSQL+MinIO Artifact 和向量/对象 projection 已在真实本地后端闭环。仍缺的是目标环境证据：正式 KMS/Vault、mTLS/OTLP TLS、真实企业微信/Telegram sandbox、HA 故障注入、容量与灾备演练。能明确区分“源码闭环”和“外部已验收”，比宣称所有生产条件已完成更可信。

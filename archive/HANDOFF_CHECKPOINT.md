# tRPC-Agent Enterprise V14 当前检查点与续作交接

> **历史检查点，已被 `TRPC_AGENT_ENTERPRISE_HANDOFF_V14_FINAL.md` 取代。** 下文“未闭环项”记录 2026-08-31 当时状态；Summary→Runner、独立 summary-worker、Knowledge/Artifact runtimeplane 及部署接线随后已经完成，不应再作为当前结论引用。

> 检查点日期：2026-08-31（Asia/Shanghai）  
> 当前源码根目录：`work/trpc-agent-enterprise-v14`  
> 性质：可编译、关键单测通过的中间检查点；不是“所有生产外部验收均完成”的最终版。

## 1. 版本与兼容性

- 项目模块：`trpc.group/trpc-go/trpc-agent-go/enterprise`
- `go.mod`：`go 1.25.14`
- tRPC-Agent-Go：`v1.11.2`
- Session/Memory Redis、PostgreSQL 子模块：`v1.11.0`
- 该组合不与 tRPC-Agent-Go 的 `Go 1.21 or later` 冲突：上游声明的是最低 Go 版本，企业应用可因自身依赖声明更高下限。
- 既有审计曾在 Go 1.27.0 上通过默认完整单测；本检查点仍须在最终归档前重新执行 race、vet、真实后端及容器门禁，不能沿用旧结果代替新代码证据。

## 2. 本轮已经落地的能力

### 2.1 Knowledge 真实数据面与向量迁移投影

- `pkg/knowledgeplane` 提供租户/Agent 作用域隔离、Qdrant collection 管理、写入、检索与 tombstone 语义。
- `pkg/dataprojection` 把 migration 037 的 fenced PostgreSQL 记录投影到 Qdrant；使用稳定、可逆的记录键和 projection ledger，区分“已复制”和“目标已应用”。
- 真实 Docker 纵切曾通过：PostgreSQL fence → Qdrant 写入 → 检索/重放。

### 2.2 Artifact 真实数据面与对象迁移投影

- `pkg/artifactplane` 支持 MinIO/S3 对象写入、不可变版本元数据、SHA-256 完整性校验、精确版本投影和 tombstone。
- 修复了 advisory scope 中非法 NUL 文本问题；同版本同内容幂等，同版本异内容冲突。
- `pkg/dataprojection` 可把 fenced 记录投影为 MinIO 对象及数据库版本。
- 真实 Docker 纵切曾通过：PostgreSQL fence → MinIO 精确版本 → load/tombstone。

### 2.3 通用迁移 projection 协调

- 新增 projection marker，只有目标真正应用后才写 `projected_at`。
- 保留 migration lease/fence，在投影副作用前后都验证所有权，避免过期 Worker 覆盖新执行者。
- migration record key 上限提升至 4096，以容纳稳定可逆的 Knowledge/Artifact 作用域键。

### 2.4 生产 Summary 协调、生成与预算

- Summary Job 现在固定 `agent_version_id`，重试使用不可变的模型、提示词和生成策略。
- `target_event_sequence=0` 表示延迟解析；解析后在 job lease 下冻结为准确正数。
- Processor 支持 heartbeat、防止旧 heartbeat 回写倒退、`ErrSummaryNotDue` 成功完成但不发布空 checkpoint。
- Worker 成功执行后，在仍持有分布式 Session lease 时读取准确事件数并返回 typed Summary scheduling receipt；后端读失败时返回 deferred target，而不是重放已经可能产生副作用的模型/工具执行。
- Inbox 完成、Outbox 回复和 Summary Job 在同一个 PostgreSQL fenced transaction 内提交；租户、Session owner、Agent app、版本状态不匹配会整体回滚。
- `pkg/summaryruntime` 按精确 Agent 版本加载配置、运行时解析 SecretRef、使用租户后端读取不可变事件前缀，并通过 tRPC-Agent-Go Summarizer 生成摘要。
- Summary 模型调用使用租户 token reservation/dispatch/settlement；provider usage 不可靠时按完整预留收费，结算错误不泄露 Redis/凭据细节。
- Summary runtime 的 deferred target 使用与 Agent Worker 相同的 Redis Session lease key。

### 2.5 控制面与迁移

- 控制面新增按 tenant/app/version 精确加载已发布或已退役不可变版本的能力。
- migration 038：Summary 加入精确 `session_owner_id`。
- migration 039：Artifact 不可变版本。
- migration 040：数据 projection marker。
- migration 041：Summary Job 固定 Agent 版本并支持 deferred target。
- `PACKAGE_MANIFEST.md` 已更新到 `migrations/001..041`。

## 3. 当前验证证据

本检查点刚执行并通过：

```powershell
go test -buildvcs=false -count=1 ./migrations ./pkg/summary ./pkg/summaryruntime ./pkg/dataprojection ./pkg/artifactplane ./pkg/knowledgeplane
```

通过包：`migrations`、`pkg/summary`、`pkg/summaryruntime`、`pkg/dataprojection`、`pkg/artifactplane`、`pkg/knowledgeplane`。

此前本轮还通过：

```powershell
go test -tags=integration ./test/integration -run TestSummaryReceiptCommitsAtomically -count=1 -v
```

该真实 PostgreSQL 用例验证：成功路径同时得到 COMPLETED Inbox、一个 Outbox、一个固定版本的 Summary Job；作用域冲突时三者全部回滚。

本轮 Docker 数据面使用隔离的 V14 容器，未删除或复用 V13 证据容器：

- `trpc-v14-postgres`：PostgreSQL 15.8
- `trpc-v14-qdrant`：Qdrant 1.16.3
- `trpc-v14-minio`：用于本地 S3 兼容验证的 MinIO

## 4. 唯一最高优先级未闭环项

生产 Summary 目前已经“可靠生成并写入平台 `summary_checkpoints`”，但 tRPC-Agent-Go Runner 构建下一轮模型历史时读取的是 tenant Session 对象中的 `Session.Summaries`。上游 Redis/PostgreSQL Session 模块把原生摘要存入各自 `session_summaries`，平台 checkpoint 不会自动进入该对象。

因此当前不能宣称 Summary 已对下一轮 Runner 生效。下一步必须完成以下契约：

1. Summary candidate 持久化最后覆盖事件的 `cutoff_at` 和 `last_event_id`，不能用摘要生成时间冒充事件边界。
2. Worker 获取 Session 后，把精确 tenant/app/owner/session/filter checkpoint 注入克隆后的 `Session.Summaries`。
3. checkpoint 读取失败时 fail closed，避免静默使用无界历史；跨租户/跨 Agent app 必须在访问后端前拒绝。
4. LLMAgent 必须显式启用 `WithAddSessionSummary(true)`，且只有完整 Summary data plane 已配置时才调度摘要。
5. 用真实 PostgreSQL + Redis Session + 本地假 OpenAI-compatible 模型验证：enqueue → target freeze → generate → fenced checkpoint → 下一轮 Runner 请求确实包含摘要并正确裁剪已覆盖事件。

该缺口已被识别但没有把未实现的红灯测试留在检查点包中；当前包保持可编译状态。

## 5. 后续执行顺序

1. 用 TDD 完成 Summary 边界字段、migration 042 和 Runner Session overlay。
2. 新增独立 `cmd/summary-worker`，包含有限并发、轮询退避、健康检查、指标、信号取消和 goroutine 回收。
3. 把 summary-worker 加入 Docker Compose/Kubernetes，并补 readiness/liveness、资源限制和 graceful termination。
4. 运行真实 Redis/PostgreSQL + 本地假模型的端到端摘要测试，不消耗外部 LLM 次数。
5. 补齐 projection 的 stale fence、同版本冲突、目标失败不写 marker、final-fence 失败等回归。
6. 最终执行 `go mod verify`、全量 `go test ./...`、`go test -race`、`go vet`、真实集成、Compose/Kubernetes 验收，再更新 V14 证据矩阵和最终归档。

## 6. 边界声明

- 没有向竞赛 GitHub 仓库推送本检查点源码。
- 本包不包含真实 `.env`、API key、IM token、数据库生产密码、Git 历史、二进制和嵌套压缩包。
- Qdrant/MinIO/PostgreSQL 本地纵切证明适配器真实读写，不等于生产 Kubernetes、真实 IM sandbox、KMS/Vault、service-mesh mTLS、OTLP TLS、灾备和容量演练全部完成。
- 当前最稳妥的提交判断仍是：核心平台能力强，V14 数据面和 Summary 协调已显著完善；Summary 对 Runner 的消费桥接与独立运行进程完成后，才可把“生产 Summary Generator”标为闭环。

# trpc-agent-service 参考仓库二次核验

核验日期：2026-08-24（Asia/Shanghai）  
参考仓库：<https://github.com/liuzengh/trpc-agent-service>  
核验提交：`aa000c8407dcd6ea7788fcdccde9574b44bbe2d2`（远端 `main` 当前 HEAD，提交时间 2026-08-20）

本次核验在临时 clone 中进行，只读检查参考仓库源码、README 和脚本；未启动 Docker/Podman，未连接 PostgreSQL、Redis、IM 或任何生产凭据。本文只把源码能证明的事实写成“已核验”，不把 README 的设计要求或包注释写成已实现能力。

## 1. 参考仓库的明确范围

README 明确说本题“以架构设计为主”，可以包含少量伪代码/接口/数据模型示例，“不要求实现完整系统”（`README.md:13-18`）。它还明确说明 `cmd`/`trpcservice` 目录只是示范目录，实际组织方式不受其约束（`README.md:108-110`）。因此，参考仓库本身不是一个应当已经具备生产运行时的验收样例。

README 列出的需求包括：

- 多租户配置与隔离、节点拓扑、无 sticky session 的共享状态策略（`README.md:21-27`）。
- 租户级 Session/Memory/Knowledge/Artifact/后端选择、并发顺序、跨节点可见性、迁移和 IM 幂等（`README.md:29-40`）。
- 至少两类 IM 通道（包含微信或企业微信）、验签、去重、身份映射、群聊/单聊 session 规则以及平台限制（`README.md:42-48`）。
- Plugin/Guardrail 治理、指标、跨组件 tracing、审计字段和密钥脱敏（`README.md:50-56`）。
- 故障恢复、`context.Context`/goroutine/Runner event channel 生命周期、灰度回滚、容量评估和 Compose/Kubernetes 部署（`README.md:58-63`）。
- 交付物包括架构文档、架构图、时序图、数据模型、同步/幂等方案、多后端说明、至少 8 个风险及一份基于设计的 GitHub 实现代码（`README.md:65-74`）。验收清单要求覆盖这些领域，并明确哪些复用框架、哪些属于平台新增（`README.md:85-105`）。

README 第 7、31、44-56 行关于 tRPC-Agent-Go、Session/Memory、storage 和 OpenClaw 的描述是“可复用能力/方案要求”，不是参考仓库已经实现这些能力的证据。

## 2. 参考仓库实际实现边界

远端 HEAD 的完整工作树只有 24 个文件；`go.mod` 只有模块名和 `go 1.21`（`go.mod:1-3`），没有 `require` 块，也没有 `trpc-agent-go` 依赖。源码检查结果如下：

| 检查项 | 一手证据 | 结论 |
|---|---|---|
| 命令入口 | `cmd/trpc-service/main.go:10-16` 只打印版本和说明，`--help` 打印 usage 后返回 | 没有 HTTP/gRPC listener、Gateway、Worker 或长驻服务 |
| Agent 执行 | `trpcservice/agent/agent.go:1-3` 只有包注释 | 没有 `Runner`、模型工厂或执行路径 |
| IM | `trpcservice/channels/channels.go:1-3` 只有包注释 | 没有 WeCom/WeChat/Telegram adapter、验签、webhook 或回复投递 |
| 租户/配置 | `trpcservice/tenant/tenant.go:1-2`、`config/config.go:1-2` 只有包注释 | 没有租户存储、隔离、CAS、密钥管理或控制面 |
| 存储/同步 | 参考树没有 `session`、`memory`、`storage`、migration 或数据库代码 | 没有 Session/Event/State/Memory/Summary/Artifact/Knowledge 数据面 |
| 观测/审计 | `metrics/metrics.go:1-2`、`log/log.go:1-2` 只有包注释 | 没有 OTel exporter、Prometheus、审计 sink 或脱敏实现 |
| 管理/前端 | `web/web.go:1-2` 只有包注释 | 没有 Admin API、路由、认证或管理 UI |
| 部署 | `build.sh:4-9` 仅构建 CLI；`start.sh:7-20` 用 `nohup` 运行该 CLI；`stop.sh:4-19` 仅操作 PID 文件 | `start.sh` 启动的进程会立即退出，不能证明服务可用 |
| 文档占位 | `docs/README.md:1-8` 只列未来应放置的架构图、时序图、数据模型和风险清单 | 交付设计文档尚未落入参考树 |
| 测试/CI/部署清单 | 完整树无 `*_test.go`、Dockerfile、Compose、Kubernetes、migration 或 `.github/workflows` | 没有自动化回归或基础设施验证 |

在该 clone 中串行执行的 `go test ./...`、`go build ./...` 和 `go vet ./...` 均退出码 0；输出全部是“`[no test files]`”。这只证明包注释和元数据 CLI 可编译，不证明任何平台功能。

## 3. 与 hardened 归档的事实对照

本地归档的既有审计结论总体仍成立：它应按参考仓库的验收要求评估，而不是把参考仓库当作竞争性生产实现。归档已经具备参考仓库没有的真实运行路径：

- `pkg/gateway/server.go` + `pkg/reliable/postgres.go`：验签、租户路由、PostgreSQL Inbox/Outbox、复合幂等/payload hash、租约、重试、DLQ 和受控 replay。
- `pkg/pipeline/consumer.go`、`pkg/worker/worker.go`、`pkg/pipeline/delivery.go`：Consumer→Runner Worker→Outbox Delivery 的组合链路；Worker 注入租户选择的 Session/Memory，并用 Redis session lease 串行化完整 invocation。
- `pkg/storage/backend_factory.go:54-187`：当前真实支持 InMemory、Redis、PostgreSQL Session/Memory；不是 README 所列 Redis/SQL/向量/对象/外部 Memory 的全集。
- `pkg/channel/adapter.go`、`pkg/channel/wework_adapter.go`、`pkg/channel/telegram_adapter.go`：企业微信和 Telegram 两条真实主路径。微信客服/公众号、媒体/撤回等仍是明确边界。
- `pkg/controlplane/service.go`、`pkg/controlplane/resolver.go`：租户配置 CAS、Agent App/Version/Deployment、stable/canary、session 稳定分桶和幂等版本绑定。
- `pkg/governance/plugin.go`、`pkg/telemetry/telemetry.go`、`pkg/telemetry/otel.go`：工具治理、审计、W3C traceparent/OTLP 与指标接线。
- `deploy/docker-compose.yml`、`deploy/kubernetes/*`、`scripts/validate.sh`：部署和本地验收结构存在，但该源代码包不包含宿主仓库 CI policy；本机未执行 Docker/真实数据库、IM sandbox、容量或 KMS/Vault/mTLS 验证。

这些结论与 [REFERENCE_SERVICE_AUDIT.md](REFERENCE_SERVICE_AUDIT.md) 的总体判断一致；参考仓库没有新增实现使本归档的比较基线发生变化。

## 4. 旧审计中已过时或需要校准的内容

`REFERENCE_SERVICE_AUDIT.md:77-83` 仍把当前归档的 Compose Redis 宿主机暴露 `6379:6379` 列为缺口。该结论已过时：当前 `deploy/docker-compose.yml:18-24` 的 Redis 服务没有 `ports`，而 `deploy/config_test.go:52-72` 明确断言未认证 Redis 不得发布宿主端口。仍应保留的边界是：Compose 内部 Redis 仍未配置 TLS/ACL，不能当作生产安全配置；生产需使用受管控 Redis、TLS/ACL 和网络策略。

后续硬化已把文档与实际迁移清单统一到 24 组，并通过新增 024 迁移补充 Summary 不变量；重新执行了 up/down 配对检查。这仍不等于在真实 PostgreSQL 上执行了迁移。

README/架构现已统一 replay 语义：Inbox 恒为 restart；Outbox 默认 `resume` 并保留 `delivery_cursor`，只有显式 `OutboxRestartStore`/`cmd/replay outbox --restart` 才从第 0 段重发。两种模式都保存 actor/reason/mode，文档明确了 restart 的重复回复风险。

## 5. 当前应优先处理的真实风险

本次参考核验没有发现参考仓库新增的 P0/P1 问题。后续硬化状态如下：

1. **持久化 FIFO：实现并完成内存回归。** 事务性 `session_sequence` 与前序排除已位于可靠存储 seam；PROCESSING/RETRY_WAIT/DLQ 暂停、重放后解锁、过期前序重领、跨 session 独立领取和重复序号均有 MemoryStore 测试。
2. **Store 兼容：已修复。** 破坏性的 Outbox restart 位于可选 `OutboxRestartStore`，基础 `Store` 与 Consumer/Delivery 不依赖它，并有不实现 restart 的 Store 兼容测试。
3. **MemoryStore replay 审计：已修复。** 内存 Adapter 保存不可由调用方篡改的 audit snapshot，并覆盖 inbox restart、outbox resume/restart 字段。
4. **可信回复路由：已修复。** Gateway 固化 provider reply target；`CompleteInbox` 只接受不含路由的 `OutboxReply`，Memory/PostgreSQL 都从租约保护的 Inbox 派生 tenant/account/conversation/reply target。
5. **真实 PostgreSQL 证据仍缺失。** integration-tag 已加入 FIFO、重复序号、DLQ/replay、审计和权威路由场景并通过编译，但本轮没有连接数据库执行。Summary job/checkpoint 的 sqlmock 和静态约束已覆盖 lease/CAS 语义；静态 SQL 和编译仍不能替代真实事务行为验证。

## 6. 结论

参考仓库当前 main 是一个诚实的架构题目脚手架，不是已实现的平台；本地 hardened 归档在可靠消息、租户隔离、两类 IM、Runner/Session/Memory、Summary 协调器和控制面方面明显超出它。持久化 FIFO、接口兼容、回放审计、可信路由和 Summary job/CAS 已完成代码级硬化，但真实 PostgreSQL 事务、Compose/Kubernetes 运行、容量、KMS/Vault、mTLS、具体 Summary Generator、Knowledge/Artifact 与跨后端迁移适配器仍未验收，因此不能宣称生产就绪。不要为了“覆盖更多名词”添加无运行路径的空壳。

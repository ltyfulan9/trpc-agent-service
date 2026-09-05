# V14 安全审计报告

审计对象：本归档的 Go 源码、migrations、Compose/Kubernetes 模板和发布校验。  
结论：关键租户边界、消息 fence、治理、密钥最小暴露和专用数据面已形成可执行控制；正式身份系统、KMS/Vault、mTLS、云 IAM 与真实 IM 仍需目标环境验收。本报告不是渗透测试或合规认证。

## 1. 威胁模型与信任边界

不可信输入包括 IM webhook、消息正文/附件 URL、Tool 参数、模型输出、租户管理员提交的配置和第三方 Provider 响应。受信边界分为公网 Gateway、内部 Consumer/Worker、Admin 控制面、共享 PostgreSQL/Redis、Qdrant/S3 数据面以及 Telemetry 出口。主要攻击目标是跨租户读取、伪造回调/内部请求、重复副作用、权限提升、秘密泄露、SSRF、队列/预算耗尽和供应链替换。

## 2. 已落地控制

| 领域 | 控制 | 证据入口 |
|---|---|---|
| Webhook authenticity | Telegram secret header；WeCom SHA1、AES-CBC、CorpID 和 URL verify | `pkg/channel`, Gateway tests |
| Internal authenticity | Consumer→Worker HMAC-SHA256 绑定 method/path/body/traceparent；Redis nonce 防重放 | `pkg/internalapi`, composition tests |
| Admin authorization | bearer→Principal；role/action/tenant allowlist；不信任 `X-Admin-Actor` | `pkg/adminauth`, `cmd/admin` |
| Tenant data scope | SQL 复合键/查询带 tenant；scoped reader；缺能力 fail-closed | tenant/reliable/controlplane tests |
| Knowledge scope | Qdrant 物理 ID=`SHA256(tenant\0app\0logicalID)`；保留 metadata 不可覆盖 | `pkg/knowledgeplane` |
| Artifact scope/integrity | tenant-scoped object key；不可变版本；最大 16 MiB；MIME/标识校验；SHA-256 load verify | `pkg/artifactplane` |
| Secret handling | profile manifest 只存 env 名；JSON unknown fields/raw secret 拒绝；仅 Worker 解析；字段私有不可序列化 | `pkg/runtimeplane`, releaseverify |
| Tool authorization | Runner BeforeTool 唯一 seam；tenant+version 双白名单；危险 Tool durable challenge 一次消费 | `pkg/governance`, `pkg/approval` |
| MCP boundary | operator-owned HTTPS profile；精确远端 Tool allowlist；Worker-only Header SecretRef；禁用 stdio/危险 Header | `pkg/platformtool/mcp.go`, MCP vertical-slice tests |
| Cost control | Redis Lua 原子 token reservation/dispatch/settlement；未知 usage 保守计费 | `pkg/budget`, `pkg/summaryruntime` |
| Idempotency/fencing | Inbox/Outbox unique+hash；owner/lease_version/expiry；结果未知进 reconciliation | `pkg/reliable`, `pkg/execution` |
| Output/log safety | 工具结果和最终模型输出递归脱敏；错误稳定分类；用户 ID 租户 HMAC 假名 | governance/audit/telemetry tests |
| Deployment | non-root、read-only rootfs、drop ALL、no-new-privileges、seccomp、default-deny、digest gate | `deploy`, `pkg/releaseverify` |

## 3. 密钥和配置隔离

租户模型/API/IM 密钥以 AES-GCM envelope 保存，也可引用 operator-owned `env://TRPC_SECRET_*`；读 API 返回遮盖值，更新时只有匹配既有 identity 的 `***REDACTED***` 可表示“保持不变”。Session/Memory 只引用 operator-owned profile，禁止租户写 DSN/URL。Knowledge/Artifact 与 MCP profile 都是无秘密 JSON：Admin 得到公开 profile 做准入，只有 Worker 得到数据面/MCP Header Secret；模型 Key 只进入 Worker/Summary Worker，Channel Secret 只进入 Gateway/Delivery，Consumer/Admin 均不得获得这些值。

Kubernetes `runtime-data-plane-profiles` ConfigMap 的 `profiles.json` 与 `mcp-profiles.json` 都只能存无秘密声明；`runtime-data-plane-credentials` 的四个 key 只可在 Worker 主容器各引用一次，releaseverify 会拒绝非 Worker、sidecar 或重复泄露。清单回归另锁定 MCP、模型与 Channel Secret 的进程范围。生产必须把 Secret 来源换成 KMS/Vault workload identity；环境变量实现只证明 resolver 契约，不构成外部密钥系统验收。

## 4. 数据面安全

Qdrant profile 校验 endpoint、TLS、dimension、collection、embedding endpoint 和 tenant allowlist；HTTP embedding 仅允许显式本地 `allowInsecure` 且 host 为 loopback。S3 profile 校验 endpoint、bucket、region、object 大小和 credential refs；生产要求 TLS。Artifact 文件名拒绝路径分隔符、控制字符和格式字符，object key 对每段 base64url 编码；PostgreSQL advisory lock 使用固定 digest，避免用户文本进入锁/活动查询。

Knowledge/Artifact migration 在副作用前后读取 lease fence，过期执行者不能写 projection marker。Artifact 同版本异内容拒绝；相同内容重放用于修复对象正文。删除先写 tombstone，再做对象删除，失败可差异扫描，而不是删除正文后丢失权威状态。

## 5. 执行与网络安全

Gateway 必须在 durable commit 后 ack。Consumer→Worker 的 HMAC 不替代传输加密：生产只接受 HTTPS，或在已验收严格 peer authentication 的 mesh 中显式开启 mesh 模式。Worker 对模型/Tool 调用设置 deadline；连接失败只有在确认请求未越过写入边界时才可重试。危险 Tool 的批准绑定 tenant、actor、owner、session、tool、canonical args 和 invocation，raw approval token 不进入 Inbox、Session 或 HTTP 响应。

当前附件 URL 只传给模型 Provider，Worker 不下载。入口拒绝 userinfo、fragment、localhost 和字面量私网/链路本地地址，但这不是 DNS rebinding 完整防护；任何未来下载器必须使用 operator allowlist、DNS pin、逐跳重定向校验、最大响应、超时、内容类型和恶意文件扫描。自定义模型 endpoint 当前 fail-closed。MCP endpoint 只能由平台运维配置，生产要求 HTTPS；代码层仍不能替代集群 egress allowlist 与 DNS rebinding 防护，因此每个真实 MCP profile 必须在目标网络做独立验收。

## 6. 可观测与审计

公网回调创建可信 root span，外部提供的 trace header 不直接成为父；内部 traceparent 纳入 HMAC。日志/trace 不允许记录 webhook token、模型 key、数据库密码、Authorization、完整 payload 或原始用户标识。审计包含 tenant/channel/pseudonymous user/session/agent version/tool/decision/latency/error/cost/trace，并与控制面变更、审批、DLQ replay 分离保存。

Prometheus `/metrics` 生产默认需要 bearer；只有 loopback Compose 使用 unauthenticated 开关。tenant/model/agent 动态标签采用严格 allowlist，其余合并，避免攻击者制造无界 series。OTLP 在目标环境必须启用 TLS、服务身份和 attribute processor 二次清洗。

## 7. 供应链和运行时

生产 Dockerfile 固定 Go 1.26.7 基础镜像 digest，最终镜像非 root、只读、无 shell 依赖的最小权限运行。Kubernetes releaseverify 拒绝 tag-only image、缺失 workload、错误 Worker transport、profile/secret 泄露和未受控 egress。正式上线还应补充：构建 provenance、SBOM、镜像签名、admission verification、依赖漏洞扫描、分支保护和双人 release approval。

## 8. 剩余高风险与上线条件

| 剩余项 | 当前判断 | 上线条件 |
|---|---|---|
| 真实 WeCom/Telegram | 自动化协议测试，不是 sandbox | 验签、重复、429、分段、失败回包各留 trace/request_id |
| KMS/Vault | 只有安全 resolver seam | 正式 workload identity、最小 policy、双 key 轮换与撤销演练 |
| mTLS | 模板/门禁存在 | 正式 CA、strict mode、身份 allow/deny、证书轮换 |
| Cloud S3/Qdrant | 本地 MinIO/Qdrant 通过 | IAM/ACL、TLS、私网 egress、备份恢复和配额测试 |
| HA/DR | 单机多容器证据 | PG/Redis failover、PITR、RPO/RTO 演练 |
| Tool/MCP 业务系统 | official MCP HTTP 纵切已通过，目标系统未验收 | 每个 profile 独立认证、域名/egress allowlist、最小权限、幂等、配额和超时审计 |

若上述外部条件未满足，交付应称为“生产级候选实现 + 本地实测证据”，不能称为已完成生产认证。

# V14 评委快速摘要与验收执行清单

本页是评委在 5–10 分钟内建立判断的入口。它把“源码已经实现”“本机已经验证”和“必须在目标环境完成”分开，不把架构设想、单元测试或本地模拟器包装成生产证据。完整设计见 [决赛方案](COMPETITION_SUBMISSION_V14.md)，逐项命令和后状态见 [验收证据矩阵](ACCEPTANCE_EVIDENCE_V14.md)。

## 一、先看什么

先看决赛方案第 2、5、6、7、8 节，分别了解组件边界、企业微信时序、多后端一致性、治理审计和故障容量；再看数据模型中的 Mermaid ER 图，确认平台表与 tRPC 后端物理表没有重复所有权。

关键取舍是“每类状态只有一个权威所有者”：PostgreSQL 负责队列、版本、审计和 fence；租户后端负责 Session/Memory；Qdrant 负责 Knowledge；S3/MinIO 负责 Artifact。Gateway 只在 Inbox 提交后确认，Consumer 以 sequence 和 lease/fence 保序，Worker 无 sticky session，Delivery 以 `DISPATCH_STARTED` 区分未知副作用。系统明确是 at-least-once，未知结果必须核对后审计 replay。

## 二、本机已验证的闭环

以下结果来自 C 盘隔离栈和真实容器服务，不依赖 E 盘缓存：

| 能力 | 证据 | 结论 |
|---|---|---|
| 源码门禁 | `go mod verify`、gofmt、build、vet、unit、race、integration | Go 1.26.7 串行门禁通过 |
| 可靠消息 | PostgreSQL Inbox/Outbox、重复消息、payload 冲突、FIFO、lease takeover、DLQ | 同一会话有序，旧 Worker 不能复活写 |
| 数据面 | Redis Session、PostgreSQL Session、Qdrant Knowledge、MinIO Artifact | 真实本地后端纵切通过 |
| Summary | enqueue→freeze→生成→fenced checkpoint→下一次 Runner 请求 | 摘要 cutoff 和旧历史裁剪有捕获断言 |
| 治理 | Plugin、白名单、预算 reservation、危险 Tool challenge/grant/consume | 未授权工具 fail-closed，审批可审计 |
| 运行时 | LLM、Chain、Graph、Parallel、Cycle factory 和拓扑测试 | 节点预算、可达性和有限循环受校验 |
| 观测 | W3C traceparent、OTel 边界、Prometheus 规则和指标鉴权 | 跨 Inbox/Worker/Outbox 的传播契约通过 |
| 部署 | C 盘 Compose 隔离栈、应用健康、migration、Prometheus targets、镜像构建 | 本地演示环境可启动，非生产认证 |

本机最短验证路径：

```powershell
Set-Location <V14 源码根目录>
.\scripts\run_c_local_stack.ps1 -ProjectName trpc-v14-c-local-final -Build
.\scripts\validate.sh
.\scripts\run_c_local_stack.ps1 -ProjectName trpc-v14-c-local-final -Down
```

源码不携带真实 `.env`、Provider token 或数据库密码；普通 Compose 缺少密码变量是预期的 fail-closed。C 盘脚本只在当前进程注入一次性值，结果应保存退出码、容器健康和 trace 证据。

## 三、题目要求逐项状态

`LOCAL_VERIFIED` 表示当前机器有命令或真实容器后状态；`IMPLEMENTED` 表示生产代码和自动化回归已具备，但缺目标环境；`EXTERNAL_REQUIRED` 表示没有账号、集群或供应商授权就不能完成。当前状态如下：

- 多租户、RBAC、密钥遮盖、Agent Version/Deployment、稳定灰度和回滚：`LOCAL_VERIFIED`。
- 无状态多节点、Session 路由、Inbox/Outbox 幂等、分段、限流、重试、DLQ：`LOCAL_VERIFIED`。
- 企业微信和 Telegram 协议适配器：`IMPLEMENTED`；企业微信 URL challenge、AES、CorpID、Telegram webhook 和真实回复：`EXTERNAL_REQUIRED`。
- Redis→PostgreSQL Session migration，以及 Session/vector/object projection：`LOCAL_VERIFIED`；正式数据量 cutover 和 rollback window：`IMPLEMENTED`，仍需目标演练。
- Summary、Knowledge、Artifact 本地真实纵切：`LOCAL_VERIFIED`；多云后端、外部 Memory 服务、Milvus/COS 适配和审计管道迁移：当前不是已验收能力，不得从接口枚举推断完成。
- Compose、Kubernetes 模板、digest/releaseverify、NetworkPolicy 和 HPA 配置：`LOCAL_VERIFIED`；正式 Kubernetes/mesh rollout、mTLS 证书轮换、HA 数据库和灾备：`EXTERNAL_REQUIRED`。
- KMS/Vault workload identity、双 key 轮换、OTLP TLS、Alertmanager 接收端：`EXTERNAL_REQUIRED`。
- 正式容量、成本、PITR/DR 和模型 Provider 额度/SLA：`EXTERNAL_REQUIRED`。本地 2,200 条公平队列数据只是实验室回归基线。

## 四、外部验收执行顺序

严格按低次数 Runbook 执行：离线门禁 → 目标 Kubernetes → 公网 callback 空探针 → Telegram → 企业微信 → KMS/Vault 轮换 → PostgreSQL/Redis 故障 → 正式容量。先验证 Telegram 的通用消息链路，再消耗企业微信 URL challenge 和加密回调次数。任何 DNS、证书、变量或签名错误都应先在本地修复，不能靠重复保存控制台配置掩盖问题。

真实企业微信验收至少保存：脱敏 MsgId/hash、Inbox、execution、Outbox、trace_id、处理延迟、回复状态和 Pod imageID；禁止保存聊天正文、CorpSecret、EncodingAESKey、Authorization、Cookie、数据库 URL、Vault token 或 projected JWT。Delivery 在 Provider 调用后若结果未知，证据必须显示 `WAITING_RECONCILIATION`，不能为了让测试“变绿”自动重发。

目标 Kubernetes 验收必须记录镜像 digest、Deployment/mesh/HPA/NetworkPolicy 后状态；数据库和 Redis 各做一次故障或主备切换，确认未提交 Inbox 不会 2xx、Worker 不降级本地锁、旧 fence 写入被拒绝。容量报告用业务 payload 和真实模型延迟给出 p50/p95/p99、吞吐、queue lag、DB/Redis QPS、资源和成本，不能把实验室基线写成生产承诺。

## 五、提交前硬门禁

V14 已发布到 [GitHub](https://github.com/ltyfulan9/trpc-agent-service/tree/v14-final)，固定检查点为 `65ace40`，标签为 `v14.0.0-final`。评委应从全新目录执行 `git clone`、`git checkout v14.0.0-final`、`./scripts/validate.sh`、Compose config 和镜像构建。仓库保留 Apache-2.0 LICENSE、`.github/workflows/verify.yml` 和不含秘密的测试配置；真实企微回调仍按本页 `EXTERNAL_REQUIRED` 边界验收。

最终评分时，应把实现质量和边界诚实同时纳入判断：本地可靠消息和治理链路是已验证优势；真实 IM、目标基础设施、跨后端迁移矩阵、正式容量与 GitHub 可复现入口是决定能否从“前列”升到“第一”的关键差距。任何未完成外部动作都应保持 `EXTERNAL_REQUIRED`，由证据而不是措辞决定状态。

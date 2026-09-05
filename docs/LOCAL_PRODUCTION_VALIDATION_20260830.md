# V13 本地生产化验证报告（2026-08-30）

> 结论：当前源码、Compose 集成栈和本地三节点 Kubernetes 实验室已经完成一轮可复现的生产化验证，并在真实并发容量场景中发现、修复和复验了一处公平队列竞态。这里的 `VERIFIED-LOCAL` 只表示所列本地证据成立，不等于目标云环境 `production certified`。

## 1. 验证对象与版本轴

| 对象 | 实际版本/标识 | 结论 |
|---|---|---|
| 模块源码下限 | `go 1.25.14` | 由当前依赖树决定，不是生产工具链推荐 |
| 生产容器构建器 | Go `1.26.7`，镜像 digest 固定 | 保持生产基线 |
| 本机默认工具链 | Go `1.26.2 windows/amd64` | build/vet/unit/race 本轮通过；安全发布仍由 `require_secure_go.sh` 拒绝旧补丁版本 |
| 前向兼容通道 | Go `1.27.0 windows/amd64` | `go mod verify` 与全仓默认测试通过，`go.mod`/`go.sum` 哈希不变 |
| tRPC-Agent-Go | `v1.11.2` | 应用依赖的框架版本；与 Go 编译器版本是两个独立版本轴 |
| Docker Engine | `29.7.2` | Compose 与 K3d 均在该引擎上运行 |
| Kubernetes 实验室 | k3d `v5.9.0`，K3s `v1.36.4+k3s1`，1 server + 2 agents | 三节点 Ready |
| Service Mesh | Linkerd `edge-26.8.4` + CNI | 因 K8s 1.36 采用 edge 版本，只作为本地验证；生产需选择供应商支持矩阵内组合 |
| Vault 实验室 | Helm chart `0.34.1`，Vault `2.0.4` dev mode | 只验证身份/策略链路，不是 HA Vault 生产部署 |

`tRPC-Agent-Go` 的 `Go 1.21 or later` 是框架最低编译器要求，不是只能使用 Go 1.21。V13 自身依赖要求更高下限，生产镜像使用 1.26.7，并且当前源码额外通过 1.27.0 前向兼容测试；三者不冲突。

## 2. 本轮实际通过的门禁

```text
go mod verify                                               PASS
gofmt -l .                                                  PASS (no output)
go build -buildvcs=false -p 1 ./cmd/...                     PASS
go vet -p 1 ./...                                           PASS
go test -buildvcs=false -count=1 -p 1 ./...                 PASS
go test -buildvcs=false -race -count=1 -p 1 ./...           PASS
go1.27.0 go mod verify                                      PASS
go1.27.0 go test -buildvcs=false -count=1 -p 1 ./...        PASS
go test -tags=integration -count=1 -p 1 ./test/integration  PASS (176.984s)
releaseverify --schema-class compatible ...                 PASS
GOTOOLCHAIN=go1.26.7 ./scripts/validate.sh                   PASS (10/10)
go1.26.7 govulncheck v1.7.0 ./...                            No vulnerabilities found
```

最终 `validate.sh` 是归档前从头执行，不是复用缓存的状态声明：安全 Go 门、模块、格式、build、vet、unit、全 race、六镜像构建、Prometheus 12 条规则、隔离真实 PostgreSQL/Redis integration 和静态门全部通过。其 Docker 同网络 integration 用时 7.286 秒；上表 176.984 秒是额外的 Windows→K3d 端口转发隔离库运行，两者网络路径不同且都通过。

真实后端集成测试使用独立数据库 `trpc_agent_integration_20260830`、PostgreSQL 15.8 和 Redis 7.4；Redis 测试键使用唯一前缀并由测试清理，没有调用 `FLUSHDB`。覆盖 Redis→PostgreSQL 迁移、可靠生命周期、公平租约回收、多连接接管/fencing、行锁等待后租约过期、删除队列策略、过期 reaper、Session FIFO/回放、滚动升级兼容、并发重复投递和 execution reconciliation。

## 3. Compose 运行证据

- 11 个服务运行：PostgreSQL、Redis、Migrate、Gateway、Consumer、Worker、Delivery、Admin、OTel Collector、Prometheus、Grafana。
- 六个 Go 服务镜像已构建；应用进程以 non-root、只读根文件系统、drop all capabilities、no-new-privileges 运行。
- 37 组迁移已应用；PostgreSQL 备份恢复后的 30 张表、309 列和迁移记录一致。
- Redis RDB 恢复和 TTL 行为通过；PostgreSQL/Redis 单容器重启后持久数据存在。
- Prometheus 5/5 应用抓取目标为 `up`，Grafana 数据库健康。
- OTLP TLS：正确 CA 成功，伪造 CA 失败；没有把 `insecure` 模式当成生产证据。
- Toxiproxy 注入数据库延迟和连接 reset 后，服务按预期失败并恢复。

Compose 是本地开发/集成拓扑，不代表托管 HA PostgreSQL、Redis Cluster/Sentinel 或跨可用区灾备已验收。

## 4. Kubernetes、mTLS 与发布验证

### 4.1 三节点拓扑

三节点均为 `Ready`，多副本 workload 通过 `topologySpreadConstraints` 分散。应用 namespace 使用 restricted Pod Security；工作负载禁用默认 ServiceAccount token 自动挂载，容器使用 non-root、seccomp 和只读根文件系统。

### 4.2 Linkerd 与 NetworkPolicy

- Linkerd CNI 和原生 sidecar 已注入应用 Pod；K8s 1.36 下代理位于 `initContainerStatuses`，`linkerd-proxy` 为 `Ready=true`、`Started=true`、零重启。
- 对完全相同的 `/v1/process` 业务请求，meshed client 得到应用层 `401`，证明请求通过 mTLS 到达鉴权边界；未注入 identity 的 client 得到 Linkerd `403`。
- 代理指标同时出现 `tls=true` 的 workload identity 和 `no_identity` 拒绝证据。
- 默认拒绝 NetworkPolicy 原先漏放透明代理入站端口 4143；先复现阻断，再在所有真实业务边界增加精确 4143 ingress 与 control-plane egress，回归测试固定该契约。

这证明本地 mesh allow/deny 生效，但生产仍需使用被供应商支持的 Kubernetes/Linkerd 组合、正式信任根轮换、受控 egress 和多命名空间身份策略。

### 4.3 发布、回滚与 HPA

- Gateway 完成 revision 1→2 灰度标记发布，再 `rollout undo` 到 revision 3；最终 2/2 Ready，标记消失，镜像 digest 未漂移。
- 兼容迁移 Job 成功后才发布 workload；发布脚本已修为 Worker/Admin Ready 后再发布 Consumer/Delivery，避免 Consumer 的 fail-closed Worker 健康预检造成正常 rollout crash loop。
- HPA 原来使用 Pod Resource 指标，Linkerd sidecar 无同名 request 时产生 `<unknown>`；已改为 `ContainerResource`，Gateway CPU/Memory 与 Worker CPU/Memory 均 `ScalingActive=True/ValidMetricFound`。
- Consumer 已滚动到：

```text
trpc-v13-registry:5000/v13/consumer@sha256:1d7758cf8261681b2701fff78d6e6af746a474d4126b77094920533138f71208
```

两副本 2/2 Ready、应用与 Linkerd 代理均零重启；`FAIR_QUEUE_ENABLED=true`、`CONCURRENCY=4` 已成为显式部署契约。

## 5. Vault workload identity 本地证据

使用官方 HashiCorp chart 的 dev-mode Vault 配置 Kubernetes Auth：

- 专用 ServiceAccount 使用 `audience=vault`、600 秒 projected token、`automountServiceAccountToken=false`。
- Vault role 只绑定 `vault-auth-test/vault-client`，policy 只允许目标测试路径。
- 正向 Pod 输出 `VAULT_WORKLOAD_IDENTITY_OK` 并成功结束。
- 默认 ServiceAccount 的负向 Pod 输出 `VAULT_UNAUTHORIZED_IDENTITY_DENIED` 并成功结束。
- 两只测试 Pod 均零重启；任何 token、root token、私钥或 secret value 均未进入报告。

该结果证明 Kubernetes JWT→Vault role→最小权限 policy 的本地链路；dev mode、静态实验室根 token 和单实例 Vault 不能作为生产 HA、auto-unseal、审计存储或云 KMS 证据。

## 6. 公平队列容量与竞态修复

### 6.1 首轮发现的问题

16 个并发 claimer 下，`ClaimInboxFair` 的候选 CTE 可能在等待期间变陈旧；旧 SQL 的最终 `UPDATE` 只按 `id` 更新，导致一个已经完成并生成 Outbox 的 Inbox 被重新写为 `PROCESSING`。

修复包括：

1. 最终 `UPDATE` 重新校验当前状态和各状态的时间条件，只允许 RECEIVED、到期 RETRY_WAIT、到期 WAITING_APPROVAL 或租约过期 PROCESSING。
2. 并发状态已变化时返回 `ErrNoWork`，事务回滚，虚拟运行时间不会被错误推进。
3. 新增 `TestPostgresFairClaimDoesNotResurrectCompletedStaleCandidate`，先红后绿。
4. 生产 Consumer 镜像重建并按 digest 发布，而不是只修源码不更新运行实例。

### 6.2 干净复验结果

独立容量数据库上的最终成功 run：

```text
CAPACITY_RUN=cap-1788060387113445463
ENQUEUE total=2200 workers=16 elapsed=12.456120016s throughput=176.6_ops_s p50=89.020022ms p95=99.016557ms p99=129.954639ms
FAIRNESS sample=200 quiet_claims=100 first_quiet_position=2 max_consecutive_noisy=1
CLAIM_COMPLETE total=2200 workers=8 elapsed=2m7.233270184s throughput=15.7_ops_s p50=101.184408ms p95=195.918054ms p99=1.050497368s
DURABILITY completed_inbox=2200 outbox=2200 errors=0
```

负载为 noisy tenant 2,000 条、quiet tenant 200 条；本地实际部署为 2 个 Consumer Pod × 每 Pod 4 workers，因此最终验收使用 8 个 claimer。16 claimer 的高竞争试验在修复后没有再复活终态消息，但对仅两条 tenant schedule row 的锁竞争明显，不能把它宣传成推荐配置。

这些数字是当前笔记本、K3d、单 PostgreSQL Pod 的基线，不能外推为目标集群容量。正式环境仍需以业务 payload、模型耗时、数据库规格和 IM 峰值重跑，并保留 1.5–2 倍 headroom。

## 7. 仍未完成、必须外部验收的项目

| 项目 | 当前边界 | 上线前最低证据 |
|---|---|---|
| 企业微信真实 sandbox | 仅代码/HTTP fake | URL challenge、加密文本回调、重复回调、出站回复、token refresh、失败重试 |
| Telegram 真实 sandbox | 仅代码/HTTP fake | webhook secret、private/group 更新、重复 update、sendMessage、429/失败恢复 |
| 目标 Kubernetes | 本地 K3d 已验 | 供应商支持版本上的 rollout/rollback、PDB、节点故障、HPA、NetworkPolicy、mesh identity |
| 生产 KMS/Vault | 本地 Vault dev auth 已验 | workload identity、最小权限、HA/auto-unseal、审计、双 key 无停机轮换 |
| PostgreSQL/Redis HA | 单实例故障注入 | primary failover、连接池耗尽、Redis 稳定端点切换、数据/RPO/RTO |
| 灾备 | 仅单机备份/恢复 | 异地恢复、PITR、密钥可用性、完整 runbook 和实测 RTO/RPO |
| 向量库/对象存储 | 设计/接口 | 真实 Qdrant/Milvus/S3 兼容服务迁移、校验、回滚和权限 |
| 正式容量/成本 | 本地 2,200 条基线 | 业务流量模型、模型 token、DB/Redis QPS、queue lag、成本和错误预算 |

## 8. 可声明与不可声明

可以声明：当前源码门禁、真实 PostgreSQL/Redis 集成、本地 Compose、三节点 K3d 发布/回滚、本地 Linkerd mTLS allow/deny、OTLP TLS、本地 Vault workload identity 和所列 2,200 条容量场景已通过。

不可声明：真实 IM sandbox 已通过、生产 KMS/Vault 已部署、供应商支持的生产 mesh 已验收、PostgreSQL/Redis 多副本 HA 已通过、正式容量已达标或整个平台已经 `production certified`。

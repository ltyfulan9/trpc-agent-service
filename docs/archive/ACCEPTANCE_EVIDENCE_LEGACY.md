# V13 验收证据矩阵

> 历史归档：当前权威矩阵为 `ACCEPTANCE_EVIDENCE_V14.md`。

状态定义：`VERIFIED` 表示本包当前可复现测试已通过；`IMPLEMENTED` 表示生产代码存在且单元/静态门已通过，但本轮缺外部基础设施验收；`DESIGNED` 表示接口与落地方案明确但没有完整运行时；`EXTERNAL` 表示必须由目标账号、集群或供应商环境证明。

| 题目验收项 | 设计/实现定位 | 当前证据 | 状态 |
|---|---|---|---|
| tenant/app/model/tool/channel/storage/audit 模型 | `pkg/tenant`, migrations 001/003/005 | tenant validation、CAS、加密、RBAC tests | VERIFIED |
| Gateway/Worker/Channel/Storage/Admin/Telemetry 拓扑 | `cmd/*`, `pkg/*`, Compose/K8s, 决赛架构图 | build、package tests、deployment config tests | VERIFIED |
| 无 sticky session、水平扩展 | `pkg/storage`, `pkg/worker`, `pkg/reliable` | 共享 Session/Memory 注入、Runner cache/lease/FIFO tests | VERIFIED |
| 跨节点 session 正确路由 | `pkg/channel/session_identity.go`, Inbox routing columns | direct/group/cross-tenant tests | VERIFIED |
| 多副本陈旧 Worker fencing | `pkg/reliable/postgres.go` | 独立 DB 连接 replica takeover integration 在真实 PostgreSQL 15.8 上连续 5 轮通过 | VERIFIED |
| 配置/数据/工具/密钥/日志隔离 | tenant AAD、scoped readers、governance、audit | security/regression tests；外部 DB role/KMS 待验 | IMPLEMENTED |
| Redis/PostgreSQL Session/Memory 路由 | operator-owned backend profiles | backend profile、scope、health tests | VERIFIED |
| InMemory 限制 | production admission | Admin/Worker fail-closed tests | VERIFIED |
| Knowledge/vector 与 Artifact/object | 架构与逻辑契约 | 无本包真实 data-plane consumer | DESIGNED |
| Session 并发写一致性 | durable `session_sequence` + full invocation Redis lease | FIFO、lease-loss、cross-session tests；真实 Redis 抖动待验 | IMPLEMENTED |
| Event → State → Summary 顺序 | `pkg/summary`, migrations 023/024 | lease/fence/CAS/late-job tests；生产 Generator adapter 待接 | IMPLEMENTED |
| Memory 跨节点可见 | tRPC Redis/Postgres Memory backend | adapter wiring/unit tests；目标 HA backend 待验 | IMPLEMENTED |
| Redis→PostgreSQL 迁移 | `pkg/datamigration`, migration 037 | 真实 Redis 7.4→PostgreSQL 15.8 integration 连续 5 轮通过；UUID 前缀清理且不调用 `FLUSHDB` | VERIFIED |
| 向量/对象迁移 | 迁移状态机和 projection contract | 无 concrete projection | DESIGNED |
| IM 重复投递幂等 | Gateway + Inbox unique key/hash | duplicate/conflict tests | VERIFIED |
| 企业微信 | `pkg/channel/wework_adapter.go` | 验签/AES/token cache/重试/脱敏 tests；无 sandbox | IMPLEMENTED |
| Telegram | `pkg/channel/telegram_adapter.go` | secret token/parse/send/429/分段 tests；无 sandbox | IMPLEMENTED |
| 群聊/单聊隔离 | canonical hashed session + group owner/actor split | channel session tests | VERIFIED |
| 工具白名单/脱敏/审批/预算 | Runner Plugin + durable approval + Redis budget | governance/approval/budget integration tests | VERIFIED |
| trace 串链路 | OTel propagation + sanitized spans | Gateway/Worker/storage telemetry tests；本地 Collector 正确 CA 成功、伪造 CA 失败；目标后端待验 | IMPLEMENTED |
| 审计字段与不可伪造 actor | audit/control_plane_audit/adminauth | RBAC/scope/transaction tests | VERIFIED |
| context/goroutine/Event drain | health shutdown、Worker event collector | timeout/cancel/drain/race tests | VERIFIED |
| 灰度与租户回滚 | immutable version/deployment + invocation binding | controlplane tests | VERIFIED |
| 本地 Compose 验收拓扑 | `deploy/docker-compose.yml` | 六镜像构建、11 服务就绪、37 migrations、控制面生命周期、3 轮安全 Desktop 重启恢复、10/10 restart policy、5/5 scrape、容器安全约束 | VERIFIED |
| Kubernetes 本地发布机制 | `deploy/kubernetes`, `releaseverify`, `scripts/k8s_apply.sh` | 三节点 K3d compatible migration、digest rollout/undo、PDB/spread、NetworkPolicy、Linkerd allow/deny、HPA ContainerResource、Consumer 新 digest 2/2 Ready | VERIFIED |
| Kubernetes 目标环境 | 同上 + 环境 overlay | 本地 K3d 不能替代供应商支持版本、正式信任根、托管后端和跨 AZ 验收 | EXTERNAL |
| Vault workload identity | Kubernetes Auth + scoped role/policy | 官方 Vault dev-mode：专用 SA 正向成功、默认 SA 越权拒绝；生产 HA/KMS/轮换待验 | IMPLEMENTED |
| 容量评估（本地基线） | `docs/LOCAL_PRODUCTION_VALIDATION_20260830.md` | 2,200 条、16 enqueue/8 claim workers；公平性与 Inbox/Outbox 2,200/2,200、0 error 可复现 | VERIFIED |
| 容量评估（目标规格） | `docs/EXTERNAL_ACCEPTANCE_RUNBOOK.md` | 尚无业务 payload、模型耗时、目标 DB/Redis QPS 与成本结果 | EXTERNAL |
| Go 供应链与兼容 | pinned CI/image Go 1.26.7 + secure toolchain gate | Go 1.26.7 `govulncheck` 无发现；当前源码 Go 1.27.0 全仓测试通过且 mod/sum 不变 | VERIFIED |

## 本轮新增的硬证据

1. `scripts/require_secure_go.sh` 将生产验证和 Go 1.25 源码兼容门分离；Go 1.26.2 会 fail-closed，Go 1.26.7 通过。
2. CI 和本地验证现在显式传入 `TEST_REDIS_URL`；integration 不再用 `miniredis` 冒充真实 Redis。
3. replica takeover 测试使用两个独立 `sql.DB` 连接：A 领取后“崩溃”，B 在数据库时钟判定租约过期后以更高 fence 接管；A 的迟到 completion 被拒绝，B 只创建一个 Outbox。
4. 真实 PostgreSQL 15.8 + Redis 7.4 integration 先完整通过 1 轮，再连续通过 5 轮；不是 integration-tag 编译门。
5. Admin 无 token 返回 401；带 bootstrap principal 后完成 Tenant→Agent App→Version→Publish→stable Deployment，模型与通道密钥在响应中均为 `***REDACTED***`。
6. PostgreSQL 与 Redis 容器分别重启后，租户/部署数据与 AOF 测试值仍在；Worker 强制重启后恢复 healthy。Docker Desktop 4.88.1 的外部安全启动器又连续完成 3 轮完整 Desktop 重启，专用 Redis 值跨重启可读并在验证后删除，PostgreSQL 计数仍为 `1|1|1|1|5`。
7. Prometheus 实际抓取 Admin/Consumer/Delivery/Gateway/Worker 5 个目标，5/5 为 `up`；Grafana 11.1.4 返回 `database=ok`。
8. 所有外部 Compose 镜像使用 tag + Docker Hub digest；六个 Go 进程以 `65532:65532`、只读根文件系统、`cap_drop: ALL`、`no-new-privileges` 和非特权模式重建并复验数据/健康。
9. 10 个长驻服务均为 `restart: unless-stopped`，migration 保持 `restart: no`；最终安全 Desktop 重启后 10/10 恢复、4 个宿主端点返回 200、Prometheus 仍为 5/5。原生 Desktop restart 仍受 Docker 上游 AF_UNIX 缺陷影响，Windows 整机重启/登录钩子尚未实测，不能写成厂商根治或 OS reboot 验收。
10. 三节点 K3d 上完成 compatible migration、Gateway rollout/undo、Linkerd identity allow/deny、默认拒绝 NetworkPolicy、HPA 有效指标和 Consumer digest 滚动；修复了漏放 Linkerd 4143、Worker/Consumer 发布顺序和 Pod 级 HPA sidecar 指标三个真实部署问题。
11. 公平队列容量测试复现了陈旧 candidate 把已完成 Inbox 重新写为 `PROCESSING` 的竞态；最终 UPDATE 增加状态/时间重检并新增回归测试。修复镜像滚动后，最终 run `cap-1788060387113445463` 完成 2,200 Inbox 和 2,200 Outbox，错误 0。
12. 独立真实后端 integration 全套通过（176.984s）；全仓 unit、race、vet、build、module verification 通过。Go 1.27.0 前向通道也全仓通过，且 `go.mod`/`go.sum` 哈希未变化。
13. 官方 Vault dev-mode 实验室完成 Kubernetes workload identity 正向/负向测试；该证据只证明 JWT→role→policy 链路，不冒充生产 HA/auto-unseal/云 KMS。
14. 归档前以 Go 1.26.7 从头执行 `scripts/validate.sh` 10/10：全仓 unit/race、六镜像构建、12 条 Prometheus 规则、隔离真实 PostgreSQL/Redis integration 和静态门全部通过；随后 `govulncheck v1.7.0 ./...` 返回 `No vulnerabilities found`。

## 不得使用的夸大表述

- 不得把 Compose/Kubernetes 清单称为“已生产部署”。
- 不得把 `env://TRPC_SECRET_*` resolver 称为“已接 KMS/Vault”。
- 不得把通用 migration executor 称为“所有后端已迁移”。
- 不得把 runtime type 枚举称为“Chain/Graph/Parallel/Cycle 已安装”。
- 不得把 provider mock/httptest 称为“企业微信或 Telegram sandbox 通过”。
- 不得把本地 2,200 条单 PostgreSQL Pod 基线外推为目标集群每节点并发或正式容量达标。

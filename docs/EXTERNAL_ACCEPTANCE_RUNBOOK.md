# 外部测试环境低次数验收 Runbook

目标：把有限的真实企业微信、Telegram、Kubernetes 和 KMS/Vault 测试次数用于不可被本地模拟替代的证据。本文中的“次数预算”是项目自己的熔断上限，不是供应商官方限额。

## 1. 总体顺序

```text
离线门禁 → 目标基础设施 → 公开回调空探针 → Telegram → 企业微信 → KMS/Vault 轮换 → 故障/灾备 → 正式容量
```

先用 Telegram 验证通用 Gateway→Inbox→Consumer→Worker→Outbox→Delivery 链路，再使用更稀缺的企业微信配置保存/回调机会。若前一阶段失败，停止后续真实 Provider 调用，不通过“多试几次”掩盖确定性错误。

## 2. 资源与次数预算

| 阶段 | 额外基础设施占用 | Provider/破坏性次数上限 | 成功标准 |
|---|---:|---:|---|
| 本包离线门禁 | 本机 CPU；无外部账号 | 0 | build/vet/unit/race/integration/releaseverify 全绿 |
| 目标 K8s 首发 | 3 节点 × 约 2 小时 | 1 次 compatible rollout | 所有 workload Ready，migration 成功，HPA 指标有效 |
| 目标 K8s 回滚 | 同上 | 1 次 canary + 1 次 rollback | session 绑定不漂移、旧 digest 恢复、无消息丢失 |
| 公网 callback 空探针 | 域名/证书/Ingress | 0 个 Provider 调用 | HTTPS 证书有效，`/webhook` 无 token 返回预期 4xx |
| Telegram 注册 | 现有 bot | `setWebhook` 最多 2 次；`getWebhookInfo` 最多 2 次 | secret token 生效、无 last error |
| Telegram 闭环 | 1 个 private chat，可选 1 个测试群 | 入站 2 条、出站最多 2 次 | Inbox/Outbox/审计/trace 一致，群/单聊 session 隔离 |
| 企业微信 URL 验证 | 1 个测试应用 | 控制台保存最多 2 次 | echostr 验签、解密、原文回复成功 |
| 企业微信闭环 | 1 个白名单测试用户 | 入站 2 条、出站最多 2 次 | 加密文本入站、token 获取、应用消息回复完整 |
| KMS/Vault identity | 目标集群 2 个短命 Pod | 正向 1、越权负向 1 | 正向最小路径成功，错误 SA 被拒绝 |
| 双 key 轮换 | 2 个测试 key version | 1 次切换、1 次回滚窗口验证 | 旧密文可解、新写只用新 key、日志无 key/value |
| DB/Redis 故障 | 测试实例约 2 小时 | 每类故障 1 次 | fail-closed、恢复后 backlog 清空、无 stale commit |
| 正式容量 | 目标规格约 2–4 小时 | warm-up 1、测量 2 | p50/p95/p99、QPS、queue lag、成本和错误率齐全 |

如果账号按调用或资源小时计费，应在执行前由账号管理员把本表换算为实际价格；不要把这里的资源时间当供应商报价。

## 3. 零调用 Preflight

Windows 本地企微沙箱已有一条不覆盖 V13、且不把秘密写入源码的固定流程：

```powershell
# 1. 启动固定 digest 的临时 HTTPS Quick Tunnel，得到 .../webhook 基础 URL
& .\scripts\wecom_sandbox_tunnel.ps1

# 2. 交互采集 CorpID/AgentID/UserID/Secret/Token/AES；输入隐藏，文件 ACL 收紧
& .\scripts\wecom_sandbox_setup.ps1

# 3. 在独立端口启动 trpc-v14-wecom Compose 项目，创建租户、发布版本并复检公网入口
& .\scripts\wecom_sandbox_bootstrap.ps1
```

前两步不会调用企业微信 API 或模型 Provider；第三步会构建/启动本地服务并写本地控制面，但仍不发送模型请求。临时 Tunnel 只用于验收，URL 会随容器重建变化，不能当生产域名。三个脚本把真实值留在被 gitignore 排除的 `deploy/.env.wecom.local`；归档前必须再次确认该文件未被收集。V13 默认端口仍可保持运行，企微沙箱使用 15432/14317/14318/18080/18081/19095/13000。

把真实凭据通过临时进程环境或 Secret Manager 注入；不要写入 `.env`、脚本参数、PowerShell 历史、CI 日志或截图。需要的变量名：

```text
TRPC_WEBHOOK_ROUTE_KEY
TRPC_SECRET_TELEGRAM_BOT_TOKEN
TRPC_SECRET_TELEGRAM_WEBHOOK
TRPC_SECRET_WECOM_TOKEN
TRPC_SECRET_WECOM_CORP_SECRET
TRPC_SECRET_WECOM_AES
WECOM_CORP_ID
WECOM_AGENT_ID
```

只做格式、公开 URL 和可选 TLS 探针，不调用 Telegram/企业微信：

```powershell
& .\scripts\external_acceptance_preflight.ps1 `
  -Channel All `
  -CallbackBaseUrl 'https://agent-test.example.com/webhook' `
  -ProbeEndpoint
```

成功输出只包含检查数量和 HTTP 状态，不包含任何 secret。完整 Provider callback URL 是基础 URL加 `?token=<TRPC_WEBHOOK_ROUTE_KEY>`；只在 Provider 控制台通过密码管理器组装/粘贴，不在命令行打印。

Windows setup 脚本会在结束时把完整 URL 临时写入剪贴板；粘贴到企业微信控制台后立即清空剪贴板。控制台保存必须等 `wecom_sandbox_bootstrap.ps1` 的 Gateway/Admin health 和公网 preflight 全部通过，否则不要消耗 URL 验证次数。

Preflight 失败时真实调用预算仍为 0，必须先修复 DNS、证书、Ingress、变量缺失或凭据格式。

## 4. 目标 Kubernetes 一次性准备

1. 锁定本次 release bundle、七个应用镜像 digest、migration schema class 和 NetworkPolicy review hash。
2. 先运行 `releaseverify`；任何 tag 镜像、缺失 4143 mesh 路径、未证明的 mesh assertion 或 breaking migration 均停止。
3. 使用 `scripts/k8s_apply.sh`，顺序固定为：NetworkPolicy→migration→PDB→Worker/Admin→Consumer/Delivery→Gateway。
4. 记录每个 Deployment 的 generation、revision、imageID、Ready、restart count、node 分布和 Linkerd identity。
5. 运行同请求 allow/deny：有 identity 的 client 必须到达应用鉴权，无 identity client 必须被 mesh 拒绝。
6. HPA 必须显示 `ScalingActive=True`；使用 `ContainerResource`，不接受 `<unknown>`。

阻断条件：migration 失败、任一 Pod restart 增长、旧/新 digest 混跑超时、Linkerd identity 缺失、NetworkPolicy 需要临时全放通、HPA 指标未知。

## 5. Telegram（先执行）

### 5.1 注册

1. 只调用一次 `setWebhook`，同时设置 HTTPS callback 和 `secret_token`。
2. 调用一次 `getWebhookInfo`，保存脱敏结果：URL host/path、pending count、last error code/time；不得保存 bot token 或 route key。
3. 若失败，先按确定性错误修复；第二次 `setWebhook` 是本阶段最后预算。

### 5.2 闭环用例

| 用例 | 操作 | 断言 |
|---|---|---|
| T-01 private | 白名单用户发送唯一短文本 | callback 2xx；1 Inbox、1 execution、1 Outbox；reply 成功 |
| T-02 group | 测试群白名单用户发送唯一短文本 | group session 与 private session 不同；actor/owner 映射正确 |
| T-03 duplicate | 对已捕获的同一脱敏 fixture 在内部测试入口重放，不再次发 Provider 消息 | Inbox 数量不增；payload hash 相同返回幂等成功 |
| T-04 bad secret | 内部测试入口发送错误 webhook secret，不触发 Provider | 401/拒绝；无 Inbox、无 tenant 泄漏 |

不要为了制造 429 对真实 Bot 高频轰炸；429、Retry-After 和 outcome-unknown 已由本地 contract/fault 测试覆盖。真实测试只确认常规限速头/错误能被脱敏记录。

## 6. 企业微信（Telegram 通过后执行）

### 6.1 URL challenge

1. 检查服务器时钟偏差小于 60 秒；代码接受窗口为 ±300 秒，但不要把上限当正常运维目标。
2. 在控制台一次保存 callback URL、callback token 和 EncodingAESKey。
3. 保存动作必须完成 `msg_signature` 校验、echostr 解密、receiver corp ID 校验并返回原文。
4. 若失败，只查看稳定错误类和脱敏 trace；先本地复现，再使用最多一次重试保存。

### 6.2 闭环用例

| 用例 | 操作 | 断言 |
|---|---|---|
| W-01 text | 白名单用户向测试应用发送唯一文本 | 加密 callback 验签/解密；1 Inbox；Agent 完成；应用消息回复 |
| W-02 duplicate | 使用保存的脱敏原始密文在受控内部入口重放 | 相同 MsgId 不产生第二次 execution/outbox |
| W-03 unsupported | 发送一个已允许的非文本测试事件 | callback 被确认但不调用 Agent，不制造 Provider 重试风暴 |
| W-04 unauthorized | 非白名单测试身份发消息 | governance 拒绝；审计 decision 存在；无工具/模型调用 |

只保留 MsgId、trace_id、状态、延迟和 hash；不得保留聊天正文、access token、corp secret、EncodingAESKey 或解密后的原始 XML。

## 7. KMS/Vault 生产验收

只选择目标环境实际采用的一种方案，不为“覆盖名词”同时开通多家 KMS。

1. workload identity 绑定到专用 ServiceAccount，禁止 node-wide/static access key。
2. 正向 Pod 只读取本服务/测试租户的一条 secret；默认或其他 ServiceAccount 读取同路径必须被拒绝。
3. 创建 key version N+1；读取 key ring 时同时保留 N/N+1 解密能力，新加密只用 N+1。
4. 滚动 Gateway/Admin/Worker/Consumer/Delivery，观察无解密错误、无 Pod restart。
5. 用 N 生成的既有密文仍可解；新写记录标记 N+1；完成观察窗口后再禁用 N 的加密，不立即销毁。
6. 搜索 log/trace/error report，确认没有 key ID 之外的密钥材料、token、DSN 或 secret value。

阻断条件：需要静态云 access key、权限范围为整库/整项目、负向身份可读、轮换必须停机、旧密文不可解、任何 secret 出现在 telemetry。

## 8. 故障、回滚与容量

- PostgreSQL：一次短暂不可用或 primary failover；Gateway 不在未提交 Inbox 时回 2xx，恢复后 backlog 可推进。
- Redis：一次稳定端点切换；Worker 不降级为本地锁，session coordination 缺失时 fail-closed。
- Worker：中断一个正在处理的副本；新副本以更高 fence 接管，旧副本迟到写被拒绝，只生成一个 Outbox。
- Delivery：在 provider 调用边界制造一次 outcome-unknown；记录必须进入 reconciliation，不自动重复发送。
- Rollback：只回滚兼容应用版本；breaking schema 使用独立停机/排空 runbook。
- Capacity：warm-up 不计入报告；两次测量取较差值，以业务 payload 和模型延迟运行，不能复用本地 2,200 条数字作为正式结论。

## 9. 每次测试必须保存的证据

```text
release digest / schema class / config version
UTC start/end / operator / environment
tenant_id（测试租户）/ channel / hashed user / session_id
provider message ID 或其 hash / inbox ID / execution ID / outbox ID
trace_id / decision / error_type / latency / token/cost（如有）
Pod imageID / restart count / node / mesh identity
DB/Redis QPS、pool wait、queue lag、p50/p95/p99
预期、实际、PASS/FAIL、是否消耗一次外部预算
```

证据包必须经过 secret scan；聊天正文、Authorization、Cookie、Bot token、WeCom token/secret/AES key、数据库 URL、Vault token、Kubernetes projected JWT 和证书私钥均禁止收集。

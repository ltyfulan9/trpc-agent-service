# V13 安全审计报告

> 历史归档：当前权威安全审计为 `SECURITY_REVIEW_V14.md`。

## 范围与结论

范围包括 Go 代码、CI/容器工具链、IM Adapter、内部服务认证、租户隔离、密钥边界、部署模板和集成测试真实性。审计未发现源码中可直接利用的 Critical 漏洞；本轮修复 1 个 High 级验证门禁缺陷和 1 个 Medium 级证据缺陷。仍有 4 个必须由生产环境关闭的开放项。严重度反映潜在生产影响，不等于当前已有攻击已发生。

## Findings

### SEC-001 — High — FIXED — 本地生产验证未阻断含已知漏洞的 Go 补丁版本

- 位置：`scripts/validate.sh:15-16`、`scripts/require_secure_go.sh:1-32`、`.github/workflows/verify.yml:39-45,74-79`
- 证据：宿主 Go 1.26.2 下 `govulncheck` 报出 14 条可达标准库漏洞；同一源码切换到 Go 1.26.7 后返回 `No vulnerabilities found`。CI 和生产镜像此前已固定 1.26.7，但本地 `validate.sh` 没有拒绝旧版本，可能产生虚假的“全门禁通过”。
- 影响：评审或运维可能用不安全工具链构建发布物，并把旧环境结果当作当前安全证据。
- 修复：新增稳定版本解析和 `>= go1.26.7` fail-closed 门；CI vulnerability job 与主验证 job 都执行该门。Go 1.25.14 只保留独立源码兼容测试。
- 缓解/误报说明：已有 digest-pinned 1.26.7 容器降低了生产镜像风险；本项针对本地/旁路构建可信度，不代表 V12 生产镜像必然使用 1.26.2。

### SEC-002 — Medium — FIXED — “Redis 集成”实际使用内存模拟器

- 位置：`test/integration/datamigration_test.go:16-36,39-87`、`scripts/validate.sh:56-86`、`.github/workflows/verify.yml:31-33`
- 证据：旧 integration tag 启动 `miniredis`，即使 CI 配置 Redis service 也没有连接；无法覆盖真实 Redis 协议、连接和持久化行为。
- 影响：迁移切片可能在模拟器通过、真实 Redis 失败，造成错误验收和迁移风险。
- 修复：integration 强制要求 `TEST_REDIS_URL` 并 `PING` 真实服务；使用测试唯一 prefix 且只清理该 prefix，不 `FLUSHDB`；validate 同时启动隔离 Redis/PostgreSQL。
- 缓解/误报说明：这不是入侵漏洞，而是安全与可靠性保证缺口；原有单元测试仍有价值，但不能被描述成真实基础设施证据。

### SEC-003 — High — OPEN/EXTERNAL — Worker 本身不终止 TLS

- 位置：`cmd/worker/main.go:706-716`、`cmd/consumer/main.go:314-357`、`pkg/releaseverify/verify.go:243-263`、`deploy/kubernetes/README.md:24-42`
- 证据：Worker 使用普通 `http.Server`；Consumer production 模式只接受 HTTPS，mesh 模式依赖运维断言和 evidence annotation。本地三节点 K3d/Linkerd 已证明有 identity 的同请求到达应用 401、无 identity 请求被 mesh 403 拒绝，并修复透明代理 4143 NetworkPolicy；但使用的是为 K8s 1.36 选择的 edge/CNI 组合，不能证明目标生产集群已部署受支持的严格双向认证。
- 影响：错误 overlay 或伪造 mesh 配置会暴露内部 Agent 执行入口；HMAC 防篡改但不能提供传输保密。
- 修复：目标环境必须使用 HTTPS terminator 或 strict mTLS mesh，验证 peer identity、authorization、NetworkPolicy 和证书轮换；保留 `releaseverify` 作为发布阻断门。
- 缓解：body-bound HMAC、nonce、防重放和 default-deny NetworkPolicy 提供纵深防御。

### SEC-004 — Medium — OPEN — Bootstrap/静态 Admin bearer 不具备短期发行语义

- 位置：`cmd/admin/main.go:58-60`、`pkg/adminauth/auth.go:120-229`、`README.md:133`
- 证据：token 以 SHA-256 后常量时间匹配，支持角色和 tenant allowlist，但本包不校验 token expiry、issuer、audience 或设备/会话上下文。
- 影响：长期 token 泄漏后的可用窗口过大。
- 修复：生产以 OIDC/IAP/mTLS 身份发行短期 principal，服务端继续执行现有 permission 和 tenant scope；bootstrap token 仅限 break-glass、私网、短期轮换并告警。
- 缓解：凭据不明文存储在 Authenticator、重复 Authorization header 被拒绝、审计 actor 不信任请求头。

### SEC-005 — Medium — OPEN — SecretRef 是安全 seam，不是外部 KMS/Vault 实现

- 位置：`pkg/tenant/secrets.go:18-101`、`pkg/tenant/keyring.go:44-165`、`README.md:92,185`
- 证据：内置 resolver 只允许 `env://TRPC_SECRET_*` 且错误脱敏；Kubernetes Secret 环境注入仍由目标环境负责。本地官方 Vault dev-mode 已完成 dedicated ServiceAccount 正向读取和 default ServiceAccount 越权拒绝，证明 JWT→role→policy seam 可行；它没有 HA、auto-unseal、正式审计存储或双 key 轮换，故本 finding 仍为 OPEN。
- 影响：若直接把长期密钥放入普通环境变量，进程权限、诊断包或节点管理员可能读取；无法证明轮换与最小权限。
- 修复：实现 workload-identity KMS/Vault resolver，按服务和租户最小授权，记录 key version，演练双 key 解密/单 key 加密轮换。
- 缓解：AES-GCM envelope 的 AAD 绑定 tenant/字段；Admin 返回遮盖值；inline/ref 冲突 fail-closed。

### SEC-006 — Medium — OPEN/EXTERNAL — 缺少真实 IM provider sandbox 合约证据

- 位置：`pkg/channel/wework_adapter.go`、`pkg/channel/telegram_adapter.go`、对应测试文件
- 证据：验签、加解密、重试、长度、错误脱敏都有单元/HTTP fake 测试，但没有使用真实企业微信或 Telegram sandbox 账号完成 callback 与外发闭环。
- 影响：供应商字段、频控、证书链或 token 生命周期差异可能只在上线后暴露。
- 修复：建立最小权限 sandbox 账号，录制不含凭据的 contract fixtures；每日 canary 验证 callback、token refresh、429、文件和撤回事件。
- 缓解：Adapter 边界严格、未知事件显式 ACK/ignore、provider 错误归一化且不传播 credential URL。

### SEC-007 — Low — ACCEPTED/EXTERNAL — WeCom 凭据按供应商协议出现在 HTTPS query

- 位置：`pkg/channel/wework_adapter.go:443,548-558,668-675`、`pkg/channel/provider_http.go:83-96`
- 证据：企业微信 access token/corpsecret 按 API 约定放入 query；代码固定 HTTPS origin、拒绝跨 origin/端口/userinfo redirect，并把 transport/provider error 转为无凭据稳定错误。
- 影响：若企业出口代理记录完整 query，凭据仍可能进入代理日志。
- 处理：目标 egress proxy 必须关闭或脱敏 query 日志，限制 `qyapi.weixin.qq.com:443`，审计代理配置与抓包证据。
- 误报说明：不能在客户端单方面改变供应商 API 认证格式；当前代码已关闭可控的 redirect/error 泄漏路径。

## 正向控制确认

- HTTP request body、JSON 深度、文本、附件、响应和错误长度均有界；未发现无界 `io.ReadAll` 入口。
- SQL 使用参数化查询；未发现把租户输入拼接成 SQL、shell 或动态文件路径。
- webhook secret/HMAC 使用常量时间比较；nonce 跨节点消费；重复 header fail-closed。
- 自定义模型 endpoint 默认禁止；附件 URL 拒绝回环、私网、link-local 和 userinfo，降低 SSRF。
- 审计结构不含原始 prompt/response/credential 字段；高基数指标有 allowlist。
- 依赖、GitHub Actions、生产构建和运行镜像均固定版本/digest；仍需目标流水线生成 SBOM、签名和 provenance。
- 公平队列最终 PostgreSQL `UPDATE` 现在重检当前状态与租约时间，避免陈旧 CTE candidate 把终态 Inbox 复活；红/绿回归、重建 digest、两副本滚动和 2,200 条 durability 对账均已完成。

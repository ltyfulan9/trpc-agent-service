# 两个版本的融合取舍

## 基线选择

本包以较小且模块关系更连贯的版本为基线。较大的版本只作为设计素材库，不整包覆盖。原因是后者包含若干“源码存在但没有生产入口调用”的模块、重复状态所有者和与运行 SQL 不一致的声明。代码量与文档量不作为合并优先级。

## 吸收并重建的长处

- 治理 Plugin：采用 Runner 生命周期的真实 BeforeTool/AfterTool 拦截，删除未进入 Runner 路径且会与审批/审计语义漂移的静态 wrapper；补上输入内容策略和最终模型输出脱敏。
- Agent 控制面：App/Version/Deployment、稳定灰度、不可变无密钥快照、回滚；进一步增加幂等请求版本固定和事务控制面审计。
- 可靠性思想：Inbox/Outbox、`SKIP LOCKED`、重试、DLQ、接管；统一重建为一套 schema、一套状态机和一条生产入口链路。
- 投递恢复：补上 provider 错误分类、429 Retry-After 与分段 delivery cursor，避免已知后段失败把所有前段从头发送。
- 运维设计：SLO、崩溃点、容量模型、部署清单；仅保留可测量定义，不把未执行的 benchmark 写成结果。
- 安全模块：HMAC、nonce、secret manager、SSRF 思路；实际接入 Consumer→Worker 和租户密钥路径，未接入的能力不宣称完成。
- 法医收口：生产拒绝 InMemory、多层遮盖占位符 fail-closed、Tenant/Agent 同事务审计、逐工具审计、SQL 审计超时和有界连接池。

## 明确拒绝带入的内容

- 同时存在多套 Inbox/Outbox、session/event/memory 表或状态机。
- 只锁 `AppendEvent`、先写 Redis commit marker 再写数据库却声称“Session exactly-once”的实现；本包改为锁完整 invocation，并保留真实一致性边界说明。
- `cmd` 入口带 TODO、只构造对象但不启动消费循环、或默认走非持久化路径。
- 独立存在但生产 composition root 没有调用的 replay、cache、tracker、RBAC、migration 模块。
- 信任可伪造用户名请求头的“RBAC”；当前 Actor 仅作审计，生产认证边界被明确写出。
- “所有测试通过”“生产就绪”“Top 2”等没有命令、退出码、基础设施和原始结果支撑的结论。
- 用文件数、行数、测试文件数量代替功能闭环证据。

## 当前统一主链路

```text
IM → Gateway → PostgreSQL Inbox → Consumer → HMAC Worker
   → pinned Agent Version → shared Session/Memory → Governance
   → invocation result → atomic Inbox complete + Outbox
   → Delivery → IM
```

任何新增功能必须能指出它在这条链路或控制面事务中的调用位置、状态所有者、失败语义和验证命令；否则只能标记为设计或候选模块。

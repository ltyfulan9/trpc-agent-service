# Enterprise Multi-Tenant Agent Platform Handoff

当前源码目录：本仓库根目录
当前最终交接：`ENTERPRISE_PLATFORM_HANDOFF.md`

平台已把检查点中列出的三项主缺口接入生产 composition root：Summary Generator 与下一轮 Runner history overlay、Qdrant Knowledge 数据面、PostgreSQL+S3/MinIO Artifact 数据面；并加入向量/对象 migration projection、独立 summary-worker、租户级 runtime profile/Secret 隔离、Compose/Kubernetes 资源与告警。

评审入口：

1. `docs/COMPETITION_SUBMISSION.md`
2. `docs/ACCEPTANCE_EVIDENCE.md`
3. `docs/DATA_MODEL.md`
4. `docs/RISK_REGISTER.md`
5. `docs/SECURITY_REVIEW.md`
6. `ENTERPRISE_PLATFORM_HANDOFF.md`

本地通过不等于目标生产验收。真实 IM sandbox、正式 Kubernetes/service-mesh、KMS/Vault、OTLP TLS、HA 故障注入、容量和灾备仍按 `docs/EXTERNAL_ACCEPTANCE_RUNBOOK.md` 执行。

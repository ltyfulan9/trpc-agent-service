# Enterprise Multi-Tenant Agent Platform Handoff

当前源码目录：`work/trpc-agent-enterprise-v14`  
当前最终交接：`TRPC_AGENT_ENTERPRISE_HANDOFF_V14_FINAL.md`

平台已把检查点中列出的三项主缺口接入生产 composition root：Summary Generator 与下一轮 Runner history overlay、Qdrant Knowledge 数据面、PostgreSQL+S3/MinIO Artifact 数据面；并加入向量/对象 migration projection、独立 summary-worker、租户级 runtime profile/Secret 隔离、Compose/Kubernetes 资源与告警。

评审入口：

1. `docs/COMPETITION_SUBMISSION_V14.md`
2. `docs/ACCEPTANCE_EVIDENCE_V14.md`
3. `docs/DATA_MODEL.md`
4. `docs/RISK_REGISTER_V14.md`
5. `docs/SECURITY_REVIEW_V14.md`
6. `TRPC_AGENT_ENTERPRISE_HANDOFF_V14_FINAL.md`

本地通过不等于目标生产验收。真实 IM sandbox、正式 Kubernetes/service-mesh、KMS/Vault、OTLP TLS、HA 故障注入、容量和灾备仍按 `docs/EXTERNAL_ACCEPTANCE_RUNBOOK.md` 执行。

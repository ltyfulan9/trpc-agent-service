# 验证指南

本指南适用于 Enterprise Multi-Tenant Agent Platform 的源码包和仓库。功能断言见 [验收证据](ACCEPTANCE_EVIDENCE.md)，目标部署见 [验收 Runbook](EXTERNAL_ACCEPTANCE_RUNBOOK.md)。所有命令在源码根目录执行。

## 1. 环境

模块兼容基线为 Go 1.25.14，构建与安全门禁使用 Go 1.26.7。完整验证还需要 Bash、Docker 和 Docker Compose。解压源码不包含 Git 元数据，Go 命令使用 `-buildvcs=false`。

## 2. 提交包源码验证

```powershell
go test -buildvcs=false -count=1 -p 1 ./...
go vet -p 1 ./...
go test -buildvcs=false -race -count=1 -p 1 ./...
go run -buildvcs=false ./cmd/demo
```

交付包 `verification-evidence/current-validation.log` 保存上述命令的工具链、时间、输出和退出码。常规测试包含测试内启动的本地 HTTP/MCP 服务；`cmd/demo` 使用 `MemoryStore` 展示租约接管、旧 fence 拒绝、Inbox/Outbox 状态转换与未知发送结果处理。

## 3. 完整源码门禁

```bash
go version
go mod verify
test -z "$(gofmt -l cmd pkg migrations test)"
go build -buildvcs=false -p 1 ./cmd/...
go vet -p 1 ./...
go test -buildvcs=false -count=1 -p 1 ./...
go test -buildvcs=false -race -count=1 -p 1 ./...
bash ./scripts/static_verify.sh
```

race 检查需要当前平台支持的 C 工具链。每个门禁分别记录退出码；CI 配置位于 [verify.yml](../.github/workflows/verify.yml)。

## 4. 后端集成与部署检查

使用 Docker 启动测试所需的 PostgreSQL、Redis、Qdrant 和 MinIO，按测试配置提供连接参数，然后执行：

```bash
go test -buildvcs=false -tags=integration -count=1 -p 1 ./test/integration
```

集成测试覆盖队列持久化、Session 迁移、Summary→Runner、Knowledge、Artifact 和数据投影，模型及 embedding 使用本地协议服务。具体环境变量与自动启动流程以 [validate.sh](../scripts/validate.sh) 为准；该脚本同时汇总源码检查、后端集成、镜像构建和 Prometheus 规则检查：

```bash
bash ./scripts/validate.sh
```

在同一测试环境下，可以单独验证租户级交叉后端组合：

```bash
go test -buildvcs=false -tags=integration -count=1 \
  -run '^TestCrossBackendStorageAdapterSharesDataAcrossNodesAndIsolatesScopes$' ./test/integration
```

该测试使用真实 Redis/PostgreSQL、两个独立生产存储适配器和 PostgreSQL execution fence。两租户分别使用 Redis Session/PostgreSQL Memory 与相反组合，检查跨客户端可见性、tenant/app/actor 隔离、未选择后端无数据以及租约释放后的资源关闭。

Windows 可使用隔离环境脚本启动应用栈：

```powershell
.\scripts\run_c_local_stack.ps1 -ProjectName agent-platform-local -Build
.\scripts\run_c_local_stack.ps1 -ProjectName agent-platform-local -Down
```

脚本按自身位置定位源码，使用独立端口和一次性测试配置。启动后检查 migration 退出码、服务 health、Prometheus targets/rules 和容器 restart count。正式环境的密码与 Provider 凭据由部署方注入。

## 5. 演示与基准

- [故障恢复演示](DEMO.md)：无需账号，观察内存状态机的接管与结果核对行为。
- [本地可靠性基准](BENCHMARK.md)：测量 `MemoryStore` 操作耗时与分配，用于同条件下的实现回归比较。
- [SLI/SLO](SLO.md)：定义目标部署中的队列延迟、处理成功率、错误预算与告警处置。

## 6. 结果记录与打包

结果记录包含源码快照、Go 版本、环境、开始/结束时间、命令、退出码和必要后状态。源码测试、真实后端、部署与外部账号验收分别标注运行范围。

[打包脚本](../scripts/package_all_materials.ps1) 生成文件清单和 SHA-256，检查新目录解包结果及秘密签名。归档包含源码、当前文档和验证证据，排除真实环境文件、运行时数据库、凭据、缓存和构建产物。交付清单中的源码身份用于对应仓库和压缩包。

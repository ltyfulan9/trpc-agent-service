# Static verification record — 2026-08-25

> Historical record: this document describes the pre-security-baseline Go 1.21
> validation. The current module requires Go 1.25 and production builds are
> pinned to Go 1.26.7; see `docs/VERIFICATION.md` for the current evidence.

## Reproduced in the packaging environment

```text
PowerShell equivalent of static_verify.sh
PowerShell migration/TODO/credential checks
PyYAML safe_load_all over deploy/**/*.yml and deploy/**/*.yaml
delimiter balance scan over cmd/pkg/migrations/test Go sources
up/down migration pair check
production TODO/FIXME and local go.mod replace scan
obvious credential/private-key pattern scan
```

Observed schema inventory after this hardening round: 29 paired schema migrations, including durable tool approvals and the additive 029 approval invariant. Deployment/Kubernetes Go tests and Summary/migration field/invariant assertions also passed. These inventory facts are not used as proof of runtime correctness.

The current Windows shell exposes `C:\Windows\System32\bash.exe` as a WSL
forwarder, but no WSL `/bin/bash`; therefore the Bash wrapper was not rerun in
this pass. Its file/policy/TODO/migration assertions were reproduced in
PowerShell and passed. This is an environment limitation, not a claim that
shell syntax was revalidated here.

## API facts checked against the pinned framework

The repository pins `trpc-agent-go v1.11.2`. Its Runner exposes both `WithSessionService` and `WithMemoryService`, passes the memory service into Invocation/MemoryReader, and schedules auto-memory work. LLMAgent exposes bounded `WithPreloadMemory`. The production Worker is wired to those real APIs rather than a locally invented wrapper:

- [Runner v1.11.2 source](https://raw.githubusercontent.com/trpc-group/trpc-agent-go/v1.11.2/runner/runner.go)
- [LLMAgent options v1.11.2 source](https://raw.githubusercontent.com/trpc-group/trpc-agent-go/v1.11.2/agent/llmagent/option.go)

## Go verification executed after static review

Go 1.21.13 was used with `GOMAXPROCS=1` and `-p 1` to keep local load bounded. `go mod verify`, `gofmt -l`, `go build -buildvcs=false -p 1 ./...`, `go vet -p 1 ./...`, `go test -count=1 -p 1 ./...`, `go test -race -count=1 -p 1 ./...` and the integration-tag compile-only `go test -tags=integration -run '^$' -count=1 -p 1 ./test/integration` completed successfully after the Summary and migration PostgreSQL fence changes. Go tests parse the deployment configuration and Kubernetes YAML and assert the default-deny plus service-boundary policies. This proves only module/source/unit/race/static-deployment behavior.

No infrastructure service was started in this low-load verification round, so no image build, Compose start, PostgreSQL integration execution, migration execution, `promtool`, service health check, real IM contract test, `govulncheck` network scan or load benchmark was run. `scripts/validate.sh` treats missing Go or Docker as an error and performs the infrastructure gates on the target machine; CI separately configures the pinned vulnerability scanner.

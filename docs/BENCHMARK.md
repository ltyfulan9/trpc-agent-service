# Local Reliability Benchmark

This benchmark is a deterministic, provider-free baseline for the durable
Inbox/Outbox state machine. It measures the complete in-memory path:

```text
Enqueue Inbox -> Claim with lease/fence -> Complete Inbox + create Outbox
```

Run it from the repository root:

```powershell
.\scripts\benchmark_local.ps1 -Count 5
```

The output is standard Go benchmark output. Record the machine, Go version,
commit, `ns/op`, `B/op`, and `allocs/op` together; these values are useful for
regression comparison only. They are not a production capacity or model
latency claim. Production capacity still requires PostgreSQL/Redis, real
payloads, provider limits, and failure-injection runs.

The benchmark intentionally exercises the same durable transition contract as
the unit tests. A change that weakens idempotency, lease ownership, FIFO
partitioning, or atomic Inbox-to-Outbox completion should either fail the
benchmark setup or be caught by the corresponding contract tests in
`pkg/reliable`.


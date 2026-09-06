# Local Failure-Recovery Demo

The demo is a provider-free, local proof of the platform's most important
reliability invariants. It runs the real reliable-store state machine and
prints JSON events for:

```text
durable Inbox -> lease takeover -> stale fence rejection
-> atomic Inbox/Outbox completion -> dispatch fence
-> unknown provider outcome -> reconciliation-required state
```

Run it from the repository root:

```powershell
go run ./cmd/demo
```

The demo does not claim a real model, IM provider, PostgreSQL/Redis HA, or
production capacity. Its purpose is to make the failure semantics observable
and repeatable during review without requiring credentials or a public URL.


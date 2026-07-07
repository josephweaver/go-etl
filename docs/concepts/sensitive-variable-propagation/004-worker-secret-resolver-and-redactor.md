# 004 Worker Secret Resolver and Redactor

Status: proposed

## Objective

Add the worker-side primitive that resolves protected references into typed sensitive values only inside the worker execution boundary, and add an attempt-local redactor that can scrub exact materialized secrets from controlled outputs.

This slice does not yet wire secrets into every work-item handler or Python subprocess. It creates the worker primitives those slices will use.

## Current State

After slice 003, the worker can receive protected references in an execution envelope, but it has no standard interface for resolving them, no typed in-memory `SensitiveValue`, and no attempt-local redactor.

## Target State

The worker has a protected-value resolver interface similar to:

```go
type ProtectedValueResolver interface {
    ResolveProtectedValue(ctx context.Context, ref ProtectedValueRef) (SensitiveValue, error)
}
```

The first real provider is `worker_env`:

```text
worker_env:GOET_GDRIVE_TOKEN
```

The worker reads the named key from its own environment when and only when the assigned operation requires it.

The worker has a `SensitiveValue` abstraction whose default string/JSON/error formatting is redacted.

The worker has an attempt-local redactor that can register materialized plaintext values and scrub exact occurrences from controlled strings/byte streams.

## Concept Decision

The worker is the first phase-1 plaintext boundary for execution secrets.

Resolution timing:

```text
assignment received -> inspect required variables -> resolve protected refs for this operation -> run handler/subprocess -> cleanup
```

Do not enumerate the worker environment. Resolve only the explicit protected refs in the assignment.

The redactor registry is attempt-local and in-memory. It must not be persisted.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/sensitive-variable-propagation/README.md`
- `internal/model/work_item.go`
- model file added by slice 003 for execution envelopes/protected refs
- `cmd/worker/main.go`
- `cmd/worker/work_*.go`
- `cmd/worker/*test.go`

Search for worker dispatch and logging/status paths before adding new abstractions.

## Allowed Production Files

Expected files:

- `cmd/worker/protected_value.go`
- `cmd/worker/redactor.go`
- `cmd/worker/work_context.go` if needed
- existing worker dispatch file if needed for wiring a context object

If protected-value types are shared with controller payloads, keep shared structs in `internal/model` and worker-only plaintext helpers in `cmd/worker`.

## Allowed Test Files

- `cmd/worker/protected_value_test.go`
- `cmd/worker/redactor_test.go`
- worker dispatch/context tests if a context object is added

## Out Of Scope

- Controller compilation changes.
- Python-specific materialization.
- Go work-item handler migration.
- End-to-end smoke.
- External secret manager providers.
- Scanning artifact files.

## Acceptance Criteria

- Worker can resolve `worker_env` protected refs from its own environment.
- Missing required worker env key produces a sanitized error.
- Unsupported provider produces a sanitized error.
- `SensitiveValue` default string formatting is redacted.
- `SensitiveValue` JSON formatting is redacted or forbidden.
- Plaintext access requires an explicit method call.
- Attempt-local redactor replaces exact materialized secret strings with a redacted label.
- Redactor treats secrets literally, not as regular expressions.
- Redactor does not persist its registry.
- Redactor handles multiple registered secrets deterministically.
- `go test ./cmd/worker` or narrower worker tests pass.

## Notes

Use this slice to keep the sensitive-value mechanics boring and testable. Do not let Python subprocess details enter this slice.

# 005 Trusted Go WorkItem Sensitive Context

Status: proposed

## Objective

Update trusted in-process Go work-item handlers so they receive public values and typed sensitive values through a worker operation context rather than through an unstructured parameter map.

In this concept, "Go plugin" means a trusted in-process worker operation/handler invoked by the Go worker. It does not require Go's dynamic `plugin` package.

## Current State

Worker operation handlers can receive work-item parameters, but there is no standard sensitive-aware context separating public resolved values from sensitive in-memory values.

After slice 004, the worker can resolve protected refs and create `SensitiveValue` instances, but those values are not yet integrated into handler dispatch.

## Target State

Worker dispatch constructs an operation context similar to:

```go
type OperationContext struct {
    WorkItem  model.WorkItem
    Public    map[string]ResolvedValue
    Sensitive map[string]SensitiveValue
    Redactor  *Redactor
    Logger    SafeLogger
}
```

Trusted Go handlers can explicitly request and use sensitive values without command-line exposure:

```go
secret, ok := ctx.Sensitive["gdrive_token"]
plaintext, cleanup, err := secret.Reveal(ctx, "gdrive adapter auth")
```

The exact API may differ, but the use site must make plaintext access visible.

## Concept Decision

Trusted in-process Go operations may receive plaintext sensitive values in memory. This is acceptable because they are part of the trusted worker runtime, but the API should still prevent accidental logging and casual formatting.

Handlers that do not declare or need a sensitive value should not receive it.

The safe logger should redact registered materialized values before logs leave the worker-controlled boundary.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/sensitive-variable-propagation/README.md`
- model files from slices 002-003
- worker files from slice 004
- `cmd/worker/main.go`
- `cmd/worker/work_*.go`
- `cmd/worker/work_python.go` only to avoid breaking Python dispatch, not to implement Python materialization here
- worker tests for existing work-item handlers

Search for:

```text
WorkItem.Type
Parameters
work item dispatch
write_demo_output
logger
```

## Allowed Production Files

Expected files:

- `cmd/worker/work_context.go`
- `cmd/worker/main.go` or dispatch file
- trusted Go work-item handler files that currently consume raw parameters
- `cmd/worker/redactor.go` only for narrow integration

## Allowed Test Files

- worker dispatch/context tests
- tests for one small trusted Go handler receiving a fake sensitive value
- regression tests proving handlers without sensitive requirements do not receive secrets

## Out Of Scope

- Python subprocess materialization.
- External secret stores.
- Controller persistence changes.
- Data asset credential use.
- Smoke tests.
- Arbitrary plugin sandboxing.

## Acceptance Criteria

- Worker dispatch can build a context with public values and sensitive values separated.
- Protected refs are resolved only for the assigned operation's declared needs.
- A trusted Go handler can consume a sensitive value in memory.
- Sensitive values are not converted to command-line arguments.
- Safe logging from the handler redacts registered sensitive plaintext.
- Formatting the operation context does not print plaintext.
- A handler that does not require sensitive values does not receive them.
- Existing worker operations still function.
- Relevant worker tests pass.

## Notes

This slice is the safe internal path. It should be simpler and safer than the subprocess path because the worker controls the code and API.

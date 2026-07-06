# 006 Resource Status And Observability

Status: Ready

## Objective

Expose enough resource-admission state through existing status/log surfaces to explain why ready work is not currently assignable.

## Current State

Dependency-aware status can report dependency stage/step state and assignable-vs-blocked dependency counts. Resource constraints are not visible.

When every queued item is resource-blocked, workers will see no assignable work, but users need a concise way to understand that the workflow is waiting on resource capacity rather than dependency readiness.

## Target State

Submission/controller status can distinguish at least:

```text
queued_resource_eligible_count
queued_resource_blocked_count
running_resource_claim_count
```

Where useful, include compact per-resource summaries:

```text
resource_key
total_units
blocked_candidate_count
```

Human-readable status should remain compact. Example:

```text
resources: 1 eligible, 2 blocked
  target:local/memory-mib running=65536 blocked=2
  ctlr/python-env:torch running=1 blocked=1
```

Logs should emit concise resource-admission observations only at meaningful transitions or debug-level polling boundaries. Avoid log spam from every worker poll.

## Concept Decision

Observability should explain resource blocking without turning status into a full scheduler trace.

Do not expose every queued work item's full resource predicate in default human-readable output. JSON status may include structured detail if bounded and deterministic.

## Required Context

Read these files first:

- `cmd/controller/main.go`
- controller status handler files if split from `main.go`
- `internal/model` status response types
- `internal/client` status rendering files
- `cmd/demo-client` status rendering files
- `docs/concepts/dependency-aware-workflows/011-surface-dependency-state-in-status-and-logs.md`
- `docs/concepts/execution-observability/README.md`
- `docs/concepts/resource-constrained-work-admission/README.md`

## Allowed Production Files

- `internal/model/*status*.go`
- `cmd/controller/*status*.go`
- `internal/client/*status*.go`
- `cmd/demo-client/*status*.go`
- controller log/observation helper files if needed

## Allowed Test Files

- `cmd/controller/*_test.go`
- `internal/client/*_test.go`
- `cmd/demo-client/*_test.go`

## Out Of Scope

- Changing claim behavior.
- Worker-side explanations.
- Full scheduler traces.
- New observability transport.

## Acceptance Criteria

- JSON status exposes resource-eligible and resource-blocked queued counts.
- Human-readable status includes a compact resource line when resource constraints exist.
- Status remains unchanged or minimally changed for workflows with no resource constraints.
- Resource summaries are bounded and deterministic.
- Logs or status can explain the case where work is dependency-ready but resource-blocked.
- No resource status field exposes large payloads, output JSON, or worker logs.
- Existing status/log tests still pass or are intentionally updated.

## Notes

- It is acceptable for the first implementation to expose only counts plus top blocked resource keys.
- Avoid logging on every failed claim poll unless log level is debug and existing logging policy supports that volume.
- Keep the JSON shape forward-compatible; future scheduler policies may add priority or reservations.

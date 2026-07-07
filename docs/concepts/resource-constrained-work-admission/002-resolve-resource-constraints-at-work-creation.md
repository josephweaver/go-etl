# 002 Resolve Resource Constraints At Work Creation

Status: Complete

## Objective

Add workflow/raw declaration support for resource constraints and resolve those declarations before a work item is persisted into queued state.

This slice should produce resolved in-memory resource constraint records. Persistence of those records is handled by the next slice.

## Current State

Workflow steps currently compile fan-out templates into `model.WorkItem` values. The step model has `ID`, optional `FanOut`, and `parallel_with`; the fan-out work-item template owns type, ID/output naming, parameters, and parameter accessors.

Resource constraints are not part of workflow compilation or raw work admission yet.

## Target State

Workflow-authored resource constraints can be declared beside the work-item template and resolved by the controller/workflow compiler before queue mutation.

Example workflow-facing shape:

```json
{
  "resource_constraints": [
    {
      "resource_key": "target:local/memory-mib",
      "requested_units": "${step.memory_allocated_mib}",
      "operator": "<=",
      "target_units": "${controller_config.local_memory_limit_mib}"
    },
    {
      "resource_key": "ctlr/python-env:torch",
      "requested_units": 1,
      "operator": "<=",
      "target_units": 1
    }
  ]
}
```

Resolved in-memory shape:

```text
work_item_id
constraint_index
resource_key
requested_units
operator
target_units
created_at
```

The compiler/admission path should resolve:

- `resource_key` to a non-empty string or path-like string;
- `requested_units` to a positive integer;
- `operator` to one of `=`, `!=`, `<`, `>`, `<=`, `>=`;
- `target_units` to a non-negative integer.

## Concept Decision

Do not put resource constraints into the worker payload as execution metadata unless an existing model boundary makes that unavoidable. Prefer carrying them on `workflow.CompiledWorkItem` or an adjacent admission result so they can be persisted separately from `worker_payload_json`.

If raw `/work` submissions need constraints, prefer a new wrapper type rather than changing the worker payload contract silently:

```json
{
  "work_item": { ... },
  "resource_constraints": [ ... ]
}
```

Preserve backward compatibility for existing raw work-item submissions without resource constraints.

## Required Context

Read these files first:

- `internal/workflow/workflow.go`
- `internal/workflow/step.go`
- `internal/workflow/fanout.go`
- `internal/variable/resolver.go`
- `internal/variable/variable.go`
- `internal/model/work_item.go`
- `cmd/controller/main.go`
- `docs/concepts/resource-constrained-work-admission/README.md`

## Allowed Production Files

- `internal/model/resource_constraint.go`
- `internal/workflow/workflow.go`
- `internal/workflow/step.go`
- `internal/workflow/fanout.go`
- `cmd/controller/main.go`
- focused helper files under `internal/workflow/` or `cmd/controller/`

## Allowed Test Files

- `internal/workflow/*_test.go`
- `cmd/controller/*_test.go`
- `internal/model/*_test.go`

## Out Of Scope

- SQLite inserts for resolved constraints.
- `/work/next` resource-aware claiming.
- Status/log changes.
- Demo smoke scripts.

## Acceptance Criteria

- Workflow fan-out work-item templates can declare resource constraints.
- Resource constraint declarations can use literal integer/string values where appropriate.
- Resource constraint declarations can use resolver expressions where appropriate.
- Fan-out item-specific values can be used to resolve requested units.
- Resolution happens before queue mutation.
- Invalid operator values are rejected before queue mutation.
- Non-integer `requested_units` or `target_units` values are rejected.
- Non-positive `requested_units` values are rejected.
- Negative `target_units` values are rejected.
- Empty `resource_key` values are rejected.
- Duplicate resource keys on one work item are rejected.
- Existing workflows without resource constraints still compile and submit.

## Notes

- Keep expression resolution consistent with existing variable resolver behavior. Do not introduce a second expression language.
- If accessor support is needed for fan-out-specific resource values, mirror the existing parameter accessor pattern rather than creating a new generic template system.
- Keep this slice focused on resolved values; do not yet change claim behavior.

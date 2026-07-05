# 007 Capture Typed Step Outputs

Status: Ready

## Objective

Convert successful terminal work outputs into typed step outputs and expose a helper that builds the generated `workflow.step[index]` resolver scope for later-stage compilation.

## Current State

`model.WorkCompletion` can carry `OutputJSON` and evidence hashes. Python work items produce a canonical logical `output.json` and return that logical output through the completion path.

Dependency state can now mark work items, steps, and stages completed, but it does not yet retain typed logical step outputs for downstream expressions.

The variable package already represents resolved objects and lists through `variable.ResolvedValue`, `variable.ResolvedObject`, and `variable.ResolvedList`. It also already supports the `workflow` namespace.

## Target State

The controller can persist or reconstruct a completed step's logical output in variable-compatible typed form.

Output conversion rules:

- A JSON object converts to `variable.TypeObject` recursively.
- A JSON array converts to `variable.TypeList` recursively.
- A JSON string converts to `variable.TypeString`.
- A JSON boolean converts to `variable.TypeBool`.
- A JSON integer converts to `variable.TypeInt`.
- Non-integer JSON numbers are rejected until a numeric type exists.
- JSON `null` is rejected until a null type exists.

Step aggregation rules:

- A non-fan-out step with one completed work item stores that item's output object as the step output.
- A fan-out step stores a list of completed item outputs ordered by `work_item_index` from queue time.
- Completion order must not affect output order.
- A skipped/reused item can satisfy output aggregation only if its terminal record provides an equivalent logical output.
- A completed step with missing required output fails output capture and should transition the workflow to failure with a clear reason.

The generated scope helper should produce a read-only `workflow.step` variable containing completed step outputs in workflow-definition order. It should include only outputs available to the stage being compiled; future unavailable outputs must cause normal resolver errors if referenced.

## Concept Decision

This slice adds a controller-owned output aggregation concept and uses the existing variable concept for typed values.

A new file such as `cmd/controller/workflow_outputs.go` is justified because JSON-to-`ResolvedValue` conversion and step-output aggregation are separate from HTTP endpoint code.

## Required Context

Read these files first:

- `docs/concepts/dependency-aware-workflows/README.md`
- `docs/concepts/dependency-aware-workflows/006-record-terminal-work-item-state.md`
- `cmd/controller/workflow_dependency_store.go`
- `internal/model/work_item.go`
- `internal/variable/variable.go`
- `internal/variable/type.go`
- `internal/variable/scope.go`
- `internal/variable/accessor.go`
- `internal/variable/resolver.go`

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

- `cmd/controller/workflow_outputs.go`
- `cmd/controller/workflow_dependency_store.go`
- `cmd/controller/workflow_completion.go`
- `internal/variable/variable.go`

Modify `internal/variable` only if a very small helper is necessary to build a typed expression or variable from a resolved value.

## Allowed Test Files

- `cmd/controller/workflow_outputs_test.go`
- `cmd/controller/workflow_dependency_store_test.go`
- `cmd/controller/workflow_completion_test.go`
- `internal/variable/variable_test.go`

## Out Of Scope

- JIT compiling downstream stages.
- Adding float, null, datetime inference, or path inference.
- Adding aliases such as `workflow.previous`.
- Flattening fan-out outputs.
- Worker-side output parsing.
- CLI output display.
- Observability except for existing error reporting in failure paths.

## Acceptance Criteria

- JSON objects convert to nested `variable.ResolvedValue` objects.
- JSON arrays convert to nested `variable.ResolvedValue` lists.
- Strings, booleans, and integers convert to typed scalar values.
- Non-integer numbers are rejected with a clear error.
- `null` is rejected with a clear error.
- A non-fan-out step stores one logical output object.
- A fan-out step stores a list ordered by original work-item index, not completion order.
- A generated `workflow.step` scope can resolve `workflow.step[0]` for a completed prior step.
- Referencing a future unavailable step index fails through the normal resolver path.
- Outputs are keyed by `submission_id`; outputs from one submission are not visible in another submission's generated scope.

## Notes

- Prefer testing conversion helpers without HTTP first.
- Keep the first output contract conservative. Reject shapes GOET cannot type safely yet.

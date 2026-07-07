# 007 Capture Typed Step Outputs

Status: Complete

## Objective

Implementation status: Implemented in controller dependency-state/output helpers.

Convert successful terminal work outputs into typed step outputs and expose a helper that builds the generated `workflow.step[index]` resolver scope for later-stage compilation.


## Implementation Handoff Note

Use the actual file names and helper/store owners introduced by slices 001-004. Where this document names example files such as `workflow_dependency_store.go`, `workflow_completion.go`, or `workflow_stage_queue.go`, treat those as placeholders if the branch implementation chose different owners.

## Current State

`model.WorkCompletion` can carry `OutputJSON` and evidence hashes. Python work items produce a canonical logical `output.json` and return that logical output through the completion path.

Dependency state can now mark work items, steps, and stages completed, but it does not yet retain typed logical step outputs for downstream expressions.

The variable package already represents resolved objects and lists through `variable.ResolvedValue`, `variable.ResolvedObject`, and `variable.ResolvedList`. It also already supports the `workflow` namespace.

Use the actual output-capable store owner from slices 003 and 006. If that owner currently stores only stage-level output, add or adapt step-level output storage so `workflow.step[index]` can represent every workflow step, including multiple steps inside one parallel stage.

Implementation note: OS 007 stores logical outputs on dependency **step** state, not on `workflow_stages.output_json`. The existing nullable stage-level column remains unused for dependency-aware `workflow.step[index]` semantics because one stage may contain multiple logical steps.

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

Output JSON is retained only as controller handoff data. Completed work output JSON and logical step aggregate JSON are byte-limited; membership-level output JSON is pruned after step aggregation; step-level output JSON is retained while the dependency workflow is running and pruned after terminal completion or failure while retaining hashes, byte counts, and pruned flags.

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
- Do not build `workflow.step` from `workflow_stages.output_json`, and do not duplicate logical step output into that stage-level column.

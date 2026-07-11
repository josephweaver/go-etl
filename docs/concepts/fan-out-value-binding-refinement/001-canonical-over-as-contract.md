# 001 Canonical `over` / `as` Contract

Status: Proposed  
Recommended model: GPT-5.4, Medium reasoning  
Reference: EC-3 / operational slice / files(3)+test+doc

## Objective

Make the canonical public fan-out document contract explicit and complete before changing compilation behavior.

The public shape is:

```yaml
fan_out:
  over: ${workflow.items[*]}
  as: item
  id: ${item.id}
  output: ${item.id}   # optional
```

## Current State

The canonical workflow decoder already requires `over`, `as`, and `id`, but `as` is not yet authoritative in the compiler. The document model has no public `output` template even though the low-level compiler has a separate output-token concept.

## Target State

`CanonicalFanOut` carries:

```text
over   required expression resolving to a list
as     required current-item alias
id     required per-item string template
output optional per-item string template; defaults to id
```

`as` must:

- be non-empty;
- use an ordinary variable identifier form;
- reject reserved namespace names such as `workflow`, `override`, `step`, `fanout`, `asset`, `data`, `work_item`, and `runtime`;
- be retained unchanged for diagnostics and compilation.

This slice defines document semantics only. It must not partially implement runtime binding through ad hoc substitutions.

## Required Context

Read first:

- `internal/document/workflow.go`
- `internal/document/workflow_test.go`
- `internal/workflow/document_adapter.go`
- `docs/concepts/canonical-workflow-data-document-model/README.md`
- this Strategic Concept README

## Allowed Production Files

- `internal/document/workflow.go`
- a narrowly scoped shared identifier-validation owner if an existing one should be reused
- `docs/concepts/fan-out-value-binding-refinement/README.md`

## Allowed Test Files

- `internal/document/workflow_test.go`
- JSON/YAML decoder fixture tests already owned by `internal/document`

## Required Changes

1. Add optional `Output string` to `CanonicalFanOut`.
2. Decode `fan_out.output` as an optional string.
3. Validate `fan_out.as` as a non-reserved identifier.
4. Preserve `id` and `output` as authored templates; do not reduce either to an accessor in the document layer.
5. Update canonical JSON/YAML equivalence fixtures to include one `output` example.
6. Emit focused document-path diagnostics such as:

```text
workflow document steps[2].fan_out.as must be a valid non-reserved identifier
```

## Data-State Transition

```text
source JSON/YAML fan_out object
  -> canonical fan-out declaration
     over expression text
     alias identifier
     id template text
     optional output template text
```

No work items are created in this slice.

## Acceptance Criteria

- Valid JSON and YAML fan-out declarations normalize equivalently.
- `output` may be omitted.
- Missing `over`, `as`, or `id` fails.
- Empty or malformed `as` fails.
- Reserved aliases fail.
- Unknown fan-out fields continue to fail under strict canonical decoding.
- Existing canonical documents without `output` remain valid.
- `go test ./internal/document` passes.

## Out of Scope

- Resolving `over`.
- Binding the current item.
- Accessor evaluation.
- ID/output rendering.
- Work parameter resolution.
- Collision detection.

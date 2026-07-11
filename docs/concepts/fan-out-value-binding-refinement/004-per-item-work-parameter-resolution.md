# 004 Per-Item Work Parameter Resolution

Status: Implemented
Recommended model: GPT-5.4, High reasoning  
Reference: EC-3 / operational slice / files(4)+test

## Objective

Resolve canonical `work.parameters` once per fan-out item using the current-item context, preserving the type of whole-value references.

## Current State

The low-level compiler can bind explicit parameter accessors and recursively copy resolved list/object values. Canonical authoring, however, still separates parameter templates from accessor maps and does not provide one natural alias-aware per-item parameter contract.

## Target State

Canonical work parameters may contain literals, whole references, templates, lists, and objects:

```yaml
work:
  type: python_script
  parameters:
    python_entrypoint: scripts/process.py
    pair: ${pair}
    year: ${pair[0]}
    tile: ${pair[1]}
    label: year-${pair[0]}-tile-${pair[1]}
    options:
      year: ${pair[0]}
      tile: ${pair[1]}
      original: ${pair}
```

Expected compiled types:

```text
pair     list
year     int
tile     string
label    string
options  object containing int, string, and list children
```

## Resolution Rules

- A whole-value reference returns the complete resolved value and original type.
- A mixed string template returns a string and allows only scalar placeholders.
- Lists and objects are resolved recursively.
- Literal values retain their canonical type.
- Sensitivity/provenance propagate from referenced values into the resulting parameter.
- A parameter containing any sensitive child is sensitive.
- Protected references continue to use existing protected-reference materialization rules.
- Parameter resolution happens in the controller compiler, not in the worker.

## Compatibility Decision

The canonical public schema should prefer direct parameter values and references.

The existing internal `parameter_accessors` mechanism may remain temporarily for legacy/internal callers, but canonical workflow documentation should no longer require placeholder parameter values plus a separate accessor map.

OS 007 may remove canonical exposure of `parameter_accessors` after fixture migration proves equivalence.

## Required Context

Read first:

- `internal/workflow/fanout.go`
- `internal/workflow/document_adapter.go`
- `internal/document/value.go`
- `internal/document/expression_directive.go`
- `internal/model/work_item.go`
- OS 002 and OS 003 implementations

## Allowed Production Files

- `internal/workflow/fanout.go`
- `internal/workflow/document_adapter.go`
- one focused recursive parameter-resolution owner
- `internal/document/*` only if raw canonical parameter values must be retained rather than prematurely converted

## Allowed Test Files

- `internal/workflow/fanout_test.go`
- `internal/workflow/document_adapter_test.go`
- one new focused parameter-resolution test file

## Required Changes

1. Retain enough canonical parameter expression information to resolve after a current item exists.
2. Reuse the OS 003 whole-reference/template parser.
3. Convert resolved values into `model.Parameter` without losing list/object structure.
4. Resolve nested list/object parameters recursively.
5. Propagate sensitivity, redaction, protected-reference, and provenance facts consistently.
6. Preserve static parameters unchanged across all generated items.
7. Ensure each generated work item receives an independent deep parameter value.

## Data-State Transition

```text
canonical parameter tree + FanOutItemContext
  -> recursively resolved typed tree
  -> independent model.Parameters for one work item
```

## Acceptance Criteria

- Integer, string, boolean, list, and object whole references preserve type.
- `${pair}` can be passed as a list parameter.
- `${pair[0]}` remains an integer parameter.
- Composite strings render correctly.
- Nested objects and lists resolve recursively.
- One work item's parameter mutation cannot affect another item or the template.
- Sensitive child values mark the appropriate containing parameter sensitive.
- Existing literal-only workflows compile unchanged.
- Canonical tests no longer need `parameter_accessors` for ordinary fan-out values.

## Out of Scope

- Data definitions and materialization bindings.
- Output-target parameter binding.
- Resource constraints.
- Worker-side expression resolution.

# 002 Typed Current-Item Binding

Status: Implemented
Recommended model: GPT-5.6-Sol, High reasoning  
Reference: EC-3 / operational slice / files(4)+test

## Objective

Create one authoritative per-item fan-out context that binds each resolved list element unchanged as both the generic current item and the alias declared by `fan_out.as`.

## Current State

The low-level fan-out compiler receives each `ResolvedValue`, but it passes that value directly to specialized token/accessor helpers. The canonical `as` field is not represented in `FanOutWorkItemTemplate`, and there is no shared current-item context for every downstream consumer.

## Target State

Introduce an internal value equivalent to:

```go
type FanOutItemContext struct {
    Alias string
    Index int
    Value variable.ResolvedValue
}
```

The context must expose equivalent reference roots:

```text
${fanout}
${<alias>}
```

For `as: pair`, these are equivalent:

```text
${fanout}
${pair}
```

The current item may be any supported resolved type:

```text
string | path | int | bool | list | object
```

No shape conversion is permitted.

## Semantic Rules

- `over` resolves exactly once per step compilation.
- `over` must resolve to a list.
- Every direct child of that list becomes one item.
- A child list remains one list-valued item.
- A child object remains one object-valued item.
- Heterogeneous child types are allowed.
- Item order is preserved.
- The context is immutable from the perspective of consumer resolution.
- Sensitivity, redaction label, protected reference, and provenance metadata remain attached.
- The alias scope exists only while compiling the current item.
- The alias shadows lower-precedence variables with the same unqualified key only inside that context.
- The generic `fanout` root is fan-out-specific syntax and must not become a mutable global variable.

## Required Context

Read first:

- `internal/workflow/fanout.go`
- `internal/workflow/document_adapter.go`
- `internal/variable/accessor.go`
- `internal/variable/namespace.go`
- `internal/variable/resolver.go`
- `internal/workflow/function_fanout_test.go`

## Allowed Production Files

- `internal/workflow/fanout.go`
- `internal/workflow/document_adapter.go`
- one new focused owner such as `internal/workflow/fanout_context.go`
- `internal/variable/*` only if a generally reusable immutable temporary-scope helper is necessary

## Allowed Test Files

- `internal/workflow/fanout_test.go`
- one new `internal/workflow/fanout_context_test.go`
- `internal/workflow/document_adapter_test.go`

## Required Changes

1. Carry canonical `as` into the internal fan-out work-item template.
2. Resolve `over` through the normal resolver and require `TypeList`.
3. Construct one `FanOutItemContext` per direct list child.
4. Add a shared helper for resolving a whole current-item root or an accessor rooted at either `fanout` or the authored alias.
5. Preserve the complete `ResolvedValue`, including nested type and sensitivity metadata.
6. Keep existing fan-out output ordering and empty-list behavior.

## Required Tests

Prove current-item binding for:

- list of strings;
- list of integers;
- list of booleans;
- list of two-item lists;
- list of objects;
- heterogeneous list;
- nested object/list values;
- sensitive item metadata;
- empty list;
- non-list `over` rejection.

## Data-State Transition

```text
resolved over value: list<T>
  -> ordered child T at index i
  -> immutable FanOutItemContext(alias, index, T)
```

`T` is not converted.

## Acceptance Criteria

- Every direct child type is accepted.
- Nested lists are not flattened.
- Objects are not required.
- Strings are not split into characters.
- Each context resolves through both generic and named roots.
- One item's alias value cannot leak into another item.
- Non-list `over` reports the actual resolved type.
- Empty fan-out still returns zero compiled items without error.
- Existing ordinary scalar fan-out behavior remains intact.

## Out of Scope

- Composite string templates.
- Parameter replacement.
- Data-asset binding.
- ID/output safety.

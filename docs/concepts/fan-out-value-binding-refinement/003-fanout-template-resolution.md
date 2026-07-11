# 003 Fan-Out Template Resolution

Status: Proposed  
Recommended model: GPT-5.6-Sol, High reasoning  
Reference: EC-3 / operational slice / files(4)+test

## Objective

Implement one fan-out-aware template resolver for per-item ID and output tokens, including list indexes and chained list/object accessors.

## Current State

The variable accessor layer supports chained field and list-index accessors, but the canonical adapter reduces `fan_out.id` to either the current value or one object-field accessor. Low-level token generation accepts only one scalar value and cannot render composite templates.

## Target State

These expressions resolve against the current item context:

```text
${fanout}
${fanout[0]}
${fanout[1].tile}
${fanout.rows[0].crop_id}
${pair}
${pair[0]}
${pair[1].tile}
${job.region.code}
```

These templates render:

```text
${pair[0]}-${pair[1]}
year-${pair[0]}-tile-${pair[1]}
${job.year}-${job.tile}
```

## Whole Reference Versus Template

The resolver must distinguish:

```text
whole reference: `${pair[0]}`
mixed template: `year-${pair[0]}`
```

For ID/output rendering both ultimately require a scalar string result, but the shared parser should retain the distinction for later typed parameter resolution.

## Scalar Rendering Rules

Allowed placeholder types:

- string;
- path;
- integer rendered base 10;
- boolean rendered as lowercase `true` or `false`.

Rejected placeholder types:

- list;
- object;
- any unsupported future composite type.

The error must tell the author to select a scalar field or index rather than silently encoding JSON.

## Required Context

Read first:

- `internal/variable/accessor.go`
- `internal/variable/reference.go` or the current reference parser owner
- `internal/workflow/fanout.go`
- `internal/workflow/document_adapter.go`
- OS 002 implementation

## Allowed Production Files

- one new focused owner such as `internal/workflow/fanout_template.go`
- `internal/workflow/fanout.go`
- `internal/workflow/document_adapter.go`
- `internal/variable/accessor.go` only for reusable accessor defects exposed by tests

## Allowed Test Files

- `internal/workflow/fanout_template_test.go`
- `internal/workflow/document_adapter_test.go`
- `internal/variable/accessor_test.go` only for generic accessor regressions

## Required Changes

1. Parse placeholders without shell-like or general expression evaluation.
2. Resolve placeholder roots only from allowed variable references, including the current-item generic and alias roots.
3. Apply chained accessors through the existing accessor owner.
4. Render composite ID templates.
5. Add optional output-template support; default output token to rendered ID when omitted.
6. Preserve the existing step prefix, output prefix, and output extension behavior.
7. Remove canonical dependence on `fanoutAccessorFromExpression` as the sole ID mechanism.
8. Keep legacy low-level accessor fields only as temporary compatibility fields until OS 007.

## Data-State Transition

```text
id/output template text + FanOutItemContext
  -> parsed literal/reference segments
  -> scalar resolved segments
  -> deterministic rendered token
```

## Acceptance Criteria

- `${fanout[0]}` resolves for a list-valued item.
- `${pair[0]}` resolves when `as: pair`.
- Object fields and mixed list/object chains resolve.
- Composite templates render in authored order.
- Boolean rendering is deterministic.
- Referencing an out-of-range index fails with the accessor path.
- Referencing a missing field fails with the field name.
- Rendering a list/object directly fails without JSON stringification.
- Output defaults to ID when `fan_out.output` is absent.
- Literal-only ID templates remain valid if safe.

## Out of Scope

- Work parameter type preservation.
- Data-operator bindings.
- Collision policy.
- Arbitrary functions inside templates.

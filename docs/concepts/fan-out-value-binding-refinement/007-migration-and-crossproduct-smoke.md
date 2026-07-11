# 007 Migration and Crossproduct Smoke

Status: Proposed  
Recommended model: GPT-5.3-codex-spark, Medium reasoning  
Reference: EC-3 / operational slice / fixtures+test+doc

## Objective

Migrate canonical fixtures and prove end-to-end that scalar, list, and object fan-out all compile correctly, including a `list.crossproduct(years, tiles)` workflow that generates deterministic year-tile items.

## Current State

The repository has canonical JSON/YAML equivalence tests and a function-produced fan-out proof. Existing examples use object-valued items and `${fanout.<field>}`. They do not prove that two-item lists can be indexed for IDs, parameters, data bindings, or outputs.

## Target State

The regression suite proves all supported item forms:

```text
list<string>
list<int>
list<bool>
list<list<any>>
list<object>
heterogeneous list<any>
empty list
```

The primary smoke workflow uses:

```yaml
variables:
  years:
    - 2008
    - 2009
  tiles:
    - h18v07
    - h18v08
    - h19v07
  pairs:
    $type: list
    $call: list.crossproduct
    $args:
      - $ref: years
      - $ref: tiles

steps:
  - id: process-pair
    fan_out:
      over: ${workflow.pairs[*]}
      as: pair
      id: ${pair[0]}-${pair[1]}
      output: ${pair[1]}-${pair[0]}
    work:
      type: write_demo_output
      parameters:
        year: ${pair[0]}
        tile: ${pair[1]}
        pair: ${pair}
```

Expected work-item count: `2 * 3 = 6`.

A scale-only compiler test should use 16 years and 88 placeholder tiles and assert exactly 1,408 unique IDs and output filenames without executing 1,408 subprocesses.

## Compatibility and Migration Decision

- Canonical documentation changes to alias-first examples such as `${pair[0]}` and `${job.year}`.
- Generic `${fanout[0]}` and `${fanout.year}` remain supported and tested.
- Canonical `parameter_accessors` examples are replaced by direct per-item parameter references.
- Internal legacy accessor fields may remain only where non-canonical callers still require them; mark them deprecated and create a focused removal issue if immediate deletion would exceed this slice.
- Do not preserve the old canonical limitation that `fan_out.id` must be one whole accessor.

## Required Context

Read first:

- `internal/workflow/function_fanout_test.go`
- `internal/workflow/canonical_equivalence_test.go`
- `internal/workflow/document_adapter_test.go`
- canonical workflow smoke fixtures
- `docs/workflow-authoring-template.md`
- this complete SC/OS bundle

## Allowed Production Files

Production changes should be limited to cleanup or deprecation discovered during fixture migration. New semantics belong in OS 001 through OS 006.

## Allowed Test and Documentation Files

- `internal/workflow/function_fanout_test.go`
- `internal/workflow/canonical_equivalence_test.go`
- `internal/workflow/document_adapter_test.go`
- existing fan-out/data-operator smoke tests
- `docs/workflow-authoring-template.md`
- `docs/concepts/README.md`
- `PROJECT_STATE.md`
- this concept tracker

## Required Tests

1. JSON/YAML semantic equivalence for list-valued crossproduct fan-out.
2. Exact generated IDs and output filenames for a small fixture.
3. Exact parameter types and values.
4. Equivalent alias and generic-current-item references.
5. Object-valued regression behavior.
6. Scalar-valued regression behavior.
7. Empty-list auto-advance regression.
8. 16-by-88 compile-only scale test with 1,408 unique items.
9. Data-asset parameter binding from pair indexes.
10. Commit output path binding from pair indexes.
11. Collision and unsafe-token failures from OS 006.

## Smoke Evidence

The test output should make the mapping visible:

```text
process-pair-2008-h18v07 -> year=2008 tile=h18v07 output=...h18v07-2008...
process-pair-2008-h18v08 -> year=2008 tile=h18v08 output=...h18v08-2008...
process-pair-2008-h19v07 -> year=2008 tile=h19v07 output=...h19v07-2008...
process-pair-2009-h18v07 -> year=2009 tile=h18v07 output=...h18v07-2009...
...
```

## Data-State Transition

```text
function-produced list of pair lists
  -> canonical fan-out contexts
  -> typed parameters and data bindings
  -> unique compiled work items in deterministic order
```

## Acceptance Criteria

- Small crossproduct smoke produces exact expected item order.
- 16 years and 88 placeholder tiles compile to 1,408 unique items.
- No pre-authored list of 1,408 objects is required.
- Pair lists remain available as whole list parameters.
- Alias-first and generic-current-item forms are equivalent.
- Existing object and scalar workflows remain valid.
- JSON and YAML compile equivalently.
- Documentation describes fan-out as an ordered type-preserving map.
- `go test ./...` passes.

## Out of Scope

- Running the real CDL/Yan/Roy data product.
- Downloading real data in tests.
- Performance benchmarking worker execution of 1,408 jobs.
- New list functions.

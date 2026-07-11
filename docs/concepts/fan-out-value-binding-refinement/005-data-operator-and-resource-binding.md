# 005 Data-Operator and Resource Binding

Status: Implemented
Recommended model: GPT-5.4, High reasoning  
Reference: EC-3 / operational slice / files(5)+test

## Objective

Use the same typed current-item context and accessor semantics for explicit `cache_data`, compute data inputs, `commit_data`, and resource-constraint declarations.

## Current State

Canonical data-asset instantiation already supports fan-out-rooted object-field references, and resource constraints have separate expression/accessor logic. These paths can diverge from the refined alias/list-index behavior unless they share one current-item resolver.

## Target State

A list-valued crossproduct item can parameterize data assets directly:

```yaml
fan_out:
  over: ${workflow.year_tile_pairs[*]}
  as: pair
  id: ${pair[0]}-${pair[1]}

data:
  materialize:
    cdl_year:
      asset: cdl
      with:
        year: ${pair[0]}

    # A separate step may materialize Yan/Roy by tile.
```

A compute data binding can use:

```yaml
data:
  inputs:
    cdl_year:
      asset: cdl
      with:
        year: ${pair[0]}
    field_tile:
      asset: yanroy_tile
      with:
        tile: ${pair[1]}
```

A commit target can use:

```yaml
data:
  outputs:
    counts_csv:
      from:
        step: count-field-crops
        artifact: counts_csv
      target: field_crop_year_csv
      with:
        year: ${pair[0]}
        tile: ${pair[1]}
```

A resource key may use scalar template rendering:

```yaml
resource_constraints:
  - resource_key: source.cdl.${pair[0]}
    requested_units: 1
    operator: <=
    target_units: 2
```

## Semantic Rules

- Data parameter bindings are whole-value typed resolutions and must match declared asset/output parameter types.
- Identity-bearing data templates such as cache keys and publish paths use scalar template rendering after parameter binding through existing data-definition templates.
- Resource key templates use scalar rendering.
- Numeric resource units remain typed integers and may resolve from integer fan-out values.
- The same alias and generic roots work everywhere.
- No subsystem may create its own reduced fan-out accessor grammar.

## Required Context

Read first:

- `internal/workflow/data_instance.go`
- `internal/workflow/explicit_cache_data.go`
- `internal/workflow/explicit_commit_data.go`
- `internal/workflow/fanout.go`
- `internal/workflow/document_adapter.go`
- `internal/model/data_definition.go`
- OS 002 through OS 004 implementations

## Allowed Production Files

- `internal/workflow/data_instance.go`
- `internal/workflow/explicit_cache_data.go`
- `internal/workflow/explicit_commit_data.go`
- `internal/workflow/fanout.go`
- one shared current-item resolution owner created by prior slices

## Allowed Test Files

- `internal/workflow/data_instance_test.go`
- `internal/workflow/explicit_cache_data_test.go`
- `internal/workflow/explicit_commit_data_test.go`
- `internal/workflow/fanout_test.go`
- focused canonical data-operator integration tests

## Required Changes

1. Replace special-case `fanout.<field>` asset resolution with the shared current-item resolver.
2. Permit list indexes and chained accessors in asset and output `with` bindings.
3. Resolve compute data input bindings through the same context where the current canonical adapter retains them.
4. Replace separate resource-constraint fan-out accessor behavior with shared resolution.
5. Preserve declared data parameter type validation.
6. Preserve canonical asset identity and shared materialization-domain behavior.
7. Ensure aliases do not alter physical asset identity; only resolved parameter values and effective selections do.

## Data-State Transition

```text
FanOutItemContext
  -> typed asset/output/resource parameters
  -> existing bound asset / bound publish target / resource constraint models
```

## Acceptance Criteria

- A `[year, tile]` item binds an integer year and string tile to data definitions.
- `${pair[0]}` and `${fanout[0]}` produce equivalent bound parameter values.
- Object-valued fan-out remains supported.
- Asset keys vary when resolved year/tile values vary.
- Alias names do not affect asset keys.
- Commit target paths render the correct year and tile.
- Resource keys render safely from scalar item components.
- Resource integer fields reject non-integer references.
- Shared materialization hydration still matches by asset identity and domain.

## Out of Scope

- Changing data provider behavior.
- Implementing worker-scoped materialization.
- Adding new resource operators.
- Publication transport changes.

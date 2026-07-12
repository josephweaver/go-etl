# 001 Finite Asset Collection Domain Model

Status: proposed

## Objective

Add an ordered finite collection-domain model to `DataInputDefinition` so a project can declare values for existing data-asset parameters, including the CDL year range 2008 through 2023.

This slice is model and validation work only. It does not compile work items or materialize files.

## Current State

`internal/model/data_definition.go` defines:

```go
type DataInputDefinition struct {
    Kind       string
    Format     string
    Parameters map[string]DataParameterDefinition
    Files      map[string]DataFileRoleDefinition
    Select     []string
    Binding    DataInputBindingDefinition
    Metadata   map[string]any
}
```

`DataInputDefinition.Parameters` declares parameter names and scalar types. `internal/workflow/data_instance.go` requires a concrete value for every declared parameter when one asset instance is created.

There is no model that says one parameter has a finite declared domain, and parameter-map iteration cannot be used as collection-dimension order.

## Target State

`DataInputDefinition` has an optional collection field equivalent to:

```go
type DataAssetCollectionDefinition struct {
    Dimensions []DataAssetCollectionDimension `json:"dimensions"`
}

type DataAssetCollectionDimension struct {
    Parameter string                    `json:"parameter"`
    Values    []any                     `json:"values,omitempty"`
    Range     *DataAssetCollectionRange `json:"range,omitempty"`
}

type DataAssetCollectionRange struct {
    From    int `json:"from"`
    Through int `json:"through"`
}
```

The exact type names may follow repository conventions, but the semantics must remain:

- dimension order is the authored slice order;
- every dimension references one existing asset parameter;
- exactly one of `values` or `range` is supplied;
- `values` is non-empty and contains scalar values matching the parameter type;
- `range` is valid only for an `int` parameter;
- the range is ascending and inclusive;
- duplicate dimension parameters are rejected;
- the Cartesian-product cardinality is computed with overflow detection;
- a collection with zero members is rejected;
- an asset with no collection remains a scalar asset.

A valid public shape is:

```yaml
parameters:
  year:
    type: int

collection:
  dimensions:
    - parameter: year
      range:
        from: 2008
        through: 2023
```

## Concept Decision

This slice adds a new model concept: a finite domain attached to a data-asset definition.

Create a separate production file for collection-domain types and validation because the domain has its own responsibility, ordering rules, type checks, cardinality calculation, and future extension surface.

Collection dimensions reference existing parameters rather than creating a second parameter-definition system.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/data-asset-collections-and-materialization/README.md`
- `docs/concepts/canonical-workflow-data-document-model/README.md`
- `internal/model/data_definition.go`
- `internal/model/data_definition_test.go`
- `internal/workflow/data_instance.go`
- `internal/workflow/data_instance_test.go`

Do not read worker, controller, persistence, scheduler, transport, provider-adapter, or archive-extraction files for this slice.

## Allowed Production Files

- `internal/model/data_definition.go`
- `internal/model/data_asset_collection.go` (new)

## Allowed Test Files

- `internal/model/data_definition_test.go`
- `internal/model/data_asset_collection_test.go` (new)

## Allowed Documentation Files

- `docs/concepts/data-asset-collections-and-materialization/001-finite-asset-collection-domain-model.md`
- `PROJECT_STATE.md` only after implementation, for a concise current-state note

## Out Of Scope

- Materialization path templates.
- Collection output manifests.
- Work-item types or payloads.
- Collection member expansion in `internal/workflow`.
- Workflow document adapter changes beyond JSON round-trip coverage owned by model tests.
- Worker acquisition or filesystem behavior.
- Controller output aggregation or persistence.
- Provider enumeration.
- Invocation-time collection subsets.
- Descending ranges.
- Range step values other than the implicit step of one.
- Floating-point dimensions.
- Object or list values as dimension members.
- A global collection registry.

## Acceptance Criteria

- A scalar data input with no `collection` field validates exactly as before.
- A `year` integer range from 2008 through 2023 validates and reports cardinality 16.
- The inclusive range expands conceptually to both endpoints.
- A string dimension with explicit non-empty values validates.
- A bool dimension with explicit values validates.
- Dimension order is preserved exactly as authored.
- Duplicate dimension parameters fail.
- A dimension referencing an unknown parameter fails.
- Explicit values that do not match the declared parameter type fail.
- A range on a non-int parameter fails.
- `from > through` fails.
- Supplying both `values` and `range` fails.
- Supplying neither `values` nor `range` fails.
- An empty explicit values list fails.
- Repeated values inside one dimension fail so one tuple cannot be generated twice.
- Cartesian-product cardinality uses checked multiplication and fails on integer overflow.
- JSON round-trip tests preserve dimension order and value types.
- Existing `internal/model` tests pass.
- `go test ./internal/model` passes.

## Notes

- Keep the first domain source deliberately small: explicit scalar values and inclusive integer ranges.
- Do not route range evaluation through the expression-function registry.
- A later concept may add submission-time narrowing or provider-discovered values; those should not be anticipated in this model.
- Suggested HCI: `EC-3 / operational slice / files(2)+test+doc+newfile`.

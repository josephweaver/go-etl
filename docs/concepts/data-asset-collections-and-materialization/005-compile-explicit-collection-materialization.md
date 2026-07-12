# 005 Compile Explicit Collection Materialization

Status: implemented pending review

## Objective

Compile one authored `asset.materialize` step for a collection asset into the deterministic concrete member work items produced by the collection plan.

The authored step must not require workflow-authored fan-out over the collection dimensions.

## Current State

After Operational Slice 004, the explicit materialization compiler handles one fully bound asset instance and emits one `asset.materialize` work item.

Collection-domain and member-plan models exist after Operational Slices 001 through 003, but the canonical document adapter and stage compiler do not consume them.

The current explicit materialization template expects one alias, one asset, optional selection, and a `with` map.

## Target State

This canonical step:

```yaml
- id: materialize-cdl

  data:
    materialize:
      cdl:
        asset: cdl

  work:
    type: asset.materialize
```

compiles the declared 2008-through-2023 domain into sixteen concrete work items.

Each work item contains:

```text
stable work-item ID
stable work-item index
collection fingerprint
member index and member count
dimension order
member bindings
source asset key
materialization identity
target/materialization domain ID
resolved provider and archive facts
resolved source-cache policy
destination-relative path
resource constraints
transfer limits
```

### Stable IDs

IDs must be deterministic from the authored step ID plus canonical dimension bindings. A human-readable form may resemble:

```text
materialize-cdl--year-2008
```

The implementation may use existing safe fan-out token helpers, but IDs must not depend on map iteration or incidental planner ordering.

### Scalar compatibility

An asset with no collection compiles to one work item through the same explicit operation.

### Phase-one restrictions

- An `asset.materialize` collection step must not also declare workflow `fan_out`.
- `with` may supply fixed non-dimension parameters.
- `with` may not narrow or override collection dimensions.
- Exactly one materialized alias remains supported per authored step.
- The selected collection shape must expose one materialized path per member.

## Concept Decision

Collection expansion is compiler behavior of an explicit authored operation.

It is not hidden work generation because:

- the author explicitly declared `asset.materialize`;
- the asset definition explicitly declared a finite domain;
- every compiled member is visible in persisted work/recovery state;
- no compute step causes acquisition work to appear implicitly.

Extend the explicit compiler path. Do not create a second collection-only compiler or a synthetic plugin.

## Required Context

Read these files first:

- `AGENTS.md`
- `docs/concepts/data-asset-collections-and-materialization/README.md`
- Operational Slices 001 through 004 in this concept
- `internal/workflow/data_collection.go`
- `internal/workflow/data_instance.go`
- `internal/workflow/explicit_asset_materialize.go`
- `internal/workflow/explicit_asset_materialize_test.go`
- `internal/workflow/document_adapter.go`
- `internal/workflow/document_adapter_test.go`
- `internal/workflow/compile_stage.go`
- `internal/workflow/compile_stage_test.go`
- `internal/workflow/fanout_binding.go`
- `internal/model/work_item.go`
- `internal/model/data_definition.go`

Do not read worker acquisition, controller completion, persistence schema, scheduler, transport, or publication files unless compile/test failures directly point there.

## Allowed Production Files

- `internal/workflow/data_collection.go`
- `internal/workflow/explicit_asset_materialize.go`
- `internal/workflow/document_adapter.go`
- `internal/workflow/compile_stage.go`
- `internal/workflow/fanout_binding.go` only for deterministic safe token reuse
- `internal/model/work_item.go` only for collection-member payload fields defined by earlier model decisions

## Allowed Test Files

- `internal/workflow/data_collection_test.go`
- `internal/workflow/explicit_asset_materialize_test.go`
- `internal/workflow/document_adapter_test.go`
- `internal/workflow/compile_stage_test.go`
- `internal/workflow/fanout_binding_test.go` only for reused token behavior
- `internal/model/work_item_test.go` only for payload round trips

## Allowed Documentation Files

- `docs/concepts/data-asset-collections-and-materialization/005-compile-explicit-collection-materialization.md`
- `PROJECT_STATE.md` after implementation

## Out Of Scope

- Worker destination promotion.
- Controller output aggregation.
- Downstream hydration.
- Invocation-time subsets.
- Workflow fan-out crossed with collection dimensions.
- Multiple assets in one materialization step.
- Multi-role collection output.
- Per-member downstream pipelining.
- Hidden planning from compute parameters.
- New persistence tables.
- Provider-specific behavior.

## Acceptance Criteria

- One authored CDL materialization step compiles exactly 16 work items.
- The authored step has no workflow `fan_out`.
- Member work-item indexes are deterministic and contiguous.
- Member IDs are deterministic, safe, and include enough binding context to diagnose a year.
- Each work item has fully resolved provider location, archive selection, source cache key, and destination-relative path.
- No member payload contains an unresolved `${asset.year}` placeholder.
- Collection fingerprint and expected member count are identical across all members.
- Member index and member bindings differ appropriately.
- Source asset key and materialization identity are present and validated.
- Resource constraints and transfer limits use the existing materialization logic.
- A scalar asset compiles into one `asset.materialize` work item.
- A collection materialization step with workflow `fan_out` fails.
- A collection-dimension override in `with` fails.
- A fixed non-dimension `with` value is supported and included in identity.
- A destination collision fails before any work is persisted.
- Duplicate explicit materializers for the same materialization identity fail under the existing explicit conflict rule.
- Equivalent JSON and YAML canonical documents produce equivalent member IDs, order, payloads, and fingerprints.
- No compute step is modified or generated by this slice.
- `go test ./internal/workflow ./internal/model` passes.

## Implementation Notes

- Canonical `asset.materialize` steps may omit `fan_out`; other work types still require it.
- No-fanout explicit materialization uses the collection planner to compile one member work item per finite collection member.
- Member work-item IDs and output filenames use deterministic dimension binding tokens such as `materialize-cdl--year-2008`.
- `asset_materialize` payloads now carry materialization domain, destination-relative path, materialization identity, and optional collection-member metadata.

## Notes

- The compiler can use the same stage/step membership machinery as ordinary fan-out; the distinction is in logical output aggregation, implemented later.
- Record enough collection metadata in each member payload/output to validate completeness after controller restart.
- Suggested HCI: `EC-3 / operational slice / files(6)+test+doc+cleanup`.

# 003 Collection Member Expansion and Identity

Status: proposed

## Objective

Add deterministic expansion of a finite asset collection into concrete asset instances and define separate source-asset, materialization, and collection identities.

This slice stops before work-item compilation and worker execution.

## Current State

`internal/workflow/data_instance.go` provides `InstantiateDataAsset` and `DataAssetInstance`. It binds one explicit `with` map, resolves provider/file templates through the temporary asset scope, and computes a canonical asset-instance key.

There is no collection planner, no ordered Cartesian-product expansion, no destination-relative path resolution, and no destination-collision check.

The current asset key includes source/binding semantics and materialization scope, but the model does not separately identify a destination materialization.

## Target State

Add workflow-owned planning types equivalent to:

```go
type DataAssetCollectionPlan struct {
    Asset                   string
    DimensionOrder          []string
    Dimensions              map[string][]variable.ResolvedValue
    FixedParameters         map[string]variable.ResolvedValue
    Selection               []string
    PathTemplate            string
    CollectionFingerprint   string
    Members                 []DataAssetCollectionMember
}

type DataAssetCollectionMember struct {
    Index                   int
    Bindings                map[string]variable.ResolvedValue
    Instance                DataAssetInstance
    DestinationRelativePath string
    MaterializationKey      string
}
```

The exact names may follow package conventions.

Expansion semantics:

1. Resolve fixed `with` values for non-dimension parameters.
2. Reject `with` values for collection-dimension parameters in phase one.
3. Expand dimensions in declared order using deterministic Cartesian-product order.
4. Bind the dimension tuple into the existing asset scope.
5. Reuse current asset instantiation to resolve provider location, archive selection, cache key, and source asset key.
6. Resolve the destination-relative path with every parameter concrete.
7. Compute the materialization identity from source asset key, materialization-domain identity input, and destination-relative path.
8. Compute one collection fingerprint from definition, domain, fixed parameters, selection, destination template, and materialization domain.
9. Reject duplicate source/member tuples.
10. Reject duplicate destination-relative paths, especially when different source asset keys collide.

Scalar assets produce one member plan.

## Concept Decision

Collection expansion belongs in `internal/workflow`, not `internal/model` and not `internal/variable`.

- `internal/model` owns validated definitions and transport shapes.
- `internal/variable` resolves fully specified typed values.
- `internal/workflow` owns turning a finite domain into concrete workflow work.

Add a separate workflow file because collection planning has its own deterministic ordering, identity, and collision responsibilities.

## Required Context

Read these files first:

- `AGENTS.md`
- `docs/concepts/data-asset-collections-and-materialization/README.md`
- `docs/concepts/data-asset-collections-and-materialization/001-finite-asset-collection-domain-model.md`
- `docs/concepts/data-asset-collections-and-materialization/002-materialization-path-template-and-collection-manifest.md`
- `internal/workflow/data_instance.go`
- `internal/workflow/data_instance_test.go`
- `internal/workflow/fanout_binding.go`
- `internal/workflow/fanout_binding_test.go`
- `internal/model/data_definition.go`
- `internal/model/data_asset_collection.go`
- `internal/fingerprint` package files used by current asset-key code

Do not read worker, controller, persistence, transport, scheduler, or provider adapter files for this slice.

## Allowed Production Files

- `internal/workflow/data_instance.go`
- `internal/workflow/data_collection.go` (new)
- `internal/workflow/fanout_binding.go` only if a current scalar-token helper can be reused without changing fan-out semantics
- `internal/model/materialized_asset_collection.go` only for shared identity input shapes if required

## Allowed Test Files

- `internal/workflow/data_instance_test.go`
- `internal/workflow/data_collection_test.go` (new)
- `internal/workflow/fanout_binding_test.go` only for reused deterministic token behavior

## Allowed Documentation Files

- `docs/concepts/data-asset-collections-and-materialization/003-collection-member-expansion-and-identity.md`
- `PROJECT_STATE.md` only after implementation

## Out Of Scope

- Work-item type renaming.
- Work-item creation or persistence.
- Workflow document adapter changes.
- Provider acquisition.
- Filesystem destination promotion.
- Controller output aggregation.
- Downstream hydration.
- Invocation-time subsets.
- Combining workflow-authored fan-out with collection expansion.
- Per-member pipelining.
- Provider enumeration.
- Changing generic variable precedence or interpolation semantics.

## Acceptance Criteria

- The CDL year range expands into exactly 16 members.
- Member indexes are contiguous from zero in deterministic order.
- Repeated compilation of equivalent definitions produces identical member order, source asset keys, materialization keys, and collection fingerprint.
- Dimension order, not Go map iteration, controls Cartesian-product order.
- A two-dimension fixture expands in left-to-right declared order.
- Fixed non-dimension parameters are resolved once and included in each member.
- Supplying a `with` override for a collection dimension fails with a phase-one subset-not-supported error.
- Missing fixed required parameters fail before expansion.
- Every member reuses current `InstantiateDataAsset` behavior for provider/archive/selection resolution.
- Every member has a concrete destination-relative path with no unresolved placeholders.
- Every materialization key includes source asset key, materialization domain, and destination.
- Step aliases do not affect source asset identity or materialization identity.
- Two members that resolve to the same destination fail.
- Two different source asset keys that resolve to the same destination fail with both identities in the diagnostic.
- The same source asset materialized to two destinations has one source identity and two materialization identities.
- Cardinality and identity hashing use canonical deterministic values.
- Scalar assets produce a one-member plan without a synthetic dimension.
- `go test ./internal/workflow` passes.

## Notes

- Keep source asset identity and materialization identity as distinct types or functions; do not hide both meanings behind one ambiguous string.
- Include destination path in the materialization identity, not the source asset key.
- Do not derive identity from human-readable work-item IDs.
- Suggested HCI: `EC-3 / operational slice / files(4)+test+doc+newfile`.

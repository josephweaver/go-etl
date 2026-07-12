# 004 Rename `cache_data` to `asset.materialize`

Status: implemented pending review

## Objective

Atomically replace the current `cache_data` work-item operation and all active compiler/controller/worker terminology with `asset.materialize` while preserving current single-asset behavior.

This is a behavior-preserving migration slice. Collection expansion and deterministic destination behavior remain for later slices.

## Current State

The current implementation uses `cache_data` at every layer:

- `internal/model/work_item.go`
  - `WorkItemTypeCacheData`
  - `CacheDataWorkItemPayload`
  - payload validation and diagnostics
- `internal/workflow/explicit_cache_data.go`
  - `ExplicitCacheDataTemplate`
  - explicit compilation
  - duplicate checks
- `internal/workflow/cache_data_plan.go`
  - asset key, payload, generated IDs, resource constraints, and legacy hidden planning
- `internal/workflow/document_adapter.go`
  - canonical `work.type: cache_data`
- `cmd/worker/work_cache_data.go`
  - worker dispatch implementation around `assetMaterializer`
- `cmd/worker/worker.go`
  - operation dispatch
- `cmd/controller/cache_data_dependencies.go`
  - dependent release/failure behavior
- `cmd/controller/cache_data_hydration.go`
  - materialized-manifest hydration

The low-level helper is already named `assetMaterializer`; the public operation name is the mismatch.

## Target State

The active contract is:

```go
const WorkItemTypeAssetMaterialize WorkItemType = "asset.materialize"

type AssetMaterializeWorkItemPayload struct {
    Operator string `json:"operator"`
    // existing fields preserved
}
```

Internal transport uses the parameter key/type:

```text
asset_materialize
```

The dot-qualified name is reserved for the work-item operation string.

Equivalent active names include:

```text
ExplicitAssetMaterializeTemplate
compileExplicitAssetMaterializeWorkItem
PlanStageAssetMaterializeWorkItems
AssetMaterializePayload
assetMaterializePayloadFromWorkItem
enqueueReadyAssetMaterializeDependents
hydrateAssetMaterializeDependencies
```

The exact helper names may be made idiomatic, but active `cache_data` symbols and diagnostics are removed.

Canonical authoring becomes:

```yaml
work:
  type: asset.materialize
```

The following operation values fail:

```text
cache_data
asset.materialization
```

The worker still delegates acquisition and archive selection to the existing `assetMaterializer`.

## Concept Decision

This slice updates an existing operation; it does not add a second operation.

The rename must be atomic across model, compiler, controller, worker, tests, and active fixtures so no supported release state exposes both names.

Use file moves where the old filename encodes the removed operation. Do not copy the implementation into parallel old/new files.

The legacy hidden planner may still exist under renamed internal terminology after this slice solely to keep the repository behavior-preserving; Operational Slice 009 removes hidden planning entirely.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/data-asset-collections-and-materialization/README.md`
- `docs/concepts/canonical-workflow-data-document-model/README.md`
- `internal/model/work_item.go`
- `internal/model/work_item_test.go`
- `internal/workflow/cache_data_plan.go`
- `internal/workflow/cache_data_plan_test.go`
- `internal/workflow/explicit_cache_data.go`
- `internal/workflow/explicit_cache_data_test.go`
- `internal/workflow/document_adapter.go`
- `internal/workflow/document_adapter_test.go`
- `cmd/worker/work_cache_data.go`
- `cmd/worker/work_cache_data_test.go`
- `cmd/worker/worker.go`
- `cmd/worker/worker_test.go`
- `cmd/controller/cache_data_dependencies.go`
- `cmd/controller/cache_data_dependencies_test.go`
- `cmd/controller/cache_data_hydration.go`
- related controller hydration tests
- `internal/model/data_asset.go`
- `cmd/worker/data_asset_materializer.go`

Search the repository for:

```text
cache_data
CacheData
cacheData
```

Classify each match as production, test, fixture, current documentation, or historical documentation before editing.

## Allowed Production Files

- `internal/model/work_item.go`
- `internal/workflow/cache_data_plan.go` → `internal/workflow/asset_materialize_plan.go`
- `internal/workflow/explicit_cache_data.go` → `internal/workflow/explicit_asset_materialize.go`
- `internal/workflow/document_adapter.go`
- `internal/workflow/compile_stage.go` only for renamed calls
- `cmd/worker/work_cache_data.go` → `cmd/worker/work_asset_materialize.go`
- `cmd/worker/worker.go`
- `cmd/controller/cache_data_dependencies.go` → `cmd/controller/asset_materialize_dependencies.go`
- `cmd/controller/cache_data_hydration.go` → `cmd/controller/asset_materialize_hydration.go`
- narrowly affected controller call sites discovered by compile errors

## Allowed Test Files

- `internal/model/work_item_test.go`
- `internal/workflow/cache_data_plan_test.go` → `internal/workflow/asset_materialize_plan_test.go`
- `internal/workflow/explicit_cache_data_test.go` → `internal/workflow/explicit_asset_materialize_test.go`
- `internal/workflow/document_adapter_test.go`
- `internal/workflow/compile_stage_test.go` only for renamed calls
- `cmd/worker/work_cache_data_test.go` → `cmd/worker/work_asset_materialize_test.go`
- `cmd/worker/worker_test.go`
- `cmd/controller/cache_data_dependencies_test.go` → `cmd/controller/asset_materialize_dependencies_test.go`
- controller hydration tests renamed or updated with the production file
- fixture JSON/YAML files that directly serialize the operation

## Allowed Documentation Files

- `docs/concepts/data-asset-collections-and-materialization/004-rename-cache-data-to-asset-materialize.md`
- `docs/concepts/canonical-workflow-data-document-model/README.md`
- affected canonical document-model OS files that currently prescribe `cache_data`
- `PROJECT_STATE.md`

## Out Of Scope

- Collection dimensions.
- Collection expansion.
- Destination path templates.
- Deterministic destination promotion.
- Collection logical output synthesis.
- Downstream collection hydration changes.
- Removing hidden planning beyond names required for this atomic migration.
- Changing provider, archive, cache, integrity, transfer, or resource semantics.
- Changing `commit_data`.
- Creating a compatibility alias.
- Renaming `assetMaterializer` merely for style.
- GOET-to-GORC API rename.

## Acceptance Criteria

- `WorkItemTypeAssetMaterialize` has JSON value `asset.materialize`.
- `AssetMaterializeWorkItemPayload` preserves the current payload facts and validation.
- Internal parameter key/type is consistently `asset_materialize`.
- Canonical `work.type: asset.materialize` compiles for the current one-asset case.
- Canonical `work.type: cache_data` fails with a clear migration diagnostic.
- `asset.materialization` fails as an unsupported work type.
- Worker dispatch executes `asset.materialize` through the existing `assetMaterializer`.
- Existing provider constraints, transfer limits, archive selection, immutable cache checks, and manifest output behavior remain unchanged.
- Controller dependency release and hydration behavior remain unchanged except for names.
- Active production symbols no longer use `CacheData` or `cacheData`.
- Active production/test/config JSON does not serialize `cache_data`.
- Historical docs may state that `cache_data` was superseded, but current examples use `asset.materialize`.
- No duplicate old/new implementation files remain.
- Focused package tests pass.
- `go test ./...` passes.

## Implementation Notes

- `WorkItemTypeAssetMaterialize` now serializes as `asset.materialize`.
- Existing payload and parameter transport remains `asset_materialize`.
- Old `cache_data` and near-miss `asset.materialization` canonical work types are rejected with focused diagnostics.
- Implementation files and tests were moved from `cache_data` filenames to `asset_materialize` filenames.

## Notes

- This slice is intentionally cross-cutting because a work-item type rename cannot leave model, compiler, controller, and worker on different protocols.
- Prefer `git mv` for filename changes so review shows the behavior-preserving nature of the migration.
- Keep diagnostics precise: operation strings use `asset.materialize`; Go identifiers cannot contain the dot and use `AssetMaterialize`.
- Suggested HCI: `EC-3 / operational slice / files(9)+test+doc+cleanup+newfile`.

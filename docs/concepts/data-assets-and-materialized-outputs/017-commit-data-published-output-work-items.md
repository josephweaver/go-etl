# 017 Commit Data Published Output Work Items

Status: Proposed

## Objective

Compile publish bindings into explicit `commit_data` work items that publish selected promoted artifacts to declared durable store locations.

This slice defines `commit_data` as outbound publication. It must not be confused with internal input-cache finalization.

## Current State

The Data Assets concept already defines:

```text
materialized artifacts
published data assets
publish bindings
registered named locations
overwrite policies
published-asset evidence
```

The current workflow shape lets the worker copy selected artifacts to publish targets as part of compute work item cleanup. That hides outbound data movement inside compute and makes upload/write pressure hard to schedule.

## Target State

A compute work item produces promoted artifacts and reports an artifact manifest.

A downstream `commit_data` work item consumes that manifest and a resolved publish binding, copies the selected artifact to the declared durable location, verifies the copy, and emits published-asset evidence.

```text
compute(...)
  -> artifact manifest

commit_data(...)
  depends_on compute(...)
  consumes artifact manifest
  publishes selected artifact
  emits published asset evidence
```

## Work Item Payload Shape

A `commit_data` payload should carry equivalent facts:

```json
{
  "operator": "commit_data",
  "target_environment_id": "target-fake-hpcc",
  "source": {
    "from_work_item_id": "work_compute_...",
    "from_artifact": "field_cdl_composition_tile"
  },
  "publish_target": {
    "name": "field_cdl_composition_tile",
    "kind": "tabular_dataset",
    "format": "csv",
    "location": {
      "type": "registered_location",
      "name": "published_data",
      "path": "field_cdl_composition/year=2023/tile=fixture_tile_001/field_cdl_composition.csv"
    },
    "overwrite_policy": "fail_if_exists",
    "parameters": {
      "year": 2023,
      "tile": "fixture_tile_001"
    }
  }
}
```

For directory artifacts, the payload must identify that a directory manifest hash is expected and must copy/verify the directory contents deterministically.

## Completed Output Shape

The completed output should be compact:

```json
{
  "schema": "goet/published-data-assets/v1",
  "published_assets": [
    {
      "name": "field_cdl_composition_tile",
      "from_work_item_id": "work_compute_...",
      "from_artifact": "field_cdl_composition_tile",
      "storage_scope": "registered_location",
      "location_name": "published_data",
      "path": "field_cdl_composition/year=2023/tile=fixture_tile_001/field_cdl_composition.csv",
      "size_bytes": 12345,
      "sha256": "...",
      "overwrite_policy": "fail_if_exists",
      "content_type": "text/csv"
    }
  ]
}
```

It may also be embedded into the existing artifact manifest if that convention has already been chosen, but the operator boundary should remain explicit.

## Publication Rules

A `commit_data` worker operation must:

```text
1. read the source artifact manifest from completed compute output
2. select exactly the named artifact
3. validate source artifact path is under the configured artifact root
4. resolve and validate target named location and relative path
5. enforce overwrite policy before writing
6. copy into target-local temporary path where possible
7. atomically reveal/rename/copy-complete into final target where possible
8. hash the final published target
9. compare source and target evidence
10. emit compact published-asset evidence
```

Default overwrite behavior must remain:

```text
fail_if_exists
```

Optional later policies:

```text
replace_if_same_hash
replace
versioned
```

Only `fail_if_exists` is required in this slice.

## Resource Constraints

`commit_data` work items should carry upload/write constraints independently from compute constraints.

Examples:

```json
{
  "resource_constraints": [
    {
      "resource_key": "target:fake-hpcc/published-data-write:published_data",
      "requested_units": 1,
      "operator": "<=",
      "target_units": 1
    }
  ]
}
```

If a future `commit_data` target uses rclone or another network provider:

```text
provider:gdrive-rclone:<remote>/upload
target:<target_id>/published-data-write:<location_name>
```

## Dependency Rules

The planner should transform:

```text
compute(field_cdl_composition)
  publish field_cdl_composition_tile
```

into:

```text
compute(field_cdl_composition)
  -> commit_data(field_cdl_composition_tile)
```

If a merge step depends on the published evidence rather than local artifact evidence, it should depend on the `commit_data` work item. If it consumes the local artifact directly within the same target environment, it may depend on the compute work item instead.

## Idempotence

A retried `commit_data` work item must be safe under conservative policy:

- If target does not exist, copy and verify.
- If target exists and `fail_if_exists`, fail unless the implementation explicitly supports `replace_if_same_hash`.
- If partial temp target exists from a failed attempt, clean only temp paths owned by that attempt/operator.
- Never delete or overwrite a final target path unless the resolved overwrite policy explicitly allows it.

## Required Context

Read these files first:

```text
docs/concepts/data-assets-and-materialized-outputs/README.md
docs/concepts/complete/dependency-aware-workflow-execution/README.md
docs/concepts/complete/workflow-execution-persistence/README.md
docs/concepts/complete/resource-constrained-work-admission/README.md
internal/model/work_item.go
internal/model/artifact*.go
internal/workflow/*compile*.go
internal/worker/*artifact*.go
internal/worker/*data*.go
```

## Allowed Production Files

```text
internal/model/artifact*.go
internal/model/data_asset*.go
internal/model/work_item.go
internal/workflow/*compile*.go
internal/worker/*artifact*.go
internal/worker/*publish*.go
internal/worker/*data*.go
internal/persistence/store.go
internal/persistence/db_adapter_sqlite.go
```

## Allowed Test Files

```text
internal/model/*artifact*_test.go
internal/workflow/*publish*_test.go
internal/worker/*publish*_test.go
internal/persistence/*artifact*_test.go
```

## Out Of Scope

```text
data catalog registration
publishing to arbitrary absolute host paths
credential propagation
real Google Drive upload
destructive overwrite by default
cache_data implementation
global retention/cleanup
```

## Acceptance Criteria

- Publish bindings can compile to `commit_data` work items.
- `commit_data` depends on the producing compute work item.
- `commit_data` consumes a completed artifact manifest and rejects missing artifact names.
- `commit_data` copies file artifacts to a configured named location under a safe relative path.
- `commit_data` verifies target hash and byte count.
- `commit_data` emits compact published-asset evidence.
- `fail_if_exists` is enforced.
- Resource constraints can be attached to `commit_data` work items.
- Unit tests prove:
  - one file artifact publish succeeds;
  - unsafe target paths are rejected;
  - missing source artifact fails;
  - existing target under `fail_if_exists` fails;
  - published evidence matches target bytes;
  - publish-location write mutex blocks concurrent commits when configured.

## Notes

This slice should make publication observable as its own work item. That is important for long-running workflows where compute may succeed but publishing may fail due to storage pressure, permissions, or target path conflicts.

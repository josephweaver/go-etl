# 010 Published Data Asset Copy To Named Location

Status: proposed

## Objective

Add worker-side publication for selected promoted artifacts by copying them to predeclared named data locations and reporting compact published-asset evidence.

This slice implements the `put_data` side as worker infrastructure, not as a primary work item type. It does not create a data registry or catalog entry. The project/workflow defines the intended target ahead of time; the worker copies bytes to that named location.

## Current State

Earlier slices let Python scripts write artifacts under `GOET_ARTIFACT_DIR`, let the worker promote those artifacts to the worker data/artifact root, and let the controller persist compact artifact manifests.

However, promoted attempt artifacts are not necessarily the final data product location. A workflow may need to publish a produced dataset to a deterministic path such as:

```text
published_data/field_cdl_composition/year=2023/tile=fixture_tile_001/field_cdl_composition.csv
```

There is not yet a controlled way to copy a named artifact to that target and report evidence.

## Target State

A work item may carry bound publish targets equivalent to:

```json
{
  "publish": {
    "field_cdl_composition_tile": {
      "from_artifact": "field_cdl_composition_tile",
      "target": "field_cdl_composition_tile",
      "location": {
        "type": "registered_location",
        "location_name": "published_data",
        "path": "field_cdl_composition/year=2023/tile=fixture_tile_001/field_cdl_composition.csv"
      },
      "overwrite_policy": "fail_if_exists"
    }
  }
}
```

After artifact promotion succeeds, the worker:

1. finds the promoted artifact by `from_artifact` name;
2. resolves the target named location root from worker configuration;
3. validates the target relative path;
4. copies the artifact file or directory into a temporary destination under the target root;
5. computes hash/size evidence from the copied target;
6. reveals the target atomically when practical;
7. returns compact published-asset evidence in the logical output.

Published evidence should be equivalent to:

```json
{
  "name": "field_cdl_composition_tile",
  "from_artifact": "field_cdl_composition_tile",
  "storage_scope": "registered_location",
  "location_name": "published_data",
  "path": "field_cdl_composition/year=2023/tile=fixture_tile_001/field_cdl_composition.csv",
  "size_bytes": 12345,
  "sha256": "...",
  "overwrite_policy": "fail_if_exists"
}
```

Directory publication should use the same deterministic directory manifest hashing rules as artifact directories.

## Concept Decision

Publication means controlled copy to a predefined named location plus compact evidence. It does not mean automatic registry insertion.

Use conservative default overwrite behavior:

```text
fail_if_exists
```

This avoids silently replacing a previously published data product. More permissive policies can be added later after provenance and retention are clearer.

The worker owns publication because only the worker execution environment can safely see worker-local artifacts and named storage mounts. The controller records evidence but should not read target bytes.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/data-assets-and-materialized-outputs/README.md`
- `internal/model/artifact_manifest.go`
- `internal/model/published_data_asset.go`
- `cmd/worker/artifact_promotion.go`
- `cmd/worker/data_locations.go`
- `cmd/worker/work_python.go`
- `cmd/worker/config.go`
- `cmd/worker/evidence.go`

Do not read controller scheduler, transport, Slurm, SSH, or container files unless compile or test failures directly require them.

## Allowed Production Files

- `cmd/worker/published_asset.go`
- `cmd/worker/data_locations.go`
- `cmd/worker/work_python.go`
- `cmd/worker/artifact_promotion.go` only for narrow integration
- `cmd/worker/evidence.go` only for shared file/directory hash helpers
- `cmd/worker/config.go` only for named publish-location roots if needed
- `internal/model/published_data_asset.go`
- `internal/model/artifact_manifest.go` only for adding published evidence fields if needed

## Allowed Test Files

- `cmd/worker/published_asset_test.go`
- `cmd/worker/data_locations_test.go`
- `cmd/worker/work_python_test.go`
- `cmd/worker/artifact_promotion_test.go` only for narrow integration
- `cmd/worker/evidence_test.go` only for shared hash helper behavior
- `internal/model/published_data_asset_test.go`
- `internal/model/artifact_manifest_test.go` only for manifest field additions

## Out Of Scope

- Data catalog or registry writes.
- Object-store publication.
- Credentialed destinations.
- Destructive overwrite by default.
- Retention cleanup for replaced or failed publications.
- Publishing arbitrary worker paths that were not declared artifacts.
- Controller reading published bytes.
- Real CDL/Yan/Roy outputs or real Google Drive writes.
- Fake HPCC automation.

## Acceptance Criteria

- A worker helper can publish a promoted file artifact to a configured named location.
- The destination path is relative to the named location root and uses slash separators.
- Unsafe destination paths are rejected before filesystem writes.
- Publishing an unknown artifact name fails.
- Publishing to an unknown named location fails.
- `fail_if_exists` prevents overwriting an existing target.
- The returned published evidence includes location name, relative path, byte count, and SHA-256 computed after copy.
- A worker helper can publish a promoted directory artifact and return deterministic directory manifest evidence.
- Publication failure does not leave a completed manifest claiming success.
- Existing artifact-only Python tests still pass.
- Tests use temporary directories and tiny files only.
- `go test ./cmd/worker` passes.

## Notes

- The named location can be the same physical root as the worker data directory in a local fixture, but it should still be addressed through a logical name.
- In real deployments, published data may live on a shared filesystem while attempt artifacts live in a worker data root.
- This slice answers the publication gap without committing to a full registry service.

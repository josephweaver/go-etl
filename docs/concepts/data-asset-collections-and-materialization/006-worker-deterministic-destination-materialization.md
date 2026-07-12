# 006 Worker Deterministic Destination Materialization

Status: proposed

## Objective

Extend the existing worker asset materializer so an `asset.materialize` member is staged and atomically promoted to its declared deterministic destination under the configured shared materialization root.

Preserve the current source cache and provider/archive implementation.

## Current State

`cmd/worker/work_asset_materialize.go` delegates one bound asset to `assetMaterializer.materialize`.

`cmd/worker/data_asset_materializer.go` currently:

- chooses source-cache paths from the cache key;
- streams HTTP/local/rclone sources into the cache;
- verifies expected size and SHA-256;
- reuses valid immutable source-cache entries;
- extracts selected archive members under a cache/extraction root;
- returns the resulting concrete local path.

The asset definition does not currently name a final deterministic destination separate from the source/extraction cache.

## Target State

An `asset.materialize` payload supplies a concrete safe destination-relative path:

```text
cdl/2017.tif
```

For phase-one shared materialization, the worker resolves:

```text
absolute destination =
    effective asset materialization root
    + destination-relative path
```

The configured root is the existing shared asset-cache/materialization root unless implementation review identifies a current separate root that must be used. Do not let the project author an absolute root.

### Source and destination phases

```text
provider acquisition
    -> verified source cache
    -> archive selection if any
    -> destination staging path
    -> destination evidence
    -> atomic promotion
```

The source cache remains reusable and independent from the destination.

### Destination evidence

Add a pinned destination manifest equivalent to:

```json
{
  "schema": "goet/materialized-asset-destination/v1",
  "asset_key": "sha256:...",
  "materialization_key": "sha256:...",
  "materialization_domain_id": "msu-hpcc",
  "destination_relative_path": "cdl/2017.tif",
  "kind": "file",
  "size_bytes": 123,
  "sha256": "...",
  "member_bindings": {"year": 2017}
}
```

Store the manifest at a deterministic protected sibling or internal metadata location. The chosen layout must work for both file and directory destinations without making project code depend on the manifest path.

### Existing destination policy

- Destination absent: stage, verify, write manifest, and promote.
- Destination present with matching pinned manifest and current bytes: reuse.
- Destination present without matching pinned manifest: fail.
- Destination or manifest mismatch: fail.
- Never silently overwrite.

### Concrete member output

The member output reports the deterministic destination as `local_path` and includes collection-member metadata required by Operational Slice 007.

## Concept Decision

Reuse `assetMaterializer`; do not add another provider/materialization engine.

Add a focused destination-promotion helper because source acquisition and final deterministic placement have separate identity, locking, and conflict responsibilities.

Use atomic file/directory promotion where the current filesystem helper permits it. The destination must not become visible as complete before content and pinned evidence are valid.

## Required Context

Read these files first:

- `AGENTS.md`
- `docs/concepts/data-asset-collections-and-materialization/README.md`
- Operational Slices 002 through 005
- `cmd/worker/work_asset_materialize.go`
- `cmd/worker/work_asset_materialize_test.go`
- `cmd/worker/data_asset_materializer.go`
- `cmd/worker/data_asset_materializer_test.go`
- `cmd/worker/archive_extractor.go`
- `cmd/worker/archive_extractor_test.go`
- `cmd/worker/artifact_promotion.go`
- `cmd/worker/config.go`
- `cmd/worker/config_test.go`
- `internal/model/data_asset.go`
- `internal/model/work_item.go`

Do not read controller stage activation, workflow output aggregation, client, scheduler, transport, or publication code unless tests directly require it.

## Allowed Production Files

- `cmd/worker/work_asset_materialize.go`
- `cmd/worker/data_asset_materializer.go`
- `cmd/worker/asset_destination.go` (new)
- `cmd/worker/archive_extractor.go` only for staging-output integration
- `cmd/worker/config.go` only if an existing effective materialization-root accessor is insufficient
- `internal/model/data_asset.go`
- `internal/model/work_item.go` only for payload/evidence fields already approved by prior slices

## Allowed Test Files

- `cmd/worker/work_asset_materialize_test.go`
- `cmd/worker/data_asset_materializer_test.go`
- `cmd/worker/asset_destination_test.go` (new)
- `cmd/worker/archive_extractor_test.go` only for deterministic selected-path promotion
- `cmd/worker/config_test.go` only for materialization-root behavior
- `internal/model/data_asset_test.go`
- `internal/model/work_item_test.go`

## Allowed Documentation Files

- `docs/concepts/data-asset-collections-and-materialization/006-worker-deterministic-destination-materialization.md`
- `PROJECT_STATE.md` after implementation

## Out Of Scope

- Controller aggregation.
- Downstream hydration.
- Remote/object-store destinations.
- Worker-scope or node-local materialization.
- Provider enumeration.
- Real CDL download.
- Arbitrary overwrite policies.
- Garbage collection of old destinations.
- Cross-host distributed locks beyond the current shared-filesystem/resource-constraint model.
- Source-cache redesign.
- Publication through `commit_data`.
- Adding geospatial libraries.

## Acceptance Criteria

- A local fixture asset materializes to the exact declared destination-relative path under the configured root.
- A fixture HTTP asset still streams through the existing source-cache path before destination promotion.
- A ZIP-selected file is promoted to the deterministic destination filename.
- Source-cache evidence and destination evidence remain separate.
- The final `MaterializedDataAsset.LocalPath` is the deterministic destination, not the source-cache archive path.
- The member output includes materialization key, domain, destination-relative path, member bindings, member index/count, and content evidence.
- Parent directories are created safely.
- Destination staging occurs outside the ready destination name.
- Promotion does not expose a partial destination as complete.
- A matching existing destination and pinned manifest is reused without provider acquisition.
- A destination with no pinned evidence fails.
- A destination whose bytes no longer match pinned evidence fails.
- A destination pinned to a different materialization key fails.
- Concurrent attempts cannot both promote conflicting content silently.
- All destination paths are revalidated at worker execution.
- Transfer throttling and source resource constraints remain effective.
- Expected source integrity checks remain effective.
- Archive path traversal protections remain effective.
- Tests use fixture bytes and local `httptest`, not the external network.
- `go test ./cmd/worker ./internal/model` passes.

## Notes

- For a file destination, a sibling metadata file is acceptable if it cannot collide with project paths and is treated atomically with promotion. A hashed metadata directory under the materialization root is also acceptable.
- Do not infer correctness from destination existence alone.
- Suggested HCI: `EC-3 / operational slice / files(7)+test+doc+newfile`.

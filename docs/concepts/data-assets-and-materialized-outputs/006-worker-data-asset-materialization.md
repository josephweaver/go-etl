# 006 Worker Data Asset Materialization

Status: implemented

## Objective

Add worker-side materialization for tiny bound `local_file`, `http`, and `registered_location` data fixtures using streaming reads, safe cache paths, reference mode for shared read-only data, immutable cache behavior, and hash/size verification.

This slice proves the core data asset boundary inside the worker execution environment. It must remain safe for local development and must not download real CDL, contact Google Drive, require rclone, require 7z, or access real Yan/Roy data by default.

## Current State

The worker executes concrete assignments and has local runtime roots for logs, temporary files, and completed data. It does not know how to materialize bound data assets or write a `GOET_DATA_ASSETS_JSON` manifest for Python scripts.

The previous slice defines provider templates, cache policies, integrity expectations, archive selectors, and concrete bound data assets. Those models are still declarations until the worker can make them available as local execution paths.

## Target State

When a work item carries bound data assets, the worker materializes or references them before dispatching the operation.

This slice should support:

- `local_file` assets by resolving a configured named root plus safe relative path, then either referencing or copying into worker cache;
- `http` assets by streaming an HTTP/HTTPS URL to a worker cache path;
- `registered_location` assets by resolving a named configured root plus safe relative path;
- `worker_cache` materialization for copied/downloaded inputs;
- `reference` materialization for common read-only data already available on a shared filesystem or mounted container path;
- immutable cache reuse rules;
- optional expected size validation;
- optional expected SHA-256 validation;
- observed SHA-256 and byte count recording;
- a small configurable maximum download/copy size for tests and local runs unless explicitly overridden;
- a materialized data assets manifest written under the attempt work directory;
- `GOET_DATA_ASSETS_JSON` passed to Python when materialized assets exist.

A worker cache shape may be:

```text
<DataDir>/cache/assets/<safe-cache-key>/source
<DataDir>/cache/assets/<safe-cache-key>/manifest.json
```

If `cache_key` is not provided, derive one from the canonical bound data asset declaration. If an expected SHA-256 is provided, include it in the derived identity or verify any existing cached file before reuse.

For common read-only data, the shape may instead be:

```text
<registered_location_root>/<relative-path-template-rendered-by-compiler>
```

In that case, materialization strategy is `reference`, and the worker exposes that path as a read-only local path without duplicating the data.

## Immutable Cache Behavior

If a bound asset has `cache.immutable: true`, the worker must not replace an existing cache entry with different bytes under the same key.

Required behavior:

```text
cache miss:
  acquire source
  verify expected integrity when present
  write source and manifest atomically

cache hit:
  read cache manifest
  verify cached file still matches manifest
  verify expected integrity when present
  reuse cached source

cache hit with mismatch:
  fail materialization before plugin execution
```

If `cache.immutable` is omitted for `worker_cache`, treat it as true unless the implementation explicitly documents a different default.

If no expected hash is provided, the cache manifest pins the first observed hash. That gives reproducible reuse even when an upstream HTTP source later changes.

## Concept Decision

Materialize data assets in the worker execution environment, not through the controller. This keeps full-size public rasters off the user's Windows laptop when the selected backend is HPCC.

Use streaming copy/hash. Do not read an entire asset into memory.

Support `reference` mode because common input data is often read-only and optimally shared once across workers through a mounted or shared filesystem. Support `worker_cache` mode because HTTP and local-file assets still need safe local reuse.

Keep archive extraction out of this slice. This slice may cache the source archive file, but `007-worker-archive-extraction-and-selection.md` exposes selected archive members.

Keep Google Drive/rclone out of this slice. `008-gdrive-rclone-data-provider.md` adds that provider after core materialization exists.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/data-assets-and-materialized-outputs/README.md`
- `internal/model/data_asset.go`
- `internal/model/data_provider.go`
- `internal/model/data_location.go`
- `cmd/worker/config.go`
- `cmd/worker/worker.go`
- `cmd/worker/work_python.go`
- `cmd/worker/state.go`

Do not read controller scheduler, Slurm, SSH, rclone, Google Drive, or container files unless compile or test failures directly require them.

## Allowed Production Files

- `cmd/worker/data_asset_materializer.go`
- `cmd/worker/data_locations.go` if named-location resolution needs a separate helper
- `cmd/worker/asset_cache.go` if cache logic is clearer as a separate helper
- `cmd/worker/work_python.go`
- `cmd/worker/worker.go` only for dispatch integration
- `cmd/worker/config.go` only for max-download-size, named data-location roots, or asset-cache-root setting if needed
- `internal/model/data_asset.go` only for narrow validation changes

## Allowed Test Files

- `cmd/worker/data_asset_materializer_test.go`
- `cmd/worker/data_locations_test.go`
- `cmd/worker/asset_cache_test.go`
- `cmd/worker/work_python_test.go`
- `cmd/worker/worker_test.go` only for dispatch integration
- `cmd/worker/config_test.go` only for new config fields
- `internal/model/data_asset_test.go` only for narrow validation changes

## Out Of Scope

- Real CDL downloads.
- Real Yan/Roy tile access.
- Real Google Drive or rclone access.
- Credentialed data assets.
- S3, FTP, Globus, Google Earth Engine, or object stores.
- Decompression, archive selection, raster tiling, GDAL, rasterio, pyarrow, or geospatial processing.
- Command argument interpolation; that is a later slice.
- Published-asset copying; that is a later slice.
- Cross-worker locking beyond a simple safe local lock if it is needed to prevent duplicate fixture downloads.
- Controller persistence changes.
- Fake HPCC smoke automation.

## Acceptance Criteria

- A tiny `local_file` bound data asset can be referenced from a configured named root in a test temp directory.
- A tiny `local_file` bound data asset can be copied into the worker asset cache when strategy is `worker_cache`.
- A tiny `httptest` bound data asset can be streamed to the worker asset cache.
- A tiny `registered_location` bound asset can be resolved under a configured named root.
- `reference` strategy returns a local path under the named root without copying bytes.
- `worker_cache` strategy copies/streams bytes into the worker asset cache.
- Expected SHA-256 is verified when present.
- Expected size is verified when present.
- A mismatched SHA-256 fails the work item before the Python script runs.
- A mismatched size fails the work item before the Python script runs.
- Unsafe registered-location and local-file relative paths are rejected before filesystem reads.
- HTTP downloads use streaming reads and enforce a configured maximum size in tests/local mode.
- A cache miss writes the cached source and cache manifest atomically or with a safe incomplete-file cleanup path.
- A cache hit revalidates the cached source against the cache manifest before reuse.
- An immutable cache hit whose observed hash differs from the cache manifest fails materialization.
- A source whose expected hash differs from an existing immutable cache entry fails materialization.
- Materialization writes a valid `goet/materialized-data-assets/v1` manifest with `binding_name`, `provider_name`, `provider_type`, `local_path`, strategy, cache key, immutability flag, and evidence.
- Python execution receives `GOET_DATA_ASSETS_JSON` when assets are materialized.
- Existing Python tests without data assets still pass.
- Tests do not use external network access.
- Tests do not create large files.
- `go test ./cmd/worker` passes.

## Notes

- The first implementation may require an explicit test-only max size such as a few MiB.
- Real data-product runbooks can later raise the limit through worker configuration on HPCC.
- `local_file` means a path visible to the worker/container runtime, not a path on the controller host.
- Prefer `registered_location` plus `reference` for shared read-only datasets like preloaded Yan/Roy tiles.
- Prefer `http` plus `worker_cache` plus `immutable: true` for public CDL downloads.

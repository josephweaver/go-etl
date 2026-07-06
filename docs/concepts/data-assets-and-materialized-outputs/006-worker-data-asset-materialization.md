# 006 Worker Data Asset Materialization

Status: proposed

## Objective

Add worker-side materialization for tiny `file`, `http`, and `https` data asset fixtures using streaming reads, safe cache paths, and hash verification.

This slice proves the data asset boundary inside the worker execution environment. It must remain safe for local development and must not download real CDL or Yan/Roy data by default.

## Current State

The worker executes concrete assignments and has local runtime roots for logs, temporary files, and completed data. It does not know how to materialize declared data assets or write a `GOET_DATA_ASSETS_JSON` manifest for Python scripts.

Data asset declarations from the previous slice are model-only.

## Target State

When a work item carries data asset declarations, the worker can materialize them before dispatching the operation.

The first materializer should support:

- `file` assets by validating and copying or referencing files under allowed execution-environment roots;
- `http` and `https` assets by streaming to a worker cache path;
- optional expected size validation;
- optional expected SHA-256 validation;
- a small configurable maximum download size for tests and local runs unless explicitly overridden;
- a materialized data assets manifest written under the attempt work directory;
- `GOET_DATA_ASSETS_JSON` passed to Python when materialized assets exist.

A worker cache shape may be:

```text
<DataDir>/assets/<safe-cache-key>/asset
<DataDir>/assets/<safe-cache-key>/manifest.json
```

If `cache_key` is not provided, derive one from the canonical data asset declaration. If an expected SHA-256 is provided, include it in the cache key or verify any existing cached file before reuse.

## Concept Decision

Materialize data assets in the worker execution environment, not through the controller. This keeps full-size public rasters off the user's Windows laptop when the selected backend is HPCC.

Use streaming copy/hash. Do not read an entire asset into memory.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/data-assets-and-materialized-outputs/README.md`
- `internal/model/data_asset.go`
- `cmd/worker/config.go`
- `cmd/worker/worker.go`
- `cmd/worker/work_python.go`
- `cmd/worker/state.go`

Do not read controller scheduler, Slurm, SSH, or container files unless compile or test failures directly require them.

## Allowed Production Files

- `cmd/worker/data_asset_materializer.go`
- `cmd/worker/work_python.go`
- `cmd/worker/worker.go` only for dispatch integration
- `cmd/worker/config.go` only for a small max-download-size or asset-cache-root setting if needed
- `internal/model/data_asset.go` only for narrow validation changes

## Allowed Test Files

- `cmd/worker/data_asset_materializer_test.go`
- `cmd/worker/work_python_test.go`
- `cmd/worker/worker_test.go` only for dispatch integration
- `cmd/worker/config_test.go` only for new config fields
- `internal/model/data_asset_test.go` only for narrow validation changes

## Out Of Scope

- Real CDL downloads.
- Real Yan/Roy tile access.
- Credentialed data assets.
- S3, FTP, Globus, Google Earth Engine, or object stores.
- Decompression, raster tiling, GDAL, rasterio, pyarrow, or geospatial processing.
- Cross-worker locking beyond a simple safe local lock if it is needed to prevent duplicate fixture downloads.
- Controller persistence changes.
- Fake HPCC smoke automation.

## Acceptance Criteria

- A tiny `file` data asset can be materialized in a test temp directory.
- A tiny `httptest` asset can be streamed to the worker asset cache.
- Expected SHA-256 is verified when present.
- Expected size is verified when present.
- A mismatched SHA-256 fails the work item before the Python script runs.
- A mismatched size fails the work item before the Python script runs.
- Materialization writes a valid `goet/materialized-data-assets/v1` manifest.
- Python execution receives `GOET_DATA_ASSETS_JSON` when assets are materialized.
- Existing Python tests without data assets still pass.
- Tests do not use external network access.
- Tests do not create large files.
- `go test ./cmd/worker` passes.

## Notes

- The first implementation may require an explicit test-only max size such as a few MiB.
- Real data-product runbooks can later raise the limit through worker configuration on HPCC.
- If `file` assets are absolute paths, they must be treated as execution-environment paths and must not be assumed to exist on the controller host.

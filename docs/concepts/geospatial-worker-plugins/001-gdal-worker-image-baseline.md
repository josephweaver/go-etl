# 001 GDAL Worker Image Baseline

Status: Proposed  
Recommended model: GPT-5.3-Codex-Spark  
Reference: EC-3 / operational slice / files(4)+test+doc

## Objective

Add a GDAL-enabled GOET worker container variant that can later build and run Go/GDAL geospatial worker operations, without making the default non-GDAL repository test path require native GDAL.

This slice establishes the environment only. It must not implement raster algorithms yet.

## Current State

The current worker image is intentionally minimal. It builds the Go worker and installs only minimal runtime packages such as certificates and Python. The container documentation explicitly says Python, R, and ETL libraries should be added later when there is a work item to exercise them.

There is no verified GDAL version in the worker image.

## Target State

The repository has a GDAL-enabled worker image variant, preferably separate from the minimal worker image:

```text
containers/goetl-worker-gdal/
  Dockerfile
  README.md
  test
```

The new image can:

- run `/goetl/goetl-worker` like the normal worker image;
- run `gdalinfo --version`;
- run `ogrinfo --version`;
- compile future `-tags gdal` Go code that imports `github.com/airbusgeo/godal`;
- report the exact GDAL version in the image smoke test output.

The existing minimal worker image and tests must remain available.

## Concept Decision

Use a dedicated GDAL worker image flavor instead of forcing GDAL into every worker image immediately.

GDAL/godal code requires native libraries and cgo. Keep that dependency inside the GDAL worker image and behind a Go build tag such as:

```text
gdal
```

Default local tests must remain:

```bash
go test ./...
```

GDAL-specific tests should run only in the GDAL container path:

```bash
go test -tags gdal ./...
```

## Required Context

Read these files first:

- `containers/README.md`
- `containers/goetl-worker/Dockerfile`
- `containers/goetl-worker/test`
- `cmd/worker`
- `go.mod`
- `docs/concepts/geospatial-worker-plugins/README.md`

Also scan the completed Data Assets implementation if it introduced worker image assumptions.

## Allowed Production Files

- `containers/goetl-worker-gdal/Dockerfile`
- `containers/goetl-worker-gdal/README.md`
- `containers/goetl-worker-gdal/test`
- `containers/README.md`
- small supporting scripts under `scripts/` only if the existing container test convention requires them

## Allowed Test Files

- `containers/goetl-worker-gdal/test`
- fixture smoke files under `containers/goetl-worker-gdal/fixtures/` if needed

## Out Of Scope

- Raster metadata reads.
- Raster reprojection.
- Raster pair counting.
- Worker dispatch changes.
- Workflow schema changes.
- Real CDL/Yan/Roy data.
- Real HPCC paths, queues, modules, accounts, or hostnames.
- Replacing the existing minimal worker image.

## Acceptance Criteria

- A GDAL-enabled worker image can be built from repository sources.
- The image contains the compiled `/goetl/goetl-worker` entrypoint.
- The image test verifies `gdalinfo --version` and `ogrinfo --version`.
- The image test prints or records the exact GDAL version.
- The image includes native build dependencies needed to compile future `godal` code, or the README explicitly documents where compilation occurs.
- Existing `containers/goetl-worker/test` still works unchanged.
- Existing default `go test ./...` does not require GDAL headers or GDAL shared libraries.
- No private institutional configuration is introduced.
- `containers/README.md` documents the difference between the minimal worker and GDAL worker image.

## Notes

Prefer deterministic image behavior over unpinned `latest` tags. If the implementation uses Debian/Ubuntu packages rather than a prebuilt OSGeo image, document the base image and assert the observed GDAL version in the smoke test.

If build-time GDAL headers are available only in a builder stage, make sure the runtime stage still contains the shared libraries and command-line tools needed by the plugin.

# 007 Worker Archive Extraction and Selection

Status: proposed

## Objective

Add worker-side archive extraction as a data-asset materialization transform so a provider can acquire a source archive and expose one selected file or a selected directory to the plugin.

This slice should implement safe ZIP member extraction using tiny generated fixtures. It should reserve a `seven_zip`/`7z` extractor path for Yan/Roy-style `.7z` archives through a configured external executable, but default tests must not require a real 7z binary or real Yan/Roy data.

## Current State

The previous slice can materialize source assets from `local_file`, `http`, and `registered_location` into a local path or cache path. Some real data assets are archives rather than directly usable files.

Examples:

```text
CDL year source:          2023_30m_cdls.zip
Yan/Roy release source:   ReleaseData.7z
```

Plugin code should not have to scan a huge archive to find the one file it needs. The worker should be able to extract declared members into a safe selected-materialization directory and expose that path through `GOET_DATA_ASSETS_JSON`.

## Target State

A bound data asset may include archive semantics equivalent to:

```json
{
  "archive": {
    "type": "zip",
    "select": [
      {
        "member": "2023_30m_cdls.tif",
        "as": "cdl.tif",
        "required": true
      }
    ],
    "expose": "selected_path"
  }
}
```

After the source asset is acquired and verified, the worker:

1. creates an extraction directory under the worker asset cache or attempt work directory;
2. validates every selected archive member path;
3. validates every output `as` path;
4. extracts only selected members when the archive format supports it;
5. rejects zip-slip/path traversal attempts;
6. computes size/hash evidence for extracted files;
7. writes archive-member evidence into the materialized data asset manifest;
8. sets `local_path` to the selected file for `selected_path`, or to the selected directory for `selected_directory`.

Example materialized manifest entry:

```json
{
  "binding_name": "cropland_year",
  "provider_name": "cdl_zip",
  "provider_type": "http",
  "kind": "raster_archive",
  "format": "geotiff_zip",
  "local_path": "/data/goetl/cache/assets/cdl/2023/30m/extracted/cdl.tif",
  "materialization_strategy": "worker_cache",
  "cache_key": "cdl/2023/30m/source.zip",
  "archive_type": "zip",
  "archive_members": [
    {
      "member": "2023_30m_cdls.tif",
      "local_path": "/data/goetl/cache/assets/cdl/2023/30m/extracted/cdl.tif",
      "size_bytes": 1234,
      "sha256": "..."
    }
  ],
  "selected_size_bytes": 1234,
  "selected_sha256": "..."
}
```

For multiple selected members:

```json
{
  "archive": {
    "type": "zip",
    "select": [
      {"member": "tile.hdr", "as": "tile.hdr", "required": true},
      {"member": "tile.dat", "as": "tile.dat", "required": true}
    ],
    "expose": "selected_directory"
  }
}
```

`local_path` should point to the selected directory, and the selected directory should receive a deterministic directory manifest hash.

## Seven-Zip / 7z Reservation

The model should accept `archive.type: "seven_zip"` for `.7z` archives. The worker implementation should handle it conservatively:

```text
if seven_zip extractor is not configured:
  fail with a clear unsupported/missing-executable error before plugin execution

if seven_zip extractor is configured:
  invoke the configured executable with structured arguments
  extract only requested members when practical
  enforce extraction root safety after extraction
```

This lets the real Yan/Roy `ReleaseData.7z` workflow be expressed now without requiring every developer machine to have 7z installed.

The worker container image can later include `7z`; that belongs to the real-data or container-follow-up work, not the default unit-test path.

## Concept Decision

Archive extraction is a materialization transform, not a domain-specific CDL or Yan/Roy operation.

The extractor should be unaware of CDL and Yan/Roy. It should operate on:

```text
source archive path
archive type
selected member declarations
selected output paths
exposure mode
```

The first implementation should prove safe ZIP extraction with generated test archives. Do not introduce geospatial dependencies.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/data-assets-and-materialized-outputs/README.md`
- `docs/concepts/data-assets-and-materialized-outputs/005-data-location-provider-and-binding-model.md`
- `docs/concepts/data-assets-and-materialized-outputs/006-worker-data-asset-materialization.md`
- `internal/model/data_archive.go` if created
- `internal/model/data_asset.go`
- `cmd/worker/data_asset_materializer.go`
- `cmd/worker/asset_cache.go` if created
- `cmd/worker/evidence.go` if file/directory hash helpers exist
- `cmd/worker/config.go`

Do not read controller scheduler, transport, Slurm, SSH, or real container image files unless compile or test failures directly require them.

## Allowed Production Files

- `cmd/worker/archive_extractor.go`
- `cmd/worker/data_asset_materializer.go`
- `cmd/worker/asset_cache.go` only for extraction cache integration
- `cmd/worker/evidence.go` only for shared hash/directory-manifest helpers
- `cmd/worker/config.go` only for optional 7z executable path configuration
- `internal/model/data_archive.go` only for narrow model adjustments
- `internal/model/data_asset.go` only for materialized manifest shape adjustments

## Allowed Test Files

- `cmd/worker/archive_extractor_test.go`
- `cmd/worker/data_asset_materializer_test.go`
- `cmd/worker/asset_cache_test.go` only for extraction cache integration
- `cmd/worker/evidence_test.go` only for shared hash/directory-manifest helpers
- `cmd/worker/config_test.go` only for optional 7z executable path configuration
- `internal/model/data_archive_test.go` only for narrow model adjustments
- `internal/model/data_asset_test.go` only for materialized manifest shape adjustments

## Out Of Scope

- Real CDL ZIP downloads.
- Real Yan/Roy `ReleaseData.7z` extraction.
- Requiring 7z in default tests.
- Rclone or Google Drive access.
- GDAL, rasterio, pyarrow, numpy, or pandas.
- Raster window processing.
- General archive formats beyond ZIP and reserved 7z.
- Wildcard-heavy archive discovery unless a very small, safe, deterministic subset is needed for tests.
- Publishing extracted files as output artifacts; extraction is input materialization.
- Controller persistence changes.

## Acceptance Criteria

- A generated tiny ZIP archive can be materialized from a `local_file` or test HTTP source and have one selected file extracted.
- `expose: "selected_path"` sets `local_path` to the selected extracted file when exactly one required member is selected.
- A generated tiny ZIP archive can extract multiple selected files into a selected directory.
- `expose: "selected_directory"` sets `local_path` to the selected extraction directory.
- Extracted file size and SHA-256 are recorded in the materialized data asset manifest.
- Selected directory evidence is deterministic and uses the same ordered file-manifest hashing convention as artifact directories.
- Missing required archive members fail materialization before plugin execution.
- Missing optional archive members do not fail materialization but are not reported as extracted.
- Archive member paths containing `..`, absolute paths, backslashes, or unsafe cleaned paths are rejected.
- Output `as` paths containing `..`, absolute paths, backslashes, or unsafe cleaned paths are rejected.
- A ZIP entry that would escape the extraction root is rejected even if the selector is malicious or the archive itself is malicious.
- Archive extraction does not read the entire archive into memory.
- Existing data materialization tests without archives still pass.
- `archive.type: "seven_zip"` fails with a clear missing-extractor error when no 7z executable is configured.
- If a fake or configured 7z executable is used in tests, it is invoked with structured arguments and never through a shell string.
- `go test ./cmd/worker` passes.

## Notes

- For CDL ZIPs, the selected member name should be configurable because exact archive layouts can vary by source/version.
- For Yan/Roy, real ENVI rasters may require more than the `.hdr` file. The provider should select all required companion files once the exact archive contents are inspected.
- Do not let archive extraction become a general search engine over huge archives in phase 1. Prefer explicit member paths or narrow fixture-proven selectors.

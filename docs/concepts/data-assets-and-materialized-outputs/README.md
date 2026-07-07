# Data Assets and Materialized Outputs Strategic Concept

Status: Proposed
Cadence: CSxIx
Revision: 2026-07-06 provider/archive/cache revision

## Purpose

Add the GOET boundary for workflows that read large external data assets and produce materialized file or directory outputs without putting those bytes in the repository-source cache, SQLite logical-output fields, or the developer's local workstation.

This concept is the fast follower to dependency-aware workflows. Dependency-aware execution gives GOET the stage model needed to fan out by year, tile, or region. Data Assets and Materialized Outputs gives those stages a safe way to refer to large inputs, run plugin work against worker-local paths, promote attempt artifacts, and optionally publish selected outputs to predeclared named storage locations.

The motivating product slice is now the CDL-by-Yan/Roy field composition product:

```text
USDA CDL annual raster + Yan/Roy field-id tile raster
  -> pixel cross-tab by field_id and CDL crop_code
  -> field/year/crop composition table
  -> dominant-crop assignment table under a declared policy
  -> RCI table as a downstream deterministic transform
```

The GOET core feature is not crop classification or RCI itself. The GOET core feature is the orchestration boundary around large data:

```text
project/workflow declares data providers and publish targets
step binds concrete input assets by year/tile/region
worker materializes or references those inputs inside the execution environment
worker optionally extracts selected files from archives
plugin receives ordinary local paths
plugin writes outputs under GOET_ARTIFACT_DIR
worker promotes attempt artifacts
worker copies selected artifacts to predeclared named publish locations
controller records compact evidence only
```

## Strategic Decision

Large input assets and large produced datasets are execution-environment storage concerns, not controller-source-cache concerns.

The controller admits source-controlled code and workflow declarations. Workers materialize input data assets, promote attempt artifacts, and copy selected outputs to named data locations using bounded manifests and hashes as the compact contract back to the controller.

GOET should store compact manifests, identities, relative paths, byte counts, hashes, schemas, parameter bindings, and provenance. It should not store multi-GB rasters, Parquet datasets, ZIP files, 7z archives, or tiled outputs in SQLite.

### Revised decision: provider/archive/cache model

Do not model ordinary data access as primary workflow work item types named `get_data` and `put_data` in phase 1.

Instead, model data movement as worker runtime phases around plugin execution:

```text
resolve input data bindings   ~= get_data infrastructure
acquire or reference source asset
verify integrity
extract selected archive members when requested
run plugin work item
promote declared artifacts
publish selected artifacts    ~= put_data infrastructure
```

Explicit `materialize_data_asset` or `publish_data_asset` work item types can be added later if workflows need standalone, stage-visible data movement steps. The first implementation should keep workflow business logic focused on the plugin operation while the worker handles data plumbing.

### Provider decision

Phase 1 should explicitly support these provider families:

```text
http             public HTTP/HTTPS download using a URL template
local_file       file already available under a worker-configured named root
registered_location shared/read-only named-location reference

gdrive_rclone    Google Drive / Shared Drive download through rclone
```

`gdrive_rclone` is intentionally an adapter over an external executable. It avoids implementing Google Drive OAuth, Shared Drive edge cases, retry behavior, and transfer semantics in GOET core. The adapter boundary must be small so a future native Go Drive provider can replace it without changing workflow declarations.

### Archive decision

Archives are materialization transforms, not separate workflow work item types.

A provider may acquire a source file such as a CDL ZIP or Yan/Roy release archive. The materializer may then extract a specific file or a small declared set of files into the worker cache and expose the extracted file/directory as the data asset's `local_path`.

Phase 1 should implement `zip` extraction with safe member selection. The model should also reserve `seven_zip`/`7z` for Yan/Roy-style archives, implemented through a configured `7z` executable when available. Default unit tests must not require a real `7z` binary or the real Yan/Roy archive.

## Why This Concept Now

Dependency-aware workflows will make later stages wait for earlier stages. That creates the natural shape for data pipelines:

```text
stage 0: verify or prepare common input data locations
stage 1: fan out by year x tile using bound data assets
stage 2: merge per-tile artifact outputs
stage 3: publish the merged dataset to a named data location
stage 4: validate and report compact dataset evidence
```

The repository already has source admission and Python execution. The missing pieces are:

- large file/directory outputs as first-class materialized artifacts;
- reusable data provider templates, such as CDL by year;
- HTTP download support for public data such as CDL;
- local-file support for manually downloaded or preloaded assets;
- Google Drive support through an `rclone` adapter for LandCore-style Shared Drive data;
- archive extraction and selected member materialization;
- cache immutability and integrity checking;
- step-level data bindings, such as `cropland_year` for `year=2023`;
- worker-local data path exposure to Python command arguments;
- publication of selected artifacts to a predeclared named location.

## Goals

- Distinguish source files, data providers, bound data assets, acquired source assets, extracted materialized input assets, attempt artifacts, and published data assets.
- Keep repository-source cache scoped to admitted source files only.
- Define a shared artifact manifest model for files and directories produced by work items.
- Validate artifact and data-location relative paths with the same defensive posture used for source paths: slash-separated, relative, non-empty, non-absolute, no drive prefixes, no backslashes, no `..`, and no escaping the configured root.
- Let workers write artifacts under an attempt-local staging directory before promotion.
- Let workers promote completed artifacts into a configured worker artifact/data root only after validation and hashing.
- Let Python scripts declare produced artifacts through the existing JSON output boundary without writing controller paths.
- Let project or workflow documents define reusable data provider templates with parameterized URI/path/cache-key templates.
- Support `http`, `local_file`, `registered_location`, and `gdrive_rclone` provider declarations.
- Support cache policies with an `immutable` flag so cached data does not silently change under the same cache key.
- Support integrity checks using expected SHA-256 and expected byte count where available.
- Support archive extraction for selected members, starting with `zip` and reserving `seven_zip`/`7z` through an external executable.
- Let workflow steps bind provider templates to concrete input aliases such as `cropland_year`, `yanroy_release`, and `crop_lookup`.
- Let worker materialization expose read-only input paths through `GOET_DATA_ASSETS_JSON` and optional command interpolation such as `${data.cropland_year.local_path}`.
- Let project or workflow documents define predeclared publish targets using named storage locations and relative path templates.
- Let workers copy selected promoted artifacts to those predeclared publish targets and report compact published-asset evidence.
- Let the controller persist compact artifact and published-asset manifests as typed logical outputs and status evidence.
- Keep local development and `go test ./...` fixture-sized.
- Prove the execution boundary on fake HPCC before using institutional HPCC configuration.
- Keep real MSU, LandCore, or other private hostnames, accounts, Shared Drive names, partitions, paths, module names, and credentials out of the reusable repository.

## Non-Goals

- Implementing CDL, RCI, Yan/Roy, or crop science inside GOET core.
- Adding GDAL, rasterio, numpy, pandas, pyarrow, or geospatial dependencies to the Go runtime.
- Downloading full national CDL rasters in unit tests, local smoke tests, or default demos.
- Passing controller-local filesystem paths to workers as data asset locations.
- Making the controller a general object store, raster server, artifact download server, S3 replacement, or artifact-byte service.
- Storing artifact bytes or data asset bytes in SQLite.
- Building a general data catalog or registry service in this concept.
- Requiring automatic registration of newly published assets. Phase 1 publication means copy bytes to a predefined named location and report evidence.
- Moving artifact cleanup and retention policy into this concept beyond safe roots and manifest evidence.
- Implementing private credential propagation. Credentialed data assets should wait for the sensitive-variable propagation concept. `gdrive_rclone` can use credentials already configured inside the worker/container environment, but GOET should not transport secrets in this concept.
- Solving arbitrary storage backends before filesystem-backed, HTTP, local-file, and rclone-backed fixtures are proven.
- Requiring a real HPCC account or real Google Drive access to verify GOET core behavior.

## Architectural Context

GOET separates controller orchestration from worker execution. The controller owns admission, source resolution, queue state, scheduling decisions, and persistence. Workers receive concrete assignments, execute them, write output, and report completion.

The repository-source concept owns project/workflow/support source bytes. It admits only explicitly declared source files and can materialize those files into worker staging. Data assets are different: they are large input data products or deployment-provided files used by workflow code, not executable source.

The Python WorkItem concept gives workers a subprocess boundary:

```text
GOET_INPUT_JSON  -> structured resolved inputs
GOET_OUTPUT_JSON -> compact logical output
```

This concept extends that execution contract with artifact roots, materialized data manifests, and optional argument binding:

```text
GOET_ARTIFACT_DIR      -> attempt-local directory where the script may write artifacts
GOET_DATA_ASSETS_JSON  -> worker-written manifest of materialized input assets
GOET_OUTPUT_JSON       -> compact output that may declare produced artifacts
${data.<alias>.local_path} -> worker-resolved argv interpolation for materialized inputs
```

The worker remains the enforcement point for local filesystem paths. Python scripts receive ordinary local paths, but the controller never trusts script-declared absolute paths and never assumes a worker path is openable from the user's machine.

## Vocabulary and Ownership Boundary

### Source files

Source files are small, admitted, source-controlled execution files such as project JSON, workflow JSON, Python entrypoints, Python environment declarations, and support files.

Owned by:

- repository-source admission;
- controller source cache;
- source-bundle staging;
- source manifest roles.

Not owned by this concept except where source-backed Python scripts produce artifacts.

### Named data locations

A named data location is a deployment/project-level logical storage root. A workflow may refer to the name, but the reusable repository should not commit private real host paths.

Examples:

```json
{
  "name": "fixture_data",
  "type": "registered_location",
  "access": "read_only",
  "root_ref": "fixture_data_root"
}
```

For phase 1, the actual mapping from `fixture_data_root`, `asset_cache_root`, or `published_data_root` to a host/container path should come from worker or demo configuration, not from private institutional constants in the concept.

### Data provider templates

A data provider template is a reusable parameterized description of where data comes from and how it should become a worker-local path.

Examples:

- `cdl_zip` provider with `year` parameter and an HTTP URL template;
- `yanroy_release` provider backed by `gdrive_rclone` or `local_file`;
- `yanroy_tile_hdr` provider that selects a specific member from `ReleaseData.7z`;
- `crop_lookup` provider with no parameters and a fixture file path.

A provider template is not yet a materialized file. It is a reusable recipe for constructing a concrete bound data asset.

### Provider types

#### `http`

`http` downloads a public URL produced from `url_template` or `uri_template`. It should stream to the worker cache, enforce max-size limits, compute SHA-256 while streaming, and validate expected integrity when provided.

Example use: USDA CDL year archive.

#### `local_file`

`local_file` references or copies a file already available inside the worker execution environment under a configured named root. It is the correct first mode for manually downloaded assets, preloaded release archives, and test fixtures.

A workflow-authored `local_file` provider should not contain arbitrary absolute host paths. It should identify a configured root and a safe relative path template.

Example use: manually downloaded `ReleaseData.7z` under a worker-mounted data root.

#### `registered_location`

`registered_location` references a shared named location and safe relative path. It is best for data that is already materialized and mounted read-only for all workers.

Example use: already extracted Yan/Roy tiles on HPCC shared storage.

#### `gdrive_rclone`

`gdrive_rclone` acquires a file from a configured rclone remote. The project/workflow declaration should contain a remote name or remote reference plus a path template. Authentication, Shared Drive selection, OAuth tokens, and rclone configuration belong to the worker/container environment, not the workflow document.

Example use: LandCore Google Drive or Shared Drive files such as a Yan/Roy release archive.

### Archive extraction

Archive extraction is a post-acquisition transform. It allows a large source archive to be cached once and expose only selected files to the plugin.

The archive model should support semantics equivalent to:

```json
{
  "archive": {
    "type": "zip",
    "select": [
      {
        "member_template": "${year}_30m_cdls.tif",
        "as": "cdl.tif",
        "required": true
      }
    ],
    "expose": "selected_path"
  }
}
```

For multiple selected members, expose a selected directory:

```json
{
  "archive": {
    "type": "seven_zip",
    "select": [
      {
        "member_template": "${tile}/WELD_${tile}_${year}_field_segments.hdr",
        "as": "field_segments.hdr",
        "required": true
      },
      {
        "member_template": "${tile}/WELD_${tile}_${year}_field_segments.*",
        "as": "field_segments_related/",
        "required": false
      }
    ],
    "expose": "selected_directory"
  }
}
```

All archive member paths and extracted destination paths must be validated against zip-slip/path-traversal behavior. The extractor must never write outside the selected extraction directory.

### Cache immutability

A cache policy controls where acquired or extracted data is stored and whether a cache entry may change.

Example:

```json
{
  "cache": {
    "strategy": "worker_cache",
    "cache_key_template": "cdl/${year}/30m/source.zip",
    "immutable": true
  }
}
```

Rules:

- `immutable: true` means the same cache key must always resolve to the same byte content.
- If expected SHA-256 is provided, an existing cache entry must match it before reuse.
- If no expected SHA-256 is provided, the first successful materialization records observed evidence. Future reuse under the same cache key must match the recorded evidence.
- If the source changes under the same immutable cache key, materialization fails rather than silently overwriting the cache.
- To intentionally refresh an immutable source, the workflow should use a new cache key, version, or explicit cache-clearing operation outside this concept.

Default for `worker_cache` should be conservative: immutable unless explicitly set otherwise.

### Integrity checks

A provider may declare expected evidence:

```json
{
  "integrity": {
    "sha256": "optional-lowercase-hex",
    "size_bytes": 123456789,
    "required": false
  }
}
```

Expected hash and size should be checked on acquired source files and, when archive extraction is used, selected extracted files/directories should also receive observed evidence in the materialized manifest. If expected integrity is present and mismatches, the work item must fail before plugin execution.

### Step data bindings

A step data binding turns a provider template into a concrete input alias for a work item.

Example:

```json
{
  "data": {
    "cropland_year": {
      "provider": "cdl_zip",
      "parameters": {"year": "${vars.year}"}
    },
    "field_release": {
      "provider": "yanroy_release",
      "parameters": {"tile": "${vars.tile}"}
    }
  }
}
```

The worker materializes or references these inputs, then exposes them to the plugin as `cropland_year` and `field_release` in `GOET_DATA_ASSETS_JSON` and, where supported, as `${data.cropland_year.local_path}` and `${data.field_release.local_path}` in command arguments.

### Materialized input assets

A materialized input asset is a data binding that is actually available inside the worker execution environment.

It contains:

- binding alias;
- provider name;
- worker-local read-only path;
- source provider type;
- materialization strategy such as `reference` or `worker_cache`;
- cache key and immutability evidence when applicable;
- source byte count and hash when available;
- selected/extracted byte count and hash when archive extraction is used.

Common read-only data should usually use a shared named location and `reference` materialization. Large public downloads may use `worker_cache` so each execution environment can reuse a local copy.

### Materialized artifacts

Materialized artifacts are files or directories produced by a work item and promoted into execution-environment storage. Examples include:

- `field_cdl_composition.csv`;
- `field_dominant_crop.csv`;
- `field_rci.csv`;
- a partitioned Parquet directory;
- a JSON validation report;
- a raster tile generated by workflow code.

Owned by this concept as worker staging, promotion, hashing, manifest construction, and compact controller persistence.

A materialized artifact is not automatically a published data asset. It is attempt output evidence first.

### Published data assets

A published data asset is a selected artifact copied to a predefined named location and relative path template.

Phase 1 does not require automatic registration or a central catalog. The project or workflow can define the intended target ahead of time, and the worker only performs the controlled copy and reports evidence.

Example target:

```json
{
  "name": "field_cdl_composition_tile",
  "kind": "tabular_dataset",
  "format": "csv",
  "location": {
    "type": "registered_location",
    "name": "published_data",
    "path_template": "field_cdl_composition/year=${year}/tile=${tile}/field_cdl_composition.csv"
  },
  "parameters": ["year", "tile"]
}
```

## Target State

A Python workflow step can consume large data inputs and produce large output files without pretending those files are JSON.

The workflow declaration binds parameterized data providers to aliases. The worker resolves those bindings into safe worker-local paths. The worker may download, reference, copy, cache, verify, and extract selected archive members before plugin execution. The Python script receives local paths through arguments and/or `GOET_DATA_ASSETS_JSON`, writes candidate artifacts under `GOET_ARTIFACT_DIR`, and reports a compact artifact declaration in `GOET_OUTPUT_JSON`.

The worker validates the declaration, rejects unsafe paths, computes file and directory evidence, promotes files from attempt-local temporary storage into the configured worker artifact root, rewrites the artifact manifest to final relative paths, optionally copies selected artifacts to predeclared named publish locations, and reports compact evidence to the controller as the step's logical output.

For CDL/Yan/Roy, the natural workflow shape becomes:

```text
project: define cdl_zip(year), yanroy_release(tile/year), crop_lookup providers
project: define field_cdl_composition_tile(year,tile) publish target
stage 1: fan out year x tile field-CDL composition work items
stage 2: merge per-tile field/year/crop composition artifacts
stage 3: derive dominant-crop sequence table using a declared crop assignment policy
stage 4: derive RCI table from dominant-crop sequence table
stage 5: publish datasets to named locations
stage 6: emit validation manifest
```

The local Windows laptop submits and observes the workflow. Heavy raster work runs in Linux worker containers through fake HPCC first, then real HPCC only after the fake boundary passes.

## Example Shape

A project or workflow may define provider templates:

```json
{
  "data_providers": {
    "cdl_zip": {
      "kind": "raster_archive",
      "format": "geotiff_zip",
      "provider": "http",
      "url_template": "https://www.nass.usda.gov/Research_and_Science/Cropland/Release/datasets/${year}_30m_cdls.zip",
      "parameters": ["year"],
      "cache": {
        "strategy": "worker_cache",
        "cache_key_template": "cdl/${year}/30m/source.zip",
        "immutable": true
      },
      "integrity": {
        "sha256": "optional-but-preferred",
        "size_bytes": null
      },
      "archive": {
        "type": "zip",
        "select": [
          {
            "member_template": "${year}_30m_cdls.tif",
            "as": "cdl.tif",
            "required": true
          }
        ],
        "expose": "selected_path"
      }
    },
    "yanroy_release_local": {
      "kind": "field_boundary_archive",
      "format": "seven_zip",
      "provider": "local_file",
      "location": {
        "name": "raw_data",
        "path_template": "yanroy/ReleaseData.7z"
      },
      "cache": {
        "strategy": "worker_cache",
        "cache_key_template": "yanroy/release-data/source.7z",
        "immutable": true
      },
      "archive": {
        "type": "seven_zip",
        "select": [
          {
            "member_template": "${tile}/WELD_${tile}_${year}_field_segments.hdr",
            "as": "field_segments.hdr",
            "required": true
          }
        ],
        "expose": "selected_directory"
      }
    },
    "yanroy_release_drive": {
      "kind": "field_boundary_archive",
      "format": "seven_zip",
      "provider": "gdrive_rclone",
      "gdrive": {
        "remote": "landcore",
        "path_template": "Risk Model 2021 2 MVP Development/Data/ReleaseData.7z"
      },
      "cache": {
        "strategy": "worker_cache",
        "cache_key_template": "gdrive/landcore/yanroy/release-data/source.7z",
        "immutable": true
      },
      "archive": {
        "type": "seven_zip",
        "select": [
          {
            "member_template": "${tile}/WELD_${tile}_${year}_field_segments.hdr",
            "as": "field_segments.hdr",
            "required": true
          }
        ],
        "expose": "selected_directory"
      }
    },
    "crop_lookup": {
      "kind": "lookup_table",
      "format": "csv",
      "provider": "registered_location",
      "location": {
        "name": "fixture_data",
        "path_template": "lookups/cdl_crop_codes.csv"
      },
      "materialization": {
        "strategy": "reference"
      }
    }
  },
  "published_data_assets": {
    "field_cdl_composition_tile": {
      "kind": "tabular_dataset",
      "format": "csv",
      "location": {
        "name": "published_data",
        "path_template": "field_cdl_composition/year=${year}/tile=${tile}/field_cdl_composition.csv"
      },
      "parameters": ["year", "tile"],
      "overwrite_policy": "fail_if_exists"
    }
  }
}
```

A workflow step may bind concrete inputs and a publish target:

```json
{
  "step": "field_cdl_composition",
  "type": "python_script",
  "variables": {
    "year": 2023,
    "tile": "fixture_tile_001"
  },
  "data": {
    "cropland_year": {
      "provider": "cdl_zip",
      "parameters": {"year": "${vars.year}"}
    },
    "field_tile": {
      "provider": "yanroy_release_local",
      "parameters": {"year": "${vars.year}", "tile": "${vars.tile}"}
    },
    "crop_lookup": {
      "provider": "crop_lookup"
    }
  },
  "args": [
    "field_cdl_composition.py",
    "--cdl", "${data.cropland_year.local_path}",
    "--yanroy", "${data.field_tile.local_path}",
    "--lookup", "${data.crop_lookup.local_path}",
    "--year", "${vars.year}",
    "--tile", "${vars.tile}",
    "--out", "${artifact_dir}/field_cdl_composition.csv"
  ],
  "publish": {
    "field_cdl_composition_tile": {
      "from_artifact": "field_cdl_composition_tile",
      "target": "field_cdl_composition_tile",
      "parameters": {
        "year": "${vars.year}",
        "tile": "${vars.tile}"
      }
    }
  }
}
```

The script still writes a compact output declaration:

```json
{
  "result": "ok",
  "artifacts": [
    {
      "name": "field_cdl_composition_tile",
      "kind": "tabular_dataset",
      "format": "csv",
      "path": "field_cdl_composition.csv",
      "metadata": {
        "year": 2023,
        "field_tile_id": "fixture_tile_001",
        "crop_assignment_policy": "dominant_share_v1"
      }
    }
  ]
}
```

The worker reports materialized input evidence, attempt-artifact evidence, and publication evidence:

```json
{
  "schema": "goet/artifact-manifest/v1",
  "run_id": "run_...",
  "work_item_id": "work_...",
  "attempt_id": "attempt_...",
  "storage_scope": "worker_data_dir",
  "artifacts": [
    {
      "name": "field_cdl_composition_tile",
      "kind": "tabular_dataset",
      "format": "csv",
      "path": "artifacts/run_.../stage-001/step-002/work_.../field_cdl_composition.csv",
      "size_bytes": 12345,
      "sha256": "...",
      "metadata": {"year": 2023, "field_tile_id": "fixture_tile_001"}
    }
  ],
  "published_assets": [
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
  ],
  "script_output": {"result": "ok"}
}
```

## Artifact Manifest Contract

The first shared artifact manifest should be compact JSON, stored as the work item's logical output. Field names may change during implementation, but the concept requires equivalent semantics.

```json
{
  "schema": "goet/artifact-manifest/v1",
  "run_id": "run_...",
  "stage_index": 1,
  "step_index": 2,
  "work_item_id": "work_...",
  "attempt_id": "attempt_...",
  "storage_scope": "worker_data_dir",
  "artifacts": [
    {
      "name": "field_cdl_composition_tile",
      "kind": "tabular_dataset",
      "format": "csv",
      "path": "artifacts/run_.../stage-001/step-002/work_.../field_cdl_composition.csv",
      "content_type": "text/csv",
      "size_bytes": 12345,
      "sha256": "...",
      "record_count": 100,
      "schema_ref": "goet/table-schema/field-cdl-composition/v1",
      "metadata": {
        "year": 2023,
        "field_tile_id": "fixture_tile_001"
      }
    }
  ],
  "published_assets": []
}
```

### File artifacts

A file artifact must have exactly one safe relative `path`, a byte count, and a raw SHA-256 hash computed after final promotion.

### Directory artifacts

A directory artifact must have one safe relative `path` and a directory manifest hash. The directory manifest hash is the canonical JSON SHA-256 of an ordered list of file entries:

```json
[
  {"path":"part-000.parquet","size_bytes":123,"sha256":"..."},
  {"path":"part-001.parquet","size_bytes":456,"sha256":"..."}
]
```

The directory path itself is not enough evidence. Directory contents must be enumerated deterministically.

### Storage scope

The first artifact storage scope is:

```text
worker_data_dir
```

It means `path` is relative to the worker's configured completed-output root. Future scopes may include named shared filesystems, object stores, or controller-exposed artifact services, but this concept should prove one filesystem-backed artifact scope first.

Published assets use a different scope shape:

```text
registered_location + location_name + relative path
```

That means the project/deployment has predeclared the storage root name, and the worker copied the bytes to a path relative to that root.

## Script-Facing Artifact Output

Python scripts should not author final worker data paths or final published paths. They should write under `GOET_ARTIFACT_DIR` and report artifact paths relative to that staging root.

The worker may preserve non-artifact script output under `script_output` or an equivalent compact field so existing typed-output behavior remains useful.

## Data Provider and Binding Contract

Data provider templates are reusable declarations. Step bindings produce concrete work-item inputs.

A bound input asset should carry equivalent semantics to:

```json
{
  "binding_name": "cropland_year",
  "provider_name": "cdl_zip",
  "kind": "raster_archive",
  "format": "geotiff_zip",
  "location": {
    "type": "http",
    "uri": "https://example.invalid/2023_30m_cdls.zip"
  },
  "integrity": {
    "sha256": "optional-but-preferred",
    "size_bytes": 2000000000
  },
  "cache": {
    "strategy": "worker_cache",
    "cache_key": "cdl/2023/30m/source.zip",
    "immutable": true
  },
  "archive": {
    "type": "zip",
    "select": [
      {"member": "2023_30m_cdls.tif", "as": "cdl.tif", "required": true}
    ],
    "expose": "selected_path"
  },
  "parameters": {
    "year": 2023
  }
}
```

For the first implementation, provider templates and bindings are durable workflow facts and worker inputs. Automatic materialization should start with small local-file, HTTP `httptest`, registered-location, zip, and fake-rclone fixtures. Real CDL downloads and real Google Drive access should be run only through explicit smoke or data-product runbooks, not through default tests.

## Worker Data Asset Materialization

When a work item carries bound data assets, the worker materializes or references them before executing the operation and writes a sanitized `GOET_DATA_ASSETS_JSON` file. That file should contain only local execution-environment paths and compact evidence for assets the worker has actually verified.

Example worker-facing manifest:

```json
{
  "schema": "goet/materialized-data-assets/v1",
  "assets": [
    {
      "binding_name": "cropland_year",
      "provider_name": "cdl_zip",
      "provider_type": "http",
      "kind": "raster_archive",
      "format": "geotiff_zip",
      "local_path": "/data/goetl/cache/assets/cdl/2023/30m/extracted/cdl.tif",
      "materialization_strategy": "worker_cache",
      "cache_key": "cdl/2023/30m/source.zip",
      "cache_immutable": true,
      "source_size_bytes": 2000000000,
      "source_sha256": "...",
      "selected_size_bytes": 1999999999,
      "selected_sha256": "..."
    },
    {
      "binding_name": "crop_lookup",
      "provider_name": "crop_lookup",
      "provider_type": "registered_location",
      "kind": "lookup_table",
      "format": "csv",
      "local_path": "/shared/fixture/lookups/cdl_crop_codes.csv",
      "materialization_strategy": "reference"
    }
  ]
}
```

This is intentionally worker-facing. The controller should not assume it can open those paths from the user's Windows machine.

## Command Argument Binding

A worker may render data path tokens after materialization. The preferred form is explicit and structured:

```text
${data.cropland_year.local_path}
${data.field_tile.local_path}
${artifact_dir}
```

Avoid nested path-like replacement such as:

```text
${data/cdl/${tile}}
```

The resolution order should be:

```text
1. compile ordinary workflow variables and fan-out values
2. bind provider templates to concrete work-item data assets
3. worker materializes or references assets
4. worker renders data-path tokens into command argv entries
5. plugin executes with ordinary local filesystem paths
```

Do not implement this as shell expansion. Render structured tokens in argv fields before subprocess execution.

## Published Data Asset Contract

Publishing copies selected promoted artifacts to a predefined named location. It does not require a runtime catalog write.

A publish binding should carry equivalent semantics to:

```json
{
  "name": "field_cdl_composition_tile",
  "from_artifact": "field_cdl_composition_tile",
  "target": "field_cdl_composition_tile",
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
```

The worker should copy from the promoted artifact into a temporary destination under the target location, then atomically reveal or complete the target when possible. It should hash the copied target and report compact evidence.

Default overwrite behavior should be conservative:

```text
fail_if_exists
```

A later option such as `replace_if_same_hash` may be useful, but destructive overwrite should not be the default.

## Path Safety

All user-authored, workflow-authored, script-authored, archive-member, rclone-path, or template-rendered relative paths must be validated before any file operation. The validator should reject:

- empty paths;
- absolute paths;
- Windows drive-qualified paths;
- backslashes;
- `.` or `..` segments;
- paths that escape the intended root after cleaning;
- reserved metadata paths such as `.goet/...` unless the worker generated them.

The worker should copy or rename only from attempt-local staging to a configured completed-output root, from provider sources into worker cache, from archive cache into selected extraction directories, and from promoted artifact roots to configured named publish locations. The controller should never instruct a worker to promote, publish, or extract into an arbitrary host path.

For `gdrive_rclone`, invoke the executable with structured arguments. Do not construct shell strings.

## Memory and Local Development Policy

The user's local workstation is not the proof target for full-size rasters. Core tests and local smoke paths must remain tiny.

Required policy:

- `go test ./...` must not download external raster data.
- `go test ./...` must not require real Google Drive credentials, real rclone remotes, or real `7z` archives.
- Local fixtures should be generated in test temp directories or checked in only when very small.
- HTTP provider tests should use `httptest` or equivalent local test servers.
- `gdrive_rclone` tests should use a fake executable that records arguments and copies a local fixture.
- Archive tests should use tiny generated ZIP files.
- Fake HPCC smoke should use small files that prove data binding, archive extraction, artifact promotion, publication copying, hashing, and Slurm/Singularity execution without stressing RAM or disk.
- Real CDL/Yan/Roy downloads must live behind explicit runbook commands, explicit backend selection, and explicit output roots.
- Raster processing workflow code must be windowed or tiled. It must not load an entire national CDL raster or the full Yan/Roy CONUS field raster into memory.
- Workflow fan-out should be bounded by variables such as years, tile IDs, and worker max count.
- Unit tests should assert path and manifest correctness, not geospatial performance.

## Fake HPCC and Real HPCC Utilization

This is the right time to utilize the existing fake-HPCC execution environment, but only after the local artifact, data-binding, provider materialization, archive extraction, and publication contracts exist.

Recommended progression:

```text
1. local unit tests for artifact, data provider/binding, cache, archive, and published asset models
2. local worker smoke with tiny local_file/http/registered_location bound data assets and tiny artifacts
3. local worker smoke with tiny ZIP extraction and selected-file exposure
4. local worker smoke with fake-rclone executable if gdrive_rclone is enabled
5. local worker smoke that publishes one artifact to a named fixture location
6. fake HPCC Slurm/Singularity smoke with tiny bound data assets and artifact publication
7. fake HPCC dependency-aware fan-out smoke by year x tile fixtures
8. real HPCC dry run with no private site details committed
9. real CDL/Yan/Roy bounded tile/year run
```

The fake HPCC path should prove GOET core mechanics:

- worker launch through configured execution environment;
- Slurm script generation;
- Singularity worker runtime;
- worker pull from controller;
- read-only fixture data root mounted inside the worker runtime;
- asset cache root mounted inside the worker runtime;
- artifact root mounted inside the worker runtime;
- published-data root mounted inside the worker runtime;
- data materialization/reference and command argument binding;
- archive extraction and selected local path exposure;
- artifact promotion and manifest reporting;
- selected artifact publication to a named location;
- status/log visibility after completion.

The real HPCC path should consume the same abstractions with site-specific configuration kept outside the reusable repository.

## CDL / Yan/Roy Vertical Slice Shape

The first product-facing proof should use fixture rasters or matrix files, not the national datasets.

Algorithmic shape:

```text
for each target year:
  for each Yan/Roy field-id tile:
    materialize CDL year asset from HTTP cache or fixture
    materialize Yan/Roy tile/release asset from local_file, registered_location, or gdrive_rclone
    extract selected files when the source is an archive
    read the matching CDL window or fixture window
    read field_id values from the field raster
    ignore background/nodata field IDs
    count pixels by (field_id, crop_code)
    compute field_pixel_count and crop_fraction
    mark dominant crop_code under a declared policy
    emit field/year/crop composition rows
    optionally emit one dominant-crop row per field/year
    publish the tile output to field_cdl_composition/year=<year>/tile=<tile>/...
```

The first engineering artifact should keep the distribution, not only the majority class:

```text
field_id
field_definition_version
field_tile_id
year
crop_code
crop_type
field_pixel_count
crop_pixel_count
crop_fraction
is_dominant_crop
dominant_crop_code
dominant_crop_type
dominant_crop_fraction
assignment_policy
cdl_resolution_m
cdl_source_asset
field_source_asset
```

The downstream RCI product should consume a declared crop-assignment policy, not raw CDL pixels directly. Example policy fields:

```yaml
crop_assignment_policy:
  name: dominant_share_v1
  method: dominant_share
  min_valid_ag_share: 0.50
  tie_policy: mark_ambiguous
  non_ag_codes: [0, 81, 82, 83, 86, 88]
  perennial_codes: [36, 37, 61, 176, 224]
```

The crop-type lookup table and crop-assignment policy should be declared source files or bound data assets, not hard-coded into GOET core.

## Relationship To Other Concepts

- `dependency-aware-workflows` is the orchestration prerequisite for downstream artifact consumption and staged fan-out/fan-in.
- `python-workitem` provides the initial script execution boundary used to produce artifact manifests and consume worker-local data paths.
- `source-control-resolution-and-cache` remains the boundary for source files only. Data assets must not expand the source cache into a multi-GB data lake.
- `workflow-execution-persistence` stores compact logical outputs and lifecycle evidence; this concept supplies the compact artifact and published-asset evidence shape for large outputs.
- `resource-constraint` can later gate shared filesystem, network download, archive extraction, cache pressure, and publish-location write pressure.
- `sensitive-variable-propagation` is required before GOET transports private credentials. `gdrive_rclone` in this concept assumes credentials are preconfigured in the worker/container runtime.
- `controller-retention-cleanup` should eventually define artifact/cache-retention policy after this concept defines manifest evidence.
- A future data catalog concept may turn published-asset evidence into a searchable registry, but that is not required for phase 1 publication.

## Proposed Slices

1. `001-artifact-manifest-model-and-path-safety.md` — add the shared artifact manifest model and safe relative path validation.
2. `002-worker-artifact-staging-and-promotion.md` — add worker attempt-local artifact staging and validated promotion into the worker data root.
3. `003-python-artifact-output-contract.md` — extend Python execution with `GOET_ARTIFACT_DIR` and artifact-output validation through `GOET_OUTPUT_JSON`.
4. `004-controller-artifact-output-recording.md` — persist and surface compact artifact manifests as typed logical outputs/status evidence.
5. `005-data-location-provider-and-binding-model.md` — add named locations, provider templates, step data bindings, provider-specific declarations, archive-selection declarations, cache immutability, integrity checks, and publish-target model types.
6. `006-worker-data-asset-materialization.md` — materialize or reference tiny `local_file`, `http`, and `registered_location` data fixtures in the worker execution environment with streaming verification and immutable cache behavior.
7. `007-worker-archive-extraction-and-selection.md` — extract selected files from cached archives, starting with ZIP and reserving 7z through a configured external executable.
8. `008-gdrive-rclone-data-provider.md` — add the `gdrive_rclone` provider adapter using a configured rclone executable and fake-rclone tests.
9. `009-python-data-argument-binding.md` — render `${data.<alias>.local_path}` and `${artifact_dir}` command arguments after materialization.
10. `010-published-data-asset-copy-to-named-location.md` — copy selected promoted artifacts to predeclared named locations and report published-asset evidence.
11. `011-fake-hpcc-artifact-and-data-asset-smoke-path.md` — prove data binding, provider materialization, archive extraction, artifact production, and publication through fake HPCC Slurm/Singularity without real raster data.
12. `012-cdl-yanroy-fixture-pipeline.md` — add a fixture-sized field/CDL composition pipeline using tiny raster-like bound inputs, artifact outputs, and published fixture outputs.
13. `013-concept-closure-and-documentation-sync.md` — close the concept phase and document deferred real-data, catalog, retention, and credential work.

## Completion Criteria

- Artifact manifests have a shared versioned JSON shape with validation tests.
- File and directory artifact paths are validated before filesystem operations.
- Python scripts can write under `GOET_ARTIFACT_DIR` and report artifact declarations through `GOET_OUTPUT_JSON`.
- The worker promotes declared artifacts from attempt-local staging to the configured completed-output root and reports hashes/byte counts.
- The controller stores compact artifact output JSON without storing large artifact bytes.
- Data locations, provider templates, step data bindings, archive selection, cache policies, integrity expectations, and publish targets can be represented and validated without source-cache involvement.
- `local_file`, `http`, and `registered_location` worker-side materialization are proven with tiny fixtures, streaming hash checks, and immutable cache behavior.
- ZIP archive extraction can expose one selected file or a selected directory with safe member-path handling.
- `seven_zip`/`7z` is documented and model-valid as an external-executable extractor, with clear failure when the executable is not configured.
- `gdrive_rclone` can acquire a tiny test asset through a fake rclone executable without real Google credentials.
- Python command arguments can receive resolved worker-local data paths without shell expansion.
- Selected artifacts can be copied to predeclared named publish locations with conservative overwrite policy and post-copy evidence.
- Downstream dependency-aware stages can receive upstream artifact/published-asset manifests as ordinary typed logical outputs.
- Fake HPCC proves the same data-binding, materialization, archive, artifact-promotion, and publication path through Slurm/Singularity worker launch.
- No default test or smoke path downloads CDL, contacts Google Drive, or requires multi-GB public data.
- Documentation explains how the later CDL/Yan/Roy full-data run should move to real HPCC without committing site-specific configuration.

## Open Questions

- Whether `DataDir` should remain the default artifact root while named publish locations and asset cache roots are configured separately, or whether distinct optional `ArtifactDir` and `AssetCacheDir` fields should be introduced immediately.
- Whether provider templates belong only in project documents or can also be declared/overridden in workflow documents.
- Whether compiled work items should carry only concrete bound data assets or also retain provider-template names for status/debugging.
- Whether `${data.<alias>}` should be a shorthand for `${data.<alias>.local_path}` or whether all data references should require explicit `.local_path`.
- Whether publication evidence should be nested inside the artifact manifest or returned as a sibling `goet/published-data-assets/v1` manifest.
- Whether later standalone `materialize_data_asset` and `publish_data_asset` work item types are worth adding after the worker-phase implementation proves useful.
- Whether the first real Yan/Roy workflow should consume a manually downloaded `local_file` `ReleaseData.7z`, a `gdrive_rclone` download, or an already-extracted `registered_location` tile set.
- Whether 7z extraction should be implemented immediately in the worker container image or deferred until the first real Yan/Roy runbook.

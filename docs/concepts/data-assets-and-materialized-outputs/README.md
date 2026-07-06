# Data Assets and Materialized Outputs Strategic Concept

Status: Proposed
Cadence: CSxIx

## Purpose

Add the GOET boundary for workflows that read large external data assets and produce materialized file or directory outputs without putting those bytes in the repository-source cache, SQLite logical-output fields, or the developer's local workstation.

This concept is the fast follower to dependency-aware workflows. Dependency-aware execution gives GOET the stage model needed to fan out by year, tile, or region. Data Assets and Materialized Outputs gives those stages a safe way to refer to large inputs and durable produced outputs.

The motivating product slice is:

```text
USDA CDL year raster + Yan/Roy field-id tile raster
  -> windowed raster cross-tab by field_id and CDL crop_code
  -> field_crop_year dataset with field_id, crop_type, year
```

The GOET core feature is not crop classification. The GOET core feature is the orchestration boundary around large data: declare inputs, materialize them inside the execution environment, run bounded work items, promote produced artifacts, and report compact metadata back to the controller.

## Strategic Decision

Large data assets and large produced datasets are execution-environment storage concerns, not controller-source-cache concerns.

The controller admits source-controlled code and workflow declarations. Workers materialize data assets and produced artifacts inside their configured runtime storage, using bounded manifests and hashes as the contract back to the controller.

GOET should store compact manifests, identities, relative paths, byte counts, hashes, schemas, and provenance. It should not store multi-GB rasters, Parquet datasets, ZIP files, or tiled outputs in SQLite.

## Why This Concept Now

Dependency-aware workflows will make later stages wait for earlier stages. That creates the natural shape for data pipelines:

```text
stage 0: materialize or verify data assets
stage 1: fan out by year x tile
stage 2: merge tile/year artifacts
stage 3: validate and publish a compact dataset manifest
```

The repository already has source admission and Python execution. The missing piece is that Python can currently return one logical JSON output, but CDL/Yan/Roy work needs large files and directories to become first-class execution results.

## Goals

- Distinguish source files, data assets, and materialized artifacts.
- Keep repository-source cache scoped to admitted source files only.
- Define a shared artifact manifest model for files and directories produced by work items.
- Validate artifact paths with the same defensive posture used for source paths: slash-separated, relative, non-empty, non-absolute, no drive prefixes, no backslashes, no `..`, and no escaping the configured root.
- Let workers write artifacts under an attempt-local staging directory before promotion.
- Let workers promote completed artifacts into a configured data/artifact root only after validation and hashing.
- Let Python scripts declare produced artifacts through the existing JSON output boundary without writing controller paths.
- Let the worker rewrite script-declared staging paths into final storage-scope-relative artifact paths.
- Let the controller persist compact artifact manifests as typed logical outputs and status evidence.
- Define data asset declarations for public or deployment-provided inputs such as CDL downloads and Yan/Roy raster tiles.
- Let data asset materialization run in the worker execution environment, not on the local Windows controller machine by default.
- Keep local development and `go test ./...` fixture-sized.
- Prove the execution boundary on fake HPCC before using institutional HPCC configuration.
- Keep real MSU or other institutional hostnames, accounts, partitions, paths, module names, and credentials out of the reusable repository.

## Non-Goals

- Implementing CDL or Yan/Roy domain science inside GOET core.
- Adding GDAL, rasterio, numpy, pandas, pyarrow, or geospatial dependencies to the Go runtime.
- Downloading full national CDL rasters in unit tests, local smoke tests, or default demos.
- Passing controller-local filesystem paths to workers as data asset locations.
- Making the controller a general object store, raster server, artifact download server, or S3 replacement.
- Storing artifact bytes in SQLite.
- Moving artifact cleanup and retention policy into this concept beyond safe roots and manifest evidence.
- Implementing private credential propagation. Credentialed data assets should wait for the sensitive-variable propagation concept.
- Solving arbitrary storage backends before local/shared filesystem storage is proven.
- Requiring a real HPCC account to verify GOET core behavior.

## Architectural Context

GOET already separates controller orchestration from worker execution. The controller owns admission, source resolution, queue state, scheduling decisions, and persistence. Workers receive concrete assignments, execute them, write output, and report completion.

The repository-source concept owns project/workflow/support source bytes. It admits only explicitly declared source files and can materialize those files into worker staging. Data assets are different: they are large input data products or deployment-provided files used by workflow code, not executable source.

The Python WorkItem concept gives workers a subprocess boundary:

```text
GOET_INPUT_JSON  -> structured resolved inputs
GOET_OUTPUT_JSON -> compact logical output
```

This concept extends that execution contract with artifact roots and manifests:

```text
GOET_ARTIFACT_DIR      -> attempt-local directory where the script may write artifacts
GOET_DATA_ASSETS_JSON  -> optional worker-written manifest of materialized input assets
GOET_OUTPUT_JSON       -> compact output that may declare produced artifacts
```

The worker remains the enforcement point for local filesystem paths. Python scripts write under directories the worker gives them; the controller never trusts a script-declared absolute path.

## Ownership Boundary

### Source files

Source files are small, admitted, source-controlled execution files such as project JSON, workflow JSON, Python entrypoints, Python environment declarations, and support files.

Owned by:

- repository-source admission;
- controller source cache;
- source-bundle staging;
- source manifest roles.

Not owned by this concept except where source-backed Python scripts produce artifacts.

### Data assets

Data assets are external or deployment-provided inputs such as CDL downloads, Yan/Roy field-id raster tiles, lookup tables, or other large files needed by work items.

Owned by this concept as declarations, worker materialization contracts, materialized asset manifests, and provenance evidence.

Data asset bytes are not source-cache bytes. A data asset declaration may point to `https`, `file`, or a future storage provider, but the first implementation should materialize only through worker-safe roots and tiny test fixtures.

### Materialized artifacts

Materialized artifacts are files or directories produced by a work item and promoted into execution-environment storage. Examples include:

- `field_crop_year.csv`;
- a partitioned Parquet directory;
- a tile-level crop-count table;
- a JSON validation report;
- a raster tile generated by workflow code.

Owned by this concept as worker staging, promotion, hashing, manifest construction, and compact controller persistence.

## Target State

A Python workflow step can produce large output files without pretending those files are JSON. The script writes candidate artifacts under `GOET_ARTIFACT_DIR` and reports a compact artifact declaration in `GOET_OUTPUT_JSON`.

The worker validates the declaration, rejects unsafe paths, computes file and directory evidence, promotes files from attempt-local temporary storage into the configured data/artifact root, rewrites the artifact manifest to final relative paths, and reports that compact manifest to the controller as the step's logical output.

A downstream dependency-aware workflow step can consume the upstream manifest through `workflow.step[index]` and receive concrete artifact references as already-resolved work-item parameters.

For CDL/Yan/Roy, the natural workflow shape becomes:

```text
stage 0: declare/materialize CDL year asset and Yan/Roy field-id tile assets
stage 1: fan out year x tile cross-tab work items
stage 2: merge per-tile crop-count artifacts
stage 3: emit field_crop_year dataset manifest
```

The local Windows laptop submits and observes the workflow. Heavy raster work runs in Linux worker containers through fake HPCC first, then real HPCC only after the fake boundary passes.

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
      "name": "field_crop_year_tile",
      "kind": "tabular_dataset",
      "format": "csv",
      "path": "artifacts/run_.../stage-001/step-002/work_.../field_crop_year.csv",
      "content_type": "text/csv",
      "size_bytes": 12345,
      "sha256": "...",
      "record_count": 100,
      "schema_ref": "goet/table-schema/field-crop-year/v1",
      "metadata": {
        "year": 2023,
        "field_tile_id": "fixture_tile_001"
      }
    }
  ]
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

The first storage scope is:

```text
worker_data_dir
```

It means `path` is relative to the worker's configured completed-output root. Future scopes may include named shared filesystems, object stores, or controller-exposed artifact services, but this concept should prove one filesystem-backed scope first.

## Script-Facing Artifact Output

Python scripts should not author final worker data paths. They should write under `GOET_ARTIFACT_DIR` and report artifact paths relative to that staging root.

Script-authored output example:

```json
{
  "result": "ok",
  "artifacts": [
    {
      "name": "field_crop_year_tile",
      "kind": "tabular_dataset",
      "format": "csv",
      "path": "field_crop_year.csv",
      "metadata": {
        "year": 2023,
        "field_tile_id": "fixture_tile_001"
      }
    }
  ]
}
```

Worker-reported output example after validation and promotion:

```json
{
  "schema": "goet/artifact-manifest/v1",
  "run_id": "run_...",
  "work_item_id": "work_...",
  "attempt_id": "attempt_...",
  "storage_scope": "worker_data_dir",
  "artifacts": [
    {
      "name": "field_crop_year_tile",
      "kind": "tabular_dataset",
      "format": "csv",
      "path": "artifacts/run_.../stage-001/step-002/work_.../field_crop_year.csv",
      "size_bytes": 12345,
      "sha256": "...",
      "metadata": {
        "year": 2023,
        "field_tile_id": "fixture_tile_001"
      }
    }
  ],
  "script_output": {
    "result": "ok"
  }
}
```

The worker may preserve non-artifact script output under `script_output` or an equivalent compact field so existing typed-output behavior remains useful.

## Data Asset Declaration Contract

A data asset declaration describes an input the workflow expects a worker to materialize or verify. It is not a source file and is not a controller cache path.

```json
{
  "schema": "goet/data-assets/v1",
  "assets": [
    {
      "name": "cdl_2023_30m",
      "kind": "raster",
      "format": "geotiff_zip",
      "location": {
        "type": "https",
        "uri": "https://example.invalid/cdl_2023_30m.zip"
      },
      "expected": {
        "sha256": "optional-but-preferred",
        "size_bytes": 2000000000
      },
      "materialization": {
        "strategy": "worker_cache",
        "cache_key": "cdl/2023/30m"
      }
    }
  ]
}
```

For the first implementation, data asset declarations are durable facts and worker inputs. Automatic materialization should start with small local/file/HTTP fixtures. Real CDL downloads should be run only through explicit smoke or data-product runbooks, not through default tests.

## Worker Data Asset Materialization

When a work item carries data asset declarations, the worker may materialize them before executing the operation and write a sanitized `GOET_DATA_ASSETS_JSON` file. That file should contain only local execution-environment paths and compact evidence for assets the worker has actually verified.

Example worker-facing manifest:

```json
{
  "schema": "goet/materialized-data-assets/v1",
  "assets": [
    {
      "name": "cdl_2023_30m",
      "kind": "raster",
      "format": "geotiff_zip",
      "path": "/data/goetl/assets/cdl/2023/30m/cdl_2023_30m.zip",
      "size_bytes": 2000000000,
      "sha256": "..."
    }
  ]
}
```

This is intentionally worker-facing. The controller should not assume it can open that path from the user's Windows machine.

## Path Safety

All user-authored or script-authored relative artifact paths must be validated before any file operation. The validator should reject:

- empty paths;
- absolute paths;
- Windows drive-qualified paths;
- backslashes;
- `.` or `..` segments;
- paths that escape the intended root after cleaning;
- reserved metadata paths such as `.goet/...` unless the worker generated them.

The worker should copy or rename only from attempt-local staging to a configured completed-output root. The controller should never instruct a worker to promote an arbitrary host path.

## Memory and Local Development Policy

The user's local workstation is not the proof target for full-size rasters. Core tests and local smoke paths must remain tiny.

Required policy:

- `go test ./...` must not download external raster data.
- Local fixtures should be generated in test temp directories or checked in only when very small.
- The fake HPCC smoke should use small files that prove artifact promotion, hashing, and Slurm/Singularity execution without stressing RAM or disk.
- Real CDL/Yan/Roy downloads must live behind explicit runbook commands, explicit backend selection, and explicit output roots.
- Raster processing workflow code must be windowed or tiled. It must not load an entire national CDL raster or the full Yan/Roy CONUS field raster into memory.
- Workflow fan-out should be bounded by variables such as years, tile IDs, and worker max count.
- Unit tests should assert path and manifest correctness, not geospatial performance.

## Fake HPCC and Real HPCC Utilization

Yes: this is the right time to utilize the existing fake-HPCC execution environment, but only after the local artifact contract exists.

Recommended progression:

```text
1. local unit tests for artifact and data asset models
2. local worker smoke with tiny artifacts
3. fake HPCC Slurm/Singularity smoke with tiny artifacts
4. fake HPCC dependency-aware fan-out smoke by year x tile fixtures
5. real HPCC dry run with no private site details committed
6. real CDL/Yan/Roy bounded tile/year run
```

The fake HPCC path should prove GOET core mechanics:

- worker launch through configured execution environment;
- Slurm script generation;
- Singularity worker runtime;
- worker pull from controller;
- artifact root mounted inside the worker runtime;
- artifact promotion and manifest reporting;
- status/log visibility after completion.

The real HPCC path should consume the same abstractions with site-specific configuration kept outside the reusable repository.

## CDL / Yan/Roy Vertical Slice Shape

The first product-facing proof should use fixture rasters or matrix files, not the national datasets.

Algorithmic shape:

```text
for each target year:
  for each Yan/Roy field-id tile:
    read the matching CDL window or fixture window
    read field_id values from the field raster
    ignore background/nodata field IDs
    count pixels by (field_id, crop_code)
    select majority crop_code for each field_id
    emit field_id, crop_code, crop_type, year, counts, crop_fraction
```

The first public dataset can expose the narrow columns:

```text
field_id, crop_type, year
```

The first engineering artifact should keep QA columns:

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
cdl_resolution_m
cdl_source_asset
field_source_asset
```

The crop-type lookup table should be a declared source or data asset, not hard-coded into GOET core.

## Relationship To Other Concepts

- `dependency-aware-workflows` is the orchestration prerequisite for downstream artifact consumption and staged fan-out/fan-in.
- `python-workitem` provides the initial script execution boundary used to produce artifact manifests.
- `source-control-resolution-and-cache` remains the boundary for source files only. Data assets must not expand the source cache into a multi-GB data lake.
- `workflow-execution-persistence` stores compact logical outputs and lifecycle evidence; this concept supplies the compact artifact manifest shape for large outputs.
- `resource-constraint` can later gate shared filesystem, network download, or HPCC resource pressure.
- `sensitive-variable-propagation` is required before private or credentialed data assets are supported.
- `controller-retention-cleanup` should eventually define artifact-retention policy after this concept defines manifest evidence.

## Proposed Slices

1. `001-artifact-manifest-model-and-path-safety.md` — add the shared artifact manifest model and safe relative path validation.
2. `002-worker-artifact-staging-and-promotion.md` — add worker attempt-local artifact staging and validated promotion into the worker data root.
3. `003-python-artifact-output-contract.md` — extend Python execution with `GOET_ARTIFACT_DIR` and artifact-output validation through `GOET_OUTPUT_JSON`.
4. `004-controller-artifact-output-recording.md` — persist and surface compact artifact manifests as typed logical outputs/status evidence.
5. `005-data-asset-declaration-model.md` — add data asset declaration and materialized-asset manifest models.
6. `006-worker-data-asset-materialization.md` — materialize tiny file/HTTP data asset fixtures in the worker execution environment with streaming verification.
7. `007-fake-hpcc-artifact-smoke-path.md` — prove artifact production through fake HPCC Slurm/Singularity without real raster data.
8. `008-cdl-yanroy-fixture-pipeline.md` — add a fixture-sized field/crop/year pipeline using tiny raster-like inputs and artifact outputs.
9. `009-concept-closure-and-documentation-sync.md` — close the concept phase and document deferred real-data and retention work.

## Completion Criteria

- Artifact manifests have a shared versioned JSON shape with validation tests.
- File and directory artifact paths are validated before filesystem operations.
- Python scripts can write under `GOET_ARTIFACT_DIR` and report artifact declarations through `GOET_OUTPUT_JSON`.
- The worker promotes declared artifacts from attempt-local staging to the configured completed-output root and reports hashes/byte counts.
- The controller stores compact artifact output JSON without storing large artifact bytes.
- Downstream dependency-aware stages can receive upstream artifact manifests as ordinary typed logical outputs.
- Data asset declarations can be represented and validated without source-cache involvement.
- Worker-side fixture asset materialization is proven with tiny files and streaming hash checks.
- Fake HPCC proves the same artifact path through Slurm/Singularity worker launch.
- No default test or smoke path downloads CDL or other multi-GB public data.
- Documentation explains how the later CDL/Yan/Roy full-data run should move to real HPCC without committing site-specific configuration.

## Open Questions

- Whether the worker's existing `DataDir` should remain the only promoted-artifact root in phase 1 or whether a distinct optional `ArtifactDir` should be introduced immediately.
- Whether data asset declarations should attach directly to `model.WorkItem` or remain resolved parameters until a second asset provider exists.
- Whether controller status should show artifact counts only or selected artifact names and paths in phase 1.

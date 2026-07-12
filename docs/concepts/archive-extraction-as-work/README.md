# Archive Extraction and Creation as Explicit Workflow Work

Status: Proposed  
Cadence: `C(SI)x`  
Target repository: `josephweaver/go-etl`  
Reviewed against repository `main`: 2026-07-12

## Purpose

Move archive-member extraction and archive creation into explicit workflow
work.

The project should distinguish these actions:

```text
asset.materialize   acquire one concrete external object, such as a ZIP file
archive.extract     extract one or more selected members from that object
archive.create      create an archive from one or more concrete files/directories
```

This keeps data assets focused on external objects and makes one-download,
many-extract workflows visible in the DAG. A workflow that downloads one Yan/Roy
release archive and extracts many extensionless raster files plus `.hdr`
companions should model that as one materialized archive followed by many
explicit extraction work items.

The symmetric creation operation lets workflows package generated outputs or
prepared input bundles without pushing archive semantics into domain-specific
plugins.

## Goals

- Treat archive files as ordinary data assets whose materialized result is the
  archive file itself.
- Remove archive-member roles, `select`, and `archive.expose` from the public
  data-asset authoring contract.
- Add an explicit archive-extraction work-item contract for extracting selected
  archive members from a materialized archive path.
- Add an explicit archive-creation work-item contract for creating a ZIP archive
  from selected local files or directories.
- Let one archive materialization feed many extraction work items through normal
  workflow dependencies, fan-out, and `parallel_with` behavior.
- Let related archive members, such as `yan-roy` and `yan-roy.hdr`, be modeled
  as separate extraction outputs rather than as hidden file roles inside one
  data asset.
- Let generated outputs be packaged by `archive.create` before later
  publication, transfer, or downstream workflow use.
- Reuse the existing safe archive extraction mechanics where possible.
- Keep provider download, integrity verification, source-cache, and
  deterministic materialization behavior owned by `asset.materialize`.
- Keep archive operations generic. They should understand archive paths, member
  selections, and entry mappings, not EPA, CDL, Yan/Roy, or geospatial formats.

## Non-Goals

- Do not preserve backwards compatibility for the current archive-in-data-asset
  JSON shape. There are no production workflows that require it.
- Do not keep `files`, `select`, or `binding.archive` as a long-term public
  authoring surface for archive members.
- Do not make `asset.materialize` extract archive members.
- Do not add hidden extraction work merely because a compute step references an
  archive member.
- Do not add hidden archive creation work merely because a workflow publishes a
  directory.
- Do not build a mutable global archive catalog.
- Do not discover remote provider contents.
- Do not add broad compression-format support in the first pass. ZIP is enough
  to prove the core contract.
- Do not require real CDL, EPA, or Yan/Roy downloads in default tests.
- Do not add GDAL, rasterio, numpy, pandas, pyarrow, or other geospatial
  dependencies to the Go runtime.
- Do not change `commit_data`.
- Do not rename `asset.materialize`.

## Architectural Context

This Strategic Concept refines the data-execution boundary described by:

```text
docs/concepts/data-assets-and-materialized-outputs/README.md
docs/concepts/data-asset-collections-and-materialization/README.md
```

The current data-asset collection concept makes `asset.materialize` the visible
operation for acquiring and placing data assets. This concept narrows that
operation: materialization acquires and places a concrete source object. If the
object is an archive, interpreting its members belongs to a later workflow step.

The ownership boundary after this concept should be:

```text
internal/model       transport and validation contracts for archive.extract and archive.create
internal/workflow    compilation of authored archive operation work
cmd/controller       dependency release, completion recording, and hydration
cmd/worker           archive extraction/creation, output evidence, and result manifesting
```

Provider acquisition remains outside archive extraction:

```text
cmd/worker/data_asset_materializer.go
```

Archive mechanics may continue to use the existing low-level helpers in:

```text
cmd/worker/archive_extractor.go
```

## Current State

### Strategic state

GOET already has explicit data materialization, source caching, integrity
evidence, deterministic destination placement, and finite asset collections.
However, archive-member extraction is still embedded inside the data-asset
definition and worker materializer. That makes the public JSON format harder to
read because one data input tries to describe both the provider object and a
member selected from inside that object.

### Operational state

The current repository has these concrete behaviors:

- `internal/model/data_definition.go` defines `DataInputDefinition.Files`,
  `DataInputDefinition.Select`, and `DataInputBindingDefinition.Archive`.
- `DataInputDefinition.archiveTemplate` turns selected file roles into a
  `DataAssetArchiveTemplate`.
- `cmd/worker/data_asset_materializer.go` acquires provider sources, verifies
  integrity, writes or reuses the worker source cache, and then calls
  `extractArchive` when `asset.Archive` is present.
- `cmd/worker/archive_extractor.go` contains the low-level ZIP and 7z extraction
  mechanics.
- `asset.materialize` may return the selected member path or selected directory
  rather than the archive file that was acquired.
- There is no first-class `archive.create` work operation. Archive creation, if
  needed, would currently have to live inside a domain plugin or ad hoc script.
- The EPA AQI demo workflow has to express one CSV inside a ZIP with data-input
  fields such as `files.table`, `select`, and `archive.expose`, even though the
  workflow author is really describing two actions: download the ZIP, then
  extract the CSV.

## Target State

### Strategic state

Data assets describe concrete external objects. A ZIP, 7z file, CSV, TIFF, JSON
file, or extensionless binary file can each be a data asset, but data assets do
not describe archive-member extraction.

Archive-member extraction is a normal workflow operation with explicit inputs
and outputs. It appears in workflow dependencies and can participate in fan-out
and parallel execution like other work.

Archive creation is also a normal workflow operation. It consumes resolved local
paths from prior work and writes one archive output with evidence. It should be
usable by later publication or transfer steps without making every domain plugin
implement its own ZIP writer.

### Operational state

An archive data asset is authored as the archive object:

```json
{
  "data": {
    "inputs": {
      "epa_annual_aqi_zip": {
        "kind": "epa_airdata_archive",
        "parameters": {
          "year": {"type": "int"}
        },
        "binding": {
          "provider": "http",
          "location": {
            "uri": "https://aqs.epa.gov/aqsweb/airdata/annual_aqi_by_county_${asset.year}.zip"
          },
          "materialization": {
            "scope": "shared",
            "strategy": "worker_cache",
            "path_template": "epa/airdata/annual_aqi_by_county/${asset.year}.zip"
          }
        }
      }
    }
  }
}
```

The extraction is authored as a later workflow step:

```json
{
  "id": "extract_aqi_csv",
  "work": {
    "type": "archive.extract",
    "archive": {
      "from": "data.epa_annual_aqi_zip",
      "type": "zip"
    },
    "members": [
      {
        "member": "annual_aqi_by_county_${year}.csv",
        "as": "${year}.csv",
        "required": true
      }
    ],
    "output": {
      "path": "${step_temp_dir}/${year}.csv"
    }
  }
}
```

The exact authoring syntax is still subject to Operational Slice refinement. The
important target decision is that `archive.extract` consumes a materialized
archive path and produces extracted file or directory outputs; it does not
download provider sources.

Archive creation is authored as another explicit step:

```json
{
  "id": "create_aqi_zip",
  "work": {
    "type": "archive.create",
    "archive": {
      "type": "zip",
      "output": "${artifact_dir}/annual_aqi_by_county_${year}.zip"
    },
    "entries": [
      {
        "from": "${steps.extract_aqi_csv.outputs.path}",
        "as": "annual_aqi_by_county_${year}.csv"
      }
    ]
  }
}
```

The exact output namespace is still subject to Operational Slice refinement. The
important target decision is that `archive.create` consumes already resolved
local paths and creates an archive artifact; it does not download provider
sources or publish the archive.

For a Yan/Roy-style release archive, the workflow can materialize the release
archive once and then fan out extraction work:

```text
asset.materialize(yan_roy_release_zip)
    -> archive.extract(member = ".../yan-roy",     output = ".../yan-roy")
    -> archive.extract(member = ".../yan-roy.hdr", output = ".../yan-roy.hdr")
```

If the workflow wants separate logical assets for those derived outputs, that
should be modeled as workflow output or a later materialized output concept, not
as hidden archive roles inside the original provider data asset.

## Proposed Slices

The current proposed order is:

1. [Archive Operation Contracts](001-archive-operation-contracts.md) - define
   `archive.extract` and `archive.create` authored-work and work-item contracts.
2. Implement worker-side ZIP extraction for `archive.extract` using generated
   tiny ZIP fixtures.
3. Implement worker-side ZIP creation for `archive.create` using generated tiny
   fixture files and directories.
4. Wire controller dependency, hydration, and result recording for archive
   work that consumes a prior materialized archive.
5. Remove archive-member fields from data-asset definitions, fixtures, and
   active documentation.
6. Update the EPA demo workflow to materialize the ZIP and then extract the CSV
   explicitly.
7. Add a small 7z reservation or follow-up slice only if the Yan/Roy workflow
   needs it before real data testing.

## Completion Criteria

- Current public examples no longer use data-asset `files`, `select`, or
  `binding.archive` for archive-member extraction.
- `asset.materialize` returns the acquired archive file when the data asset is a
  ZIP or 7z object.
- `archive.extract` can extract at least one selected file from a tiny ZIP
  fixture and report extracted-path evidence.
- `archive.create` can create a ZIP from at least one tiny fixture file and
  report archive-path evidence.
- A workflow can express one archive materialization feeding multiple extraction
  work items.
- A workflow can express generated files feeding one archive creation work item.
- The EPA AQI demo workflow uses explicit `asset.materialize` plus
  `archive.extract` steps.
- Tests prove path traversal defenses still reject unsafe archive members and
  unsafe output paths.
- Project state documentation describes archive extraction as workflow work, not
  data-asset materialization behavior.

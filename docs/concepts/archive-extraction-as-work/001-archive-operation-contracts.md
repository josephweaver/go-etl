# 001 Archive Operation Contracts

Status: implemented pending review

## Objective

Add the model and canonical workflow contracts for two explicit archive work
operations:

```text
archive.extract
archive.create
```

This slice should make the operations compile into typed work-item payloads. It
does not implement worker-side archive extraction or creation.

## Current State

`internal/model/work_item.go` defines these active work-item types:

```go
WorkItemTypeAssetMaterialize = "asset.materialize"
WorkItemTypeCommitData       = "commit_data"
```

There is no `archive.extract` or `archive.create` work-item type.

Archive extraction currently exists only as a data-asset materialization
transform:

- `internal/model/data_definition.go` lets a data input declare `files`,
  `select`, and `binding.archive`.
- `cmd/worker/data_asset_materializer.go` calls archive extraction after source
  acquisition when a bound data asset has `Archive`.
- `cmd/worker/archive_extractor.go` contains the low-level ZIP and 7z extraction
  helpers.

Archive creation is not a first-class operation. A workflow that needs to create
a ZIP currently has to do that inside a domain plugin or script.

The canonical workflow adapter already uses explicit data sections for
data-oriented operations:

- `data.materialize` for `asset.materialize`
- `data.outputs` for `commit_data`
- `data.inputs` for compute-step data bindings

## Target State

`internal/model/work_item.go` defines these additional work-item types:

```go
WorkItemTypeArchiveExtract WorkItemType = "archive.extract"
WorkItemTypeArchiveCreate  WorkItemType = "archive.create"
```

The typed payloads are transported through ordinary work-item parameters:

```text
archive_extract
archive_create
```

The canonical authoring shape for extraction is:

```yaml
steps:
  - id: extract_aqi_csv
    fan_out:
      over: ${workflow.years}
      as: year
      id: ${fanout}
    work:
      type: archive.extract
    data:
      archive:
        extract:
          source:
            materialized_asset:
              step: materialize_aqi_zip
              binding: annual_aqi_zip
          type: zip
          members:
            - member: annual_aqi_by_county_${fanout}.csv
              as: annual_aqi_by_county_${fanout}.csv
              required: true
          output:
            path: annual_aqi_by_county_${fanout}.csv
```

The canonical authoring shape for creation is:

```yaml
steps:
  - id: create_aqi_zip
    fan_out:
      over: ${workflow.years}
      as: year
      id: ${fanout}
    work:
      type: archive.create
    data:
      archive:
        create:
          type: zip
          entries:
            - from:
                artifact:
                  step: extract_aqi_csv
                  name: annual_aqi_by_county_${fanout}.csv
              as: annual_aqi_by_county_${fanout}.csv
          output:
            path: annual_aqi_by_county_${fanout}.zip
```

The first implementation may support both fan-out and standalone archive
operation steps. If a canonical archive step has no `fan_out`, the compiled
work-item ID is the step ID and dependency references use the unqualified
source step ID.

### Extract payload

The compiled `archive_extract` parameter should carry a typed payload with these
facts:

```text
operator
archive type
source materialized-asset reference or source local path
selected members
output path
resource constraints
```

The materialized-asset source form is the normal workflow form:

```json
{
  "source": {
    "materialized_asset": {
      "from_work_item_id": "materialize_aqi_zip-2024",
      "binding_name": "annual_aqi_zip"
    }
  }
}
```

The direct local-path source form is allowed only as a low-level worker-contract
escape hatch for fixture or direct-worker tests:

```json
{
  "source": {
    "local_path": "fixtures/source.zip"
  }
}
```

### Create payload

The compiled `archive_create` parameter should carry a typed payload with these
facts:

```text
operator
archive type
entry source references or entry local paths
archive entry names
output path
resource constraints
```

Artifact entry references are the normal workflow form:

```json
{
  "from": {
    "artifact": {
      "from_work_item_id": "extract_aqi_csv-2024",
      "name": "annual_aqi_by_county_2024.csv"
    }
  },
  "as": "annual_aqi_by_county_2024.csv"
}
```

Direct local-path entry sources are allowed only as a low-level worker-contract
escape hatch for fixture or direct-worker tests.

### Validation

The contract validates:

- `operator` equals the work-item type string.
- archive `type` is `zip` in this slice.
- extract `members` is non-empty.
- create `entries` is non-empty.
- archive member names and entry `as` names are slash-relative safe paths.
- output `path` is a slash-relative safe path.
- exactly one source form is set for an extract source.
- exactly one source form is set for each create entry.
- step references are resolved to concrete `from_work_item_id` values during
  workflow compilation.

## Concept Decision

This slice adds two new trusted in-worker archive operation contracts. It does
not add an external executable plugin and does not use Go's dynamic `plugin`
package.

The archive operation details belong under `step.data.archive` rather than as
new ad hoc fields under `step.work`. This matches the existing data-operation
pattern and avoids putting structured archive configuration into generic worker
parameters.

Use a new workflow helper file because archive operations are a separate
concept from asset materialization, data input hydration, and data publication.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/archive-extraction-as-work/README.md`
- `internal/model/work_item.go`
- `internal/model/work_item_test.go`
- `internal/document/workflow.go`
- `internal/workflow/document_adapter.go`
- `internal/workflow/document_adapter_test.go`
- `internal/workflow/fanout.go`
- `internal/workflow/explicit_asset_materialize.go`
- `internal/workflow/explicit_commit_data.go`

Do not read worker extraction internals unless a model or compiler test failure
requires a type already defined there.

## Allowed Production Files

- `internal/model/work_item.go`
- `internal/document/workflow.go`
- `internal/workflow/document_adapter.go`
- `internal/workflow/fanout.go`
- `internal/workflow/explicit_archive_operations.go` (new)

## Allowed Test Files

- `internal/model/work_item_test.go`
- `internal/document/workflow_test.go`
- `internal/workflow/document_adapter_test.go`
- `internal/workflow/explicit_archive_operations_test.go` (new)

## Allowed Documentation Files

- `docs/concepts/archive-extraction-as-work/001-archive-operation-contracts.md`
- `docs/concepts/archive-extraction-as-work/README.md`
- `PROJECT_STATE.md`

## Out Of Scope

- Worker dispatch for `archive.extract`.
- Worker dispatch for `archive.create`.
- ZIP extraction implementation changes.
- ZIP creation implementation.
- 7z creation.
- Real CDL, EPA, or Yan/Roy data.
- Removing `files`, `select`, or `binding.archive` from data assets.
- Updating the demo project workflow.
- Publishing created archives with `commit_data`.
- Controller hydration of materialized-asset source paths.
- Controller hydration of artifact source paths.

## Acceptance Criteria

- `model.WorkItemTypeArchiveExtract` serializes as `archive.extract`.
- `model.WorkItemTypeArchiveCreate` serializes as `archive.create`.
- `ArchiveExtractWorkItemPayload.Validate` accepts a valid ZIP extraction
  payload with one selected member.
- `ArchiveCreateWorkItemPayload.Validate` accepts a valid ZIP creation payload
  with one entry.
- Payload validation rejects unsupported archive types.
- Payload validation rejects missing sources.
- Payload validation rejects create entries with multiple source forms.
- Payload validation rejects unsafe member paths, entry names, and output paths.
- Canonical `work.type: archive.extract` requires `data.archive.extract`.
- Canonical `work.type: archive.create` requires `data.archive.create`.
- `data.archive.extract` requires `work.type: archive.extract`.
- `data.archive.create` requires `work.type: archive.create`.
- A fan-out `archive.extract` step compiles source step references into
  concrete `from_work_item_id` values using the fan-out ID token.
- A fan-out `archive.create` step compiles artifact step references into
  concrete `from_work_item_id` values using the fan-out ID token.
- Standalone archive operation steps are accepted without `fan_out`.
- Focused tests pass:

```text
go test ./internal/model ./internal/document ./internal/workflow
```

## Notes

- Implementation added typed model payloads and canonical workflow compilation
  for `archive.extract` and `archive.create`. Worker dispatch is intentionally
  still absent.
- The worker slices should decide where `output.path` is rooted. The contract
  only requires that the path is slash-relative and deterministic after workflow
  compilation.
- The source-reference forms intentionally mirror `commit_data`'s compiled
  `FromWorkItemID` pattern.
- Suggested implementation HCI: `EC-3 / Operational Slice / files(5)+test+doc+newfile`.

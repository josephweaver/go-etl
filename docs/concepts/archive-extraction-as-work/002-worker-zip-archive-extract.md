# 002 Worker ZIP Archive Extract

Status: implemented pending review

## Objective

Implement worker dispatch for `archive.extract` when the payload describes a ZIP
archive with exactly one selected required file.

The worker should extract the selected member, promote it as a normal artifact,
and return an artifact manifest as the work evidence output JSON.

## Current State

OS-001 added the model and canonical workflow contract:

- `model.WorkItemTypeArchiveExtract`
- `model.ArchiveExtractWorkItemPayload`
- canonical `step.data.archive.extract`
- compiled `archive_extract` work-item parameters

The worker does not dispatch `archive.extract`.

`cmd/worker/archive_extractor.go` already contains safe ZIP selection mechanics
used by data-asset materialization. It validates archive member names, output
paths, selected members, and ZIP entry names, then extracts matching files under
a caller-supplied extraction root.

`cmd/worker/artifact_promotion.go` already promotes declared file artifacts from
an attempt staging directory into the worker data directory and returns a
validated `model.ArtifactManifest`.

## Target State

`cmd/worker/worker.go` dispatches `archive.extract` to a worker handler.

The handler:

1. decodes the `archive_extract` parameter into
   `model.ArchiveExtractWorkItemPayload`;
2. validates the payload;
3. resolves the archive source from either:
   - direct `source.local_path`, or
   - `source.materialized_asset` plus an existing `materialized_data_assets`
     parameter;
4. builds a `model.DataAssetArchive` request for one selected ZIP member;
5. extracts the selected member under the attempt work directory;
6. copies the selected file to the attempt artifact staging directory at
   `payload.output_path`;
7. promotes that staged file through `PromoteArtifacts`;
8. returns the promoted `model.ArtifactManifest` as canonical output evidence.

The artifact descriptor should use:

```text
name = payload.output_path
kind = file
format = zip_member
path = payload.output_path before promotion
metadata.archive_type = zip
metadata.archive_member = <selected member>
```

## Concept Decision

This slice adds a trusted in-worker operation handler. It does not add an
external executable plugin and does not use Go's dynamic `plugin` package.

For this slice, support only one selected required ZIP file. Multiple-member
directory extraction belongs to a later slice because it needs a deliberate
artifact-directory output contract.

## Required Context

Read these files first:

- `docs/concepts/archive-extraction-as-work/README.md`
- `docs/concepts/archive-extraction-as-work/001-archive-operation-contracts.md`
- `internal/model/work_item.go`
- `cmd/worker/worker.go`
- `cmd/worker/archive_extractor.go`
- `cmd/worker/artifact_promotion.go`
- `cmd/worker/evidence.go`
- `cmd/worker/work_asset_materialize.go`
- `cmd/worker/worker_test.go`

## Allowed Production Files

- `cmd/worker/worker.go`
- `cmd/worker/work_archive_extract.go` (new)

## Allowed Test Files

- `cmd/worker/work_archive_extract_test.go` (new)
- `cmd/worker/worker_test.go`

## Allowed Documentation Files

- `docs/concepts/archive-extraction-as-work/002-worker-zip-archive-extract.md`
- `docs/concepts/archive-extraction-as-work/README.md`
- `PROJECT_STATE.md`

## Out Of Scope

- `archive.create` worker execution.
- Multiple-member directory extraction.
- 7z extraction for `archive.extract`.
- Controller hydration for materialized archive sources.
- Workflow dependency activation changes.
- Removing data-asset archive fields.
- Updating the demo project workflow.
- Real EPA, CDL, or Yan/Roy downloads.

## Acceptance Criteria

- Worker dispatch accepts `model.WorkItemTypeArchiveExtract`.
- A direct-local ZIP source with one required selected member extracts that
  member and returns a promoted `goet/artifact-manifest/v1` output.
- The promoted artifact exists under the worker data directory and contains the
  selected member bytes.
- The output artifact descriptor records file size, SHA-256, and archive member
  metadata.
- A `materialized_asset` source can resolve a local archive path from a
  `materialized_data_assets` parameter.
- Missing `archive_extract` parameter fails before extraction.
- Unsupported archive type fails before extraction.
- More than one selected member fails with a clear phase-one diagnostic.
- ZIP path traversal protections from `archive_extractor.go` remain effective.
- Focused tests pass:

```text
go test ./cmd/worker
```

## Notes

- Implementation added worker dispatch for `archive.extract`, direct local-path
  source resolution, materialized-asset source resolution from an existing
  `materialized_data_assets` parameter, single required ZIP member extraction,
  and promotion of that member as a normal artifact manifest.
- The `materialized_asset` source path is resolved from existing worker-side
  manifest hydration mechanics. Controller assignment hydration for archive
  operations remains a later slice.

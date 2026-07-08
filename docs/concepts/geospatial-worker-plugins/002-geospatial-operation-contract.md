# 002 Geospatial Operation Contract

Status: implemented  
Recommended model: GPT-5.5 with high reasoning  
Reference: EC-3 / operational slice / files(4)+test+doc

## Objective

Define the stable external request/result contract for GOET geospatial worker operations and add a minimal standalone `goet-geospatial` CLI skeleton that validates the contract without performing real raster transformations yet.

This slice creates the boundary future operations will share.

## Current State

GOET has an architectural plugin boundary and worker-operation concept, but geospatial operations do not yet have a concrete request/result contract.

Data Assets and Materialized Outputs is assumed complete and should provide worker-local data paths and artifact directories. This slice should not redesign that system.

## Target State

The repository has a geospatial operation contract equivalent to:

```json
{
  "api_version": "goet.geospatial/v1alpha1",
  "kind": "GeospatialOperationRequest",
  "operation": "raster_pair_value_counts",
  "inputs": {},
  "outputs": {},
  "options": {}
}
```

And a compact result contract equivalent to:

```json
{
  "api_version": "goet.geospatial/v1alpha1",
  "kind": "GeospatialOperationResult",
  "operation": "raster_pair_value_counts",
  "artifacts": [],
  "summary": {},
  "warnings": []
}
```

The `goet-geospatial` CLI can:

```bash
goet-geospatial --request request.json --response result.json
```

For this slice, it may implement only:

- request parsing;
- schema validation;
- operation enum validation;
- output path safety validation;
- a `version` or `validate` no-op operation;
- deterministic JSON result writing.

## Concept Decision

The first geospatial plugin is an external CLI-style worker operation, not Go runtime dynamic plugin loading.

The CLI must avoid shell-style string command construction. When later operations need GDAL CLI utilities, use `exec.CommandContext` with explicit argv arrays.

The request contract must use worker-local paths supplied by the Data Assets layer. It must not contain controller-local absolute paths or private host paths.

Output paths in the request must be relative artifact paths, not absolute paths.

## Required Context

Read these files first:

- `docs/PLUGIN_CONTRACT.md`
- `docs/OWNERSHIP_BOUNDARY.md`
- `docs/concepts/data-assets-and-materialized-outputs/README.md`
- implemented Data Assets worker/artifact files
- `internal/model/work_item.go`
- `cmd/worker`
- `docs/concepts/geospatial-worker-plugins/README.md`

If Data Assets introduced a generic executable plugin work item, use that existing boundary. If no generic boundary exists, keep this slice as a standalone CLI contract and record a future integration note rather than overloading the Python work item permanently.

## Allowed Production Files

- `cmd/goet-geospatial/main.go`
- `internal/geospatial/contract.go`
- `internal/geospatial/validation.go`
- `internal/geospatial/doc.go` if useful
- `docs/concepts/geospatial-worker-plugins/README.md` only for tracker/status updates

Use `internal/geospatial` only if that package does not introduce native GDAL dependencies in this slice.

## Allowed Test Files

- `cmd/goet-geospatial/*_test.go`
- `internal/geospatial/*_test.go`
- small JSON fixtures under `internal/geospatial/testdata/`

## Out Of Scope

- GDAL imports.
- Raster reads.
- Worker dispatch changes unless Data Assets already provides the exact hook.
- Real data assets.
- Raster/vector algorithms.
- Parquet output.
- Python/R analytics.

## Acceptance Criteria

- `goet-geospatial --request valid.json --response result.json` produces a deterministic result for a supported no-op/validate operation.
- Invalid `api_version` is rejected.
- Invalid `kind` is rejected.
- Missing or unsupported `operation` is rejected.
- Output artifact paths are validated as safe relative slash-separated paths.
- Absolute paths, Windows drive paths, backslashes, empty paths, and `..` path traversal are rejected for output artifact paths.
- Input paths are accepted as worker-local paths but are not assumed to be controller-openable.
- Default `go test ./...` passes without GDAL installed.
- The README tracker is updated for this slice.

## Notes

Do not overfit the contract to CDL only. The first product is CDL/Yan/Roy, but operations should remain generic enough for other raster/vector workflows.

If the implementation discovers that the completed Data Assets branch already defines a conflicting plugin-operation contract, stop and append the conflict to `docs/concepts/geospatial-worker-plugins/issues.md` instead of creating a second incompatible contract.

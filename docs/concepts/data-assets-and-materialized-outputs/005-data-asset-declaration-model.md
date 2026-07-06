# 005 Data Asset Declaration Model

Status: proposed

## Objective

Add shared model types for data asset declarations and materialized data asset manifests.

This slice defines how workflows and work items name large external inputs without treating those inputs as source files. It does not implement downloads, caching, Python environment changes, or HPCC runs.

## Current State

Workflow source documents can declare source files through `source_manifest`. That is appropriate for Python entrypoints, environment specs, and support files. It is not appropriate for multi-GB public rasters or HPCC-resident tile datasets.

Work items can carry resolved parameters, but there is no dedicated model for external data asset identity, expected hashes, materialization policy, or worker-facing materialized paths.

## Target State

`internal/model` exposes data asset declaration types equivalent to:

```go
type DataAssetDeclaration struct {
    Name            string                 `json:"name"`
    Kind            string                 `json:"kind"`
    Format          string                 `json:"format,omitempty"`
    Location        DataAssetLocation      `json:"location"`
    Expected        DataAssetExpected      `json:"expected,omitempty"`
    Materialization DataAssetMaterialization `json:"materialization,omitempty"`
    Metadata        map[string]any         `json:"metadata,omitempty"`
}

type DataAssetLocation struct {
    Type string `json:"type"` // file, https, http initially
    URI  string `json:"uri"`
}

type DataAssetExpected struct {
    SHA256    string `json:"sha256,omitempty"`
    SizeBytes *int64 `json:"size_bytes,omitempty"`
}

type DataAssetMaterialization struct {
    Strategy string `json:"strategy,omitempty"` // worker_cache initially
    CacheKey string `json:"cache_key,omitempty"`
}
```

A separate worker-facing materialized manifest type should represent assets actually present in the execution environment:

```go
type MaterializedDataAssetManifest struct {
    Schema string                  `json:"schema"`
    Assets []MaterializedDataAsset `json:"assets"`
}

type MaterializedDataAsset struct {
    Name      string `json:"name"`
    Kind      string `json:"kind"`
    Format    string `json:"format,omitempty"`
    Path      string `json:"path"`
    SizeBytes int64  `json:"size_bytes"`
    SHA256    string `json:"sha256,omitempty"`
}
```

The model should validate declaration names, supported location types, URI presence, hash shape, and cache-key safety. It should not fetch data.

## Concept Decision

Data asset declarations belong in `internal/model` because they cross workflow compilation, worker payloads, Python input manifests, and future client surfaces.

Do not overload `source_manifest` for data assets. `source_manifest` remains for admitted execution source. Data asset declarations are execution inputs, not code.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/data-assets-and-materialized-outputs/README.md`
- `docs/concepts/source-control-resolution-and-cache/README.md`
- `internal/model/work_item.go`
- `internal/model/artifact_manifest.go`
- `internal/reposource/source_declaration.go`

Do not read worker, controller, scheduler, or transport files unless compile or test failures directly require them.

## Allowed Production Files

- `internal/model/data_asset.go`
- `internal/model/work_item.go` only if adding an optional `DataAssets []DataAssetDeclaration` field is chosen in this slice

## Allowed Test Files

- `internal/model/data_asset_test.go`
- `internal/model/work_item_test.go` only if `WorkItem` JSON round-trip changes

## Out Of Scope

- Controller workflow parsing of top-level `data_assets`.
- Worker downloading or caching assets.
- Writing `GOET_DATA_ASSETS_JSON`.
- Python runner changes.
- Persistence schema changes.
- Sensitive variables or private credentials.
- Real CDL/Yan/Roy downloads.

## Acceptance Criteria

- A valid `https` data asset declaration validates.
- A valid `file` data asset declaration validates without assuming the controller can open the path.
- Missing asset name, kind, location type, or URI fails validation.
- Unsupported location types fail validation unless explicitly documented as reserved.
- Invalid SHA-256 values fail validation.
- Unsafe cache keys are rejected using slash-relative path rules.
- Materialized data asset manifests validate required name, kind, path, size, and optional hash fields.
- JSON round-trip tests preserve declarations and materialized asset manifests.
- If `WorkItem` gains an optional `data_assets` field, existing work-item tests still pass and omission remains backward-compatible.
- `go test ./internal/model` passes.

## Notes

- CDL URLs and Yan/Roy tile paths should be examples in docs or fixtures, not hard-coded into the model.
- A missing expected hash may be allowed for exploratory runs, but the model should make it obvious that verified reuse requires stronger evidence.

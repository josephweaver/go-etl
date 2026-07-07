# 001 Artifact Manifest Model and Path Safety

Status: implemented

## Objective

Add a shared model for materialized artifact manifests and a safe relative artifact-path validator.

This slice defines the compact JSON contract that workers can report and controllers can persist when a work item produces files or directories. It does not change Python execution, worker promotion, controller persistence, data provider binding, data asset downloading, published-asset copying, or HPCC behavior.

## Current State

`internal/model` owns shared controller-worker payload shapes such as `WorkItem`, `WorkCompletion`, `WorkFailure`, and `WorkSkip`.

Python work items can currently write one logical `GOET_OUTPUT_JSON` document. That works for compact outputs, but it does not provide a typed, reusable contract for large files and directories produced by a work item.

Path safety already matters elsewhere in the repository. Repository source paths are slash-separated, relative, and validated before provider or filesystem operations. Artifact paths need the same posture but under a worker output root instead of a repository root.

## Target State

`internal/model` exposes versioned artifact manifest types equivalent to:

```go
type ArtifactManifest struct {
    Schema       string               `json:"schema"`
    RunID        string               `json:"run_id,omitempty"`
    StageIndex   *int                 `json:"stage_index,omitempty"`
    StepIndex    *int                 `json:"step_index,omitempty"`
    WorkItemID   string               `json:"work_item_id,omitempty"`
    AttemptID    string               `json:"attempt_id,omitempty"`
    StorageScope string               `json:"storage_scope"`
    Artifacts    []ArtifactDescriptor `json:"artifacts"`
    ScriptOutput any                  `json:"script_output,omitempty"`
}

type ArtifactDescriptor struct {
    Name          string         `json:"name"`
    Kind          string         `json:"kind"`
    Format        string         `json:"format,omitempty"`
    Path          string         `json:"path"`
    ContentType   string         `json:"content_type,omitempty"`
    SizeBytes     *int64         `json:"size_bytes,omitempty"`
    SHA256        string         `json:"sha256,omitempty"`
    ManifestSHA256 string        `json:"manifest_sha256,omitempty"`
    RecordCount   *int64         `json:"record_count,omitempty"`
    SchemaRef     string         `json:"schema_ref,omitempty"`
    Metadata      map[string]any `json:"metadata,omitempty"`
}
```

Naming may differ, but the model must distinguish:

- the manifest schema version;
- the storage scope;
- one or more artifact descriptors;
- file hash evidence from directory manifest hash evidence;
- optional producer/run metadata from required artifact identity.

Add an artifact path validator that accepts only slash-separated relative paths inside an artifact root. It should reject empty paths, absolute paths, Windows drive prefixes, backslashes, `.` segments, `..` segments, and any path that escapes after cleaning.

## Concept Decision

Put the shared manifest model in `internal/model` because it is part of the controller-worker completion contract and downstream logical-output contract.

Do not place artifact manifest types in `cmd/worker`; the controller, persistence, status, workflow output propagation, and future client surfaces all need the same shape.

Path validation may live in `internal/model` if it is purely structural. If implementation needs filesystem joins and root checks, keep the root-aware helper in the worker slice and keep the shared model helper limited to relative slash-path validation.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/data-assets-and-materialized-outputs/README.md`
- `internal/model/work_item.go`
- `internal/model/work_item_test.go`
- `internal/reposource/path.go`
- `internal/reposource/path_test.go`

Do not read controller, worker, scheduler, transport, or persistence files unless compile or test failures directly require them.

## Allowed Production Files

- `internal/model/artifact_manifest.go`
- `internal/model/artifact_path.go`
- `internal/model/work_item.go` only if model package organization requires a small shared constant or compile fix

## Allowed Test Files

- `internal/model/artifact_manifest_test.go`
- `internal/model/artifact_path_test.go`
- `internal/model/work_item_test.go` only if existing tests need fixture updates

## Out Of Scope

- Worker filesystem staging or promotion.
- Python environment variables.
- Python output parsing changes.
- Controller completion handling.
- Persistence schema changes.
- Data asset declarations.
- Data asset downloading.
- Fake HPCC, Slurm, SSH, Singularity, or container changes.
- Artifact retention or cleanup policy.
- Published-data-asset copy behavior.

## Acceptance Criteria

- `ArtifactManifest` and `ArtifactDescriptor` or equivalent shared types exist.
- The default schema string is `goet/artifact-manifest/v1` or a documented equivalent.
- A manifest with one valid file artifact validates.
- A manifest with one valid directory artifact using `manifest_sha256` validates.
- A manifest with no artifacts fails validation.
- An artifact descriptor with empty `name`, empty `kind`, or empty `path` fails validation.
- A file artifact may carry `sha256` and `size_bytes`.
- A directory artifact may carry `manifest_sha256`.
- Artifact paths reject absolute paths, backslashes, drive-qualified paths, `.` segments, `..` segments, and empty segments where they would change path meaning.
- Tests cover Windows-style unsafe path examples even when running on Unix.
- `go test ./internal/model` passes.

## Notes

- Keep the model compact. Do not add geospatial-specific fields to the shared artifact descriptor.
- `metadata` is the escape hatch for domain-specific values such as `year`, `tile_id`, or `cdl_resolution_m`.
- Do not define object-store, remote-download, or published-location copy behavior in this slice.
- A later slice may extend the manifest or add a sibling manifest for compact `published_assets` evidence.

# 002 Worker Artifact Staging and Promotion

Status: implemented

## Objective

Add worker-side artifact staging and promotion for declared artifacts.

This slice lets worker code validate script- or operation-declared artifacts under an attempt-local staging directory, compute evidence, and promote them into the worker's configured completed-output root. It does not publish artifacts to final named data locations yet and does not change the Python subprocess contract yet; tests can call the worker artifact helper directly.

## Current State

The worker owns runtime directories for logs, temporary output, and completed data. Existing operations follow a temp-to-data promotion pattern for ordinary output files.

There is no shared helper for promoting multiple declared files or directories as materialized artifacts, computing artifact evidence, or rewriting staging-relative paths into final data-root-relative manifest paths.

## Target State

The worker package has a small artifact promotion helper equivalent to:

```go
type ArtifactPromotionRequest struct {
    StagingRoot string
    DataRoot    string
    RunID       string
    StageIndex  *int
    StepIndex   *int
    WorkItemID  string
    AttemptID   string
    Manifest    model.ArtifactManifest
}

func PromoteArtifacts(ctx context.Context, req ArtifactPromotionRequest) (model.ArtifactManifest, error)
```

The helper must:

1. validate every declared staging-relative artifact path;
2. resolve it under `StagingRoot`;
3. ensure the resolved source stays inside `StagingRoot`;
4. compute file SHA-256 and size for file artifacts;
5. compute ordered file-entry hashes and a directory manifest SHA-256 for directory artifacts;
6. promote artifacts into a deterministic destination below `DataRoot`;
7. ensure the destination stays inside `DataRoot`;
8. return a manifest whose paths are relative to `DataRoot`, not to the staging directory;
9. avoid leaving partially promoted outputs visible as complete artifacts when a validation or copy error occurs.

A reasonable first destination shape is:

```text
artifacts/<run_id>/stage-<stage_index>/step-<step_index>/<work_item_id>/<artifact-relative-path>
```

If stage or step indexes are unavailable for raw work items, use a raw-work shape such as:

```text
artifacts/raw/<work_item_id>/<artifact-relative-path>
```

## Concept Decision

Use the worker's existing completed-output root as the first physical artifact root. Do not introduce a second mandatory `ArtifactDir` until there is evidence that `DataDir` and artifact root must diverge. Treat this promoted root as attempt-output evidence, not necessarily the final published data-product location.

Keep promotion in the worker. The controller should not open worker-local or HPCC-local paths to validate bytes.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/data-assets-and-materialized-outputs/README.md`
- `internal/model/artifact_manifest.go`
- `internal/model/artifact_path.go`
- `cmd/worker/README.md`
- `cmd/worker/config.go`
- `cmd/worker/work_demo.go`
- `cmd/worker/work_python.go`
- `cmd/worker/evidence.go`

Do not read controller, scheduler, transport, or container files unless compile or test failures directly require them.

## Allowed Production Files

- `cmd/worker/artifact_promotion.go`
- `cmd/worker/evidence.go` only if shared hash helpers already belong there
- `cmd/worker/config.go` only if tests reveal a narrow need to expose the data root consistently

## Allowed Test Files

- `cmd/worker/artifact_promotion_test.go`
- `cmd/worker/evidence_test.go` only if shared hash helper tests belong there
- `cmd/worker/config_test.go` only for narrow config fixture adjustments

## Out Of Scope

- Python subprocess environment changes.
- Parsing `GOET_OUTPUT_JSON` artifact declarations.
- Controller completion handling.
- Persistence changes.
- Data asset declarations or downloads.
- Slurm, SSH, Docker, Singularity, or fake HPCC changes.
- Artifact cleanup or retention policy.
- Copying selected artifacts to predeclared named publish locations.

## Acceptance Criteria

- A worker helper can promote a declared file artifact from staging to the data root.
- The returned manifest path is data-root-relative and uses slash separators.
- The returned file descriptor includes raw SHA-256 and byte count computed after promotion.
- A worker helper can promote a declared directory artifact.
- The returned directory descriptor includes a deterministic directory manifest hash.
- Unsafe staging paths are rejected before filesystem reads.
- Unsafe destination paths are impossible or rejected before filesystem writes.
- Missing declared artifacts fail the promotion.
- Promotion failure does not leave a completed manifest claiming success.
- Tests use temporary directories and tiny files only.
- `go test ./cmd/worker` passes.

## Notes

- Prefer copy-then-atomic-rename within the destination root when practical.
- Cross-filesystem rename may fail; the helper may copy then rename a temporary destination inside the data root.
- Directory manifest order must be stable across platforms.
- Publication to a named data location is deliberately deferred so artifact promotion can be tested independently.

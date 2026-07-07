# 008 Google Drive Rclone Data Provider

Status: implemented

## Objective

Add a `gdrive_rclone` data provider implementation that acquires a file from a configured rclone remote into the worker asset cache, verifies evidence, and exposes it to the existing data materialization and archive extraction path.

This slice should use fake-rclone tests. It must not require real Google Drive access, real LandCore Shared Drive credentials, OAuth setup, or internet access in default tests.

## Current State

The model can represent a `gdrive_rclone` provider, and the worker can materialize core providers such as `local_file`, `http`, and `registered_location`. LandCore workflows often depend on Google Drive or Shared Drive files, such as a Yan/Roy release archive.

GOET should not implement Google Drive OAuth and Shared Drive behavior directly in the first pass. The worker/container can instead carry a known-good `rclone` executable and a preconfigured remote.

## Target State

A provider declaration can express a Google Drive-backed asset equivalent to:

```json
{
  "name": "yanroy_release_drive",
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
}
```

After binding parameters, the worker:

1. resolves the rclone remote and drive path;
2. validates the remote/path against the restricted model;
3. chooses or derives a cache key;
4. reuses a valid immutable cache entry when available;
5. otherwise invokes the configured rclone executable to copy the remote file to a temporary cache path;
6. computes SHA-256 and byte count while or after copying;
7. verifies expected integrity when present;
8. atomically records the cache entry and cache manifest;
9. passes the acquired file to archive extraction if the bound asset has `archive` configuration;
10. writes materialized evidence into `GOET_DATA_ASSETS_JSON`.

## Rclone Boundary

The worker should have a narrow adapter, for example:

```text
RcloneProvider.Stat(ctx, remote, path) -> optional size/modtime evidence
RcloneProvider.CopyTo(ctx, remote, path, localTempPath) -> local file
```

Implementation constraints:

- Invoke rclone with `exec.Command` and structured argument arrays.
- Do not construct shell command strings.
- Do not allow workflow JSON to pass arbitrary rclone flags.
- Do not expose credentials in logs.
- Redact local config paths or tokens if any subprocess output includes them.
- Treat nonzero exit codes as materialization failures before plugin execution.
- Apply worker max-size limits after stat when size is available and after download when not.
- Reuse the same immutable cache verification behavior as other providers.

The worker configuration may include:

```json
{
  "rclone_executable": "rclone",
  "rclone_config_path": "/run/secrets/rclone/rclone.conf",
  "enable_gdrive_rclone_provider": true
}
```

Exact config field names may differ, but credentials and remote setup must remain environment/config concerns, not workflow JSON.

## Fake-Rclone Test Strategy

Default tests should create a small fake executable in a temp directory. The fake executable should:

- record argv to a temp log;
- copy a local fixture file to the requested destination;
- optionally simulate failure;
- optionally emit metadata if the implementation uses a stat/lsjson call.

This proves that GOET invokes rclone through the adapter and integrates the resulting file with cache and integrity checks without real Google Drive.

## Concept Decision

Use `rclone` first because Google Drive and Shared Drive behavior is operationally complex and already solved by a mature external tool. Keep it behind a provider interface so a future native Go provider can replace it.

Do not make rclone a mandatory dependency for all workers. The provider should fail clearly when a workflow uses `gdrive_rclone` and the worker has not enabled or configured it.

In containerized worker deployments, include rclone in the image when Google Drive-backed workflows are expected. That reduces architecture mismatch issues and avoids requiring the user's Windows laptop to have rclone installed.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/data-assets-and-materialized-outputs/README.md`
- `docs/concepts/data-assets-and-materialized-outputs/005-data-location-provider-and-binding-model.md`
- `docs/concepts/data-assets-and-materialized-outputs/006-worker-data-asset-materialization.md`
- `docs/concepts/data-assets-and-materialized-outputs/007-worker-archive-extraction-and-selection.md`
- `internal/model/data_asset.go`
- `internal/model/data_provider.go`
- `cmd/worker/data_asset_materializer.go`
- `cmd/worker/asset_cache.go` if created
- `cmd/worker/config.go`

Do not read controller scheduler, SSH, Slurm, or container files unless compile or test failures directly require them.

## Allowed Production Files

- `cmd/worker/gdrive_rclone_provider.go`
- `cmd/worker/data_asset_materializer.go`
- `cmd/worker/asset_cache.go` only for cache integration
- `cmd/worker/config.go` only for rclone executable/config/enabled settings
- `internal/model/data_provider.go` only for narrow validation adjustments
- `internal/model/data_asset.go` only for narrow bound-location or manifest adjustments
- Worker container files only if the repository already has a generic worker image and adding rclone is a small, non-private dependency change

## Allowed Test Files

- `cmd/worker/gdrive_rclone_provider_test.go`
- `cmd/worker/data_asset_materializer_test.go`
- `cmd/worker/asset_cache_test.go` only for cache integration
- `cmd/worker/config_test.go` only for rclone config settings
- `internal/model/data_provider_test.go` only for narrow validation adjustments
- `internal/model/data_asset_test.go` only for narrow bound-location or manifest adjustments

## Out Of Scope

- Real Google Drive access.
- Real LandCore Shared Drive paths, file IDs, credentials, or OAuth setup.
- Asking GOET to create or modify Google Drive files.
- Syncing entire folders by default.
- Arbitrary rclone backends beyond the `gdrive_rclone` provider declaration.
- Arbitrary rclone flags from workflow JSON.
- Credential propagation through GOET.
- Native Go Drive API implementation.
- Real Yan/Roy archive extraction unless a tiny local/fake-rclone fixture is used.
- Data catalog registration.

## Acceptance Criteria

- A `gdrive_rclone` bound data asset can be represented as a concrete worker input.
- The worker refuses to use `gdrive_rclone` when the provider is disabled or no executable is configured.
- The worker validates rclone remote and drive path before invocation.
- The worker invokes a fake rclone executable with structured arguments, not through a shell string.
- The fake rclone copies a tiny fixture file into a temporary cache path.
- The downloaded/copied file is hashed and size-counted.
- Expected SHA-256 is verified when present.
- Expected size is verified when present.
- Mismatched expected SHA-256 or size fails before plugin execution.
- The immutable cache behavior from the previous slice applies to rclone-acquired files.
- A valid cached rclone-acquired file is reused without invoking fake rclone again when cache evidence matches.
- rclone subprocess failure produces a clear materialization error and does not leave a valid cache entry.
- rclone stdout/stderr captured in errors/logs does not expose secrets from config paths or tokens.
- If the bound asset has archive configuration, the acquired file is passed to the archive extraction path from slice 007.
- Existing non-rclone materialization tests still pass.
- Tests do not require internet access or real Google Drive credentials.
- `go test ./cmd/worker` passes.

## Notes

- Implemented in `cmd/worker/gdrive_rclone_provider.go` with worker config fields
  `rclone_executable`, `rclone_config_path`, and
  `enable_gdrive_rclone_provider`.
- The worker invokes `rclone copyto` through `exec.CommandContext` with an
  argument slice. Workflow JSON cannot pass arbitrary rclone flags.
- Default tests use a fake rclone executable re-entered through the Go test
  binary. They do not require real Google Drive access, rclone configuration,
  OAuth credentials, internet access, or private LandCore paths.
- The implementation uses path-based rclone access. `file_id` remains reserved
  and fails clearly before invocation.
- Prefer path-based rclone access for the first implementation. File-ID based access can be reserved in the model and implemented later if needed.
- Rclone remote configuration should be supplied by deployment/container configuration, not project workflow files.
- For the first LandCore run, it is acceptable to use `local_file` for a manually downloaded `ReleaseData.7z` while `gdrive_rclone` matures.

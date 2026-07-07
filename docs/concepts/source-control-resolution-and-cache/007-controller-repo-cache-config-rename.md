# 007 Controller Repo Cache Config Rename

Status: Complete

## Objective

Rename controller cache configuration from Git-specific names to repository
cache names.

This slice replaces `controller_git_cache_path`,
`controller_git_cache_max_size_mb`, and
`controller_git_cache_retention_milliseconds` with
`controller_repo_cache_path`, `controller_repo_cache_max_size_mb`, and
`controller_repo_cache_retention_milliseconds`.

No compatibility aliases are added because GOET is still pre-production.

## Current State

The Strategic Concept now treats GitHub and local filesystem inputs as
repository sources. The controller cache is no longer Git-only.

Startup code and defaults now use repository-cache names instead of the old
Git-specific names:

- `controller_repo_cache_path`;
- `controller_repo_cache_max_size_mb`;
- `controller_repo_cache_retention_milliseconds`.

The current Go structs also use repository-cache field names:

- `controllerFilesystemPaths.RepoCache`;
- `controllerOperationalPolicy.RepoCacheMaxSizeMB`;
- `controllerOperationalPolicy.RepoCacheRetentionMillis`.

Checked-in defaults and tests expect the new names. `PROJECT_STATE.md` also
describes the current startup filesystem paths using
`controller_repo_cache_path`.

## Target State

Controller startup resolves repository-cache configuration names:

- `controller_repo_cache_path`;
- `controller_repo_cache_max_size_mb`;
- `controller_repo_cache_retention_milliseconds`.

The normalized controller structs use repository-cache vocabulary:

- `controllerFilesystemPaths.RepoCache`;
- `controllerOperationalPolicy.RepoCacheMaxSizeMB`;
- `controllerOperationalPolicy.RepoCacheRetentionMillis`.

Checked-in defaults, startup tests, and current-state documentation use the new
names. Submitting an old `controller_git_cache_*` variable does not satisfy the
new required configuration key.

## Concept Decision

This slice updates the controller startup configuration concept and the
repository-source cache concept.

The rename belongs in controller startup code because that code owns resolving
controller filesystem paths and operational policy values. The repository cache
package introduced by earlier slices should consume an already-resolved cache
root; it should not know controller variable names.

## Required Context

Read these files first:

- `docs/concepts/source-control-resolution-and-cache/README.md`
- `cmd/controller/main.go`
- `cmd/controller/main_test.go`
- `cmd/controller/config_test.go`
- `cmd/controller/defaults.json`
- `PROJECT_STATE.md`

Do not read unrelated controller files unless compile or test failures directly
require it.

## Allowed Production Files

- `cmd/controller/main.go`
- `cmd/controller/defaults.json`

## Allowed Test Files

- `cmd/controller/main_test.go`
- `cmd/controller/config_test.go`

## Allowed Documentation Files

- `PROJECT_STATE.md`
- `docs/concepts/source-control-resolution-and-cache/README.md`
- `docs/concepts/source-control-resolution-and-cache/003-repository-cache-access-layer-and-layout.md`
- `docs/concepts/source-control-resolution-and-cache/004-cached-admission-and-verified-reads.md`

## Out Of Scope

- Adding compatibility aliases for `controller_git_cache_*`.
- Implementing repository cache publication, reads, pins, or materialization.
- Wiring `internal/reposource` into controller admission.
- Renaming GitHub fetch policy names such as
  `controller_git_fetch_timeout_milliseconds` or
  `controller_git_fetch_concurrency`.
- Changing artifact cache, temp, log, or database configuration.
- Updating historical completed concept documents unless tests or current
  documentation require it.
- Changing persistence schema fields such as `source_commit`.

## Acceptance Criteria

- Controller filesystem startup resolves `controller_repo_cache_path`.
- Controller operational policy resolves `controller_repo_cache_max_size_mb`.
- Controller operational policy resolves
  `controller_repo_cache_retention_milliseconds`.
- `controllerFilesystemPaths` uses `RepoCache` vocabulary instead of
  `GitCache`.
- `controllerOperationalPolicy` uses `RepoCacheMaxSizeMB` and
  `RepoCacheRetentionMillis` vocabulary instead of Git cache vocabulary.
- `cmd/controller/defaults.json` defines the new repository-cache keys.
- Checked-in defaults tests expect the new repository-cache keys.
- Startup resolver tests use the new repository-cache keys.
- Old `controller_git_cache_*` keys are not accepted as aliases.
- GitHub fetch timeout and concurrency settings keep their Git-specific names.
- Current-state documentation no longer describes the active controller cache
  config as `controller_git_cache_*`.
- Targeted controller config/startup tests pass.

## Notes

- The default path should become `${controller_root_dir}/repo_cache`.
- This slice is a rename and vocabulary cleanup, not a behavior change to cache
  implementation.
- Error messages should naturally mention the new required key when it is
  missing or has the wrong type.
- Leave unrelated historical references alone unless they are part of the active
  current-state docs or test expectations.

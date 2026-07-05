# 002 Log Configuration

Status: Ready

## Objective

Define and resolve the controller logging configuration needed by controller-owned execution observability.

This slice establishes configuration shape and defaults only. It must not open log files, add HTTP endpoints, submit worker logs, or implement durable sinks.

## Current State

`cmd/controller/config.go`, controller defaults, and controller startup resolution already own controller configuration loading and validation.

Execution Observability needs controller startup to know:

- whether controller filesystem logging is enabled;
- where controller-owned log files should live;
- what minimum log level should be written durably;
- what bounds should apply to submission-log read requests later.

If the current branch already has partially introduced logging-related fields such as filesystem logging enabled, log root, or log level, they are not yet connected to a complete observation model, endpoint, sink, read API, or CLI surface.

## Target State

Controller startup configuration can represent and resolve the following logging policy without performing logging side effects:

```text
controller_config.filesystem_logging_enabled
controller_config.log_root_path
controller_config.log_level
controller_config.log_read_default_tail_lines
controller_config.log_read_max_tail_lines
```

The exact variable names should reuse existing names if they already exist in the controller config/defaults. Do not create duplicate names for the same concept.

Expected behavior:

- `filesystem_logging_enabled` resolves to a boolean.
- `log_root_path` resolves to a controller-owned filesystem path.
- Relative `log_root_path` values resolve relative to the controller working directory or existing controller filesystem-root convention.
- `log_level` resolves to one of the allowed levels from `internal/model`.
- `log_read_default_tail_lines` resolves to a positive integer.
- `log_read_max_tail_lines` resolves to a positive integer greater than or equal to the default.
- Missing logging config uses documented defaults.
- Invalid log levels fail controller startup configuration validation.
- Invalid log-read bounds fail controller startup configuration validation.

## Concept Decision

This slice updates the existing controller configuration concept. It should stay near existing controller startup configuration code rather than creating a separate configuration authority.

If a small `controllerLoggingPolicy` struct clarifies startup resolution, it may live near other resolved controller startup policy structs.

## Required Context

Read these files first:

- `docs/concepts/execution-observability/README.md`
- `docs/concepts/execution-observability/001-logging-model.md`
- `docs/ARCHITECTURE_OVERVIEW.md`
- `cmd/controller/README.md`
- `cmd/controller/config.go`
- `cmd/controller/config_test.go`
- `cmd/controller/defaults.json`
- `cmd/controller/controller-default-config.json`

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

- `cmd/controller/config.go`
- `cmd/controller/defaults.json`
- `cmd/controller/controller-default-config.json`

## Allowed Test Files

- `cmd/controller/config_test.go`

## Out Of Scope

- Controller log-ingestion endpoint.
- Submission-log read endpoint.
- Worker logging client.
- Filesystem sink implementation.
- Opening, creating, or writing log files.
- Python subprocess stdout/stderr emission.
- Worker fallback logging.
- Attempt Ledger changes.
- Execution event generalization.
- CLI log command.

## Acceptance Criteria

- Controller configuration can represent filesystem logging enabled/disabled state.
- Controller configuration can represent a log root path.
- Controller configuration can represent a minimum log level.
- Controller configuration can represent default and maximum tail-line bounds for later log-read requests.
- Missing logging config resolves to documented defaults.
- Relative log root values are resolved consistently with existing controller path rules.
- Invalid log levels produce configuration errors.
- Invalid tail-line bounds produce configuration errors.
- Existing tests for unrelated controller configuration continue to pass.
- No HTTP endpoint, worker client, log file creation, or log file write behavior is added.

## Notes

- Prefer reusing existing variable/default infrastructure over adding a separate config file.
- Do not add a configurable human line formatter in this slice. The durable sink for this concept is structured JSONL, and human rendering belongs to later read/CLI behavior.
- Logging is best-effort at runtime, but malformed startup configuration should still fail closed during configuration validation.

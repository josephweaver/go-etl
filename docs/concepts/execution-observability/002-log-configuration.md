# 002 Log Configuration

Status: proposed

## Objective

Define the configuration model for GOET logging. This slice establishes how logging behavior, filesystem roots, enabled sinks, log levels, and rendered line formats are represented in configuration, without implementing controller endpoints or filesystem writing.

## Required Context

Read these files first:

- docs/concepts/execution-observability/README.md
- docs/concepts/execution-observability/001-logging-model.md
- docs/ARCHITECTURE_OVERVIEW.md
- cmd/controller/config.go
- cmd/controller/config_test.go

Do not read unrelated files unless test failures directly require it.

## Allowed Production Files

- cmd/controller/config.go

## Allowed Test Files

- cmd/controller/config_test.go

## Out Of Scope

- Controller logging HTTP endpoints.
- Worker logging clients.
- Filesystem log sink implementation.
- Opening, creating, or writing log files.
- Stdout/stderr subprocess streaming.
- Worker fallback logging implementation.
- Attempt Ledger changes.
- Execution event generalization.

## Acceptance Criteria

- Controller configuration can represent logging configuration.
- Configuration supports a log root path.
- Configuration supports enabling or disabling controller-managed filesystem logging.
- Configuration supports the intended filesystem sink categories: controller-wide, workflow/run, and attempt-level.
- Configuration supports a minimum log level or verbosity threshold.
- Configuration supports a rendered line format string for filesystem sinks.
- Configuration parsing tests cover valid logging config.
- Configuration parsing tests cover missing or default logging config.
- Configuration parsing tests cover invalid log levels or invalid sink configuration where applicable.
- No actual logging endpoint, log streaming, or filesystem sink behavior is added.

## Notes

- The structured log observation model is defined in slice 001.
- Rendered log-line format should be configuration-driven, similar in spirit to Apache-style configurable log formats.
- This slice should avoid over-designing the final formatter. It only needs to preserve enough configuration shape for later sink slices.
- Logging is best-effort; this configuration should not introduce behavior where logging failures fail work execution.
- Worker fallback logging may eventually have its own configuration, but implementing fallback behavior belongs to a later slice.

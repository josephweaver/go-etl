# 007 Controlled Sink Redaction Tests

Status: proposed

## Objective

Add focused sentinel tests proving that exact materialized sensitive values do not appear in GOET-controlled persistence, status, logs, events, worker errors, captured subprocess output, or structured output records.

This slice is a cross-boundary verification slice. It should not add new secret semantics unless a test exposes a concrete gap.

## Current State

Earlier slices add sensitivity metadata, protected refs, worker resolution, trusted Go handler context, and Python materialization. However, accidental leaks can still happen where logs, errors, HTTP responses, database rows, or captured outputs serialize internal values.

## Target State

The test suite has named sentinel secrets and checks controlled sinks for exact leakage.

Example sentinel:

```text
goet-secret-sentinel-007-do-not-persist
```

Representative controlled sinks:

- safe rendering;
- error messages;
- controller status JSON;
- worker completion/failure reports;
- captured stdout/stderr;
- structured event payloads if implemented;
- persistence rows containing workflow-run context, work-item snapshots, attempts, completed work, failed work, and output JSON;
- log buffers/files owned by GOET test harnesses.

## Concept Decision

Use exact sentinel leak tests as a safety net. Do not try to solve transformed leaks in phase 1.

Structured output policy:

```text
Logs/stdout/stderr: redact exact materialized secret values.
GOET_OUTPUT_JSON / dependency output: reject if exact materialized secret value is present.
Artifacts: do not scan by default; document limitation.
```

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/sensitive-variable-propagation/README.md`
- all files changed by slices 001-006
- controller status tests
- persistence tests
- worker Python tests
- worker dispatch tests
- execution events/logging tests if present

Search for test helpers that serialize status, database rows, logs, events, completed work, or failed work.

## Allowed Production Files

Prefer no production changes.

Allowed only if tests expose narrow leak bugs:

- worker redactor/safe logger files
- controller status rendering files
- model safe-rendering helpers
- persistence serialization helper that was accidentally using raw formatting

## Allowed Test Files

- `internal/variable/*sensitivity*_test.go`
- `cmd/worker/*redactor*_test.go`
- `cmd/worker/work_python_test.go`
- controller status tests
- persistence tests
- execution event/logging tests if present

## Out Of Scope

- New providers.
- Real secret managers.
- Real credentialed data assets.
- HPCC smoke.
- Scanning artifacts.
- Preventing transformed or encoded leaks.

## Acceptance Criteria

- At least one sentinel test verifies safe variable rendering.
- At least one sentinel test verifies controller status does not expose plaintext.
- At least one sentinel test verifies worker failure paths do not expose plaintext.
- At least one sentinel test verifies captured stdout/stderr redaction.
- At least one sentinel test verifies structured output JSON containing an exact secret is rejected or not persisted.
- At least one persistence-oriented test inspects serialized records or stored JSON where practical.
- Tests document that artifacts are not scanned by phase-1 policy.
- Existing tests still pass.

## Notes

This slice is not a good fit for `5.3-codex-spark`. It requires multi-surface reasoning and often involves smoke-like debugging.

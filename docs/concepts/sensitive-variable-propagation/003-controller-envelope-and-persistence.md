# 003 Controller Envelope and Persistence

Status: implemented

## Objective

Preserve public resolved values and sensitive protected references through controller compilation, work-item assignment, workflow-run snapshots, status output, and fingerprints without persisting plaintext secrets.

This slice makes controller-side behavior concrete. It does not add worker secret lookup or subprocess materialization.

## Current State

After slices 001 and 002, `internal/variable` can represent sensitivity and protected references, but the controller may still compile and persist work-item parameters as ordinary resolved values.

GOET's dependency-aware and execution-persistence concepts rely on durable controller state. This slice ensures that sensitive values do not become ordinary durable state.

## Target State

The controller builds an execution envelope or equivalent payload containing:

```json
{
  "schema": "goet/execution-envelope/v1",
  "variables": {
    "public": {
      "year": { "type": "int", "value": 2026 }
    },
    "protected_refs": {
      "gdrive_token": {
        "type": "string",
        "provider": "worker_env",
        "key": "GOET_GDRIVE_TOKEN",
        "redaction_label": "${worker_env.GOET_GDRIVE_TOKEN}",
        "materialize": {
          "mode": "env",
          "target": "GDRIVE_TOKEN"
        }
      }
    }
  }
}
```

The controller may persist this envelope because it contains non-secret references. It must not persist plaintext sensitive values.

## Concept Decision

Do not pass a flat `map[string]any` once sensitivity exists. Use a structured payload that separates:

- public resolved values;
- protected references;
- redaction labels;
- optional materialization hints;
- non-secret provenance.

Controller status and diagnostics should render protected references safely. Fingerprints should exclude plaintext and include only non-secret identity/version metadata.

If an existing `model.WorkItem.Parameters` map remains for compatibility, it must either:

1. support typed sensitive/protected values safely; or
2. be converted into an execution envelope before persistence/assignment.

Do not add plaintext secret fields to existing work-item rows, attempt rows, or status JSON.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/sensitive-variable-propagation/README.md`
- `docs/concepts/workflow-compilation-resolution/README.md`
- `docs/concepts/workflow-execution-persistence/README.md`
- `docs/concepts/dependency-aware-workflows/README.md`
- `internal/variable/README.md`
- `internal/model/work_item.go`
- `internal/model/workflow_dependency.go` if present
- `internal/persistence/store.go`
- `internal/persistence/db_adapter_sqlite.go`
- `cmd/controller/main.go`
- controller workflow compilation files
- controller status files

Search for:

```text
Parameters
ResolvedValue
output_json
fingerprint
submission_context_json
workflow_dependency
work_item
status
```

## Allowed Production Files

Expected areas:

- `internal/model/work_item.go`
- `internal/model/*execution*` or a new narrow model file if needed
- `cmd/controller/*workflow*`
- `cmd/controller/*status*`
- `internal/persistence/store.go`
- `internal/persistence/db_adapter_sqlite.go`

Update `PROJECT_STATE.md` only if behavior is implemented and tested.

## Allowed Test Files

- controller workflow compilation tests
- controller status tests
- persistence tests touching work-item snapshots or workflow-run context
- model JSON round-trip tests for the execution envelope

## Out Of Scope

- Worker protected-value resolution.
- Environment-variable lookup.
- Python execution.
- Log redactor implementation.
- External secret stores.
- Data asset credential support.
- End-to-end smoke tests.

## Acceptance Criteria

- Controller compilation preserves protected references without resolving them.
- Assignment payloads sent to workers contain protected references, not plaintext.
- Controller status output shows redacted labels, not plaintext.
- Workflow-run snapshots and work-item records do not contain plaintext sentinel secrets.
- Fingerprints exclude plaintext sensitive values.
- If fingerprints include protected-reference identity, it is non-secret and documented.
- Unsupported client-env-to-remote-worker secret propagation is rejected or explicitly deferred.
- Existing non-sensitive workflows still compile and run as before.
- Relevant controller and persistence tests pass.

## Notes

Use a sentinel such as:

```text
goet-controller-should-not-store-this-secret-003
```

Tests should inspect serialized JSON/database fields where practical and fail if this exact value appears.

## Implemented State

Implemented on 2026-07-08.

- `model.WorkItem` can carry a `goet/execution-envelope/v1` execution envelope that separates public parameter values from protected references.
- Controller work-item persistence builds the envelope before writing `work_items.worker_payload_json`.
- `/work/next` rebuilds the envelope after assignment-time public parameter hydration so worker assignment JSON contains protected references and redaction labels, not plaintext secret values.
- Sensitive plaintext work-item parameters are rejected at the controller persistence boundary unless they use `protected_ref`.
- Client-submitted sensitive plaintext workflow-run variables are rejected before `submission_context_json` is created; protected-reference variables remain serializable because they contain provider/key metadata, not plaintext.
- Work-item and input fingerprints use execution-envelope variables. For protected references, the included identity is non-secret provider, key, redaction label, and optional materialization hint metadata. Plaintext sensitive values are not accepted into these fingerprints.

# 006 Python Subprocess Secret Materialization

Status: implemented

## Objective

Extend Python work-item execution so protected refs can be materialized for user-authored Python subprocesses through explicit, minimal runtime surfaces without placing secrets on the command line.

This slice accepts that user code is a weaker boundary. GOET can avoid accidental leaks through its own surfaces, but it cannot prevent malicious or careless scripts from exfiltrating secrets.

## Current State

Python execution can stage source, write `GOET_INPUT_JSON`, run an entrypoint, capture stdout/stderr, validate `GOET_OUTPUT_JSON`, and report completion. It does not yet materialize sensitive values from protected refs.

After slices 004-005, worker-side protected values and redaction primitives exist.

## Target State

A Python work item can request a sensitive value materialization:

```json
{
  "variables": {
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

Supported phase-1 materialization modes:

| Mode | Use |
|---|---|
| `env` | Small tokens expected by libraries or CLIs. |
| `file` | Credential JSON, config blobs, or values too awkward for env. |
| `stdin` | Deferred unless the current runner has a clean stdin contract. |

The Python command line must never contain the sensitive plaintext.

## Concept Decision

Use explicit materialization hints. Do not dump every sensitive value into the subprocess environment.

For `file` materialization:

- create files under attempt-local temp directories;
- use restrictive permissions where supported;
- pass the file path through env or `GOET_INPUT_JSON`, not the file content;
- delete the file after execution.

For `env` materialization:

- add only the requested env key to the subprocess environment;
- register the plaintext with the attempt redactor before execution;
- scrub captured stdout/stderr after execution.

If `GOET_OUTPUT_JSON` contains an exact materialized secret value, reject the output before persistence. Do not silently convert the secret into a downstream dependency value.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/sensitive-variable-propagation/README.md`
- `docs/concepts/python-workitem/README.md`
- `cmd/worker/work_python.go`
- `cmd/worker/work_python_test.go`
- worker protected-value/redactor files from slice 004
- worker context files from slice 005
- `internal/model/work_item.go`
- model execution envelope/protected-ref files from slice 003

Do not read controller scheduler, SSH, Slurm, or artifact files unless Python tests fail due to shared assumptions.

## Allowed Production Files

Expected files:

- `cmd/worker/work_python.go`
- `cmd/worker/python_secret_materialization.go` if a helper improves clarity
- `cmd/worker/redactor.go` only for narrow integration
- `cmd/worker/evidence.go` only if output validation must report sanitized evidence

## Allowed Test Files

- `cmd/worker/work_python_test.go`
- `cmd/worker/python_secret_materialization_test.go`
- `cmd/worker/redactor_test.go` only for missing helper coverage

## Out Of Scope

- Real Google Drive access.
- Real cloud secret stores.
- Artifact file scanning.
- Shell, R, or generic subprocess runners unless they already share the Python runner path.
- HPCC smoke.
- Controller persistence changes.

## Acceptance Criteria

- Python runner can materialize a `worker_env` protected ref into a subprocess env var.
- Python runner can materialize a protected ref into a temp file if file mode is included in the slice.
- Sensitive values are never placed in subprocess command-line arguments.
- Captured stdout/stderr are scrubbed for exact materialized secret values before persistence/reporting.
- A script that prints the secret does not cause the raw secret to appear in captured output evidence.
- A script that writes the exact secret to `GOET_OUTPUT_JSON` fails validation before output persistence.
- Temp secret files are removed after execution on success and failure.
- Missing worker env secret produces a sanitized failure.
- Existing Python work-item tests still pass.
- `go test ./cmd/worker` passes or the narrow Python worker tests pass with a documented reason for not running the full package.

## Notes

Do not claim this prevents arbitrary user code from leaking credentials. The protection is against GOET-controlled accidental leakage and ordinary output paths.

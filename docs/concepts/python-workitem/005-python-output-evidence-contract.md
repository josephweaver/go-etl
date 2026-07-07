# 005 Python Output Evidence Contract

Status: Complete

## Objective

Define and implement the worker-side evidence contract for successful Python script execution.

The runner should decode the script's `GOET_OUTPUT_JSON`, canonicalize the logical output, atomically promote the canonical output to `DataDir/{output_filename}`, and return deterministic `WorkEvidence` compatible with the controller completion path.

## Current State

`cmd/worker/work_demo.go` already contains helper behavior for:

- computing output file state;
- hashing files;
- hashing canonical JSON observations;
- writing output into `DataDir` through a temporary file and rename;
- returning `WorkEvidence` with input, output, pre-state, and post-state hashes.

`cmd/worker/state.go` converts `WorkEvidence` into `model.WorkCompletion` and posts it to `/work/complete`.

The controller completion path already expects input/output/pre/post hashes and JSON evidence.

After slice 004, Python execution should produce `work/output.json` but may not yet canonicalize, wrap, or promote that output in the final evidence shape.

## Target State

The Python runner treats `GOET_OUTPUT_JSON` as the script-produced logical output.

On success, the runner:

1. Reads `GOET_OUTPUT_JSON`.
2. Decodes exactly one JSON document.
3. Rejects invalid JSON.
4. Rejects trailing non-whitespace content.
5. Canonicalizes the logical output.
6. Atomically writes the canonical logical output to `DataDir/{output_filename}`.
7. Computes deterministic input/output/pre-state/post-state hashes.
8. Captures stdout/stderr hashes when the log files exist.
9. Returns a `WorkEvidence` whose `OutputJSON` is an evidence wrapper, not just the script output.

The evidence wrapper uses a schema name such as:

```text
goet/python-workitem-output/v1
```

The wrapper includes at least:

```text
schema
work_item_id
operation
entrypoint
environment
exit_code
logical_output
input_sha256
output_sha256
pre_state_sha256
post_state_sha256
```

The wrapper may include:

```text
stdout_sha256
stderr_sha256
```

## Concept Decision

This slice updates the Python worker operation concept and may also extract shared evidence helpers if that reduces duplication with existing demo evidence behavior.

If evidence helper extraction is needed, keep it small and local to `cmd/worker`. Do not introduce a general evidence package unless the code already has a clear package boundary.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/python-workitem/README.md`
- `internal/model/work_item.go`
- `cmd/worker/work_python.go`
- `cmd/worker/work_demo.go`
- `cmd/worker/state.go`
- `cmd/controller/main.go`

Do not read scheduler, transport, repository-source internals, or client setup files unless compile or test failures directly require it.

## Allowed Production Files

- `cmd/worker/work_python.go`
- `cmd/worker/work_demo.go`
- `cmd/worker/evidence.go`
- `cmd/worker/state.go`

## Allowed Test Files

- `cmd/worker/work_python_test.go`
- `cmd/worker/work_demo_test.go`
- `cmd/worker/evidence_test.go`
- `cmd/worker/state_test.go`
- `cmd/controller/main_test.go`

## Allowed Documentation Files

- `PROJECT_STATE.md`

## Out Of Scope

- Python subprocess launch mechanics unless needed to finish evidence behavior.
- Controller source-bundle endpoint.
- Worker bundle download/staging changes.
- Workflow compiler changes.
- Python environment creation.
- Worker-side skip/reuse for Python scripts.
- Observability/log streaming framework.
- Changing the public meaning of existing demo work evidence unless tests require helper extraction.

## Acceptance Criteria

- Python runner rejects invalid `GOET_OUTPUT_JSON`.
- Python runner rejects multiple JSON documents or trailing non-whitespace content.
- Equivalent logical output JSON formatting produces the same `output_sha256`.
- Canonical logical output is atomically promoted to `DataDir/{output_filename}`.
- Returned `WorkEvidence.OutputJSON` is a JSON evidence wrapper.
- Evidence wrapper includes top-level `input_sha256` and `output_sha256`.
- Evidence wrapper includes `logical_output` containing the script-produced output.
- `WorkEvidence.InputSHA256`, `OutputSHA256`, `PreStateSHA256`, and `PostStateSHA256` are populated.
- Existing controller completion tests still pass.
- `go test ./cmd/worker ./cmd/controller` passes.

## Notes

- The controller currently extracts reuse candidate facts from completion evidence. Preserve top-level `input_sha256` and `output_sha256` fields in the evidence wrapper so that behavior remains compatible.
- The first Python runner should not skip execution based on reuse candidates. It should still emit hashes so later reuse policies have evidence.
- Hash inputs should include enough operation context to distinguish the Python runner contract from existing demo work.
- Do not put stdout/stderr contents into `OutputJSON`; store hashes and log paths/evidence instead.


# 006 Add Wait Support

Status: Complete

## Objective

Implement `--wait` support for `goet submit`.

The `--wait` option allows automation, shell scripts, and CI jobs to block until a submitted workflow reaches a terminal state.

GOET intentionally does not implement a built-in watch capability in this concept. The CLI should provide stable submission/status primitives that can be composed with operating-system tools.

## Current State

Before this slice:

- `goet submit` can load CLI input files and submit a workflow.
- Successful submission returns a structured acknowledgement with `submission_id`.
- The controller exposes `GET /submissions/{submission_id}/status`.
- `goet status <submission_id>` can retrieve and print one submission's status.
- `--wait` is accepted by the parser but does not yet poll submission status until terminal state.
- There is no built-in `--watch` option.

## Target State

Implement:

```text
goet submit \
  --controller controller.json \
  --project project.json \
  --workflow workflow.json \
  --wait
```

and:

```text
goet submit \
  --controller-url http://localhost:8080 \
  --project project.json \
  --workflow workflow.json \
  --wait
```

Behavior:

1. Submit the workflow.
2. Receive the submission acknowledgement.
3. Poll:

   ```text
   GET /submissions/{submission_id}/status
   ```

4. Stop when the submission reaches a terminal state.
5. Print the final status in human-readable form.

Terminal states for this slice:

- `completed`
- `failed`

Exit behavior:

- Completed submission exits with status code `0`.
- Failed submission exits with a non-zero status code.
- Controller communication failure exits with a non-zero status code and prints a useful error.
- Unknown submission status or an unrecognized terminal state exits non-zero unless the implementation has a clearly safe interpretation.

The polling interval should come from the existing client status polling interval variable when available. If no interval is configured, use a small documented default such as `1s`.

## Concept Decision

This slice updates the existing `internal/client` polling concept. The wait loop belongs in reusable client behavior or a small helper called by the CLI, not as a one-off hidden loop that duplicates status HTTP handling.

The CLI remains a client of controller-owned status. It must not infer completion by inspecting local worker directories, output files, logs, or aggregate `/status` counts.

## Required Context

Read these files first:

- `docs/concepts/submission-cli-status/README.md`
- `docs/concepts/submission-cli-status/001-cli-client-contract.md`
- `docs/concepts/submission-cli-status/003-return-submission-ack.md`
- `docs/concepts/submission-cli-status/004-add-submission-status-api`
- `docs/concepts/submission-cli-status/005-add-cli-status-command.md`
- `cmd/demo-client/README.md`
- `cmd/demo-client/main.go`
- `cmd/demo-client/main_test.go`
- `internal/client/README.md`
- `internal/client/controller_client.go`
- `internal/client/controller_client_test.go`
- `internal/model/submission.go`

Do not read unrelated files unless test failures directly require them.

## Allowed Production Files

- `cmd/demo-client/main.go`
- `internal/client/controller_client.go`

## Allowed Test Files

- `cmd/demo-client/main_test.go`
- `internal/client/controller_client_test.go`

## Out Of Scope

- Implementing a built-in watch capability.
- Implementing continuous status display.
- Implementing a separate `goet wait` command.
- Implementing JSON output mode.
- Implementing hierarchical workflow or step rendering.
- Implementing new controller endpoints.
- Changing submission status semantics.
- Client-side remembered state.
- Artifact reporting.
- Attempt reporting.
- Retry behavior.
- Durable queue redesign.
- Python or R SDKs.
- Authentication or authorization.

## Acceptance Criteria

- `goet submit ... --wait` waits until the submitted workflow reaches a terminal state.
- The client polls the submission status endpoint during execution.
- Successful completion exits with status code `0`.
- Failed submissions exit with a non-zero status code.
- Controller communication failures return useful errors.
- The final human-readable status is printed before exit.
- Unit tests cover polling behavior.
- Unit tests cover completed, failed, and communication-error exit conditions.
- The implementation does not add or accept `--watch`.

## Notes

- `--wait` is intended primarily for automation, shell scripting, and CI pipelines.
- Users who want repeated display can compose the CLI with operating-system tools where available:

  ```bash
  watch -n 5 goet status <submission_id>
  ```

- This follows the design principle: do not duplicate capabilities the operating system already provides well.
- If GOET later needs native watch behavior for an environment without an equivalent OS-level tool, that should be a separate future Strategic Concept or Operational Slice.


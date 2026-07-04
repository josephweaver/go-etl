# 006 Add Wait Support

Status: Proposed

## Objective

Implement `--wait` support for `goet submit`.

The `--wait` option allows automation and shell scripts to block until a submitted workflow reaches a terminal state.

GOET intentionally does **not** implement a built-in watch capability. Existing operating system tools (for example, `watch` on Unix-like systems) already provide this functionality well. The CLI should provide stable status primitives rather than duplicate operating system features.

## Required Context

Read these files first:

* docs/concepts/submission-cli-status/README.md
* docs/concepts/submission-cli-status/001-upgrade-demo-client-cli-arguments.md
* docs/concepts/submission-cli-status/003-return-submission-acknowledgement.md
* docs/concepts/submission-cli-status/004-add-submission-status-api.md
* docs/concepts/submission-cli-status/005-add-cli-status-command.md
* cmd/demo-client/main.go
* internal/client/local_controller.go

Do not read unrelated files unless test failures require them.

## Allowed Production Files

* cmd/demo-client/main.go
* internal/client/local_controller.go

## Allowed Test Files

* cmd/demo-client/main_test.go
* internal/client/local_controller_test.go

## Required Behavior

Implement support for:

```text
goet submit \
    --controller controller.json \
    --project project.json \
    --workflow workflow.json \
    --wait
```

Behavior:

* Submit the workflow.
* Receive the submission acknowledgement.
* Poll:

```text
GET /submissions/{submission_id}/status
```

until the submission reaches a terminal state.

On successful completion:

* Print the final submission status.
* Exit with status code `0`.

On workflow failure:

* Print the final submission status.
* Exit with a non-zero status code.

On controller communication failure:

* Print a useful error.
* Exit with a non-zero status code.

## Terminal States

At minimum, the CLI should recognize:

* `completed`
* `failed`

Future terminal states may be added without changing the overall polling behavior.

## Out Of Scope

* Implementing a built-in watch capability.
* Implementing continuous status display.
* Implementing JSON output mode.
* Implementing hierarchical workflow or step rendering.
* Implementing new controller endpoints.
* Changing submission status semantics.
* Client-side remembered state.
* Python or R SDKs.
* Authentication or authorization.
* Artifact reporting.
* Retry behavior.
* Durable queue redesign.

## Acceptance Criteria

* `goet submit ... --wait` waits until the submission reaches a terminal state.
* The client polls the submission status endpoint during execution.
* Successful completion exits with status code `0`.
* Failed submissions exit with a non-zero status code.
* Controller communication failures return useful errors.
* Unit tests cover polling behavior and exit conditions.

## Notes

* `--wait` is intended primarily for automation, shell scripting, and CI pipelines.
* GOET intentionally does not implement a `--watch` option. Users can compose the CLI with operating system tools such as:

```bash
watch -n 5 goet status <submission_id>
```

This follows the design principle:

> **Do not duplicate capabilities that the operating system already provides well.**

If GOET is later ported to an environment without an equivalent facility, a native watch capability can be introduced as a separate future enhancement without changing the submission or status APIs.

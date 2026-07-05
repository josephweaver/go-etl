# 008 Update CLI Documentation And Examples

Status: Complete

## Objective

Update GOET documentation and project state to describe the implemented CLI submission and status workflow.

This slice records the user-facing behavior introduced by this Strategic Concept and makes the repo's current-state docs match the implemented CLI/status contract.

## Current State

Before this slice:

- The code implements the `submit`, `status`, `--wait`, and `--json` behavior from slices 001 through 007.
- `docs/CUSTOMER_API.md` describes the intended CLI-first API direction, but it does not yet reflect the verified implementation details from this concept.
- `PROJECT_STATE.md` still describes the demo client as a local runtime foundation and does not yet record the completed submission/status interface.
- The root `README.md` may not show the new CLI submission/status examples.
- The Strategic Concept README may still read like a plan unless updated to describe the implemented current state.

## Target State

User-facing documentation describes the current supported behavior without implying broader capabilities that GOET does not yet support.

Documentation should include examples of:

```text
goet submit \
  --controller controller.json \
  --project project.json \
  --workflow workflow.json
```

```text
goet submit \
  --controller-url http://localhost:8080 \
  --project project.json \
  --workflow workflow.json
```

```text
goet status <submission_id>
```

```text
goet submit \
  --controller controller.json \
  --project project.json \
  --workflow workflow.json \
  --wait
```

```text
goet submit \
  --controller controller.json \
  --project project.json \
  --workflow workflow.json \
  --json
```

```text
goet status <submission_id> --json
```

Documentation should also show repeated display using operating-system tools where available:

```bash
watch -n 5 goet status <submission_id>
```

The docs should explicitly state that GOET intentionally does not include a built-in `--watch` option in this concept.

## Concept Decision

This slice updates existing documentation concepts. It should not create new implementation code.

Update only the docs needed to make the implemented CLI/status behavior discoverable and to keep the project-state document accurate.

## Required Context

Read these files first:

- `docs/concepts/submission-cli-status/README.md`
- `docs/concepts/submission-cli-status/001-cli-client-contract.md`
- `docs/concepts/submission-cli-status/002-deserialize-cli-json-inputs.md`
- `docs/concepts/submission-cli-status/003-return-submission-ack.md`
- `docs/concepts/submission-cli-status/004-add-submission-status-api`
- `docs/concepts/submission-cli-status/005-add-cli-status-command.md`
- `docs/concepts/submission-cli-status/006-add-wait-support.md`
- `docs/concepts/submission-cli-status/007-add-json-output-support.md`
- `README.md`
- `docs/CUSTOMER_API.md`
- `docs/ARCHITECTURE_OVERVIEW.md`
- `PROJECT_STATE.md`
- `cmd/demo-client/README.md`
- `internal/client/README.md`
- `cmd/controller/README.md`

Do not read unrelated files unless documentation references directly require them.

## Allowed Production Files

- `README.md`
- `docs/CUSTOMER_API.md`
- `PROJECT_STATE.md`
- `docs/concepts/submission-cli-status/README.md`
- `cmd/demo-client/README.md`
- `internal/client/README.md`
- `cmd/controller/README.md`

## Allowed Test Files

None. This slice updates documentation only.

## Out Of Scope

- Changing implementation code.
- Creating broad workflow-authoring instructions for agents.
- Claiming support for arbitrary workflow generation.
- Documenting unsupported worker operations.
- Creating Python or R SDK documentation.
- Artifact command documentation.
- Attempt command documentation.
- Authentication or multi-user documentation.
- Durable queue or retry documentation.
- Execution observability documentation beyond mentioning it as future work.
- Adding `--watch` documentation as a supported GOET option.

## Acceptance Criteria

- The root `README.md` explains the implemented CLI submission path.
- `docs/CUSTOMER_API.md` reflects the implemented CLI behavior without overstating future SDKs.
- `PROJECT_STATE.md` records the new verified submission/status current state.
- Documentation shows human-readable submit and status examples.
- Documentation shows JSON submit and status examples.
- Documentation explains `--wait` behavior and exit-code intent.
- Documentation explains why there is no built-in `--watch` option.
- Documentation avoids implying unsupported workflow-authoring capabilities.
- The Strategic Concept README is updated only if the implemented state should be recorded there.

## Notes

- Keep examples aligned with currently supported demo worker operations.
- Do not document future workflow capabilities as current capabilities.
- A future concept can create agent-facing workflow-authoring instructions once the workflow language and worker operation set are stable enough for agents to author useful workflows.
- Do not hide implementation limitations. If a behavior is demo-only or local-only, state that precisely.

## Implementation Result

- Updated `README.md` to show the implemented `submit` and `status` commands, `--wait`, `--json`, and OS-level repeated status display.
- Updated `docs/CUSTOMER_API.md` to reflect the implemented CLI contract without introducing a built-in `--watch` option.
- Updated `cmd/demo-client/README.md`, `internal/client/README.md`, and `cmd/controller/README.md` so the package-level docs match the current submission/status boundary.
- Updated `PROJECT_STATE.md` to record the documented current state.


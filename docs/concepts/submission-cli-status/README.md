# Submission CLI Status

Status: Implemented

## Purpose

This Strategic Concept defined the first stable, scriptable GOET command-line submission and status interface. The contract is now implemented in the repository docs and current CLI boundary.

GOET already has a controller, worker runtime, workflow submission path, source-admitted Python work-item execution path, and aggregate controller status endpoint. The missing public boundary is a user-facing submission handle. This concept introduces that boundary through:

```text
goet submit
goet status <submission_id>
goet submit ... --wait
goet submit ... --json
goet status ... --json
```

The core product gain is a first-class `submission_id` that a CLI user, shell script, CI job, or future Python/R wrapper can use to monitor one submitted workflow without knowing internal queue, worker, or persistence details.

## Goals

The completed Strategic Concept should enable GOET to:

- Accept workflow submissions through a command-shaped CLI rather than the current positional demo executable behavior.
- Accept exactly one controller selection mode for submission:
  - `--controller <controller.json>` for a client that may start or contact a local controller from a controller document.
  - `--controller-url <url>` for a client that submits to an already-running controller.
- Require explicit `--project <project.json>` and `--workflow <workflow.json>` paths for CLI submission.
- Treat `project.json` as project-scoped input that becomes `project_config` variables unless a later public schema says otherwise.
- Reuse the existing controller-owned workflow submission boundary rather than making the CLI compile workflows or manage work items.
- Return a structured submission acknowledgement after successful controller admission.
- Include a stable `submission_id` in that acknowledgement.
- Provide a controller-owned status endpoint for one submitted workflow:

  ```text
  GET /submissions/{submission_id}/status
  ```

- Implement `goet status <submission_id>` as a thin client of the controller status endpoint.
- Implement `goet submit ... --wait` by polling submission status until the submission reaches a terminal state.
- Implement `--json` for submission acknowledgement and status output without mixing human-readable text into standard output.
- Keep human-readable CLI output as the default interactive interface.
- Keep JSON CLI output stable enough for shell scripts and future Python/R adapters.
- Preserve the controller as the only owner of orchestration state, workflow compilation, scheduling decisions, worker coordination, and queue/status facts.

## Non-Goals

This Strategic Concept does not include:

- Python SDK implementation.
- R SDK implementation.
- Permanent binary packaging, installation, or repository-wide command renaming outside the current `cmd/demo-client` executable.
- Authentication or authorization redesign.
- Multi-user controller semantics.
- Durable queue schema redesign beyond the narrow status facts needed by this concept.
- Retry policy implementation.
- Artifact browsing, artifact download, or artifact status commands.
- Attempt browsing or attempt detail commands.
- Execution observability, logs streaming, or hierarchical event reporting.
- Dependency-aware workflow scheduling.
- Resource-aware admission or worker-capacity policy changes.
- Python environment creation, caching, or dependency installation.
- Workflow language redesign beyond the minimum input loading needed for CLI submission.
- Built-in `--watch` behavior. Repeated display should be composed with operating-system tools such as `watch` where available.
- Client-side remembered state, cached submission IDs, or hidden controller selection state.

## Architectural Context

GOET's public direction is `controller.submit(project, workflow)`. The near-term customer interface is the CLI; future Python and R APIs should adapt the same controller/project/workflow JSON files and submission/status contract rather than inventing a separate configuration model.

The controller remains the authoritative orchestration boundary. It owns workflow admission, workflow compilation, work-item generation, queue transitions, worker startup decisions, status facts, attempt recording, and persistence access. The CLI is a transport and presentation layer: it loads user-supplied files, selects or reaches a controller, submits the payload, prints acknowledgements/status, and optionally waits.

This concept belongs between the completed Python WorkItem phase and later SDK/client work. Python scripts can now execute through admitted source. The next missing capability is a stable way for a user or wrapper to submit that work and monitor one submission by ID.

## Current State

### Strategic current state

GOET has a local Go controller/worker runtime, a completed first Python WorkItem execution path, and an implemented CLI submission/status contract. The current public interaction model is now the command-shaped submission boundary rather than only a local demo path.

The project documentation now records the implemented CLI-first customer interface that consumes canonical `controller.json`, `project.json`, and `workflow.json` files, with Python and R wrappers still expected to follow as thin adapters over the same public model.

### Operational current state

- `cmd/demo-client/main.go` recognizes `submit` and `status` and keeps the zero-argument compatibility path.
- The CLI supports explicit controller, project, and workflow JSON inputs, submission acknowledgements, submission-scoped status, `--wait`, and `--json`.
- `internal/client` owns controller reachability checks, CLI input loading, submission acknowledgement handling, submission status retrieval, wait polling, and client-initiated shutdown after aggregate idle state.
- The controller returns `submission_id`, `workflow_id`, and `initial_work_item_count` for successful submission.
- `cmd/controller/main.go` owns the HTTP API surface, including `POST /workflow`, `GET /status`, `GET /submissions/{submission_id}/status`, worker assignment/report endpoints, and shutdown.
- `goet submit` and `goet status` remain thin clients of controller-owned state.
- Repeated status display is documented as an operating-system composition, such as `watch -n 5 goet status <submission_id>`, rather than a built-in `--watch` option.

## Target State

### Strategic target state

GOET now has a CLI-first submission/status contract that can serve as the first public customer interface and as the future substrate for Python and R wrappers. A submitted workflow has a public `submission_id`, and all status/wait behavior is expressed through that stable identifier.

The controller remains the only authority for orchestration state. The CLI does not infer workflow progress from local files, remember submissions, inspect worker directories, or own scheduling decisions.

### Operational target state

- `cmd/demo-client` recognizes the public command shape intended to become `goet`:

  ```text
  goet submit --controller <controller.json> --project <project.json> --workflow <workflow.json>
  goet submit --controller-url <url> --project <project.json> --workflow <workflow.json>
  goet status <submission_id>
  ```

- `goet submit` validates controller selection, project path, workflow path, `--wait`, and `--json`.
- `goet submit` does not accept `--watch`.
- `goet status` validates `submission_id`, accepts optional `--controller-url`, accepts `--json`, and defaults the controller URL to `http://localhost:8080` when no URL is supplied.
- The CLI can load `controller.json`, `project.json`, and `workflow.json` from explicit user paths.
- The controller returns a structured acknowledgement for successful workflow admission:

  ```json
  {
    "submission_id": "sub_1234",
    "workflow_id": "annual-report",
    "initial_work_item_count": 47
  }
  ```

- The controller exposes submission-scoped status:

  ```json
  {
    "submission_id": "sub_1234",
    "workflow_id": "annual-report",
    "status": "running",
    "known_work_items": 47,
    "queued": 20,
    "running": 4,
    "completed": 23,
    "failed": 0,
    "skipped": 0
  }
  ```

- `goet status <submission_id>` prints a compact human-readable status summary by default.
- `goet submit ... --wait` polls `GET /submissions/{submission_id}/status` until a terminal state is reached.
- `--json` emits valid JSON to standard output and writes diagnostics/errors to standard error.
- Documentation describes the implemented CLI behavior and explicitly explains that repeated display should use operating-system tools rather than a built-in `--watch` option.

The concept is implemented; the remaining text serves as the durable description of the delivered contract.

## Proposed Slices

These Operational Slices should be implemented one at a time. Each slice is intentionally small enough to give Codex a bounded file list and a concrete observable target.

| Slice | Document | Purpose |
| --- | --- | --- |
| 001 | `001-cli-client-contract.md` | Add the top-level `submit` and `status` command parser/validation contract in `cmd/demo-client`, without `--watch`. |
| 002 | `002-deserialize-cli-json-inputs.md` | Load explicit CLI JSON input files and wire `goet submit` to the existing workflow submission path while it still accepts the old no-content response. |
| 003 | `003-return-submission-ack.md` | Change successful workflow submission to return a structured acknowledgement with `submission_id`, `workflow_id`, and initial work-item count. |
| 004 | `004-add-submission-status-api` | Add `GET /submissions/{submission_id}/status` as the controller-owned status API for one submission. |
| 005 | `005-add-cli-status-command.md` | Make `goet status <submission_id>` call the submission status API and print human-readable output. |
| 006 | `006-add-wait-support.md` | Implement `goet submit ... --wait` by polling submission status until terminal state. |
| 007 | `007-add-json-output-support.md` | Implement `--json` output for submit/status while keeping stderr diagnostics separate. |
| 008 | `008-update-cli-docs.md` | Update user-facing docs and project state to describe the implemented CLI/status behavior. |

## Completion Criteria

This Strategic Concept is complete when:

- All agreed Operational Slices are implemented and accepted.
- `cmd/demo-client` provides the intended `submit` and `status` command shape.
- `goet submit` validates controller, project, and workflow arguments.
- `goet submit` can submit a workflow through the existing controller-owned workflow admission boundary.
- Successful workflow admission returns a structured submission acknowledgement.
- The acknowledgement includes a stable `submission_id`.
- The acknowledgement includes the submitted `workflow_id`.
- The acknowledgement includes the number of work items initially created or queued during admission.
- `GET /submissions/{submission_id}/status` returns status for one submission.
- `goet status <submission_id>` displays that status.
- `goet submit ... --wait` exits `0` for completed submissions and non-zero for failed submissions or communication errors.
- `--json` output for submit/status is valid JSON with no extra human-readable text on standard output.
- The CLI intentionally has no built-in `--watch` option.
- Unit tests cover parser validation, JSON input loading, acknowledgement handling, status API behavior, CLI status rendering, wait polling, and JSON output separation.
- User-facing documentation reflects the implemented behavior and does not imply unsupported SDKs, artifact commands, retry policies, or workflow-authoring capabilities.
- Public interfaces remain consistent with GOET's controller-owned orchestration architecture.

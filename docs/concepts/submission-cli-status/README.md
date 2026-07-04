# Submission CLI Status

Status: Proposed

## Purpose

This epic upgrades GOET from a demonstration client to the first production-oriented command-line interface.

The new CLI becomes the canonical customer entry point for submitting workflows, monitoring execution, and interacting with a GOET controller.

The epic also introduces the concept of a **Submission** as a first-class controller object. A submission represents a single invocation of a workflow for a project and provides a stable identifier that can be queried throughout execution.

This capability establishes the architectural foundation for future Python and R SDKs, which are expected to be thin adapters over the same submission model.

---

# Goals

The completed epic should enable GOET to:

* Submit workflows through a production-oriented CLI.
* Accept either:

  * `--controller controller.json`, or
  * `--controller-url <url>`,
    but never both.
* Submit canonical:

  * `controller.json`
  * `project.json`
  * `workflow.json`
* Treat `project.json` as the contents of the `project_config` variable namespace.
* Return a successful submission acknowledgement containing:

  * `submission_id`
  * number of initially queued work items
* Introduce a first-class Submission model within the Controller.
* Allow submission status to be queried using:

  * `goet status <submission_id>`
* Support:

  * `--wait`
* Report execution progress using submission-oriented status information.
* Preserve the existing controller ownership of orchestration decisions.
* Preserve compatibility with future Python and R APIs.

---

# Non-Goals

This epic does **not** include:

* Python SDK implementation.
* R SDK implementation.
* Authentication redesign.
* User management.
* Durable queue redesign.
* Retry policies.
* Artifact browsing or downloading.
* Plugin architecture redesign.
* Remote SSH setup automation.
* Scheduler-specific enhancements.
* Workflow language redesign beyond what is required for submission.

---

# Architectural Context

This epic defines the first public-facing customer interaction model for GOET.

The CLI should remain a thin interface over the controller rather than embedding orchestration logic.

The Controller continues to own:

* workflow compilation
* work-item generation
* scheduling
* worker management
* execution state

The CLI owns:

* configuration loading
* controller discovery/startup
* submission
* status presentation

The Submission model becomes the public execution handle returned by the Controller after successful workflow compilation.

Future Python and R SDKs should build upon this same submission model rather than introducing alternative APIs.

---

# Proposed CLI

Examples:

```text
goet submit \
    --controller controller.json \
    --project project.json \
    --workflow workflow.json

goet submit \
    --controller-url http://localhost:8080 \
    --project project.json \
    --workflow workflow.json

goet submit \
    --controller controller.json \
    --project project.json \
    --workflow workflow.json \
    --wait

goet status <submission_id>


The options `--controller` and `--controller-url` are mutually exclusive.

When a controller configuration file is supplied, the client may start a local controller before submitting the workflow.

---

# Status Model

The exact presentation remains subject to refinement during implementation.

The intended hierarchy is:

```text
Submission
    Workflow
        Step
            Work Item State
```

Representative work-item states include:

* Queued
* Running
* Completed
* Failed
* Skipped

The CLI may present both summary counts and hierarchical execution information.

---

# Proposed Slices

These represent the current understanding of the work and may change during planning.

```text
001 Upgrade Demo Client CLI Arguments
002 Deserialize CLI JSON Inputs
003 Return Submission Acknowledgement
004 Add Submission Status API
005 Add CLI Status Command
006 Add Wait Support
007 Add JSON Output Support
008 Update CLI Documentation And Examples
```

---

# Completion Criteria

This epic is complete when:

* A user can submit a workflow using the CLI.
* The controller returns a `submission_id`.
* The controller reports how many work items were created.
* A user can query submission progress using the returned submission ID.
* `--wait` waits until the submission reaches a terminal state.
* The CLI uses canonical controller, project, and workflow configuration files.
* The submission model is suitable as the future foundation for Python and R SDKs.
* Public interfaces remain consistent with the GOET architecture.

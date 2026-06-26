# Customer API

Last updated: 2026-06-26

## Purpose

This document defines the intended customer-facing API direction for GOET.

GOET should expose a stable submission model that lets customers run workflows without modifying GOET core. The near-term customer interface is expected to be a scriptable CLI. Python and R APIs are expected to follow as thin adapters over the same canonical JSON files and controller API.

The central public model is:

```text
controller.submit(project, workflow)
```

In this model:

```text
Controller = where and how work executes
Project    = who the work is for and what defaults/assets apply
Workflow   = what work should be performed
```

## API Goals

The customer API should:

- Keep GOET core independent from customer-specific workflows.
- Make workflows reusable across projects and customers.
- Allow one project to run many workflows.
- Allow one workflow to run for many projects.
- Keep JSON files usable across CLI, Python, R, and future clients.
- Treat Python and R APIs as ergonomic adapters, not separate sources of truth.
- Preserve the current controller-worker split.
- Avoid exposing internal Go package structure as the public API.

## Canonical Objects

### Controller

A `Controller` describes the execution service and execution environment.

It answers:

```text
Where and how should this work execute?
```

Controller configuration may include:

- Controller URL.
- Controller startup settings.
- Ledger path or ledger backend.
- Execution environment.
- Transport configuration.
- Shell dialect configuration.
- Scheduler configuration.
- Runtime configuration.
- Worker scaling defaults.
- Status polling defaults.

Examples:

```text
local process controller
local Docker controller
Dockerized Slurm fake-HPCC controller
SSH-accessible HPCC controller
cloud-backed controller
```

### Project

A `Project` describes the customer, research project, or logical work context.

It answers:

```text
Who is this work for, and what project-level assets and defaults apply?
```

Project configuration may include:

- Project ID.
- Customer or lab context.
- Data roots.
- Artifact roots.
- Secret references.
- Default variables.
- Default code version.
- Default worker image or plugin choices.
- Project-level storage policy.
- Project-level resource limits.

A project should not be required to own a workflow. The same public workflow may run for multiple projects.

### Workflow

A `Workflow` describes reusable work.

It answers:

```text
What should be performed?
```

Workflow configuration may include:

- Workflow ID.
- Workflow variables.
- Steps.
- Fan-out rules.
- Work-item templates.
- Parameter bindings.
- Expected artifacts.
- Workflow-level defaults.

A workflow should be portable where possible. Customer-specific paths, secrets, and deployment details should usually live in the project or controller configuration rather than inside a reusable workflow.

### Overrides

Overrides provide run-specific values.

They answer:

```text
What is different for this submission?
```

Overrides should map naturally onto the existing `override` variable namespace.

Examples:

```text
override.code_version
override.worker_max_count
override.controller_url
override.client_status_poll_interval
```

Overrides are useful for experiments, one-off capacity changes, temporary paths, or explicitly selected code versions.

## CLI-First API

The near-term public API should be a scriptable CLI.

Representative shape:

```bash
goet submit \
  --controller controller.json \
  --project project.json \
  --workflow workflow.json \
  --override code_version=experiment-17 \
  --wait \
  --json
```

The CLI should be designed as a stable machine interface, not only as an interactive demo tool.

Important CLI properties:

- Accept explicit file paths.
- Produce parseable JSON output when requested.
- Return meaningful process exit codes.
- Support non-interactive execution.
- Support optional interactive setup as a separate subcommand.
- Keep submission, status, setup, and administrative commands distinct.

Possible command family:

```text
goet submit
goet status
goet wait
goet artifacts
goet attempts
goet setup
goet shutdown
```

## Canonical JSON Files

The JSON files used by the CLI should remain usable by future Python and R adapters.

This means the JSON files are not merely CLI flags serialized to disk. They are the canonical public data model.

Near-term file set:

```text
controller.json
project.json
workflow.json
```

The intended cross-language relationship is:

```text
CLI        reads controller.json, project.json, workflow.json
Python     reads or constructs the same objects
R          reads or constructs the same objects
REST       receives equivalent JSON payloads
Go client  uses equivalent structures internally
```

The JSON model should therefore be kept language-neutral.

## CLI to Python/R Evolution

### Phase 1: CLI

The first stable customer interface is CLI-first.

Example:

```bash
goet submit \
  --controller controller.json \
  --project project.json \
  --workflow workflow.json \
  --wait
```

This is sufficient for:

- Shell users.
- CI systems.
- R users via `system()` or `processx`.
- Python users via `subprocess`.
- Scheduled jobs.
- Early customer deployments.

### Phase 2: Thin Python/R Wrappers

Python and R wrappers can initially call the CLI or the controller HTTP API while still relying on the same JSON files.

Python example:

```python
goet.submit(
    controller="controller.json",
    project="project.json",
    workflow="workflow.json",
    wait=True,
)
```

R example:

```r
goet_submit(
  controller = "controller.json",
  project = "project.json",
  workflow = "workflow.json",
  wait = TRUE
)
```

These wrappers should not introduce a second configuration model.

### Phase 3: Object APIs

Later Python and R APIs may expose first-class objects.

Python example:

```python
controller = goet.Controller.from_file("controller.json")
project = goet.Project.from_file("project.json")
workflow = goet.Workflow.from_file("workflow.json")

submission = controller.submit(project, workflow)
```

R example:

```r
controller <- goet_controller("controller.json")
project <- goet_project("project.json")
workflow <- goet_workflow("workflow.json")

submission <- goet_submit(controller, project, workflow)
```

The object APIs should still serialize cleanly back to the canonical JSON form.

## Public Submission Model

The public submission model combines four sources:

```text
controller configuration
project configuration
workflow configuration
runtime overrides
```

Conceptual flow:

```text
controller.submit(project, workflow, overrides)
        |
        v
build submission envelope
        |
        v
resolve typed variables by namespace and precedence
        |
        v
compile workflow into work items
        |
        v
append pending work to controller queue
        |
        v
start or scale workers through execution environment
        |
        v
workers pull, execute, and report
        |
        v
controller records attempts and status
```

## REST Boundary

The controller HTTP API is an implementation boundary and may also become a public API.

The preferred customer-facing REST shape should be workflow submission, not raw work-item submission.

Possible target endpoint:

```text
POST /submissions
```

Payload shape:

```json
{
  "project": {},
  "workflow": {},
  "overrides": {}
}
```

The controller configuration may already be known to the running controller. In local or self-started modes, the CLI may use `controller.json` to find or start the controller before submitting the project/workflow payload.

Existing low-level endpoints such as raw work submission can remain useful for tests and administration, but should not be treated as the primary customer API.

## Reusable Workflow Pattern

A major reason to keep `controller.submit(project, workflow)` is workflow reuse.

Example:

```python
controller = goet.Controller.from_file("hpcc-controller.json")

customer_a = goet.Project.from_file("customer-a/project.json")
customer_b = goet.Project.from_file("customer-b/project.json")

annual_report = goet.Workflow.from_file("workflows/annual-report.json")

controller.submit(customer_a, annual_report)
controller.submit(customer_b, annual_report)
```

The workflow is reusable. The project supplies the customer-specific context.

The reverse should also work:

```python
project = goet.Project.from_file("customer-a/project.json")

controller.submit(project, annual_report)
controller.submit(project, monthly_report)
controller.submit(project, validation_workflow)
```

One project can run many workflows.

## Public vs Internal Concepts

The public API should emphasize:

```text
Controller
Project
Workflow
Submission
Status
Attempt
Artifact
Override
```

The internal implementation may include:

```text
Transport
ShellDialect
Scheduler
Runtime
WorkItem
VariableScope
Ledger row
WorkerLaunchConfig
Controller.env
```

Some internal concepts may eventually become public extension points, but they should not become customer-facing accidentally.

## Stability Rules

### Stable First

The JSON file structure should become the most stable surface first. The CLI, Python API, R API, and REST API should all map to it.

### Thin Adapters

Python and R should initially adapt the canonical model rather than invent independent models.

### Explicit Versioning

Public JSON files should eventually include a schema or API version field.

Example:

```json
{
  "api_version": "goet/v1alpha1",
  "kind": "Workflow",
  "metadata": {
    "id": "annual-report"
  },
  "spec": {}
}
```

The exact schema is not yet fixed, but versioning should be introduced before external customer commitments.

### Backward Compatibility

Once customers depend on a JSON schema, changes should be additive where possible. Breaking changes should require a version change or migration tool.

## Non-Goals

The customer API should not require customers to:

- Edit GOET source code.
- Fork the GOET repository.
- Understand internal Go package layout.
- Submit raw work items for normal workflow use.
- Put customer secrets in GOET core.
- Duplicate configuration models across CLI, Python, and R.

## Near-Term Recommendation

Build the CLI first, but design it as the backend for future SDKs.

The immediate target should be:

```bash
goet submit --controller controller.json --project project.json --workflow workflow.json --wait --json
```

Then Python and R can follow quickly as thin adapters over the same canonical files.

This preserves speed now while avoiding an API dead end later.

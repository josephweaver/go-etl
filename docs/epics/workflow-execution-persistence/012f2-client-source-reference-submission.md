# 012f2 Client Source-reference Workflow Submission

Status: implemented

## Objective

Update the Go client boundary so clients submit source-control references for
project and workflow JSON documents instead of submitting inline workflow JSON.

This slice aligns `internal/client` and `cmd/demo-client` with the corrected
`/workflow` contract: `/workflow` admits a project/workflow run by source
reference. The controller may later compile work items from the admitted
workflow, but the client does not submit work items or raw workflow documents.

## Required Context

Read these files first:

- `docs/epics/workflow-execution-persistence/012f-remove-in-memory-queue-authority.md`
- `docs/epics/workflow-execution-persistence/012-controller-integration-cutover.md`
- `internal/client/workflow.go`
- `internal/client/workflow_test.go`
- `internal/client/README.md`
- `cmd/demo-client/main.go`
- `cmd/demo-client/main_test.go`
- `cmd/demo-client/README.md`
- `cmd/controller/main.go`

## Current State

`internal/client.WorkflowClient` currently exposes inline workflow submission:

```go
type WorkflowSubmission struct {
    Workflow  workflow.Workflow   `json:"workflow"`
    Variables []variable.Variable `json:"variables"`
}

func (c WorkflowClient) SubmitWorkflow(submission WorkflowSubmission) error
func (c WorkflowClient) SubmitWorkflowFile(path string) error
func LoadWorkflowSubmissionFile(path string) (WorkflowSubmission, error)
```

`cmd/demo-client` calls:

```go
workflowClient.SubmitWorkflowFile(demoWorkflowPath(os.Args))
```

This decodes `demo-workflow.json` and posts the inline workflow document to
`/workflow`. That is the old path. The new client path should send source
references.

## New Client Contract

Introduce a source-reference submission envelope owned by `internal/client` or
shared transport model code:

```go
type WorkflowRunSubmission struct {
    Project  SourceDocumentReference `json:"project"`
    Workflow SourceDocumentReference `json:"workflow"`
    Variables []variable.Variable `json:"variables,omitempty"`
}

type SourceDocumentReference struct {
    Repository string `json:"repository"`
    Ref        string `json:"ref"`
    Path       string `json:"path"`
}
```

Field names are implementation candidates, not final API commitments. The
important contract is:

- `repository` identifies the source-control repository.
- `ref` is the user-provided ref, such as a branch, tag, or commit.
- `path` is the repository-relative document path.

The controller is responsible for resolving `ref` to an immutable commit before
admitting the run. The client sends the requested source reference; it does not
load the JSON document and does not compute the durable commit identity.

## Client API Changes

Preferred public client methods:

```go
func (c WorkflowClient) SubmitWorkflowRun(submission WorkflowRunSubmission) error
func LoadWorkflowRunSubmissionFile(path string) (WorkflowRunSubmission, error)
func (c WorkflowClient) SubmitWorkflowRunFile(path string) error
```

The old inline methods should either be removed or renamed clearly as legacy
test helpers:

```go
SubmitInlineWorkflow(...)
LoadInlineWorkflowSubmissionFile(...)
```

The preferred implementation is to stop using the old inline methods from
`cmd/demo-client`. If keeping them temporarily avoids a large test rewrite, mark
them deprecated and keep them out of the demo executable.

## Submission File Shape

The new demo/client submission file should look like:

```json
{
  "project": {
    "repository": "owner/repository",
    "ref": "main",
    "path": "project.json"
  },
  "workflow": {
    "repository": "owner/repository",
    "ref": "main",
    "path": "workflows/demo-workflow.json"
  },
  "variables": []
}
```

For a local source-control cache or fixture-backed development mode,
`repository` may use a local identity such as:

```text
local:demo
```

The exact local repository identity belongs to the source-reference resolver
slice. The client should treat it as an opaque string.

## Demo Client Change

`cmd/demo-client` should change from workflow-file selection to workflow-run
submission-file selection.

Current:

```go
demoWorkflowPath(args []string) string
SubmitWorkflowFile(path)
```

Target:

```go
demoWorkflowRunPath(args []string) string
SubmitWorkflowRunFile(path)
```

The demo file should no longer contain the workflow definition itself. It should
contain project/workflow source references.

## Acceptance Criteria

- `internal/client` has a source-reference workflow run submission type.
- `internal/client` can load a workflow run submission file containing
  project/workflow source references.
- `internal/client` submits that envelope to `/workflow`.
- `cmd/demo-client` uses the source-reference submission file path and no
  longer calls `SubmitWorkflowFile`.
- Existing controller reachability, startup, status polling, and shutdown
  behavior remain unchanged.
- Inline workflow JSON submission is no longer the normal client/demo path.
- Tests assert that the posted `/workflow` body contains project/workflow source
  references, not a decoded workflow definition.

## Out Of Scope

- Controller implementation of source-reference admission.
- GitHub API integration.
- Local source-control cache implementation.
- Resolving refs to immutable commits.
- Materializing project/workflow files.
- Retiring every old inline workflow test in the controller.
- Python/R client API changes.

## Ambiguity To Review

The client needs a stable source-reference field vocabulary. `repository`,
`ref`, and `path` are the likely minimum. It is still open whether the client
should send separate project and workflow repositories, or whether workflow
should normally inherit the project repository/ref with only a workflow path.

Recommendation for 012f2: support separate project and workflow references in
the envelope even when they point to the same repository/ref. That keeps the
transport explicit and avoids hidden inheritance rules in the first cut.

It is also open what local demo repository identity should look like before the
source-control resolver exists. Use an opaque placeholder in the client fixture
and let the controller/source-reference slice define how it is resolved.

## Implementation Notes

- `internal/client` now defines `WorkflowRunSubmission` and
  `SourceDocumentReference`.
- `WorkflowClient.SubmitWorkflowRun` and `SubmitWorkflowRunFile` post the
  source-reference envelope to `/workflow`.
- `LoadWorkflowRunSubmissionFile` loads a source-reference submission file.
- The old inline workflow methods remain as legacy helpers and are marked in
  comments.
- `cmd/demo-client` now defaults to `../go-etl-demo-project/submissions/demo-workflow-run.json` and calls
  `SubmitWorkflowRunFile`.
- `demo-workflow-run.json` uses opaque `local:demo` repository references for
  the project and workflow documents.

Controller-side source-reference admission remains out of scope for this slice,
so the demo client now sends the new form but the persisted controller still
needs a follow-up slice to accept and resolve it.

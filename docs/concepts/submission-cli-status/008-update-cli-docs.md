# 008 Update CLI Documentation And Examples

Status: Proposed

## Objective

Update GOET documentation and examples to describe the new CLI submission and status workflow.

This slice documents the user-facing behavior introduced by this epic:

* submitting workflows through the CLI
* using controller, project, and workflow JSON files
* receiving a submission acknowledgement
* checking submission status
* waiting for completion
* requesting JSON output

The documentation should describe the current supported behavior without implying broader workflow-authoring capabilities that GOET does not yet support.

## Required Context

Read these files first:

* docs/concepts/submission-cli-status/README.md
* docs/concepts/submission-cli-status/001-upgrade-demo-client-cli-arguments.md
* docs/concepts/submission-cli-status/002-deserialize-cli-json-inputs.md
* docs/concepts/submission-cli-status/003-return-submission-acknowledgement.md
* docs/concepts/submission-cli-status/004-add-submission-status-api.md
* docs/concepts/submission-cli-status/005-add-cli-status-command.md
* docs/concepts/submission-cli-status/006-add-wait-support.md
* docs/concepts/submission-cli-status/007-add-json-output-support.md
* README.md
* docs/CUSTOMER_API.md
* PROJECT_STATE.md

Do not read unrelated files unless documentation references require them.

## Allowed Production Files

* README.md
* docs/CUSTOMER_API.md
* PROJECT_STATE.md
* docs/concepts/submission-cli-status/README.md

## Allowed Test Files

None.

This slice updates documentation only.

## Required Behavior

Update documentation to include examples of:

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
goet submit ... --wait
```

```text
goet submit ... --json
```

```text
goet status <submission_id> --json
```

Also document that repeated status display should use operating-system tools where available, for example:

```bash
watch -n 5 goet status <submission_id>
```

The documentation should explain that GOET intentionally does not include a built-in `--watch` option at this stage because existing operating-system tools already provide that behavior.

## Out Of Scope

* Creating broad workflow-authoring instructions for agents.
* Claiming support for arbitrary workflow generation.
* Documenting unsupported worker operations.
* Creating Python or R SDK documentation.
* Artifact command documentation.
* Attempts command documentation.
* Authentication or multi-user documentation.
* Durable queue or retry documentation.
* Changing implementation code.

## Acceptance Criteria

* The README explains the new CLI submission path.
* `docs/CUSTOMER_API.md` reflects the implemented CLI behavior.
* Documentation shows human-readable and JSON examples.
* Documentation explains `--wait`.
* Documentation explains why there is no built-in `--watch`.
* Documentation avoids implying unsupported workflow-authoring capabilities.
* `PROJECT_STATE.md` is updated to reflect the new verified CLI behavior.

## Notes

* Keep examples aligned with currently supported demo worker operations.
* Do not document future workflow capabilities as current capabilities.
* A future epic or slice should create `AGENT_INSTRUCTIONS_FOR_WORKFLOWS.md` once the workflow language and worker operation set are stable enough for agents to author useful workflows.

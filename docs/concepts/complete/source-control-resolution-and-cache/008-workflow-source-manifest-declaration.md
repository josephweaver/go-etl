# 008 Workflow Source Manifest Declaration

Status: Complete

## Objective

Define and load the workflow-declared source manifest that names supplemental
repository files required by a run before admission.

This slice gives workflow authors a concrete JSON shape for declaring Python
entrypoints, Python environment specifications, and support files. It does not
read those supplemental files, publish cache entries, materialize staging
directories, or execute Python work.

## Current State

Source-reference workflow run submissions name:

- project document repository/ref/path;
- workflow document repository/ref/path;
- optional override variables.

The workflow source document now decodes into `WorkflowSubmission`, which
contains `workflow.Workflow`, optional `source_manifest`, and variables.
`internal/reposource` defines and validates the user-facing declaration for
supplemental repository files. Later admission behavior still needs to consume
that declaration when building the admitted source manifest.

The first intended Python executor needs at least:

- one or more Python script files;
- a Python environment specification;
- optional helper/support files.

Those files must be declared before run admission. They must not be discovered
by a worker during execution.

## Target State

Workflow source documents may include a top-level `source_manifest` object:

```json
{
  "workflow": {
    "ID": "python-demo",
    "Steps": []
  },
  "source_manifest": {
    "files": [
      {
        "role": "python_entrypoint",
        "path": "scripts/train.py",
        "content_type": "text/x-python"
      },
      {
        "role": "python_environment",
        "path": "environments/python.json",
        "content_type": "application/json"
      },
      {
        "role": "support_file",
        "path": "scripts/lib/helpers.py",
        "content_type": "text/x-python"
      }
    ]
  },
  "variables": []
}
```

The declaration names source files relative to the workflow repository. The
controller later converts each validated `path` into both `source_path` and
`cache_path` in the admitted source manifest. Workflow authors do not specify
cache paths.

Project and workflow JSON files remain implicit required source files. The
`source_manifest` names supplemental files only.

## Concept Decision

This slice adds a new user-facing source declaration concept.

The declaration model should live beside repository-source model code so OS 002
can convert it into admitted source manifest entries. The current client and
controller workflow-source document shapes may each embed the declaration type
or duplicate a narrow transport shape if that avoids an import cycle.

## Required Context

Read these files first:

- `docs/concepts/complete/source-control-resolution-and-cache/README.md`
- `docs/concepts/complete/source-control-resolution-and-cache/001-repository-source-model-and-path-safety.md`
- `docs/concepts/complete/source-control-resolution-and-cache/002-provider-reads-and-admission-manifest.md`
- `internal/reposource/model.go`
- `internal/reposource/path.go`
- `internal/client/controller_client.go`
- `internal/client/controller_client_test.go`
- `cmd/controller/main.go`
- `cmd/controller/main_test.go`
- `../go-etl-demo-project/workflows/demo-workflow.json`

Do not read unrelated controller or worker files unless compile or test
failures directly require it.

## Allowed Production Files

- `internal/reposource/source_declaration.go`
- `internal/client/controller_client.go`
- `cmd/controller/main.go`

This slice needs a new production file. If the active HCI mode does not include
`+newfile`, pause before implementation and ask for an updated budget.

## Allowed Test Files

- `internal/reposource/source_declaration_test.go`
- `internal/client/controller_client_test.go`
- `cmd/controller/main_test.go`

## Allowed Fixture Files

- `../go-etl-demo-project/workflows/*.json`
- `../go-etl-demo-project/submissions/*.json`

## Out Of Scope

- Reading supplemental source files from GitHub or local filesystem providers.
- Publishing supplemental files into the repository cache.
- Materializing supplemental files into worker staging directories.
- Changing `/workflow` admission to use the new declaration for cache
  publication.
- Implementing Python executor work items.
- Inferring Python imports or environment dependencies.
- Letting workers add files to the admitted source manifest at runtime.
- Supporting multi-source or multi-repository declarations.
- Allowing workflow authors to specify cache paths.
- Declaring secrets, credentials, or local absolute paths as source files.

## Acceptance Criteria

- Workflow source documents can decode an optional top-level `source_manifest`.
- `source_manifest.files` entries include `role`, `path`, and optional
  `content_type`.
- Accepted roles are `python_entrypoint`, `python_environment`, and
  `support_file` for supplemental files.
- Project and workflow document roles are not accepted in `source_manifest`
  because those files are already implicit in the run submission.
- Declared paths are validated as slash-separated repository-relative paths.
- Declarations reject empty paths, `.`, absolute paths, Windows
  drive-qualified paths, backslash paths, and paths containing an original `..`
  segment.
- Declarations reject duplicate paths.
- Declarations reject unsupported roles.
- Workflow authors cannot specify `cache_path`; the controller derives
  `cache_path` from the validated source path in later admission behavior.
- A workflow with no `source_manifest` remains valid for non-Python workflows.
- Tests cover decoding an absent manifest, decoding a valid Python manifest,
  rejecting unsafe paths, rejecting duplicate paths, and rejecting unsupported
  roles.
- Existing demo workflow submission loading tests continue to pass.

## Notes

- This slice defines declaration and validation only. OS 002 consumes the
  declaration when building admitted source manifests.
- The declaration is intentionally workflow-owned because required supplemental
  files are part of the workflow's source contract.
- Keep the JSON shape simple. Do not add remapping, glob patterns, recursive
  directories, or generated file lists in this slice.
- If a later Python executor needs multiple entrypoints, it can use multiple
  `python_entrypoint` entries.

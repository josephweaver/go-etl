# 004 Workflow Document Normalization

Status: Proposed

## Objective

Replace the public Go-shaped workflow wrapper with one canonical workflow document and normalize it into the existing internal workflow/compiler structures.

## Minimum Model

Codex 5.5, high reasoning. This is a breaking public-schema migration touching workflow admission and compile boundaries.

## Required Context

Read:

- OS-001 through OS-003
- `internal/workflow/workflow.go`
- workflow step, fan-out, and stage normalization files
- `cmd/controller/main.go` workflow admission/decode functions
- `docs/workflow-authoring-template.md`
- current smoke workflow JSON files

## Allowed Production Files

- `internal/document/workflow.go` new
- `internal/workflow/document_adapter.go` new
- `cmd/controller/main.go`
- `internal/client` or `cmd/demo-client` only if submission serialization must change

## Allowed Test Files

- `internal/document/workflow_test.go`
- `internal/workflow/document_adapter_test.go`
- focused controller/client workflow admission tests

## Data State Transition

```text
canonical Workflow document
        -> validate envelope
        -> load workflow variable map
        -> normalize snake_case steps/fan-out/work
        -> existing internal workflow structures
```

No data overlay occurs until OS-005.

## Implementation Requirements

- Flatten `workflow`, `source_manifest`, and external `variables` wrappers into one Workflow document.
- Define canonical `id`, `variables`, `data`, `steps`, and `source_manifest` fields.
- Add explicit `fan_out.over`, `fan_out.as`, and stable item-ID expression fields.
- Normalize step `work.type` and `work.parameters` into the current internal work-item template.
- Add or reserve ephemeral `fanout` namespace semantics for later asset binding.
- Reject mixed Go casing and canonical casing in one document.
- Keep internal persistence/work-item shapes unchanged where practical.

## Out of Scope

- Project/workflow data overlay.
- Explicit cache/commit migration.
- Expression functions.
- Backward compatibility after the final migration slice.

## Acceptance Criteria

- Canonical JSON workflow decodes and compiles a no-data fixture.
- Canonical YAML workflow follows the same path.
- Workflow variables use the implicit workflow namespace.
- Go-field-cased authoring is not required by new tests.
- Existing internal compiler types remain usable.

## Test Commands

```bash
go test ./internal/document ./internal/workflow ./cmd/controller ./cmd/demo-client
```

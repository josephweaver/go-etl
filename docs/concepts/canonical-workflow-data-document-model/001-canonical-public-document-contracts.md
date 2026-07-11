# 001 Canonical Public Document Contracts

Status: Proposed

## Objective

Define the canonical public envelopes and field ownership for controller, project, workflow, and submission override documents before runtime code is changed.

This slice is documentation plus public decode-model scaffolding only. It fixes the target shape so later slices do not independently invent incompatible JSON/YAML structures.

## Minimum Model

Codex 5.5 or stronger, high reasoning. This is the contract-setting slice and should receive the strongest review.

## Required Context

Read:

- `README.md` in this concept
- `docs/CUSTOMER_API.md`
- `docs/workflow-authoring-template.md`
- `cmd/controller/config.go`
- `cmd/controller/main.go`
- `internal/workflow/workflow.go`
- `internal/model/data_asset.go`
- the three current Google Drive smoke workflows supplied with this concept review

## Allowed Production Files

- `docs/concepts/canonical-workflow-data-document-model/README.md`
- `internal/document/schema.go` new file allowed
- `internal/document/schema_test.go` new file allowed

Do not change workflow execution in this slice.

## Allowed Test Files

- `internal/document/schema_test.go`

## Data State Transition

```text
current Go-shaped public JSON
        -> documented canonical envelope definitions
        -> decode-only public document structs
```

No internal workflow or variable state changes yet.

## Implementation Requirements

- Define `api_version`, `kind`, and required identity fields.
- Define canonical `snake_case` field names.
- Keep `variables`, `data`, `steps`, and `source_manifest` as distinct sections.
- Define project, workflow, controller, and override ownership.
- Do not expose `variable.Variable` or `TypedExpression` as the default public value shape.
- Retain `goet/v1alpha1` in examples until a separate API-version migration is approved.
- Reject unknown top-level fields in strict decode tests.

## Out of Scope

- JSON/YAML parsing.
- Workflow compilation.
- Data overlay.
- Legacy workflow migration.
- Expression evaluation.

## Acceptance Criteria

- Canonical envelope structs exist or are precisely documented.
- JSON examples decode into the public structs.
- Unknown top-level fields are rejected.
- Public structs do not contain `[]variable.Variable` as the authoring representation.
- No controller or worker behavior changes.

## Test Commands

```bash
go test ./internal/document
```

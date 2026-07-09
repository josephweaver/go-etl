# Canonical JSON Variable Loading

Status: Proposed

## Purpose

Define how controller, project, and workflow JSON documents load into GOET's typed variable system using canonical JSON value shapes and implicit source namespaces.

This is a placeholder Strategic Concept for the document-loading contract. It is narrower than the full workflow-compilation boundary and complements the existing structured-variable-resolution work.

## Goals

- Load JSON numbers, strings, lists, and objects as typed values instead of ad hoc string data.
- Preserve the source document shape when importing values into the variable system.
- Make the source document determine the namespace implicitly.
- Let a value loaded from `workflow.json` behave as `workflow.var-name` internally without requiring the workflow file to repeat that namespace in every field.
- Keep controller, project, workflow, and override inputs aligned with the same typed variable model.
- Keep the loading rule understandable from the document source alone.

## Non-Goals

- Changing the variable precedence order.
- Designing a new expression language.
- Replacing the existing structured variable model.
- Changing workflow compilation, fan-out, or assignment-time resolution behavior.
- Defining customer-specific workflow schemas.

## Architectural Context

This concept sits between the customer-facing JSON files and `internal/variable`.

Related documents:

- `docs/CUSTOMER_API.md`
- `docs/workflow-authoring-template.md`
- `docs/concepts/structured-variable-resolution/README.md`
- `docs/concepts/workflow-compilation-resolution/README.md`

The structured-variable-resolution concept already defines the recursive typed-expression model. This concept owns the earlier loading step that decides how a source document becomes a namespace of typed values before workflow compilation uses those values.

## Current State

- The repository already treats `controller.json`, `project.json`, and `workflow.json` as canonical public inputs.
- The structured-variable-resolution concept already defines typed variables, namespaces, and recursive resolution.
- The loading rule is still distributed across several docs, so the source-document-to-namespace mapping is not yet a single, explicit contract.
- Workflow authors can already write JSON with typed values, but the namespace binding is not yet framed as an explicit loading rule in one place.

## Target State

- A JSON document's source determines its namespace on load.
- `workflow.json` values load as `workflow.*` variables.
- `project.json` values load as `project.*` variables.
- `controller.json` values load as `controller.*` variables.
- Runtime overrides load as `override.*` variables.
- Canonical JSON containers and scalars remain typed values after import rather than becoming plain strings.
- The controller and compiler can rely on the implicit namespace rule without extra per-field namespace boilerplate in the source documents.

## Proposed Slices

These are candidate slices only and are not yet agreed.

1. Define the source-document-to-namespace loading contract for controller, project, workflow, and override JSON.
2. Define the canonical JSON-to-typed-variable mapping for numbers, strings, lists, and objects.
3. Update public JSON examples and authoring docs to use the implicit namespace rule consistently.
4. Add a minimal loader or resolver test that proves a workflow-local value is visible as `workflow.*` after import.

## Completion Criteria

- The source-document namespace rule is written in one place and uses precise file names.
- JSON values imported from controller, project, workflow, and override documents retain typed meaning.
- Workflow-local values are treated as workflow namespace values without requiring explicit namespace repetition in the public JSON shape.
- The concept is decomposed into approved slices or folded into a neighboring concept if the team decides this should remain a smaller doc-only rule.

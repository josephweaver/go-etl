# 006 Data Asset Definition Binding and Selection

Status: Proposed

## Objective

Introduce canonical data input/output definitions separating logical asset metadata, named file roles, physical binding, effective selection, and step-local aliases.

## Minimum Model

Primary: `GPT-5.5`, `High` reasoning. First escalation or review: `GPT-5.6-Terra`, `Medium` reasoning. See `MODEL_RECOMMENDATIONS.md` for the cost-conservative rationale and escalation policy.

## Required Context

Read:

- OS-005
- `internal/model/data_asset.go`
- `internal/model/data_archive.go`
- `internal/model/data_location.go`
- data-assets-and-materialized-outputs README and addendum
- current Google Drive smoke workflows

## Allowed Production Files

- `internal/model/data_definition.go` new
- `internal/model/data_asset.go`
- `internal/model/data_archive.go` only as required
- `internal/document/data.go`

## Allowed Test Files

- `internal/model/data_definition_test.go`
- `internal/document/data_test.go`

## Data State Transition

```text
named logical asset definition
    + effective project/workflow binding
    + effective ordered selection
    -> validated unresolved asset template
```

No fan-out parameter values are bound yet.

## Implementation Requirements

- Define asset parameters, `files` role map, `select`, and `binding` sections.
- Keep provider/location/archive/cache/integrity/transfer policy vocabulary compatible with existing models.
- Generate archive member selections from named file roles and the effective `select` list.
- Validate every selected role exists and is required/optional consistently.
- Preserve selection order.
- Allow workflow bindings to override project provider details.
- Define analogous `data.outputs` logical target and physical binding sections.
- Do not infer primary-file semantics; the public projection is an ordered list.

## Out of Scope

- Fan-out instantiation.
- Materialization scope implementation.
- Worker path projections.
- Publication execution.

## Acceptance Criteria

- General Yan-Roy asset with raster/header roles validates.
- Header-only workflow selection compiles an archive selection containing only the `.hdr` member.
- Unknown or duplicate selected roles fail.
- Selection order is retained.
- Provider and cache fields still map to existing bound asset models.

## Test Commands

```bash
go test ./internal/model ./internal/document
```

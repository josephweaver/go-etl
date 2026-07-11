# 010 Shared Materialization Hydration

Status: Proposed

## Objective

Match later compute data requirements to completed explicit shared materialization manifests by canonical asset identity and materialization domain.

## Minimum Model

Codex 5.5, high reasoning. This crosses stage persistence, restart-safe outputs, assignment hydration, and alias projection.

## Required Context

Read:

- OS-009
- `cmd/controller/cache_data_hydration.go`
- `cmd/controller/cache_data_dependencies.go`
- workflow stage activation/queue files
- persistence terminal-attempt and step-output APIs
- workflow-compilation-resolution concept

## Allowed Production Files

- `cmd/controller/cache_data_hydration.go`
- `cmd/controller/cache_data_dependencies.go`
- `cmd/controller/workflow_stage_queue.go` or focused new helper
- persistence model only if existing output facts are insufficient

## Allowed Test Files

- corresponding controller tests
- persistence tests only if schema changes

## Data State Transition

```text
completed explicit cache_data manifests
    -> index by asset key + domain
compute asset requirements
    -> match prior completed manifest
    -> project under compute step alias
    -> assignment materialized-data input
```


## Implementation Requirements

- Do not depend solely on generated dependency work-item IDs.
- Match canonical asset key and materialization domain.
- Require the producer to be in a completed prior dependency/stage context.
- Detect zero matches, duplicate/conflicting matches, and domain mismatch.
- Preserve binding aliases by projecting one physical manifest under each consumer alias.
- Use durable completed output facts so restart reconstruction works.
- Do not create a global mutable catalog.
- Permit cache_data to verify an already-ready physical cache and emit a fresh run-scoped manifest.

## Out of Scope

- Cross-run catalog lookup.
- Worker-local materializations.
- Same-stage per-instance release.
- Garbage collection.

## Acceptance Criteria

- Compute receives the matching header-only manifest.
- Wrong selection does not match.
- Wrong domain does not match.
- Two aliases can project the same physical manifest.
- Missing explicit producer fails before assignment with a useful error.
- Restart/reconstruction test continues to hydrate from durable facts.

## Test Commands

```bash
go test ./cmd/controller ./internal/persistence
```

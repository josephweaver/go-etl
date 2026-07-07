# Resource-Constrained Work Admission State

Last updated: 2026-07-07

Completed Strategic Concept docs: [`../resource-constrained-work-admission/README.md`](../resource-constrained-work-admission/README.md)

This file preserves resource-constrained work-admission current-state excerpts moved out of the root `PROJECT_STATE.md`.

## Current-State Excerpts
- Resource-constrained work admission is fully implemented through slice `008-docs-project-state-and-cleanup` and all associated docs and status/tracker updates were completed.

- Resource-constrained work admission concept slice `005-claim-next-resource-eligible-work` is now implemented: persisted `/work/next` claims now evaluate queued candidates in deterministic queue order against `queued_resource_constraint_checks` using the Go resource constraint evaluator, skip resource-blocked candidates without head-of-line blocking, leave all-blocked queues unchanged, and claim eligible work atomically through `work_item_attempts`, `running_work`, and `queued_work`. The controller serializes local claim evaluation so a second worker poll sees resource usage from the first committed claim. Focused store tests cover constrained oldest-eligible claims, blocked-oldest/eligible-later claims, all-blocked no-op behavior, second-claim running-resource visibility, resource release after completion, and all six operators affecting claim eligibility.

- Resource-constrained work admission operational slice `006-resource-status-and-observability` is now implemented: `/status` now reports resource-aware queued counts and running resource claim count, and queue constraints contribute deterministic compact per-resource summaries. `go run ./cmd/demo-client` final status adds a compact `resources:` line when resource constraints are present. Controller status tests now cover eligible/blocked/running resource claim counts and summary shaping.

- Resource-constrained work admission concept slice `004-operator-evaluator-and-check-reader` is now implemented: `internal/model.ResourceConstraintAllows` evaluates all six resource operators with explicit `int64` overflow detection, `ResourceConstraintChecksAllow` treats per-candidate checks as an AND with zero checks eligible, and `internal/persistence.Store.ListQueuedResourceConstraintChecks` reads `queued_resource_constraint_checks` rows in deterministic queued-time/work-item/constraint-index order using `int64` unit fields. Focused tests cover all operator true/false cases, overflow, zero-check eligibility, and store reader totals for matching versus unrelated running resource keys.

- Resource-constrained work admission concept slice `003-persist-constraints-with-work-items` is now implemented: raw `/work` submissions, initial workflow-stage admission, and just-in-time downstream stage activation persist resolved `work_item_resource_constraints` beside work-item insertion and queueing through one store transaction. Work items with zero constraints still queue normally, duplicate/conflicting constraint insertion rolls back associated work-item and queue mutations, and dependency/output JSON remains separate from constraint facts.

- Resource-constrained work admission concept slice `002-resolve-resource-constraints-at-work-creation` is now implemented: workflow fan-out work-item templates can declare resource constraints, literals and whole-reference resolver expressions are resolved before queue staging, fan-out item fields can supply requested units through the existing accessor/reference pattern, invalid constraint values are rejected before queue mutation, raw `/work` submissions can use a backward-compatible wrapper with resource constraints, and resolved in-memory constraint records are carried beside compiled work items for the next persistence slice.

- Resource-constrained work admission concept slice `001-resource-constraint-model-and-schema` is now implemented: `work_item_resource_constraints`, `queued_resource_constraint_checks`, and schema-version `5` are in place in `internal/persistence`, and model validation was added for constraint operators and numeric invariants.

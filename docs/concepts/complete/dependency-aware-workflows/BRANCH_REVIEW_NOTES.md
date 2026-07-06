# Dependency-Aware Workflow Branch Review Notes

Review date: 2026-07-05

Reviewed branch:

```text
https://github.com/josephweaver/go-etl/tree/main
```

## Applied Document Changes

- Kept the bundle path under `docs/concepts/complete/dependency-aware-workflows/`, which matches the current concept branch.
- Removed the earlier `docs/epics/...` alignment from the prior zip because that was based on the wrong branch.
- Marked slices 001, 002, and 003 as implemented on the visible branch and converted them into regression/verification checklists.
- Marked slice 004 as the active in-progress handoff slice.
- Left slices 005-012 as pending/ready, with language updated so they consume the actual 001-004 implementation files rather than creating duplicate planner/compiler/store helpers.
- Added explicit guidance to reuse the slice 003 store owner and the slice 004 queue/membership helper names after they stabilize.

## Verification Notes

The visible branch contains the Strategic Concept bundle in `docs/concepts/complete/dependency-aware-workflows/` and lists slices `001` through `012`.

The visible branch also contains:

```text
internal/workflow/stage.go
internal/workflow/stage_test.go
internal/workflow/compile_stage.go
internal/workflow/compile_stage_test.go
cmd/controller/workflow_dependency_store.go
cmd/controller/workflow_dependency_store_test.go
internal/model/workflow_dependency.go
internal/model/workflow_dependency_test.go
```

Those files support treating 001-003 as implemented. The public controller tree did not show `cmd/controller/workflow_stage_queue.go` at review time, so this bundle treats 004 as active/in-progress rather than complete.

## Next Handoff

1. Finish or review `004-stamp-work-items-with-step-instance-metadata.md` in the local Codex workspace.
2. Update slices 005-011 if 004 lands helper/store names different from the placeholders in this bundle.
3. Start `005-submit-only-initial-ready-stage.md` only after 004 has stable tests and a clear implementation report.

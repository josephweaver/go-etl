# Dependency-Aware Workflows Strategic Concept Bundle Manifest

This bundle replaces and expands the current Strategic Concept document:

```text
docs/concepts/complete/dependency-aware-workflows/README.md
```

It keeps these Operational Slice files under the same directory:

```text
docs/concepts/complete/dependency-aware-workflows/001-normalize-workflow-stages.md
docs/concepts/complete/dependency-aware-workflows/002-compile-single-workflow-stage.md
docs/concepts/complete/dependency-aware-workflows/003-persist-workflow-stage-state.md
docs/concepts/complete/dependency-aware-workflows/004-stamp-work-items-with-step-instance-metadata.md
docs/concepts/complete/dependency-aware-workflows/005-submit-only-initial-ready-stage.md
docs/concepts/complete/dependency-aware-workflows/006-record-terminal-work-item-state.md
docs/concepts/complete/dependency-aware-workflows/007-capture-typed-step-outputs.md
docs/concepts/complete/dependency-aware-workflows/008-compile-next-ready-stage.md
docs/concepts/complete/dependency-aware-workflows/009-handle-empty-fanout-and-auto-advance.md
docs/concepts/complete/dependency-aware-workflows/010-propagate-step-and-workflow-failure.md
docs/concepts/complete/dependency-aware-workflows/011-surface-dependency-state-in-status-and-logs.md
docs/concepts/complete/dependency-aware-workflows/012-update-dependency-workflow-docs-and-smoke.md
```

It also includes:

```text
docs/concepts/complete/dependency-aware-workflows/BRANCH_REVIEW_NOTES.md
docs/concepts/complete/dependency-aware-workflows/MODEL_RECOMMENDATIONS.md
docs/concepts/complete/dependency-aware-workflows/MANIFEST.md
```

Current handoff tracker:

```text
001 implemented on visible branch
002 implemented on visible branch
003 implemented on visible branch
004 in progress
005-012 pending
```

Suggested Codex prompt pattern while 004 is active:

```text
please read docs/concepts/complete/dependency-aware-workflows/004-stamp-work-items-with-step-instance-metadata.md and finish or review only that slice
```

After 004 lands and helper names are stable, continue one file at a time:

```text
please read docs/concepts/complete/dependency-aware-workflows/005-submit-only-initial-ready-stage.md and implement only that slice
```

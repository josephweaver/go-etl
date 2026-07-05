# Dependency-Aware Workflows Concept Bundle Manifest

This bundle replaces and expands:

```text
docs/concepts/dependency-aware-workflows/README.md
```

It adds these Operational Slice files:

```text
docs/concepts/dependency-aware-workflows/001-normalize-workflow-stages.md
docs/concepts/dependency-aware-workflows/002-compile-single-workflow-stage.md
docs/concepts/dependency-aware-workflows/003-persist-workflow-stage-state.md
docs/concepts/dependency-aware-workflows/004-stamp-work-items-with-step-instance-metadata.md
docs/concepts/dependency-aware-workflows/005-submit-only-initial-ready-stage.md
docs/concepts/dependency-aware-workflows/006-record-terminal-work-item-state.md
docs/concepts/dependency-aware-workflows/007-capture-typed-step-outputs.md
docs/concepts/dependency-aware-workflows/008-compile-next-ready-stage.md
docs/concepts/dependency-aware-workflows/009-handle-empty-fanout-and-auto-advance.md
docs/concepts/dependency-aware-workflows/010-propagate-step-and-workflow-failure.md
docs/concepts/dependency-aware-workflows/011-surface-dependency-state-in-status-and-logs.md
docs/concepts/dependency-aware-workflows/012-update-dependency-workflow-docs-and-smoke.md
```

It also includes:

```text
docs/concepts/dependency-aware-workflows/MODEL_RECOMMENDATIONS.md
docs/concepts/dependency-aware-workflows/MANIFEST.md
```

Suggested Codex prompt pattern:

```text
please read docs/concepts/dependency-aware-workflows/001-normalize-workflow-stages.md and implement only that slice
```

Then continue one file at a time after review and commit.

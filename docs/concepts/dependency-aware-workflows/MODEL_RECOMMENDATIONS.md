# Model Recommendations For Dependency-Aware Workflows

Goal: reduce token/quota usage while still using stronger reasoning for slices that touch controller state transitions, output typing, and just-in-time scheduling.

## Current Handoff State

Use this tracker first:

```text
001: implemented on visible branch
002: implemented on visible branch
003: implemented on visible branch
004: in progress
005-012: pending
```

Do not spend model quota reimplementing 001-003 unless verification shows the local branch is missing or regressed against the visible concept branch. While 004 is running, use the model budget to finish or review that slice and capture its actual helper/store names for downstream slices.

## Recommended Defaults

Use this policy for remaining work:

```text
Default model: gpt-5.4-mini
Default thinking: Low for docs and isolated workflow/compiler verification; Medium for controller helper wiring.
Escalate to gpt-5.5 for durable state-machine transitions, output typing, JIT activation, and terminal failure hardening.
Avoid xhigh unless Codex repeatedly fails to preserve idempotency or diagnose a non-obvious integration failure.
```

## Slice-by-Slice Recommendations

| Slice | Status | Recommended model | Thinking | Reason |
|---|---|---:|---:|---|
| `001-normalize-workflow-stages.md` | Implemented | `gpt-5.4-mini` | Low | Verification/regression only. Do not rerun unless missing or regressed. |
| `002-compile-single-workflow-stage.md` | Implemented | `gpt-5.4-mini` | Low-Medium | Verification/regression only. |
| `003-persist-workflow-stage-state.md` | Implemented | `gpt-5.4-mini` | Medium | Verify parallel-stage state shape before 005 if needed. |
| `004-stamp-work-items-with-step-instance-metadata.md` | In progress | `gpt-5.4-mini` | Medium | Active helper/association slice crossing compiler, controller, model/store metadata. |
| `005-submit-only-initial-ready-stage.md` | Pending | `gpt-5.5` | Medium | Changes live admission behavior and must preserve source admission, submission ack, and queue semantics. |
| `006-record-terminal-work-item-state.md` | Pending | `gpt-5.5` | Medium | Completion/failure paths must be idempotent and must not corrupt existing status/attempt behavior. |
| `007-capture-typed-step-outputs.md` | Pending | `gpt-5.5` | High | Typed JSON-to-variable conversion and `workflow.step[index]` scope construction are correctness-sensitive. |
| `008-compile-next-ready-stage.md` | Pending | `gpt-5.5` | High | JIT compilation, retained resolver context, idempotent activation, queue insertion, and status transitions meet here. |
| `009-handle-empty-fanout-and-auto-advance.md` | Pending | `gpt-5.4-mini` | Medium | Important edge case but bounded if slice 008 is clean. Escalate only if auto-advance loops or idempotency get confused. |
| `010-propagate-step-and-workflow-failure.md` | Pending | `gpt-5.5` | Medium-High | Failure transitions must remain terminal under late sibling reports and duplicate messages. |
| `011-surface-dependency-state-in-status-and-logs.md` | Pending | `gpt-5.4-mini` | Medium | Public output and logs over existing surfaces; not a scheduler change. |
| `012-update-dependency-workflow-docs-and-smoke.md` | Pending | `gpt-5.4-mini` | Low | Docs and smoke coverage over completed behavior. |

## Cost-Conscious Alternative

If quota pressure is high, use this cheaper plan for the remaining work:

```text
004: gpt-5.4-mini medium
005: gpt-5.4-mini medium, escalate to gpt-5.5 only if source admission/status coupling is unclear
006: gpt-5.4-mini medium, escalate to gpt-5.5 if idempotency is failing
007: gpt-5.5 high
008: gpt-5.5 high
009: gpt-5.4-mini medium
010: gpt-5.4-mini high, escalate to gpt-5.5 if late terminal reports are confusing
011: gpt-5.4-mini medium
012: gpt-5.4-mini low
```

Escalate only the failed slice, not the whole concept.

## Prompt Pattern

While 004 is active:

```text
please read docs/concepts/dependency-aware-workflows/004-stamp-work-items-with-step-instance-metadata.md and finish or review only that slice
```

After 004 lands:

```text
please read docs/concepts/dependency-aware-workflows/005-submit-only-initial-ready-stage.md and implement only that slice
```

For slices 005, 006, 007, 008, and 010, add:

```text
Prioritize idempotency and keep the implementation inside the allowed files. Report if the allowed file boundary is insufficient before broadening scope.
```

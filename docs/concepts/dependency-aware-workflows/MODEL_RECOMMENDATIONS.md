# Model Recommendations For Dependency-Aware Workflows

Goal: reduce token/quota usage while still using stronger reasoning for slices that touch controller state transitions, output typing, and just-in-time scheduling.

## Recommended Defaults

Use this policy first:

```text
Default model: gpt-5.4-mini
Default thinking: Low for isolated model/compiler/docs slices; Medium for controller endpoint/store changes.
Escalate to gpt-5.5 only for slices with durable state-machine risk, typed output/resolver risk, or JIT scheduling risk.
Avoid xhigh unless Codex repeatedly fails to preserve idempotency or diagnose a non-obvious integration failure.
```

## Slice-by-Slice Recommendations

| Slice | Recommended model | Thinking | Reason |
|---|---:|---:|---|
| `001-normalize-workflow-stages.md` | `gpt-5.4-mini` | Low | Pure `internal/workflow` model/validation work with bounded tests. |
| `002-compile-single-workflow-stage.md` | `gpt-5.4-mini` | Medium | Compiler change with ordering and duplicate-ID behavior; still local to `internal/workflow`. |
| `003-persist-workflow-stage-state.md` | `gpt-5.5` | Medium | Introduces controller-owned state records and duplicate/idempotency constraints. Use `gpt-5.4-mini` Medium only if the store shape after prior concepts is already simple and obvious. |
| `004-stamp-work-items-with-step-instance-metadata.md` | `gpt-5.4-mini` | Medium | Mostly transformation/helper logic, but it crosses `internal/workflow`, `internal/model`, and controller queue metadata. |
| `005-submit-only-initial-ready-stage.md` | `gpt-5.5` | Medium | Changes live admission behavior and must preserve source admission, submission ack, and queue semantics. |
| `006-record-terminal-work-item-state.md` | `gpt-5.5` | Medium | Completion/failure paths must be idempotent and must not corrupt existing status/attempt behavior. |
| `007-capture-typed-step-outputs.md` | `gpt-5.5` | High | Typed JSON-to-variable conversion and `workflow.step[index]` scope construction are correctness-sensitive. |
| `008-compile-next-ready-stage.md` | `gpt-5.5` | High | JIT compilation, retained resolver context, idempotent activation, queue insertion, and status transitions meet here. |
| `009-handle-empty-fanout-and-auto-advance.md` | `gpt-5.4-mini` | Medium | Important edge case but bounded if slice 008 is clean. Escalate only if auto-advance loops or idempotency get confused. |
| `010-propagate-step-and-workflow-failure.md` | `gpt-5.5` | Medium | Failure transitions must remain terminal under late sibling reports and duplicate messages. |
| `011-surface-dependency-state-in-status-and-logs.md` | `gpt-5.4-mini` | Medium | Public output and logs over existing surfaces; not a scheduler change. |
| `012-update-dependency-workflow-docs-and-smoke.md` | `gpt-5.4-mini` | Low | Docs and smoke coverage over completed behavior. |

## Cost-Conscious Alternative

If quota pressure is high, try this cheaper plan:

```text
001: gpt-5.4-mini low
002: gpt-5.4-mini medium
003: gpt-5.4-mini medium
004: gpt-5.4-mini medium
005: gpt-5.4-mini medium
006: gpt-5.4-mini medium
007: gpt-5.5 high
008: gpt-5.5 high
009: gpt-5.4-mini medium
010: gpt-5.4-mini high
011: gpt-5.4-mini medium
012: gpt-5.4-mini low
```

Escalate only the failed slice, not the whole Strategic Concept.

## Prompt Pattern

```text
please read docs/concepts/dependency-aware-workflows/00N-<slice-name>.md and implement only that slice
```

For slices 003, 005, 006, 007, 008, and 010, add:

```text
Prioritize idempotency and keep the implementation inside the allowed files. Report if the allowed file boundary is insufficient before broadening scope.
```

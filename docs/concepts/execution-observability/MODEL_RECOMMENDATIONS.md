# Codex Model And Thinking Recommendations

These recommendations assume Codex is implementing the Execution Observability Operational Slices one at a time with the prompt pattern:

```text
please read docs/concepts/execution-observability/<slice>.md and implement exactly that slice
```

The goal is to reduce reasoning-token use without causing Codex to over-read or over-edit the repository.

| Slice | Recommended model | Thinking level | Why this should be sufficient |
| --- | --- | --- | --- |
| 001 Logging Model | GPT-5.5 | Low | Isolated shared model and tests in `internal/model`; no HTTP, filesystem, or runtime behavior. |
| 002 Log Configuration | GPT-5.5 | Low | Bounded config/default parsing. Use Medium only if existing controller variable/default code is unexpectedly tangled. |
| 003 Controller Logging Endpoint | GPT-5.5 | Medium | Adds HTTP handler/registration and request validation in the large controller package. |
| 004 Worker Logging Client | GPT-5.5 | Medium | Adds HTTP client behavior and failure semantics in worker communication code. |
| 005 Controller Filesystem Log Sinks | GPT-5.5 | Medium | Requires safe path handling, JSONL append behavior, and concurrency-aware tests. |
| 006 Worker Fallback Logging | GPT-5.5 | Medium | Requires careful non-failing error behavior across HTTP failure and local file-write failure. |
| 007 Python Subprocess Log Emission | GPT-5.5 | Medium | Touches Python runner behavior and must preserve existing evidence/output tests. Escalate to High only if live stdout/stderr teeing causes race or deadlock failures. |
| 008 Log Levels and Filtering | GPT-5.5 | Low | Small filtering/routing rules after model, config, and sink abstractions already exist. Use Medium if routing has spread across files. |
| 009 Submission Log Read API | GPT-5.5 | Medium | Adds bounded JSONL reading, query validation, and public endpoint behavior. |
| 010 CLI Logs Command | GPT-5.5 | Low | Should reuse the completed submit/status CLI and internal client patterns. Use Medium if the previous CLI implementation is less factored than expected. |
| 011 Docs And Smoke | GPT-5.5 | Low | Documentation and smoke-script update after behavior exists. |

## Cost-Control Guidance

- Start with the recommended thinking level instead of defaulting to High.
- Keep Codex to the slice's `Required Context` and `Allowed Files` sections.
- Escalate one level only after a concrete failure mode, such as repeated test failures, inability to locate an existing concept owner, or a reported need to edit files outside the approved boundary.
- Avoid Extra High for this concept. None of these slices should require long autonomous planning if implemented in order.
- Do not combine slices unless you deliberately want to trade review clarity for fewer Codex sessions.

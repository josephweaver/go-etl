# Model Assignments for Expression Function Framework

Status: Proposed

## Summary

The framework should use stronger models for parser and resolver integration, then cheaper models for one-function slices once the framework is stable.

| OS | Slice | Minimum capable model | Reasoning level | Notes |
|---:|---|---|---|---|
| 001 | Expression container forms | Codex 5.4-mini | Medium | JSON decoding and validation changes are local but subtle. |
| 002 | Namespaced parser and registry | Codex 5.4 | Medium | Parser boundary and error semantics benefit from a stronger model. |
| 003 | Resolver JIT evaluation | Codex 5.4 | High | Must preserve depth, cycle, accessor, type, and sensitivity behavior. |
| 004 | `list.crossproduct` | Codex 5.3-codex-spark | Medium | Small pure function after framework lands; escalate on sensitivity/type failures. |
| 005 | `list.zip` | Codex 5.3-codex-spark | Medium | Simple arity/type/length function. |
| 006 | `list.flatten` | Codex 5.3-codex-spark | Medium | Simple one-level list transformation. |
| 007 | `list.length` | Codex 5.3-codex-spark | Low | Very small scalar-returning function. |
| 008 | Fan-out integration proof | Codex 5.4-mini | Medium | Mostly tests, but touches workflow assumptions. |

## Token-Saving Guidance

Use Codex 5.4 for framework structure and resolver integration. Use 5.3-codex-spark for isolated one-function additions only after OS-001 through OS-003 are green.

Do not use 5.3-codex-spark for broad smoke-test diagnosis. If a function slice fails only in focused unit tests, it is still a reasonable spark task. If failure crosses workflow compilation or resolver recursion, escalate to 5.4-mini or 5.4.

## Suggested Batch Order

1. Run OS-001 alone.
2. Run OS-002 alone.
3. Run OS-003 alone and review carefully.
4. Run OS-004 through OS-007 as separate independent Codex tasks.
5. Run OS-008 as the integration proof.

Avoid batching OS-002 and OS-003 together. The parser and resolver integration are different cognitive tasks and failures will be harder to localize if combined.

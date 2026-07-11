# Model Assignments

Status: Proposed

| OS | Slice | Minimum capable model | Reasoning | Why |
|---:|---|---|---|---|
| 001 | Canonical public document contracts | Codex 5.5 | High | Sets the public contract. |
| 002 | JSON/YAML source decoder | Codex 5.5 | High | Parser ambiguity and duplicate-key safety. |
| 003 | Canonical typed variable loader | Codex 5.4-mini | Medium-high | Local recursive loading with namespace consequences. |
| 004 | Workflow document normalization | Codex 5.5 | High | Breaking public schema and compiler adapter. |
| 005 | Named data-tree overlay | Codex 5.5 | High | Precedence errors can silently alter data. |
| 006 | Asset definition/binding/selection | Codex 5.5 | High | Reframes implemented data model. |
| 007 | Parameterized asset instantiation | Codex 5.5 | High | Fan-out, resolver, templates, fingerprints. |
| 008 | Materialization scope/domain | Codex 5.4-mini | Medium-high | Error taxonomy and environment capability. |
| 009 | Explicit cache_data step | Codex 5.5 | High | Integrates visible authoring with current runtime. |
| 010 | Shared materialization hydration | Codex 5.5 | High | Persistence/restart/assignment boundary. |
| 011 | Data projections/worker resolution | Codex 5.5 | High | Typed paths and assignment-time resolver. |
| 012 | Explicit commit_data step | Codex 5.5 | High | Output lineage and durable publication. |
| 013 | Migration/equivalence smoke | Codex 5.5 | High | Breaking cleanup; individual fixture edits may use mini. |
| 014 | Structured call model/directives | Codex 5.5 | High | Semantic expression boundary. |
| 015 | Resolver JIT function evaluation | Codex 5.5 | High | Depth/cycle/type/sensitivity integration. |
| 016 | list.crossproduct | Codex 5.3-spark | Medium | Small pure function after framework. |
| 017 | list.zip | Codex 5.3-spark | Medium | Small pure function. |
| 018 | list.flatten | Codex 5.3-spark | Medium | Small one-level transformation. |
| 019 | list.length | Codex 5.3-spark | Low-medium | Very small scalar result. |
| 020 | Function fan-out proof | Codex 5.4-mini | Medium-high | Crosses several established components. |

## Guidance

- Use the stronger model for OS-001, OS-004 through OS-007, OS-009 through OS-015.
- Do not batch OS-009 and OS-010; visible operator compilation and hydration are different state transitions.
- Use the smaller model for OS-016 through OS-019 only after the function framework is green.
- Escalate any failure involving resolver cycles, persistence reconstruction, data identity, or workflow stages.
- Keep each list function in its own implementation task.

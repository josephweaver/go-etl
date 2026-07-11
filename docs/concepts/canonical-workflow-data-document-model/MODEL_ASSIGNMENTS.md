# Model Assignments

Status: Proposed

This compact matrix follows the cost-conservative policy in `MODEL_RECOMMENDATIONS.md` and treats `GPT-5.3-codex-spark` as free.

| OS | Slice | Primary model | Reasoning | Review / escalation | Reasoning |
|---:|---|---|---|---|---|
| 001 | Canonical Public Document Contracts | GPT-5.5 | High | GPT-5.6-Terra | Medium |
| 002 | JSON YAML Source Decoder | GPT-5.4 | High | GPT-5.6-Luna | Medium |
| 003 | Canonical Typed Variable Loader | GPT-5.3-codex-spark | High | GPT-5.4-mini | Medium |
| 004 | Workflow Document Normalization | GPT-5.5 | High | GPT-5.6-Terra | High |
| 005 | Named Data Tree Overlay and Precedence | GPT-5.4 | High | GPT-5.5 | Medium |
| 006 | Data Asset Definition Binding and Selection | GPT-5.5 | High | GPT-5.6-Terra | Medium |
| 007 | Parameterized Asset Instantiation | GPT-5.5 | Extra High | GPT-5.6-Terra | High |
| 008 | Materialization Scope and Shared Domain | GPT-5.3-codex-spark | High | GPT-5.4-mini | Medium |
| 009 | Explicit Cache Data Step | GPT-5.5 | Extra High | GPT-5.6-Terra | High |
| 010 | Shared Materialization Hydration | GPT-5.5 | High | GPT-5.6-Terra | High |
| 011 | Step Data Projections and Worker Resolution | GPT-5.5 | High | GPT-5.6-Terra | High |
| 012 | Explicit Commit Data Step | GPT-5.5 | High | GPT-5.6-Terra | Medium |
| 013 | Workflow Migration and Equivalence Smoke | GPT-5.4 | High | GPT-5.5 | Medium |
| 014 | Structured Function Call Model and Loader Directives | GPT-5.5 | High | GPT-5.6-Luna | High |
| 015 | Resolver JIT Function Evaluation | GPT-5.5 | Extra High | GPT-5.6-Terra | Extra High |
| 016 | List Crossproduct Function | GPT-5.3-codex-spark | Medium | GPT-5.4-mini | Light |
| 017 | List Zip Function | GPT-5.3-codex-spark | Medium | GPT-5.4-mini | Light |
| 018 | List Flatten Function | GPT-5.3-codex-spark | Medium | GPT-5.4-mini | Light |
| 019 | List Length Function | GPT-5.3-codex-spark | Light | GPT-5.4-mini | Light |
| 020 | Function Produced Fanout Proof | GPT-5.3-codex-spark | High | GPT-5.4-mini | High |

## Guidance

- Start with Spark on every slice where it is the listed primary model.
- Use GPT-5.4-mini as the first paid fallback rather than jumping to a 5.5 or 5.6 model.
- Use GPT-5.6-Sol only for a documented unresolved architecture conflict or final audit.
- No OS requires Ultra by default.
- Review the smallest failing boundary rather than rerunning an entire OS with a stronger model.

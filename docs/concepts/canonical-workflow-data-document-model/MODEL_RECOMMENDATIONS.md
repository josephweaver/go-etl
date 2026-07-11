# Cost-Conservative Model Recommendations by Operational Slice

Status: Proposed

## Purpose

Recommend the cheapest model and reasoning level likely to complete each Operational Slice accurately, while preserving clear escalation paths for architectural or cross-cutting failures.

The plan uses only these models:

- `GPT-5.5`
- `GPT-5.6-Sol`
- `GPT-5.6-Terra`
- `GPT-5.6-Luna`
- `GPT-5.4`
- `GPT-5.4-mini`
- `GPT-5.3-codex-spark`

Reasoning levels are limited to `Light`, `Medium`, `High`, `Extra High`, and `Ultra`.

## Cost Assumptions

- Treat `GPT-5.3-codex-spark` as free, per the project owner's usage model.
- Use Spark first for isolated, test-driven implementation, mechanical migrations, and integration proofs with established contracts.
- Use `GPT-5.4-mini` as the first paid fallback for a Spark failure.
- Use `GPT-5.4` for bounded multi-file logic where semantics matter more than architecture.
- Use `GPT-5.5` for public contracts, lifecycle transitions, resolver behavior, identity/fingerprint logic, and breaking migrations.
- Treat the 5.6 family as targeted review or escalation capacity rather than the default implementation path. This guide assumes Luna is the economical 5.6 reviewer, Terra is the deeper balanced reviewer, and Sol is the highest-cost final escalation. Preserve the roles if actual account multipliers differ.
- No OS defaults to `Ultra`. Use Ultra only after a concrete failure shows that lower reasoning cannot reconcile the issue.

## Operating Rule

For each slice:

1. Run the listed primary implementation model.
2. Use the review model only when the slice changes a public contract, lifecycle boundary, persisted identity, resolver behavior, or shared execution semantics.
3. Escalate one step at a time. Do not jump directly from Spark to Sol.
4. Prefer a focused stronger-model review over re-running the entire implementation with a stronger model.
5. Keep each OS independent; do not batch slices merely to reduce prompts.

## Summary Matrix

| OS | Slice | Primary implementation | Reasoning | Review / first escalation | Reasoning |
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

---

## OS-001 — Canonical Public Document Contracts

**Primary implementation:** `GPT-5.5`, `High`  
**Review / first escalation:** `GPT-5.6-Terra`, `Medium`  
**Free Spark role:** Use Spark for tests/reconnaissance only

### Why this is the conservative choice

Public contract and ownership decisions; use Sol only for unresolved cross-document conflicts.

### Escalation rule

Escalate to `GPT-5.6-Sol`, `High` only if the primary and Terra review disagree about the governing state transition or public contract.

## OS-002 — JSON YAML Source Decoder

**Primary implementation:** `GPT-5.4`, `High`  
**Review / first escalation:** `GPT-5.6-Luna`, `Medium`  
**Free Spark role:** Use Spark for tests/reconnaissance only

### Why this is the conservative choice

Strict YAML/JSON behavior is bounded but correctness-sensitive; Spark can generate adversarial tests.

### Escalation rule

Do not use Sol by default. Escalate to `GPT-5.6-Terra`, `High` before considering Sol.

## OS-003 — Canonical Typed Variable Loader

**Primary implementation:** `GPT-5.3-codex-spark`, `High`  
**Review / first escalation:** `GPT-5.4-mini`, `Medium`  
**Free Spark role:** Yes

### Why this is the conservative choice

Existing conversion logic makes this a strong free-model candidate; escalate only if loader directives leak into runtime types.

### Escalation rule

Sol is not justified for the expected slice scope.

## OS-004 — Workflow Document Normalization

**Primary implementation:** `GPT-5.5`, `High`  
**Review / first escalation:** `GPT-5.6-Terra`, `High`  
**Free Spark role:** Optional

### Why this is the conservative choice

Breaking workflow authoring migration across admission and compiler adapters.

### Escalation rule

Escalate to `GPT-5.6-Sol`, `High` only if the primary and Terra review disagree about the governing state transition or public contract.

## OS-005 — Named Data Tree Overlay and Precedence

**Primary implementation:** `GPT-5.4`, `High`  
**Review / first escalation:** `GPT-5.5`, `Medium`  
**Free Spark role:** Use Spark for tests/reconnaissance only

### Why this is the conservative choice

Recursive overlay is compact but precedence mistakes silently alter data semantics.

### Escalation rule

Do not use Sol by default. Escalate to `GPT-5.6-Terra`, `High` before considering Sol.

## OS-006 — Data Asset Definition Binding and Selection

**Primary implementation:** `GPT-5.5`, `High`  
**Review / first escalation:** `GPT-5.6-Terra`, `Medium`  
**Free Spark role:** Use Spark for tests/reconnaissance only

### Why this is the conservative choice

Reframes the implemented data model and separates definition, binding, selection, and aliases.

### Escalation rule

Do not use Sol by default. Escalate to `GPT-5.6-Terra`, `High` before considering Sol.

## OS-007 — Parameterized Asset Instantiation

**Primary implementation:** `GPT-5.5`, `Extra High`  
**Review / first escalation:** `GPT-5.6-Terra`, `High`  
**Free Spark role:** Use Spark for tests/reconnaissance only

### Why this is the conservative choice

Crosses fan-out, templates, asset identity, deduplication, and fingerprint semantics.

### Escalation rule

Escalate to `GPT-5.6-Sol`, `High` only if the primary and Terra review disagree about the governing state transition or public contract.

## OS-008 — Materialization Scope and Shared Domain

**Primary implementation:** `GPT-5.3-codex-spark`, `High`  
**Review / first escalation:** `GPT-5.4-mini`, `Medium`  
**Free Spark role:** Yes

### Why this is the conservative choice

Localized vocabulary, validation, sentinel error, and shared-domain checks.

### Escalation rule

Sol is not justified for the expected slice scope.

## OS-009 — Explicit Cache Data Step

**Primary implementation:** `GPT-5.5`, `Extra High`  
**Review / first escalation:** `GPT-5.6-Terra`, `High`  
**Free Spark role:** Use Spark for tests/reconnaissance only

### Why this is the conservative choice

Moves existing implicit cache planning to explicit authored work without creating a second runtime.

### Escalation rule

Escalate to `GPT-5.6-Sol`, `High` only if the primary and Terra review disagree about the governing state transition or public contract.

## OS-010 — Shared Materialization Hydration

**Primary implementation:** `GPT-5.5`, `High`  
**Review / first escalation:** `GPT-5.6-Terra`, `High`  
**Free Spark role:** Use Spark for tests/reconnaissance only

### Why this is the conservative choice

Restart-safe matching of explicit materialization results to later compute assignments.

### Escalation rule

Escalate to `GPT-5.6-Sol`, `High` only if the primary and Terra review disagree about the governing state transition or public contract.

## OS-011 — Step Data Projections and Worker Resolution

**Primary implementation:** `GPT-5.5`, `High`  
**Review / first escalation:** `GPT-5.6-Terra`, `High`  
**Free Spark role:** Use Spark for tests/reconnaissance only

### Why this is the conservative choice

Assignment-local data namespace and late path resolution across controller/worker boundary.

### Escalation rule

Escalate to `GPT-5.6-Sol`, `High` only if the primary and Terra review disagree about the governing state transition or public contract.

## OS-012 — Explicit Commit Data Step

**Primary implementation:** `GPT-5.5`, `High`  
**Review / first escalation:** `GPT-5.6-Terra`, `Medium`  
**Free Spark role:** Use Spark for tests/reconnaissance only

### Why this is the conservative choice

Explicit output publication while preserving artifact lineage and overwrite semantics.

### Escalation rule

Escalate to `GPT-5.6-Sol`, `High` only if the primary and Terra review disagree about the governing state transition or public contract.

## OS-013 — Workflow Migration and Equivalence Smoke

**Primary implementation:** `GPT-5.4`, `High`  
**Review / first escalation:** `GPT-5.5`, `Medium`  
**Free Spark role:** Use Spark for tests/reconnaissance only

### Why this is the conservative choice

Use Spark for individual fixture conversions; stronger model coordinates migration and removal of legacy planning.

### Escalation rule

Sol is not justified for the expected slice scope.

## OS-014 — Structured Function Call Model and Loader Directives

**Primary implementation:** `GPT-5.5`, `High`  
**Review / first escalation:** `GPT-5.6-Luna`, `High`  
**Free Spark role:** Use Spark for tests/reconnaissance only

### Why this is the conservative choice

Defines the durable semantic function-call model and loader directive boundary.

### Escalation rule

Do not use Sol by default. Escalate to `GPT-5.6-Terra`, `High` before considering Sol.

## OS-015 — Resolver JIT Function Evaluation

**Primary implementation:** `GPT-5.5`, `Extra High`  
**Review / first escalation:** `GPT-5.6-Terra`, `Extra High`  
**Free Spark role:** Use Spark for tests/reconnaissance only

### Why this is the conservative choice

Most delicate runtime change: functions, recursion, cycles, types, sensitivity, and provenance.

### Escalation rule

Escalate to `GPT-5.6-Sol`, `High` only if the primary and Terra review disagree about the governing state transition or public contract.

## OS-016 — List Crossproduct Function

**Primary implementation:** `GPT-5.3-codex-spark`, `Medium`  
**Review / first escalation:** `GPT-5.4-mini`, `Light`  
**Free Spark role:** Yes

### Why this is the conservative choice

Small pure function after registry and evaluator are stable.

### Escalation rule

Sol is not justified for the expected slice scope.

## OS-017 — List Zip Function

**Primary implementation:** `GPT-5.3-codex-spark`, `Medium`  
**Review / first escalation:** `GPT-5.4-mini`, `Light`  
**Free Spark role:** Yes

### Why this is the conservative choice

Small pure function with explicit length-mismatch behavior.

### Escalation rule

Sol is not justified for the expected slice scope.

## OS-018 — List Flatten Function

**Primary implementation:** `GPT-5.3-codex-spark`, `Medium`  
**Review / first escalation:** `GPT-5.4-mini`, `Light`  
**Free Spark role:** Yes

### Why this is the conservative choice

Small one-level list transformation with focused tests.

### Escalation rule

Sol is not justified for the expected slice scope.

## OS-019 — List Length Function

**Primary implementation:** `GPT-5.3-codex-spark`, `Light`  
**Review / first escalation:** `GPT-5.4-mini`, `Light`  
**Free Spark role:** Yes

### Why this is the conservative choice

Minimal scalar result and straightforward tests.

### Escalation rule

Sol is not justified for the expected slice scope.

## OS-020 — Function Produced Fanout Proof

**Primary implementation:** `GPT-5.3-codex-spark`, `High`  
**Review / first escalation:** `GPT-5.4-mini`, `High`  
**Free Spark role:** Yes

### Why this is the conservative choice

Integration proof can start free; escalate only if failures cross loader/resolver/fan-out boundaries.

### Escalation rule

Sol is not justified for the expected slice scope.

## Recommended Execution Bands

### Free-first band

Use `GPT-5.3-codex-spark` as the primary implementer for OS-003, OS-008, and OS-016 through OS-020. It may also perform fixture-by-fixture edits inside OS-013 after the coordinator fixes the migration contract.

### Low-cost paid band

Use `GPT-5.4-mini` as the first fallback for free-first slices. It is also appropriate for focused test repair after a stronger model has already fixed the design.

### Mid-cost implementation band

Use `GPT-5.4` for OS-002, OS-005, and OS-013. These slices require careful semantics but have bounded architecture once their prerequisites are complete.

### Architecture and lifecycle band

Use `GPT-5.5` for OS-001, OS-004, OS-006, OS-007, OS-009 through OS-012, OS-014, and OS-015. These slices should not be downgraded solely because the file budget appears small.

### 5.6 review band

- Use `GPT-5.6-Luna` for focused semantic review where a concise second opinion is sufficient, especially OS-002 and OS-014.
- Use `GPT-5.6-Terra` for cross-cutting review of workflow normalization, asset identity, explicit data operators, hydration, worker resolution, and resolver JIT behavior.
- Reserve `GPT-5.6-Sol` for a final architecture audit or an actual disagreement that remains after Terra review.

## Reasoning-Level Policy

- `Light`: trivial pure functions, mechanical edits, or review of already-complete focused tests.
- `Medium`: bounded implementation with an established contract.
- `High`: multi-file semantics, parser behavior, migrations, or integration proof.
- `Extra High`: identity, resolver, or lifecycle changes where a subtle error can silently corrupt behavior.
- `Ultra`: never routine; only for a documented unresolved contradiction after a failed Extra High attempt.

## Token-Saving Review Policy

A full second implementation is usually wasteful. Instead:

- Ask the reviewer to inspect only the diff, the OS acceptance criteria, and the relevant invariants.
- Use Spark to generate missing edge-case tests before paying for another implementation pass.
- Escalate the smallest failing subproblem, not the whole OS.
- For OS-013, split fixture conversions into separate Spark tasks while retaining one GPT-5.4 coordinator.
- For OS-016 through OS-019, do not use a paid review unless tests expose ambiguity.

## Final Recommendation

The expected paid-model concentration is OS-001, OS-004, OS-006, OS-007, OS-009 through OS-012, OS-014, and OS-015. Most remaining implementation and test work can begin with Spark or GPT-5.4-mini. A single Terra review at major integration gates is preferable to repeated Sol usage.

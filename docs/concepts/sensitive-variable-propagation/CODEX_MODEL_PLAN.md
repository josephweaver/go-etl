# Codex Model Plan for Sensitive Variable Propagation

Purpose: choose lower-cost Codex models where the slice is local and mechanical, while reserving stronger reasoning for security boundaries, controller/worker crossing, and smoke/debug work.

General rule:

```text
Use 5.5 for architecture, cross-boundary, security-sensitive, or smoke/debug slices.
Use 5.4 for normal implementation with tests.
Use 5.4-mini for narrow internal helpers after the OS is stable.
Use 5.3-codex-spark only for docs, tiny local helpers, or obvious tests.
Do not use 5.3-codex-spark for smoke tests.
```

## Recommended Assignments

| OS | Slice | Recommended model | Reasoning level | Spark-safe? | Notes |
|---:|---|---|---|---:|---|
| 001 | Sensitive metadata and safe rendering | 5.4-mini or 5.4 | Medium | Maybe | Local `internal/variable` change. Use 5.4 if the variable model is already complex or tests are failing. |
| 002 | Protected reference model | 5.4 | Medium | Maybe, with review | Still mostly local, but schema choices matter. Spark can attempt only after README/OS are accepted. |
| 003 | Controller envelope and persistence | 5.5 or 5.4 | High | No | Crosses controller compilation, persistence, status, and fingerprints. Use 5.5 if schema is unclear. |
| 004 | Worker secret resolver and redactor | 5.4 | Medium-High | No | Security-sensitive worker boundary. Redactor tests require care. |
| 005 | Trusted Go WorkItem sensitive context | 5.4 | Medium | Maybe | Worker dispatch refactor. Spark only if very small and after 004 is stable. |
| 006 | Python subprocess secret materialization | 5.5 or 5.4 | High | No | Subprocess/env/file cleanup and output validation are easy to get subtly wrong. |
| 007 | Controlled sink redaction tests | 5.5 or 5.4 | High | No | Cross-boundary leak tests often expose persistence/status/logging edge cases. |
| 008 | Credentialed worker fixture smoke | 5.5 | High | No | Smoke tests are poor Spark tasks. Environment choreography and debugging need stronger reasoning. |
| 009 | Concept closure and doc sync | 5.3-codex-spark or 5.4-mini | Low | Yes | Documentation-only if all implementation slices are complete. |

## Token-Saving Strategy

### Best low-cost candidates

- `001` with `5.4-mini`, if limited to `internal/variable` and tests.
- `002` with `5.4`, or `5.4-mini` after the schema is settled.
- `005` with `5.4-mini`, if worker context changes are narrow.
- `009` with `5.3-codex-spark`.

### Do not use Spark for

- smoke scripts;
- local/worker/controller orchestration debugging;
- Python subprocess materialization;
- persistence leak tests;
- anything requiring interpretation of failed end-to-end runs.

### Suggested execution pattern

1. Run `001` with `5.4-mini` medium.
2. Run a quick `5.5` review of the resulting model if the output looks invasive.
3. Run `002` with `5.4` medium.
4. Run `003` with `5.5` high because this is the main controller boundary.
5. Run `004` with `5.4` high.
6. Run `005` with `5.4-mini` medium if 004 produced a clean context API; otherwise use `5.4`.
7. Run `006` with `5.5` high.
8. Run `007` with `5.4` high, escalating to `5.5` if leaks appear in persistence/status.
9. Run `008` with `5.5` high.
10. Run `009` with `5.3-codex-spark` low.

## Review Passes

Recommended review checkpoints:

| After OS | Review model | Purpose |
|---:|---|---|
| 002 | 5.5 Medium | Verify the model does not accidentally turn secrets into plaintext controller data. |
| 003 | 5.5 High | Verify persistence/status/fingerprint surfaces are safe. |
| 006 | 5.5 High | Verify subprocess materialization cannot leak through command-line args or output JSON. |
| 008 | 5.5 Medium | Verify docs do not overclaim malicious-code protection or real secret-manager support. |

## Prompt Add-On

Recommended add-on for every implementation prompt in this SC:

```text
Sensitive-value safety rule:
Do not introduce any field, log, error, status response, database value, command-line argument, or test fixture that stores or prints plaintext sensitive values except inside a narrowly scoped worker-side test that immediately asserts the value is redacted or rejected. If a test needs a secret, use a sentinel value and add an assertion that the sentinel does not appear in serialized outputs, logs, status, or persistence.
```

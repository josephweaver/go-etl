# 001 Sensitive Metadata and Safe Rendering

Status: proposed

## Objective

Add sensitivity metadata to the typed variable and resolved-value model, propagate that metadata through ordinary resolution, and make safe rendering the default for sensitive values.

This slice establishes the metadata contract only. It does not add secret lookup, worker materialization, controller persistence changes, or Python execution changes.

## Current State

`internal/variable` owns typed variables, namespaces, references, resolved values, structured access, and recursive resolution. Resolved values currently carry type and value information, but sensitivity is not yet a first-class invariant.

The current Sensitive Variable README already says sensitivity should be metadata on typed variables and resolved values, and that safe rendering should be the default. This slice turns that into executable tests and model behavior.

## Target State

A variable declaration can mark a value as sensitive:

```json
{
  "name": "api_token",
  "type": "string",
  "value": "fixture-secret",
  "sensitive": true
}
```

A resolved value carries:

```text
type
value
sensitive bool
redaction_label
non-secret provenance label
```

Normal formatting and diagnostic rendering of a sensitive value must not emit plaintext. Safe rendering should produce a label such as:

```text
${worker_env.GOET_API_TOKEN}
[REDACTED:api_token]
```

The exact label can be adjusted to existing naming conventions, but it must be non-secret and deterministic enough for diagnostics.

## Concept Decision

Sensitivity is metadata, not a separate value type.

Propagation rules:

- Literal sensitive value resolves to a sensitive resolved value.
- Reference to a sensitive value resolves as sensitive.
- Object containing any sensitive field is sensitive at the aggregate level.
- List containing any sensitive element is sensitive at the aggregate level.
- Accessor into a sensitive field returns sensitive.
- Interpolation involving any sensitive component returns sensitive.
- Explicit `sensitive: false` on a destination must not declassify a sensitive dependency.

The resolver must remain deterministic and in-memory. It must not read environment variables, secret stores, files, network services, or worker runtime state.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/sensitive-variable-propagation/README.md`
- `internal/variable/README.md`
- `internal/variable/variable.go`
- `internal/variable/resolver.go`
- `internal/variable/literal.go`
- `internal/variable/reference.go`
- `internal/variable/accessor.go`
- `internal/variable/expression.go`
- all existing `internal/variable/*_test.go`

Do not read controller, worker, scheduler, transport, Python, artifact, or persistence code unless `internal/variable` tests cannot compile without a narrow import update.

## Allowed Production Files

Expected files:

- `internal/variable/variable.go`
- `internal/variable/resolver.go`
- `internal/variable/literal.go`
- `internal/variable/reference.go`
- `internal/variable/accessor.go`
- `internal/variable/expression.go`

Optional helper file if useful:

- `internal/variable/sensitivity.go`

## Allowed Test Files

- `internal/variable/variable_test.go`
- `internal/variable/resolver_test.go`
- `internal/variable/literal_test.go`
- `internal/variable/reference_test.go`
- `internal/variable/accessor_test.go`
- `internal/variable/expression_test.go`
- `internal/variable/sensitivity_test.go`

## Out Of Scope

- Secret providers.
- Environment-variable lookup.
- Controller persistence.
- Work-item assignment payload changes.
- Worker execution changes.
- Python subprocess changes.
- Redacting captured stdout/stderr.
- End-to-end smoke tests.

## Acceptance Criteria

- Variable declarations accept a `sensitive` flag without breaking existing non-sensitive declarations.
- Resolved values expose sensitivity metadata.
- Safe rendering of sensitive resolved values never returns plaintext.
- JSON or diagnostic serialization used by tests has a safe representation for sensitive values.
- Sensitivity propagates through references.
- Sensitivity propagates through object and list aggregates.
- Sensitivity propagates through field/index accessors.
- Sensitivity propagates through string/path interpolation.
- A non-sensitive destination cannot declassify a sensitive source.
- Existing `internal/variable` tests still pass.
- `go test ./internal/variable` passes.

## Notes

Use sentinel values that would be obvious if leaked, such as:

```text
goet-secret-sentinel-001
```

Tests should fail if this exact value appears in safe rendering, diagnostic rendering, or safe JSON output.

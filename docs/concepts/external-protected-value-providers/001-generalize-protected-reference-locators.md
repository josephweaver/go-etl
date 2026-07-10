# 001 Generalize Protected-Reference Locators

Status: proposed

## Objective

Update the protected-reference model so `provider` may be a syntactically valid logical worker-provider alias rather than one of two hard-coded names, and add optional `field` and `version` locator metadata that can cross controller persistence without containing plaintext.

## Current State

`internal/variable/protected_ref.go` currently defines:

```go
type ProtectedRef struct {
    Provider       string
    Key            string
    RedactionLabel string
}
```

`ProtectedRef.Valid` accepts only `worker_env` and `test`.

`internal/model/work_item.go` copies only `Provider`, `Key`, and `RedactionLabel` into `ExecutionEnvelopeProtectedReference`.

This is sufficient for worker-local environment lookup, but an external KV provider needs:

- a logical provider alias selected by deployment configuration;
- a secret path;
- one selected field;
- an optional version.

The controller should preserve this metadata without knowing the provider's network configuration or authentication method.

## Target State

`variable.ProtectedRef` supports a shape equivalent to:

```go
type ProtectedRef struct {
    Provider       string `json:"provider"`
    Key            string `json:"key"`
    Field          string `json:"field,omitempty"`
    Version        int    `json:"version,omitempty"`
    RedactionLabel string `json:"redaction_label,omitempty"`
}
```

Rules:

- `Provider` is required and must be a safe logical identifier.
- Recommended provider identifier grammar is:

  ```text
  [A-Za-z][A-Za-z0-9_.-]*
  ```

- `Key` is required.
- `Field` is optional at the generic model layer.
- `Version` must be zero or positive.
- A backend may impose stricter rules; OpenBao will require `Field`.
- Strict JSON decoding remains enabled.
- Unknown JSON properties remain errors.
- Default redaction rendering remains safe and does not include plaintext.
- Existing `worker_env` references remain valid without modification.
- `ExecutionEnvelopeProtectedReference` carries `Field` and `Version`.
- Envelope validation reconstructs and validates the complete protected-reference locator.
- Any safe fingerprint or canonical serialization based on the execution envelope preserves the declared field and version as non-secret metadata.

Example:

```json
{
  "provider": "project_secrets",
  "key": "projects/landcore/api",
  "field": "token",
  "version": 3
}
```

## Concept Decision

This slice updates the existing protected-reference concept. It does not add plaintext lookup or a provider registry.

The controller validates syntax and durable reference shape, not worker deployment availability.

An unknown but syntactically valid provider alias is allowed through controller planning. The worker will fail closed if the alias is not registered when materialization is attempted.

Keep `Field` provider-neutral. It means "select one named member from a provider result" and is not limited in the generic model to OpenBao.

Keep `Version` numeric. `0` means unspecified/latest; positive values are explicit provider versions.

Do not put provider endpoint, mount, token source, CA path, or authentication configuration into `ProtectedRef`.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/external-protected-value-providers/README.md`
- `docs/concepts/sensitive-variable-propagation/README.md`
- `docs/concepts/sensitive-variable-propagation/002-protected-reference-model.md`
- `docs/concepts/sensitive-variable-propagation/003-controller-envelope-and-persistence.md`
- `internal/variable/protected_ref.go`
- `internal/variable/protected_ref_test.go`
- `internal/model/work_item.go`
- model tests covering execution-envelope JSON and validation

Do not read worker provider implementation files except to confirm compatibility with the existing `Provider` and `Key` fields.

## Allowed Production Files

- `internal/variable/protected_ref.go`
- `internal/model/work_item.go`

If execution-envelope fingerprint canonicalization manually enumerates protected-reference fields outside `internal/model/work_item.go`, stop and report the concrete file before editing it rather than silently widening the slice.

## Allowed Test Files

- `internal/variable/protected_ref_test.go`
- the existing `internal/model` work-item or execution-envelope test file

## Out Of Scope

- Worker resolver registration.
- OpenBao HTTP calls.
- Worker config.
- Authentication.
- TLS configuration.
- Controller-side provider discovery.
- Plaintext materialization.
- Redactor changes.
- Changes to public-variable semantics.
- New fingerprint policy beyond preserving the declared non-secret locator fields.

## Acceptance Criteria

- Existing `worker_env` and `test` protected references still parse and round-trip.
- A syntactically valid alias such as `project_secrets` parses and round-trips.
- Empty provider and key values are rejected.
- Invalid provider identifiers are rejected.
- Negative versions are rejected.
- `field` and positive `version` survive variable JSON, parameter conversion, execution-envelope construction, and envelope JSON round-trip.
- Unknown JSON fields remain rejected.
- Default formatting and JSON never contain plaintext.
- Safe provenance distinguishes references that differ by field or declared version.
- Controller/model tests show no plaintext field was added.
- `go test ./internal/variable ./internal/model` passes.

## Minimum Implementation Model

Minimum recommended model: `Codex 5.4-mini`, high reasoning.

The change is bounded, but it modifies a durable reference model shared by variable parsing, envelope validation, persistence serialization, and fingerprints. A Spark-class implementation is not recommended.

## Notes

Do not replace strict decoding with a permissive `map[string]any`.

Do not hard-code `openbao` into `internal/variable`. The variable model should remain backend-neutral.

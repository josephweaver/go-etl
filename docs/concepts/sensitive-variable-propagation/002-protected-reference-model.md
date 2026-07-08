# 002 Protected Reference Model

Status: implemented

## Objective

Add a language-neutral protected-reference model for sensitive values that should not be resolved by the controller.

This slice allows workflow/config declarations to say, "this variable is a sensitive string provided by `worker_env:GOET_GDRIVE_TOKEN`," without causing the resolver or controller to read that environment variable.

## Current State

Sensitive metadata may exist after slice 001, but sensitive values are still ordinary resolved values. There is not yet a durable, non-secret reference shape that can cross controller planning and persistence boundaries.

The current draft concept mentions durable protected references, but it still leans toward client-environment capture. The revised architecture should prefer worker-local references in phase 1.

## Target State

A variable declaration can describe a protected reference:

```json
{
  "name": "gdrive_token",
  "type": "string",
  "sensitive": true,
  "protected_ref": {
    "provider": "worker_env",
    "key": "GOET_GDRIVE_TOKEN"
  }
}
```

Resolution produces a typed sensitive protected reference, not plaintext.

Safe representation:

```text
${worker_env.GOET_GDRIVE_TOKEN}
```

Transport representation may use a model such as:

```json
{
  "type": "string",
  "sensitive": true,
  "protected_ref": {
    "provider": "worker_env",
    "key": "GOET_GDRIVE_TOKEN",
    "redaction_label": "${worker_env.GOET_GDRIVE_TOKEN}"
  }
}
```

## Concept Decision

Phase 1 supports protected references but not plaintext lookup.

Required provider names:

- `worker_env` — worker resolves the value from its own process environment.
- `test` — deterministic test provider usable only in unit tests if helpful.

Deferred provider names:

- `controller_env`
- `client_env`
- `worker_file`
- `secret_store`
- `vault`
- `aws_secrets_manager`
- `gcp_secret_manager`
- `kubernetes_secret`

`client_env` must not be treated as portable execution secret transport in this slice. If the workflow declares a client-only secret for remote worker execution, the controller should reject it or leave it unsupported until an explicit protected store exists.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/sensitive-variable-propagation/README.md`
- `internal/variable/README.md`
- `internal/variable/variable.go`
- `internal/variable/resolver.go`
- `internal/variable/type.go`
- `internal/variable/expression.go`
- existing `internal/variable/*_test.go`

Search for existing JSON variable declaration parsing before adding new shapes.

## Allowed Production Files

Expected files:

- `internal/variable/variable.go`
- `internal/variable/resolver.go`
- `internal/variable/type.go`

Optional helper files:

- `internal/variable/protected_ref.go`
- `internal/variable/protected_ref_test.go`

If transport models already live outside `internal/variable`, add only the smallest shared model needed and document the choice in `PROJECT_STATE.md`.

## Allowed Test Files

- `internal/variable/variable_test.go`
- `internal/variable/resolver_test.go`
- `internal/variable/protected_ref_test.go`

## Out Of Scope

- Reading actual environment variables.
- Validating that a worker environment key exists.
- Encrypting or storing secret plaintext.
- Controller persistence changes.
- Worker assignment payload changes.
- Python execution changes.
- External secret manager integration.

## Acceptance Criteria

- A protected-reference declaration parses and validates.
- Missing provider is rejected.
- Missing key is rejected.
- Unsupported provider is rejected unless deliberately allowed as a deferred reference.
- A protected reference is implicitly sensitive even if the author forgets `sensitive: true`; either set it true or reject the mismatch.
- A protected reference never stores plaintext in the resolved value.
- Safe rendering uses the redaction label, not a placeholder that could be confused for plaintext.
- Type information is preserved so downstream code knows whether the secret should be a string, bytes, JSON file, etc.
- Existing variable tests still pass.
- `go test ./internal/variable` passes.

## Notes

Do not make `protected_ref` a new general value type unless the existing model requires it. The preferred design is:

```text
typed value + sensitivity metadata + optional protected reference
```

not:

```text
secret is a fourth scalar type
```

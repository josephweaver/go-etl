# 002 Canonical JSON and SHA-256 Helpers

Status: proposed

## Objective

Introduce a small shared helper package for deterministic JSON canonicalization
and SHA-256 hashing. The helpers produce stable bytes for semantic fingerprints
used by persistence, source-control provenance, workflow compilation, and future
plugin state observations.

This slice defines canonical JSON v1 behavior, but does not wire the helpers
into persistence tables yet.

## Required Context

Read these files first:

- `docs/concepts/workflow-execution-persistence/README.md`
- `docs/fingerprints.md`
- `internal/variable/expression.go`
- `internal/variable/literal.go`

Do not read unrelated controller files unless compile or test failures directly
require it.

## Allowed Production Files

- `internal/fingerprint/canonical_json.go`

This slice needs a new production file. If the active HCI mode does not include
`+newfile`, pause before implementation and ask for an updated budget.

## Allowed Test Files

- `internal/fingerprint/canonical_json_test.go`

## Out Of Scope

- Wiring fingerprints into `internal/persistence`.
- Hashing project, workflow, work-item, output, or state-observation records.
- Implementing schema-aware omission of non-semantic fields.
- Implementing decimal normalization for JSON numbers.
- Accepting arbitrary floating-point JSON numbers.
- Replacing existing ad hoc hashes in controller code.
- Adding UUIDv7 generation.
- Adding source-control or GitHub behavior.

## Acceptance Criteria

- `CanonicalJSON` returns deterministic JSON bytes with stable object-key
  ordering and no insignificant whitespace.
- `SHA256Hex` returns a lowercase hex SHA-256 digest for input bytes.
- `CanonicalJSONSHA256` returns both canonical bytes and the corresponding
  digest.
- `ValidateSHA256Hex` accepts valid lowercase SHA-256 hex strings and rejects
  malformed values.
- Canonical JSON v1 accepts integer JSON numbers.
- Canonical JSON v1 rejects decimal, exponent, NaN, and infinity values.
- Decimal values must be represented as schema-defined strings, not JSON
  numbers.
- Null and missing fields remain distinct.
- Lists preserve order.
- The tests prove equivalent maps with different insertion order produce the
  same canonical bytes and digest.

## Notes

- Prefer standard-library JSON token handling where practical, but do not rely
  on map iteration order.
- Keep the package independent from persistence, workflow, and variable
  packages to avoid circular dependencies.
- The first implementation may canonicalize ordinary Go values and decoded JSON
  values. Schema-aware omission of non-semantic fields belongs to a later slice
  once schemas need it.
- This helper owns byte-level determinism, not semantic decisions about which
  fields matter.

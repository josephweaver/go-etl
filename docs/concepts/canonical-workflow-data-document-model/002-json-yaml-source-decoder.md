# 002 JSON YAML Source Decoder

Status: Proposed

## Objective

Add one source-decoding boundary that accepts JSON and the agreed strict YAML subset and returns a JSON-compatible value tree plus source-location diagnostics.

## Minimum Model

Codex 5.5, high reasoning. Duplicate keys, scalar normalization, and YAML ambiguity require careful parser review.

## Required Context

Read:

- OS-001
- `internal/fingerprint` canonical JSON helpers
- current controller JSON decoding helpers
- current source admission and canonical hash code in `cmd/controller/main.go`

## Allowed Production Files

- `internal/document/decode.go` new
- `internal/document/yaml.go` new if needed
- `internal/document/value.go` new if needed
- `go.mod` and `go.sum` only for one YAML dependency

## Allowed Test Files

- `internal/document/decode_test.go`
- `internal/document/yaml_test.go`

## Data State Transition

```text
JSON bytes or YAML bytes
        -> one generic value tree
        -> normalized strings/bools/integers/maps/lists
        -> source path and line/column diagnostics
```

No typed variables are created in this slice.

## Implementation Requirements

- Detect format from explicit file extension or caller-supplied media type; do not guess from arbitrary content when the caller already knows.
- Preserve integers without converting them through floating point.
- Reject duplicate keys in JSON and YAML.
- Reject non-string mapping keys, null, arbitrary tags, and unsupported fractional numbers.
- Prevent unbounded alias expansion; rejection is acceptable in phase one.
- Do not implicitly convert timestamp-looking YAML strings to datetime values.
- Normalize YAML maps to `map[string]any` and sequences to `[]any`.
- Provide deterministic errors with document path and location where possible.

## Out of Scope

- Typed variable creation.
- Public workflow normalization.
- Semantic fingerprints beyond proving equivalent trees canonicalize equally.

## Acceptance Criteria

- Equivalent JSON and YAML fixtures produce deeply equal value trees.
- Duplicate keys fail.
- YAML booleans and integers match JSON semantics.
- YAML timestamps remain strings.
- null and fractional numbers fail with clear errors.
- Alias bombs cannot cause unbounded expansion.

## Test Commands

```bash
go test ./internal/document
```

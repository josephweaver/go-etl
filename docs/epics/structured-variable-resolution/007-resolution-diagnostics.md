# 007 Resolution Diagnostics

Status: proposed

## Objective

Add consistent structured-resolution diagnostics that identify the root
variable and nested JSON Pointer path, distinguish reference cycles from
maximum-depth failures, and preserve the underlying resolution cause.

## Required Context

Read these files first:

- `docs/epics/structured-variable-resolution/README.md`
- `docs/epics/structured-variable-resolution/005-recursive-whole-value-references.md`
- `docs/epics/structured-variable-resolution/006-string-path-interpolation.md`
- `internal/variable/resolver.go`
- `internal/variable/resolver_test.go`

Do not read unrelated files unless test or compile failures directly require
it.

## Allowed Production Files

- `internal/variable/resolver.go`

## Allowed Test Files

- `internal/variable/resolver_test.go`

## Out Of Scope

- Changing successful resolved values.
- Changing namespace definitions or precedence.
- Changing reference, accessor, or interpolation grammar.
- Adding recovery, fallback values, or partial-resolution results.
- Adding domain-specific object or list validation.
- Translating diagnostics into HTTP status codes or CLI output schemas.
- Adding a general logging or telemetry system.

## Acceptance Criteria

- A structured-resolution error identifies the qualified root variable being
  resolved.
- An error below the root includes the failing node's JSON Pointer path, such
  as `/gpu/capacity` or `/2/environment/key`.
- The root node is represented consistently without inventing an object-field
  segment.
- JSON Pointer segments escape `~` as `~0` and `/` as `~1` so arbitrary object
  field names remain unambiguous.
- Errors from missing references, declared-type mismatches, invalid accessors,
  non-scalar interpolation values, and malformed resolved expressions retain
  the root variable and failing node path.
- Nested error wrapping preserves the original cause text rather than replacing
  it with a generic resolution failure.
- Direct and indirect cycles produce an explicit reference-cycle error.
- A cycle error reports the qualified variable chain, for example
  `workflow.a -> workflow.b -> workflow.a`.
- Cycle detection tracks the qualified variable selected after namespace
  lookup, so unqualified references are diagnosed according to the scope they
  actually resolve from.
- A long acyclic chain that exceeds `MaxDepth` still reports a distinct maximum
  resolution-depth error.
- Diagnostic tracking applies to references originating at the root, in object
  fields, in list items, and inside interpolation tokens.
- Existing successful resolution, interpolation, accessor, precedence, and
  fan-out tests continue to pass.
- Relevant variable-package tests pass.

## Notes

- JSON Pointer is used only as a diagnostic location format. It does not replace
  GOET's expression accessor syntax.
- The error should conceptually read as `resolve <qualified variable> at
  <pointer>: <cause>`. Exact punctuation may follow existing Go error style as
  long as the required information is stable and testable.
- The active reference chain is per resolution call; it must not introduce
  global resolver state.

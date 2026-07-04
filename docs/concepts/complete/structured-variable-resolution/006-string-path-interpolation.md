# 006 String and Path Interpolation

Status: implemented

## Objective

Resolve `${...}` reference tokens mixed with literal text in typed `string` and
`path` expressions, formatting referenced scalar values canonically while
preserving the enclosing node's declared type.

## Required Context

Read these files first:

- `docs/epics/structured-variable-resolution/README.md`
- `docs/epics/structured-variable-resolution/003-expression-definition-validation.md`
- `docs/epics/structured-variable-resolution/005-recursive-whole-value-references.md`
- `internal/variable/expression.go`
- `internal/variable/resolver.go`
- `internal/variable/resolver_test.go`

Do not read unrelated files unless test or compile failures directly require
it.

## Allowed Production Files

- `internal/variable/expression.go`
- `internal/variable/resolver.go`

## Allowed Test Files

- `internal/variable/expression_test.go`
- `internal/variable/resolver_test.go`

## Out Of Scope

- Interpolation in `int`, `bool`, `datetime`, `object`, or `list` nodes.
- Converting object or list values into text.
- Adding formatting directives, functions, arithmetic, or conditionals.
- Normalizing path separators or translating paths for a target environment.
- Changing namespace definitions, precedence, accessor syntax, or fan-out
  behavior.
- Establishing the final nested resolver error-path format.
- Adding domain-specific validation for the resulting string or path.

## Acceptance Criteria

- A typed `string` expression may combine literal text with one or more
  `${...}` reference tokens.
- A typed `path` expression supports the same interpolation grammar and
  resolves to a value whose type remains `path`.
- Qualified and unqualified references use the existing namespace behavior.
- Supported field and scalar index accessors may select values for
  interpolation.
- Referenced `string`, `path`, `int`, `bool`, and `datetime` values are rendered
  using their canonical serialized text.
- Object and list values selected for interpolation return a type error.
- Missing references and invalid accessors return errors rather than leaving an
  unresolved token in the output.
- Multiple tokens are resolved from left to right without changing surrounding
  literal text.
- `\${` emits a literal `${` and does not perform a variable lookup.
- A whole-value expression consisting only of `${...}` continues to use slice
  005 behavior, including declared-type matching, rather than interpolation
  conversion.
- Each reference resolved during interpolation counts toward the configured
  maximum resolution depth.
- Interpolation can occur in string and path nodes nested at any object or list
  depth.
- Relevant variable-package tests pass.

## Notes

- Canonical formatting should be deterministic so resolved values remain
  suitable for fingerprints and reproducible work-item inputs.
- For example, an `int` uses base-10 text, a `bool` uses `true` or `false`, and
  a `datetime` uses the variable system's canonical datetime representation.
- Interpolation constructs text. It does not reinterpret the resulting text as
  another expression or recursively expand `${...}` sequences produced by a
  referenced value.

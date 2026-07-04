# 006 Controller Environment Accessor

Status: proposed

## Objective

Allow a bounded variable resolver to resolve string-only `controller_env` keys
through an injected lookup function, caching each present or missing key for
that resolver without copying or enumerating the process environment.

## Required Context

Read these files first:

- `docs/concepts/complete/controller-startup-resolution/README.md`
- `internal/variable/resolver.go`
- `internal/variable/resolver_test.go`
- `internal/variable/namespace.go`

Do not read unrelated files unless a targeted test failure directly requires
it.

## Allowed Production Files

- `internal/variable/resolver.go`

## Allowed Test Files

- `internal/variable/resolver_test.go`

## Out Of Scope

- Calling `os.LookupEnv` from `internal/variable`
- Wiring the accessor into controller startup source assembly
- Supporting `client_env`, `worker_env`, or arbitrary accessor namespaces
- Converting environment strings to integers, booleans, paths, or structured
  values
- Enumerating, snapshotting, serializing, or exposing the process environment
- Sensitivity metadata, redaction, protected storage, or provenance
- Command-line overrides, runtime values, and resolver-depth bootstrap

## Acceptance Criteria

- `ResolverConfig` can receive an optional string lookup function dedicated to
  the `controller_env` namespace.
- A qualified reference such as `controller_env.DB_PASSWORD` calls the lookup
  with exactly `DB_PASSWORD` and resolves a present value as `TypeString`.
- Controller-config string interpolation and whole-value string references can
  consume `controller_env` values through the normal recursive resolver path.
- An unqualified key uses an existing variable from the resolver set before
  consulting `controller_env`; the accessor is the lowest startup fallback.
- A qualified `controller_env` reference always uses the accessor rather than
  an enumerable scope entry with the same name.
- Each key is looked up at most once per resolver, including when it is missing
  or referenced repeatedly through different expressions.
- Copies of one resolver share its bounded cache, while two independently
  constructed resolvers perform independent lookups.
- Concurrent resolution through one resolver does not race or invoke the
  lookup more than once for the same key.
- A missing key produces the normal contextual variable-not-found diagnostic
  naming `controller_env.KEY` without exposing any other environment value.
- An empty environment string with `ok: true` remains a present string value;
  existing required-string helpers may still reject it as empty.
- With no configured lookup, `controller_env.KEY` behaves as missing.
- Existing resolver behavior and tests for ordinary scopes remain unchanged.

## Notes

- Use a function signature equivalent to `os.LookupEnv` so controller startup
  can inject that function later without adapting its semantics.
- Resolver methods currently use value receivers. Store cache state behind a
  pointer created by `NewResolver` so copied resolver values still represent
  one bounded observation.
- Cache both the returned string and the boolean presence result. Caching only
  successful lookups would let a newly-created environment value appear during
  one resolution operation.
- The cache is ephemeral resolver state. It must not be exposed as a variable
  scope or persisted after the bounded resolver is discarded.

# 002 Worker Protected-Value Resolver Registry

Status: proposed

## Objective

Add a deterministic worker-side registry that maps a protected reference's logical provider alias to one `ProtectedValueResolver`, while preserving `worker_env` as a built-in provider.

## Current State

`cmd/worker/protected_value.go` defines:

```go
type ProtectedValueResolver interface {
    ResolveProtectedValue(ctx context.Context, ref ProtectedValueRef) (SensitiveValue, error)
}
```

It also defines `WorkerEnvProtectedValueResolver`.

The execution paths directly instantiate `WorkerEnvProtectedValueResolver`, so the interface exists but there is no provider dispatch mechanism.

## Target State

The worker package has a registry or composite resolver equivalent to:

```go
type ProtectedValueResolverRegistry struct {
    providers map[string]ProtectedValueResolver
}

func NewProtectedValueResolverRegistry() *ProtectedValueResolverRegistry
func (r *ProtectedValueResolverRegistry) Register(name string, resolver ProtectedValueResolver) error
func (r *ProtectedValueResolverRegistry) ResolveProtectedValue(
    ctx context.Context,
    ref ProtectedValueRef,
) (SensitiveValue, error)
```

Required behavior:

- provider names use the same identifier validation as protected references;
- registration rejects empty names;
- registration rejects nil resolvers;
- duplicate registration is rejected rather than silently replaced;
- resolution dispatches only to `ref.Provider`;
- an unregistered provider fails closed;
- unsupported-provider errors contain the safe provider name or redaction label, never plaintext;
- registry iteration order is irrelevant and no environment or provider enumeration is exposed;
- `worker_env` can be registered under the reserved name `worker_env`;
- unit tests may register a deterministic fake provider under `test`.

## Concept Decision

This slice adds one new concept: the worker resolver registry. It deserves its own file because it has independent ownership, validation, registration behavior, dispatch behavior, and tests.

Keep each backend resolver responsible for resolving its own locator fields.

The registry must not:

- read worker configuration;
- construct HTTP clients;
- load tokens;
- modify protected references;
- cache plaintext values;
- persist provider state;
- expose provider enumeration to workflows.

Do not merge the registry with `Redactor`. Resolution and output scrubbing are separate responsibilities.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/external-protected-value-providers/README.md`
- `docs/concepts/sensitive-variable-propagation/004-worker-secret-resolver-and-redactor.md`
- `cmd/worker/protected_value.go`
- `cmd/worker/protected_value_test.go`
- `cmd/worker/redactor.go`
- `internal/variable/protected_ref.go`

Do not read controller packages.

## Allowed Production Files

- `cmd/worker/protected_value.go`
- `cmd/worker/protected_value_registry.go` (new)

## Allowed Test Files

- `cmd/worker/protected_value_test.go`
- `cmd/worker/protected_value_registry_test.go` (new)

## Out Of Scope

- Changing `Worker`.
- Changing trusted Go work-item dispatch.
- Changing Python materialization.
- Worker JSON configuration.
- OpenBao.
- HTTP or TLS.
- Token sources.
- Redactor behavior.
- Controller or persistence changes.

## Acceptance Criteria

- A registry can register and resolve through `worker_env`.
- A registry can register and resolve through a deterministic fake provider.
- Missing provider names are rejected during registration.
- Nil resolvers are rejected.
- Duplicate provider aliases are rejected.
- An unregistered provider fails closed with a sanitized error.
- The registry does not include secret plaintext in `String`, `GoString`, JSON, or errors.
- Resolution honors context cancellation.
- Existing `WorkerEnvProtectedValueResolver` tests continue to pass.
- `go test ./cmd/worker` passes.

## Minimum Implementation Model

Minimum recommended model: `Codex 5.4-mini`, medium reasoning.

This is a small independent abstraction with a narrow test surface.

## Notes

Prefer explicit registration over a global mutable map.

The registry should be constructed per worker process so tests and future multi-worker embedding do not share mutable global provider state.

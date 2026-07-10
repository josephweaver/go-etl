# 003 Inject Worker Resolver Across Execution Paths

Status: proposed

## Objective

Make `Worker` own one protected-value resolver and route both trusted Go operation contexts and Python subprocess materialization through it, removing direct construction of `WorkerEnvProtectedValueResolver` from execution code.

## Current State

`cmd/worker/work_context.go` currently contains:

```go
resolver := WorkerEnvProtectedValueResolver{}
```

`cmd/worker/work_python.go` independently contains the same direct construction.

`cmd/worker/worker.go` currently stores only:

```go
type Worker struct {
    Config Config
}
```

`cmd/worker/main.go` constructs the worker with a struct literal.

This means adding a provider registry without changing both call sites would leave inconsistent secret-resolution behavior.

## Target State

`Worker` owns or can obtain one injected `ProtectedValueResolver`.

One acceptable shape is:

```go
type Worker struct {
    Config                  Config
    ProtectedValueResolver ProtectedValueResolver
}
```

or a constructor-enforced private field:

```go
func NewWorker(cfg Config, resolver ProtectedValueResolver) (Worker, error)
```

Required behavior:

- production worker startup constructs one registry containing `worker_env`;
- trusted Go `operationContext` resolves through the worker-owned resolver;
- Python `materializePythonProtectedRefs` resolves through the same worker-owned resolver;
- tests can inject deterministic fake resolvers;
- no execution path silently falls back to direct `os.LookupEnv` when a resolver was explicitly injected;
- a nil resolver is rejected during worker validation or construction;
- the existing `worker_env` fixture continues to work;
- resolved plaintext still enters the existing `SensitiveValue`, `Redactor`, safe logger, env materialization, and temp-file materialization paths unchanged.

## Concept Decision

This slice updates the existing worker execution boundary. It does not add a new external provider.

Prefer constructor or validation enforcement over a hidden global default.

A compatibility helper may build the default registry:

```go
func defaultProtectedValueResolver() (ProtectedValueResolver, error)
```

but operation code must not instantiate provider implementations directly.

The resolver is process-scoped configuration. It is not controller data and is not serialized into work items.

Do not move resolution into `internal/variable.Resolver`; protected-value lookup must remain an explicit worker operation.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/external-protected-value-providers/README.md`
- `docs/concepts/external-protected-value-providers/002-worker-protected-value-resolver-registry.md`
- `docs/concepts/sensitive-variable-propagation/README.md`
- `cmd/worker/main.go`
- `cmd/worker/worker.go`
- `cmd/worker/work_context.go`
- `cmd/worker/work_python.go`
- `cmd/worker/protected_value.go`
- `cmd/worker/protected_value_registry.go`
- current worker tests for Go sensitive context and Python materialization

Do not read OpenBao or controller code.

## Allowed Production Files

- `cmd/worker/main.go`
- `cmd/worker/worker.go`
- `cmd/worker/work_context.go`
- `cmd/worker/work_python.go`

A narrow helper may be added to:

- `cmd/worker/protected_value_registry.go`

only if it constructs the built-in `worker_env` registry.

## Allowed Test Files

- `cmd/worker/worker_test.go`
- existing work-context tests
- existing Python protected-materialization tests
- `cmd/worker/protected_value_registry_test.go`

## Out Of Scope

- Worker provider JSON configuration.
- OpenBao.
- HTTP, TLS, or token files.
- Controller changes.
- New materialization modes.
- Redactor redesign.
- Artifact scanning.
- Provider result caching.

## Acceptance Criteria

- `Worker` cannot execute protected-reference materialization without an explicit valid resolver.
- Production startup registers `worker_env`.
- Trusted Go execution uses the injected resolver.
- Python subprocess materialization uses the same injected resolver.
- A fake resolver can be injected in tests without modifying process environment.
- An unregistered provider produces a sanitized execution failure in both paths.
- A resolved value is registered with the existing attempt-local redactor before handler or subprocess use.
- Existing environment and temp-file materialization behavior remains unchanged.
- The implemented credentialed worker fixture based on `worker_env` still passes.
- `go test ./cmd/worker` passes.

## Minimum Implementation Model

Minimum recommended model: `Codex 5.4-mini`, high reasoning.

The behavior is conceptually simple but crosses the two independently implemented sensitive execution paths. The implementation must avoid leaving one path on the old resolver.

## Notes

Search for every direct construction of:

```text
WorkerEnvProtectedValueResolver
```

before completing the slice.

Do not widen this slice into provider configuration merely because `main.go` is already being edited.

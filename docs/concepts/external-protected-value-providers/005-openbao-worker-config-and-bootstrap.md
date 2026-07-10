# 005 OpenBao Worker Config and Bootstrap

Status: proposed

## Objective

Add strict worker configuration for logical protected-value provider aliases, construct OpenBao KV v2 providers at worker startup, and support worker-local token environment/file sources plus optional additional CA trust without storing plaintext tokens in worker JSON.

## Current State

After slice 004, the OpenBao resolver can be constructed directly in tests, but production worker startup still creates only the built-in `worker_env` resolver.

`cmd/worker/config.go` has no protected-value provider section and currently uses permissive top-level `json.Unmarshal`.

There is no production token-source or TLS-client assembly.

## Target State

Worker configuration supports a shape equivalent to:

```json
{
  "protected_value_providers": {
    "project_secrets": {
      "type": "openbao_kv_v2",
      "address": "https://openbao.example.internal:8200",
      "mount": "secret",
      "auth": {
        "method": "token_file",
        "path": "/run/goet/openbao-token"
      },
      "ca_cert_file": "/etc/goet/openbao-ca.pem",
      "request_timeout_seconds": 10
    }
  }
}
```

Environment bootstrap example:

```json
{
  "protected_value_providers": {
    "project_secrets": {
      "type": "openbao_kv_v2",
      "address": "https://127.0.0.1:8200",
      "mount": "secret",
      "auth": {
        "method": "token_env",
        "name": "GOET_OPENBAO_TOKEN"
      }
    }
  }
}
```

### Provider aliases

- The object key is the logical provider alias used by workflows.
- Alias validation matches `ProtectedRef.Provider`.
- `worker_env` is reserved for the built-in provider.
- Duplicate or reserved aliases are rejected.
- Multiple OpenBao aliases may coexist.

### Strict provider-block decoding

Each provider block is decoded strictly with unknown fields rejected.

This is required so a configuration like:

```json
{
  "token": "plaintext-secret"
}
```

does not silently appear accepted while being ignored.

Top-level worker config strictness may remain unchanged if changing it would exceed this slice. Provider and auth blocks must be strict.

### Supported provider type

Initial supported configured type:

```text
openbao_kv_v2
```

Unknown types fail worker startup.

### Supported auth methods

#### `token_env`

```json
{
  "method": "token_env",
  "name": "GOET_OPENBAO_TOKEN"
}
```

The token source calls `os.LookupEnv` for the declared name at each provider request.

#### `token_file`

```json
{
  "method": "token_file",
  "path": "/run/goet/openbao-token"
}
```

The token source reads and trims the file at each provider request.

The file path is configuration metadata. Its content is sensitive and must never be logged.

Token sources reject empty values.

### Path resolution

Relative `token_file.path` and `ca_cert_file` values are resolved relative to the worker config file, following existing worker config path behavior.

### TLS

- HTTPS uses normal system certificate roots.
- Optional `ca_cert_file` adds PEM certificates to a cloned/system root pool.
- `insecure_skip_verify` is not supported.
- Plain HTTP is rejected except for an explicit loopback-only fixture option.
- A development-only option, if required for the smoke, must be named narrowly such as:

  ```json
  "allow_loopback_http": true
  ```

- The option must accept only loopback hosts and must not permit arbitrary remote HTTP.

### Timeouts and body limits

- `request_timeout_seconds` must be positive when set.
- A safe default is used when omitted.
- The provider continues to enforce its bounded response body.

### Startup assembly

Worker startup:

1. creates the resolver registry;
2. registers built-in `worker_env`;
3. parses each provider alias block;
4. builds its token source and HTTP client;
5. constructs the OpenBao resolver;
6. registers the logical alias;
7. creates `Worker` with the completed resolver.

Invalid provider configuration fails before the worker requests assignments.

## Concept Decision

This slice adds two related concepts:

- provider configuration/factory assembly;
- bootstrap token and TLS construction.

They may live in one narrow file if cohesive:

```text
cmd/worker/protected_value_provider_config.go
```

Token sources may receive their own file if needed for independent tests.

Do not put bootstrap tokens in `SensitiveValue`. They authenticate the provider client and must never be exposed to work-item handlers or subprocesses.

Do not pass OpenBao provider configuration through the controller or execution envelope.

Do not cache token contents. Reading env/file at request time allows external rotation.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/external-protected-value-providers/README.md`
- slices 001-004 in this concept
- `cmd/worker/config.go`
- `cmd/worker/config_test.go`
- `cmd/worker/main.go`
- `cmd/worker/worker.go`
- `cmd/worker/protected_value_registry.go`
- `cmd/worker/openbao_kv_v2.go`
- existing relative-path config tests

Do not read controller configuration unless a worker-config generation path is proven to copy worker fields and must be updated. If that occurs, report the concrete boundary before editing.

## Allowed Production Files

- `cmd/worker/config.go`
- `cmd/worker/main.go`
- `cmd/worker/worker.go`
- `cmd/worker/protected_value_provider_config.go` (new)
- `cmd/worker/openbao_token_source.go` (new if not created in slice 004)

A narrow update to an existing worker-config generation file is allowed only if the controller currently serializes worker `Config` fields explicitly. Name and report that file before editing.

## Allowed Test Files

- `cmd/worker/config_test.go`
- `cmd/worker/protected_value_provider_config_test.go` (new)
- `cmd/worker/openbao_token_source_test.go` (new)
- narrow worker startup tests

## Out Of Scope

- Controller secret custody.
- Secret writes or administration.
- AppRole, OIDC, Kubernetes auth, cloud IAM, or response wrapping.
- Plaintext token properties.
- `insecure_skip_verify`.
- Arbitrary remote HTTP.
- Automatic provider discovery.
- Reloading the complete worker config while the process is running.
- Retry/backoff policy.
- Cloud secret-manager providers.
- OS keychain providers.

## Acceptance Criteria

- A worker config with no external providers still starts with built-in `worker_env`.
- A valid `openbao_kv_v2` alias builds and registers.
- Multiple valid aliases register independently.
- Invalid alias names are rejected.
- Reserved alias `worker_env` cannot be replaced.
- Unknown provider types are rejected.
- Provider and auth blocks reject unknown fields.
- Plaintext properties such as `token`, `root_token`, or `secret_id` are rejected as unknown.
- `token_env` reads the declared environment variable just in time.
- `token_file` reads the declared file just in time and trims terminal whitespace.
- Empty or missing bootstrap tokens fail with sanitized errors.
- Relative token and CA paths resolve relative to the worker config file.
- HTTPS uses normal verification.
- Additional PEM CA certificates can be loaded.
- Invalid CA data fails startup without exposing file contents.
- `insecure_skip_verify` is not accepted.
- Plain HTTP is rejected unless explicit loopback-only fixture mode is enabled.
- Loopback-only mode rejects non-loopback hosts.
- Request timeout validation and defaults are tested.
- Worker startup fails before fetching work when provider assembly fails.
- Token and secret sentinels are absent from configuration errors and startup errors.
- Existing worker config and `worker_env` tests pass.
- `go test ./cmd/worker` passes.

## Minimum Implementation Model

Minimum recommended model: `Codex 5.5`, high reasoning.

This slice combines strict nested configuration, path resolution, bootstrap-secret handling, TLS trust construction, provider factory assembly, and startup failure behavior. Further decomposition is preferable to assigning it to a smaller model if the actual worker-config generation boundary is wider than expected.

## Notes

Preferred HPCC bootstrap posture:

```text
scheduler/site helper/OpenBao agent
            ↓ writes or refreshes
worker-local token file with restrictive permissions
            ↓ read just in time
GOET OpenBao provider
```

The worker config may identify the token file, but must not contain the token itself.

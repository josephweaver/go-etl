# External Protected-Value Providers

Status: proposed

Delivery cadence: `CSxIx` — define and review the complete Operational Slice set before implementation begins.

## Purpose

Extend GOET's implemented sensitive-variable boundary so a worker can resolve protected references from configurable external secret stores without making the controller a secret custodian.

The existing `sensitive-variable-propagation` concept already establishes the critical boundary:

```text
controller plans and persists protected references
                         ↓
worker resolves plaintext only when an operation requires it
                         ↓
trusted Go handlers or deliberate subprocess materializations use it
                         ↓
GOET-controlled outputs are redacted before persistence
```

This Strategic Concept adds a provider layer behind that worker-side resolution boundary. The first production external provider is OpenBao KV version 2.

GOET will use the OpenBao HTTP API directly through Go's standard library. It will not add a HashiCorp Vault SDK or other HashiCorp-owned runtime dependency.

## Goals

- Keep protected-reference plaintext resolution inside the worker process.
- Preserve the controller's current reference-only persistence and assignment behavior.
- Treat the `provider` value in a protected reference as a logical deployment-local provider alias rather than a hard-coded backend type.
- Allow multiple configured provider instances, including multiple OpenBao servers or mounts.
- Preserve the built-in `worker_env` provider for local development, fixtures, and bootstrap credentials.
- Add a worker-side resolver registry that dispatches a protected reference to the configured provider alias.
- Support OpenBao KV v2 read operations for one explicitly named field at one secret path.
- Support an optional declared KV version while allowing version `0` or omission to mean the provider's latest version.
- Bootstrap OpenBao authentication from a worker-local environment variable or worker-local token file.
- Read bootstrap credentials just in time so token rotation does not require restarting the controller.
- Support normal system TLS trust and an optional additional CA certificate file.
- Reject plaintext OpenBao tokens in worker JSON configuration.
- Preserve exact-value redaction, structured-output rejection, and restrictive temporary-file cleanup already implemented by `sensitive-variable-propagation`.
- Fail closed with sanitized errors when a provider alias is missing, misconfigured, unavailable, unauthorized, or returns an invalid value.
- Prove through a repeatable fixture that the raw secret does not enter controller status, logs, workflow persistence, work-item persistence, or completion evidence.

## Non-Goals

- Making the controller a secret broker, encrypted keystore, or plaintext secret store.
- Sending client-machine environment variables through the controller to remote workers.
- Writing, rotating, deleting, listing, or administering secrets in OpenBao.
- Deploying or operating an OpenBao cluster as part of GOET.
- Adding HashiCorp Vault-specific packages or SDK dependencies.
- Adding AWS Secrets Manager, Google Secret Manager, Azure Key Vault, Kubernetes Secrets, SOPS, or OS keychain providers in this concept.
- Implementing OpenBao AppRole, Kubernetes auth, OIDC, cloud IAM auth, response wrapping, dynamic database credentials, PKI issuance, or transit encryption in the first provider.
- Persisting a bootstrap token in `worker_config.json`.
- Supporting `insecure_skip_verify`.
- Treating an entire KV object as one implicit secret value.
- Automatically scanning user artifacts or outbound network traffic for leaked secrets.
- Preventing deliberate exfiltration by arbitrary Python, R, Bash, or other user-authored code.
- Redesigning workflow variables, dependency scheduling, retries, artifact promotion, or resource admission beyond the narrow protected-reference changes required here.

## Architectural Context

This concept extends:

```text
docs/concepts/sensitive-variable-propagation/
```

It does not replace that concept.

Relevant current implementation points include:

- `internal/variable/protected_ref.go`
  - defines `ProtectedRef`;
  - currently accepts only the built-in `worker_env` and test providers;
  - preserves strict JSON decoding and safe redaction labels.
- `internal/model/work_item.go`
  - carries protected references through `ExecutionEnvelopeProtectedReference`;
  - persists non-secret provider/key/materialization metadata.
- `cmd/worker/protected_value.go`
  - defines `ProtectedValueResolver`, `SensitiveValue`, and the built-in `WorkerEnvProtectedValueResolver`.
- `cmd/worker/work_context.go`
  - directly constructs `WorkerEnvProtectedValueResolver` for trusted Go operations.
- `cmd/worker/work_python.go`
  - directly constructs `WorkerEnvProtectedValueResolver` before subprocess materialization.
- `cmd/worker/config.go`
  - currently has no protected-value-provider configuration.

The completed architecture should remain:

```text
workflow/project declaration
    protected_ref:
      provider: project_secrets
      key: projects/landcore/api
      field: token
      version: 3
                 │
                 │ non-secret reference metadata
                 ▼
controller compile / persist / assign
                 │
                 │ same protected reference
                 ▼
worker resolver registry
                 │
                 ├── worker_env
                 └── project_secrets
                         type: openbao_kv_v2
                         address: https://...
                         mount: secret
                         auth: worker-local env or token file
                 │
                 │ plaintext only inside worker boundary
                 ▼
SensitiveValue -> trusted Go context or deliberate subprocess materialization
```

## Core Decisions

### 1. Provider names are logical aliases

A protected reference should not contain a server address, mount configuration, authentication method, CA path, or token source.

Example workflow declaration:

```json
{
  "name": "landcore_api_token",
  "type": "string",
  "sensitive": true,
  "protected_ref": {
    "provider": "project_secrets",
    "key": "projects/landcore/api",
    "field": "token",
    "version": 3
  },
  "materialize": {
    "mode": "env",
    "target": "LANDCORE_API_TOKEN"
  }
}
```

Example worker configuration:

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

The same workflow remains portable when each execution environment provides the same logical alias with environment-specific backend configuration.

### 2. Unknown provider aliases may cross the controller

`internal/variable` should validate provider-name syntax and protected-reference shape. It should not require the controller to know every worker-side provider alias.

An unconfigured provider is rejected by the worker resolver registry when execution attempts to materialize the protected value.

This is safe because the controller still stores only non-secret reference metadata.

### 3. OpenBao is read-only from GOET

The first provider supports only:

```text
GET /v1/<mount>/data/<path>?version=<n>
```

The provider selects exactly one explicitly declared field from the returned `data.data` object.

GOET does not expose create, update, patch, delete, list, metadata administration, or secret-engine configuration operations.

### 4. Use the standard library HTTP client

The provider should use:

- `net/http`;
- `crypto/tls`;
- `crypto/x509`;
- `encoding/json`;
- bounded response-body reads;
- context cancellation and request deadlines.

Do not add a HashiCorp Vault SDK merely to perform a KV v2 read.

The compatible OpenBao API retains the `X-Vault-Token` request header. Using that protocol header does not require a HashiCorp software dependency.

### 5. Bootstrap credentials remain worker-local

The initial OpenBao provider may authenticate with:

- `token_env`: read a named worker environment variable at request time;
- `token_file`: read a worker-local file at request time.

The worker JSON config stores only the environment-variable name or file path. It never stores the token value.

A token file is preferred for HPCC deployments because a scheduler prologue, site credential helper, or future OpenBao agent can refresh the file independently of GOET.

### 6. Provider results remain `SensitiveValue`

External provider plaintext enters the same implemented protection path as `worker_env` plaintext:

- `SensitiveValue` default rendering is redacted;
- the exact plaintext is registered with the attempt-local redactor;
- trusted Go handlers receive it only through `OperationContext.Sensitive`;
- subprocesses receive it only through declared env or restrictive temp-file materialization;
- captured stdout/stderr is scrubbed;
- structured output containing the exact plaintext is rejected;
- temporary materializations are removed.

### 7. Provider errors must be sanitized

Errors may identify:

- logical provider alias;
- safe protected-reference redaction label;
- HTTP status category;
- whether configuration, authentication, transport, response shape, field selection, or type validation failed.

Errors must not include:

- bootstrap token;
- secret plaintext;
- unbounded response bodies;
- authorization response bodies;
- request headers;
- complete worker environment;
- token-file contents.

### 8. OpenBao field values are strings in the first implementation

The provider requires `field`.

The selected KV field must decode as a JSON string. GOET does not silently stringify objects, arrays, numbers, booleans, or null values.

This keeps materialization behavior explicit and avoids creating accidental credential files from arbitrary JSON objects.

### 9. Version metadata is non-secret

`version` is optional non-secret locator metadata.

- omitted or `0`: retrieve the current OpenBao version;
- positive integer: retrieve that exact version;
- negative integer: invalid.

The declared version may participate in safe fingerprints because it is reference metadata, not plaintext.

This concept does not require the worker to report the resolved latest version back to the controller.

## Proposed Slices

1. `001-generalize-protected-reference-locators.md`
   - make protected references provider-alias neutral;
   - add optional `field` and `version` metadata;
   - preserve strict decoding, envelope transport, and safe fingerprints.

2. `002-worker-protected-value-resolver-registry.md`
   - add a deterministic worker-side provider registry;
   - retain `worker_env` as a built-in registered provider.

3. `003-inject-worker-resolver-across-execution-paths.md`
   - make `Worker` own the resolver;
   - remove direct `WorkerEnvProtectedValueResolver` construction from trusted Go and Python paths.

4. `004-openbao-kv-v2-read-provider.md`
   - implement a read-only OpenBao KV v2 provider using standard-library HTTP;
   - unit test against an `httptest` server.

5. `005-openbao-worker-config-and-bootstrap.md`
   - add strict worker provider configuration;
   - assemble aliases, token sources, TLS trust, timeouts, and the resolver registry at worker startup.

6. `006-openbao-protected-value-fixture-smoke-and-doc-sync.md`
   - prove the end-to-end boundary with a disposable OpenBao fixture;
   - inspect controlled logs, status, and SQLite persistence for a sentinel secret;
   - synchronize project-state and runbook documentation.

## Completion Criteria

- All agreed Operational Slices are implemented and accepted.
- Existing `worker_env` protected references continue to function without workflow changes.
- `ProtectedRef` accepts syntactically valid logical provider aliases and preserves `field` and `version`.
- The controller continues to persist and transmit only non-secret protected-reference metadata.
- Worker execution uses one injected resolver registry for both trusted Go and Python subprocess paths.
- A configured OpenBao KV v2 alias resolves exactly one string field from the requested path and optional version.
- OpenBao retrieval uses no HashiCorp Vault SDK dependency.
- Worker provider configuration contains no plaintext token field.
- Bootstrap tokens can be read from a worker-local environment variable or token file.
- Additional CA trust can be configured without disabling TLS verification.
- Missing aliases, invalid references, unavailable fields, non-string values, HTTP failures, and invalid responses fail closed with sanitized errors.
- Exact-value redaction and structured-output rejection work for OpenBao-resolved values.
- A repeatable fixture smoke proves the raw sentinel is absent from controller status, submission status, worker output, captured stdout/stderr, controller logs, and inspected SQLite persistence.
- `PROJECT_STATE.md` and relevant concept documentation describe the implemented provider boundary and its limitations.

## External Protocol Reference

Implementation should verify behavior against the current OpenBao source documentation:

```text
openbao/openbao
website/content/api-docs/secret/kv/kv-v2.mdx
```

At the time this concept was drafted, that document specifies a KV v2 read as:

```text
GET /:secret-mount-path/data/:path?version=:version-number
```

with the secret field map under:

```text
response.data.data
```

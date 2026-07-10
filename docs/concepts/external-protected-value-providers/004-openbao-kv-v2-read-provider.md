# 004 OpenBao KV v2 Read Provider

Status: proposed

## Objective

Implement a read-only OpenBao KV version 2 protected-value resolver using Go's standard-library HTTP stack, with an injected token source and HTTP client, without adding a HashiCorp Vault SDK dependency.

## Current State

After slices 001-003:

- protected references can carry provider alias, key, field, and version;
- the worker has a provider registry;
- both trusted Go and Python execution paths use the injected registry;
- only `worker_env` has a production resolver.

There is no external secret-store implementation.

## Target State

The worker package has an OpenBao KV v2 resolver equivalent to:

```go
type OpenBaoTokenSource interface {
    Token(context.Context) (string, error)
}

type OpenBaoKVV2Provider struct {
    Address     string
    Mount       string
    TokenSource OpenBaoTokenSource
    HTTPClient  *http.Client
}
```

Resolution behavior:

1. Validate provider configuration and protected-reference locator.
2. Require a non-empty `Key`.
3. Require a non-empty `Field`.
4. Accept `Version == 0` as latest.
5. Build a path equivalent to:

   ```text
   GET <address>/v1/<mount>/data/<key>?version=<version>
   ```

6. Obtain the token from the injected token source immediately before the request.
7. Send it only through the `X-Vault-Token` request header.
8. Decode the bounded JSON response.
9. Select:

   ```text
   response.data.data[ref.field]
   ```

10. Require the selected value to be a JSON string.
11. Return it through `NewSensitiveValue`.
12. Never include token, selected value, or response body in an error.

The provider should be registerable under any logical alias. It must not require the alias itself to equal `openbao`.

## Concept Decision

This slice adds a new provider implementation and its protocol client in one file or one small pair of files.

Use the standard library rather than a HashiCorp SDK.

The OpenBao API is compatible with a KV v2 read using:

```text
GET /v1/<mount>/data/<path>?version=<n>
X-Vault-Token: <token>
```

The implementation should verify this against the current OpenBao source documentation before coding:

```text
openbao/openbao
website/content/api-docs/secret/kv/kv-v2.mdx
```

### URL handling

- Parse `Address` with `net/url`.
- Reject addresses containing userinfo.
- Normalize only the trailing slash needed for joining.
- Preserve escaped path segments safely.
- Reject path traversal semantics rather than cleaning a malicious key into a different secret path.
- Do not log the complete URL if the key is considered operationally sensitive; prefer the protected-reference redaction label.

### Response handling

- Limit the response body before decoding.
- Treat non-2xx status as a sanitized provider error.
- Do not echo OpenBao error bodies.
- Reject missing `data`, missing nested `data`, missing field, null field, and non-string field.
- Accept metadata without exposing it.
- Do not return the bootstrap token or entire secret document.

### Context behavior

- Create the HTTP request with the caller context.
- Honor cancellation and timeout errors.
- Do not retry in this first slice; retry policy belongs in a later provider-hardening concept if operational evidence requires it.

## Required Context

Read these files first:

- `AGENTS.md`
- `PROJECT_STATE.md`
- `docs/concepts/external-protected-value-providers/README.md`
- slices 001-003 in this concept
- `cmd/worker/protected_value.go`
- `cmd/worker/protected_value_registry.go`
- current OpenBao KV v2 API source documentation
- standard-library `net/http`, `net/url`, and `encoding/json` behavior

Do not read controller packages.

## Allowed Production Files

- `cmd/worker/openbao_kv_v2.go` (new)

A second narrow file is allowed only if token-source interfaces would otherwise mix protocol resolution and bootstrap-source behavior:

- `cmd/worker/openbao_token_source.go` (optional new interface-only file)

Do not modify `go.mod`.

## Allowed Test Files

- `cmd/worker/openbao_kv_v2_test.go` (new)

Use `httptest.Server`. Do not require a real OpenBao process in unit tests.

## Out Of Scope

- Worker JSON configuration.
- Constructing token environment or file sources.
- Custom CA loading.
- OpenBao writes, list, delete, metadata administration, transit, PKI, or dynamic credentials.
- AppRole, Kubernetes auth, OIDC, cloud IAM, or response wrapping.
- Retries.
- Provider-side caching.
- Controller-side secret access.
- Artifact scanning.
- Adding external SDK dependencies.

## Acceptance Criteria

- A valid KV v2 response resolves the explicitly selected string field.
- Version `0` omits or safely treats the query as latest.
- Positive version adds the correct query parameter.
- Missing key is rejected.
- Missing field is rejected.
- Invalid address or mount is rejected.
- Missing token fails with a sanitized error.
- Non-2xx responses fail without returning the response body.
- Malformed JSON fails without leaking body contents.
- Missing nested data fails.
- Missing selected field fails.
- Null, object, array, number, and boolean selected values fail.
- Context cancellation stops the request.
- Response bodies are bounded.
- Returned plaintext appears only inside `SensitiveValue`.
- Default formatting and JSON of the result remain redacted.
- Tests inspect errors and prove a token sentinel and secret sentinel are absent.
- `go.mod` contains no HashiCorp Vault SDK.
- `go test ./cmd/worker` passes.

## Minimum Implementation Model

Minimum recommended model: `Codex 5.4-mini`, high reasoning.

The provider is isolated, but secure URL construction, bounded decoding, error sanitization, and sentinel tests require careful implementation.

## Notes

Use distinct sentinels:

```text
goet-openbao-token-004-do-not-log
goet-openbao-secret-004-do-not-log
```

Assert that neither sentinel appears in any returned error or formatted provider value.

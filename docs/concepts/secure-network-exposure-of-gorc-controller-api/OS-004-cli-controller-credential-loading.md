# OS-004: CLI Controller Credential Loading

Status: Proposed  
Minimum recommended model: GPT-5.4-mini  
Reference: EC-3 / operational slice / files(4)+tests+doc

## Objective

Allow the GORC CLI/client package to authenticate to a remote HTTPS controller
without accepting a raw token command-line argument.

## Current State

The CLI can receive `--controller-url` or controller configuration and uses the
existing controller client for workflow submission, status, logs, and shutdown
behavior.

The client does not yet have a protected controller credential source.

## Target State

Support these credential sources:

```text
--controller-token-file <path>
GOET_CONTROLLER_TOKEN_FILE
GOET_CONTROLLER_TOKEN
```

Recommended precedence:

```text
explicit token-file flag
environment token-file path
environment token value
no token
```

There must not be a `--controller-token <literal>` flag because process arguments
are commonly observable.

## Requirements

- Add a client credential loader that reads at most a bounded token size.
- Trim one trailing line ending; do not aggressively rewrite token bytes.
- Reject empty tokens.
- Reject files larger than the configured small credential limit.
- On Unix-like systems, warn or fail according to documented policy when the token
  file is group/world readable.
- On Windows, document that POSIX mode checks are unavailable and rely on ACLs.
- Keep the token in memory only.
- Never print the token.
- Migrate all controller-client requests to OS-003.
- Remote URLs must not trigger the local-controller auto-starter.
- Existing local auto-start behavior may remain only for loopback/local
  controller URLs.
- Preserve CLI exit-code and JSON-output contracts.
- Add clear safe errors for:
  - missing token when server returns `401`;
  - wrong role when server returns `403`;
  - TLS trust failure;
  - unreachable endpoint.

## Required Context

Read first:

- current `cmd/demo-client` files;
- current `internal/client` package files;
- local controller starter implementation;
- CLI tests;
- OS-003 shared client;
- root README CLI examples.

## Allowed Production Files

- current `internal/client/*.go`
- `internal/client/credential.go`
- current `cmd/demo-client/*.go`
- root `README.md` only for CLI usage updates
- this concept README only for tracker/status updates

Do not redesign submission payloads or controller API paths.

## Allowed Test Files

- current `internal/client/*_test.go`
- `internal/client/credential_test.go`
- current `cmd/demo-client/*_test.go`

## Out of Scope

- Worker authentication.
- Interactive login.
- OS keychain integration.
- Keystore selection.
- OAuth/OIDC.
- Token creation or rotation.
- Client certificate authentication.
- New workflow submission semantics.

## Acceptance Criteria

- A token-file flag authenticates all CLI controller requests.
- Environment-based token loading works.
- A raw token is never accepted as a command argument.
- Missing token and wrong-role errors are actionable but do not reveal server
  response secrets.
- Token sentinels are absent from CLI output, logs, and errors.
- HTTPS works with the normal platform trust store.
- Plain HTTP remote URLs are rejected by default.
- Loopback HTTP development remains supported.
- A remote unreachable controller does not cause the CLI to start a local
  controller process.
- Existing submission/status/log JSON output remains stable.

## Stop Conditions

Stop and append to `issues.md` if:

- the CLI currently has no reliable distinction between local auto-start and
  remote URL mode;
- a token would need to be serialized into controller JSON;
- a credential loader would be duplicated outside the client package;
- tests require real external HTTPS.

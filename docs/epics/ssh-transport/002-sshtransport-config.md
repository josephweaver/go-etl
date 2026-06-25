# 002 SSHTransport Config

Status: proposed

EC Mode:
EC-4 / file(1)+test+doc

Slice:
cmd/controller / SSHTransportConfig / Validate / rejects incomplete or unsafe SSH transport config

Objective:
Add the serializable SSH transport configuration shape and validation behavior needed before implementing SSH connections.

Allowed Production Files:
- cmd/controller/ssh_transport.go

Tests:
- cmd/controller/ssh_transport_test.go

Out Of Scope:
- SSH connection establishment
- Execute behavior
- CopyInto behavior
- Preflight checks
- controller config factory wiring
- in-process SSH server implementation beyond what exists from feature 001
- real HPCC connection
- Slurm or Singularity execution

Acceptance:
- Defines an `SSHTransportConfig` struct suitable for JSON/controller config ingestion.
- Validates required connection fields.
- Validates timeout fields without opening a network connection.
- Validates host-key policy values.
- Rejects ambiguous or unsafe auth configuration where practical.
- Adds focused table-driven tests for valid and invalid config cases.
- Does not add production SSH execution or copy behavior.

## Artifact

This feature should produce or modify:

```text
cmd/controller/ssh_transport.go
cmd/controller/ssh_transport_test.go
```

The production file should contain only the config type and validation logic unless a minimal `SSHTransport` type shell is required to keep ownership clear.

## Proposed Config Shape

Initial fields should be small and explicit:

```go
type SSHTransportConfig struct {
    Host            string `json:"host"`
    Port            int    `json:"port,omitempty"`
    User            string `json:"user"`
    IdentityFile    string `json:"identity_file,omitempty"`
    IdentityEnv     string `json:"identity_env,omitempty"`
    KnownHostsFile  string `json:"known_hosts_file,omitempty"`
    HostKeyPolicy   string `json:"host_key_policy,omitempty"`
    PinnedHostKey   string `json:"pinned_host_key,omitempty"`
    ConnectTimeout  string `json:"connect_timeout,omitempty"`
    CommandTimeout  string `json:"command_timeout,omitempty"`
    KeepAlive       bool   `json:"keep_alive,omitempty"`
}
```

Default behavior should be conservative:

- Empty `Port` means `22`.
- Empty `HostKeyPolicy` should not mean "ignore host keys" in production behavior.
- Empty timeouts should resolve to bounded defaults when runtime behavior is implemented.

## Host-Key Policy

Allowed values should be explicit:

```text
known_hosts
pinned
insecure_ignore
```

Validation expectations:

- `known_hosts` may use `KnownHostsFile` or the user's normal SSH known-hosts location in later runtime code.
- `pinned` requires `PinnedHostKey`.
- `insecure_ignore` is allowed only as an explicit value and should be documented as development-only.
- Unknown values are invalid.

## Auth Policy

Initial auth support should avoid secret storage inside goetl.

Validation expectations:

- `Host` is required.
- `User` is required.
- At least one noninteractive auth source is required.
- `IdentityFile` and `IdentityEnv` are acceptable initial auth sources.
- Password auth and SSH agent auth are deferred.

If both `IdentityFile` and `IdentityEnv` are set, validation should either reject the ambiguity or define a clear precedence. Prefer rejecting ambiguity for the first slice.

## Timeout Validation

Timeout fields should be parseable Go durations when present.

Examples:

```text
5s
30s
2m
```

Invalid examples:

```text
five seconds
0
-1s
```

## Test Cases

Add table-driven tests covering:

- valid minimal config with `identity_file` and `known_hosts`
- valid minimal config with `identity_env` and `pinned`
- missing host
- missing user
- missing auth source
- both identity sources set
- invalid host-key policy
- pinned policy without pinned key
- invalid connect timeout
- invalid command timeout
- invalid port, such as negative or greater than 65535

## Later Features Enabled

This feature enables later slices to build `SSHTransport` from validated settings without mixing config design into connection, execute, copy, retry, or preflight behavior.

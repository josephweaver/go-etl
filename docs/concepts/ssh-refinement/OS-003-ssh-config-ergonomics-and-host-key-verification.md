# OS-003: SSH Config Ergonomics and Host-Key Verification

Status: Complete
Scope: GOET SSH configuration and validation

## Purpose

Make rendered SSH configs practical and safe for real operators.

The current transport can use a pinned host key, but `known_hosts` is not
implemented and file paths such as `~/.ssh/id_rsa` are read literally. This
creates unnecessary friction for HPCC configs and makes users copy host keys by
hand.

## Requirements

- Expand `~` and environment variables in local-only SSH file paths before
  reading identity or known-host files.
- Implement `host_key_policy: "known_hosts"` using a configured
  `known_hosts_file`.
- Require `known_hosts_file` when `host_key_policy` is `known_hosts`, except a
  jump host may inherit the target transport's `known_hosts_file`.
- Preserve `host_key_policy: "pinned"` for reproducible rendered configs.
- Keep `host_key_policy: "insecure_ignore"` available only as explicit local
  bootstrap/debug behavior.
- Ensure path expansion never applies to remote paths.
- Do not log private key material or full secret environment values.

## Candidate Config Shape

```json
{
  "host": "dev-amd20",
  "user": "weave151",
  "identity_file": "~/.ssh/id_weave151_rsa",
  "host_key_policy": "known_hosts",
  "known_hosts_file": "~/.ssh/known_hosts"
}
```

## Implementation Notes

- Use a small local path expansion helper for fields that are explicitly local:
  `identity_file` and `known_hosts_file`.
- Support `$VAR`, `${VAR}`, `~`, and `~/...` for those local paths. Do not
  support `~otheruser`.
- Do not expand `root`, `data_dir`, Slurm script paths, worker paths, or any
  other remote filesystem path.
- Prefer `golang.org/x/crypto/ssh/knownhosts` for known-host checking.
- For jump-host chains, apply host-key policy independently per host.

## Validation

- Unit tests for:
  - `~` expansion;
  - environment variable expansion if supported;
  - identity file read from expanded path;
  - known-host success;
  - known-host mismatch failure;
  - hashed known-host entries if supported by the library;
  - remote paths remain unexpanded.

## Stop Conditions

- Any implementation logs or persists private key contents.
- Path expansion changes remote runtime paths.
- `known_hosts` silently falls back to insecure behavior.

## Completion Criteria

- Rendered local configs can use normal `~/.ssh/...` paths.
- Users can choose `known_hosts` instead of copying a pinned key into a rendered
  config.
- Host-key verification failures are clear and actionable.

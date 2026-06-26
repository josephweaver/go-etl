# 014 Client SSH Setup Wizard

Status: proposed

Slice:
client setup / execution environment wizard / creates SSH-backed controller config without making SSHTransport interactive

Objective:
Add a client-side setup flow that helps users create an execution-environment config for local fake-HPCC or SSH-backed environments.

This is not an SSHTransport feature. `SSHTransport` should remain a non-interactive runtime component that consumes validated config. The setup wizard belongs in the client or CLI layer because it asks questions, creates local files, and may help the user establish trust.

Allowed Production Files:
- internal client setup package or CLI setup command files, to be decided by the implementation slice

Allowed Configuration Files:
- generated config templates or examples, if needed

Tests:
- focused prompt/answer tests using fake prompt and filesystem adapters
- no real SSH server required for the first slice

Documentation:
- docs/fake-hpcc.md
- setup runbook or CLI help text, if needed

Out Of Scope:
- changing SSHTransport runtime behavior
- controller-side interactive prompts
- silently trusting host keys
- silently writing to the user's real `~/.ssh`
- real HPCC credential management
- password authentication
- SSH agent integration

Acceptance:
- Defines a client-side setup flow for execution-environment config.
- Prompts for transport type: local, SSH, or Docker.
- For SSH, prompts for host, port, user, and identity source.
- Can generate a project-local SSH key pair when the user explicitly asks for it.
- Can capture a host key fingerprint and ask the user whether to pin it.
- Writes a controller/execution-environment config using `transport.type = "ssh"`.
- Keeps generated private keys and trust material outside source control.
- Does not mutate host trust or credentials without explicit user confirmation.
- Leaves `SSHTransport` and the controller non-interactive.

## Suggested Command Shape

The first CLI shape can be decided later, but the target interaction is:

```text
go-etl-cli configure execution-environment
```

or:

```text
go-etl-cli setup ssh
```

The setup command should ask short questions such as:

```text
Transport: (l)ocal, (s)sh, or (d)ocker
SSH host:
SSH port:
SSH user:
Use existing key or create a new local project key?
Private key path:
Capture host key now?
Pin this host key in generated config?
Output config path:
```

Avoid vague prompts such as:

```text
Do you trust this host?
```

Prefer concrete trust prompts:

```text
Host key fingerprint:
SHA256:...

Pin this host key in the generated config? y/n
```

## Suggested Internal Shape

Go does not use classes, so the implementation should likely use small structs and interfaces:

```go
type SSHSetup struct {
    Prompter Prompter
    Runner   CommandRunner
    Files    FileStore
}

func (s SSHSetup) Run(ctx context.Context) (SSHTransportConfig, error)
```

The concrete CLI command can wire real stdin/stdout, shell commands, and filesystem access. Tests can use fake prompt answers and fake filesystem/command adapters.

## Generated Files

Generated setup state should default to a project-local ignored directory, for example:

```text
.run/goetl/ssh/id_ed25519
.run/goetl/ssh/id_ed25519.pub
.run/goetl/ssh/known_hosts
.run/goetl/generated/fake-hpcc-ssh-config.json
```

The wizard should not write to the user's real `~/.ssh` by default. If a later feature supports that, it must be explicit and documented.

## Fake-HPCC Path

For fake-HPCC, the wizard may eventually offer an explicit setup path:

- generate a project-local client key
- install the public key into the fake login container or mounted fake login home
- capture the fake login host key
- write a generated fake-HPCC SSH controller config under `.run/`
- verify `ssh goetl@127.0.0.1 -p <port> true`

This fake-HPCC automation should remain local-only and must not be reused as real HPCC credential management.

## Runtime Boundary

The runtime boundary stays:

```text
setup wizard
  asks questions
  generates keys when requested
  captures/pins host keys when confirmed
  writes config

SSHTransport
  loads validated config
  connects/copies/executes
  does not ask questions
  does not mutate trust

controller
  loads config
  builds execution environment
  does not prompt
```

## Later Features Enabled

This feature enables:

- smoother fake-HPCC onboarding
- guided SSH config generation for non-Go users
- safer host-key pinning workflows
- future real-cluster setup guidance without mixing interactivity into runtime transport code

# 013 Controller Config Fake-HPCC Via SSH

Status: implemented

Slice:
cmd/controller config / fake-HPCC SSH config / selects SSHTransport instead of DockerTransport

Objective:
Create a controller configuration file that targets the fake-HPCC backend through `SSHTransport` rather than `DockerTransport`.

Allowed Production Files:
- none by default

Allowed Configuration Files:
- a new controller config JSON fixture for fake-HPCC over SSH

Tests:
- focused config loading or environment construction test, if the implementation slice changes tracked config files
- opt-in integration verification if it depends on the fake SSH server from feature 012

Documentation:
- docs/fake-hpcc.md

Out Of Scope:
- real HPCC credentials or hostnames
- automatic SSH key generation
- automatic host-key trust mutation
- changing scheduler behavior
- changing runtime behavior
- full worker execution through Slurm unless already supported by the fake-HPCC SSH environment

Acceptance:
- Adds a fake-HPCC controller config that uses `execution_environment.transports[].type = "ssh"`.
- Keeps scheduler selection independent from transport selection.
- Keeps runtime selection independent from transport selection.
- Uses generic fake values only.
- Does not commit private keys or real host keys.
- Documents the local values a developer must provide, such as SSH host, port, user, identity file, and pinned host key.
- Can be loaded by the existing controller config loader.
- Can construct an `ExecutionEnvironment` with an `*SSHTransport`.

## Intended Artifact

The implementation slice should add a new config fixture with a name similar to:

```text
cmd/controller/fake-hpcc-ssh-config.json
```

The exact filename may change if the controller config naming pattern changes first.

## Config Shape

The config should use the current execution-environment model:

```json
{
  "execution_environment": {
    "name": "fake-hpcc-ssh",
    "transports": [
      {
        "name": "login",
        "type": "ssh",
        "settings": {
          "host": "127.0.0.1",
          "port": "2222",
          "user": "goetl",
          "identity_file": ".run/fake-hpcc-ssh/id_ed25519",
          "host_key_policy": "pinned",
          "pinned_host_key": "ssh-ed25519 AAAA..."
        }
      }
    ],
    "dialect": {
      "type": "bash"
    },
    "scheduler": {
      "type": "slurm"
    },
    "runtime": {
      "type": "worker",
      "settings": {
        "root": "/data/goetl",
        "controller_url": "http://host.docker.internal:8080"
      }
    }
  }
}
```

The real fixture must also include whatever controller variables are required by the current controller config loader, such as controller URL and ledger path.

## Ownership Rules

The SSH transport settings own:

- host
- port
- user
- identity file or identity environment variable
- host-key policy
- pinned host key or known-hosts settings
- connection and command timeout values

The scheduler settings own:

- Slurm script path
- job name
- `sbatch`-related settings

The runtime settings own:

- worker runtime root
- worker config path through derived runtime paths
- worker artifact path
- controller URL visible to workers
- Singularity settings if `singularity_worker` is selected

## Verification

Minimum non-network verification:

- load the config file
- validate the controller config
- build the execution environment
- assert the first transport is `*SSHTransport`
- assert scheduler and runtime selections are unchanged

Opt-in fake-HPCC verification after feature 012:

```text
start fake-HPCC SSH server
load fake-hpcc SSH controller config
connect SSH transport
prepare runtime
submit or dry-run scheduler command
cleanup remote runtime root when practical
```

## Later Features Enabled

This feature enables:

- local fake-HPCC demos through SSH transport
- Slurm submission over SSH using the existing scheduler boundary
- documentation and tests that more closely resemble real HPCC deployment without committing real deployment data

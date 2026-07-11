# Model and Implementation Plan

## Objective

Implement worker direct one-shot execution at the lowest practical model cost without losing backward compatibility or creating a second worker execution path.

## Recommended order

```text
OS-001 -> review -> OS-002 -> review -> OS-003 -> end-of-concept review
```

Do not implement the slices concurrently. `cmd/worker` files overlap, and the source-provider design should build on the accepted direct command rather than competing with it.

## Model selection

### OS-001: Direct Worker Command

Minimum recommended:

```text
GPT-5.4-mini, normal-to-high reasoning
```

Why:

- touches the process entry point;
- must preserve positional production invocation;
- splits config validation without weakening controller security;
- introduces an exit/result contract;
- must remove stale results and pass every `Worker.Run` type through without a
  second allow-list.

Escalate to GPT-5.5 Thinking only if the current branch has materially changed CLI/config startup since the reviewed snapshot or if production launch tests reveal hidden command construction assumptions.

### OS-002: Source-Bundle Provider Boundary

Minimum recommended:

```text
GPT-5.4-mini, normal reasoning
```

Why:

- narrow interface extraction;
- concrete production and file adapters;
- most safety behavior remains in existing ZIP staging code.

A smaller model is not preferred for the first implementation because an apparently simple refactor can accidentally bypass source ZIP validation or leave production Python work unwired.

### OS-003: Direct Python Target Smoke

Minimum recommended:

```text
GPT-5.4-mini, normal reasoning
```

After the first passing integration test, a cheaper smoke/documentation model such as GPT-5.3-spark may add another fixture or polish the runbook.

## HCI recommendation

Use the repository default unless explicitly changed:

```text
EC-3 / Operational Slice / file(4)+test+doc+cleanup+newfile
```

The change budget is literal and per prompt. Do not reinterpret `file(1)` to
cover several production files. A smaller budget may be selected, but an
Operational Slice may then require multiple `next` prompts. New production and
test files require `+newfile`, and direct call-site consistency may require
`+cleanup`. Keep every prompt within both the selected HCI budget and the active
OS allowed-file boundary.

## Implementation boundaries

### OS-001 primary concept

```text
cmd/worker/direct.go
```

Supporting changes:

```text
main.go
config.go
tests
docs
```

### OS-002 primary concept

```text
cmd/worker/source_bundle_provider.go
```

Supporting changes:

```text
source_bundle.go
worker/main wiring
tests
docs
```

### OS-003 primary artifact

```text
cmd/worker/direct_integration_test.go
```

Supporting changes:

```text
testdata
docs/state
```

## Review checkpoints

After OS-001 verify:

- production positional config still works;
- direct config can omit controller URL;
- direct execution has no separate work-item-type allow-list;
- cache and commit items reach their normal `Worker.Run` behavior;
- a configured sentinel controller observes zero requests;
- stale results are removed before new input preparation;
- result evidence uses explicit snake_case JSON fields.

After OS-002 verify:

- source staging receives bytes from the provider only;
- production explicitly wires the controller provider;
- local provider does not extract or reinterpret source metadata;
- missing source bookkeeping receives the approved direct defaults;
- Python retains local logs without controller observation delivery;
- all ZIP traversal/symlink/collision tests remain green.

After OS-003 verify:

- Python success and failure are both represented locally;
- a sentinel server observes zero total requests, including source retrieval,
  log observations, terminal reports, and future heartbeat behavior;
- the runbook does not imply that direct mode allocates Slurm resources;
- the runbook labels direct mode development-only and unsuitable for production
  credentials;
- project state distinguishes worker-runtime testing from orchestration-chain testing.

## End-of-concept review question

The concept succeeds if the developer can answer these independently:

```text
Does the resolved work execute in this environment?
```

using direct mode, and:

```text
Does GORC compile, queue, schedule, claim, execute, and record the work correctly?
```

using the full orchestration chain.

Direct mode should shorten the first feedback loop without weakening the second test.

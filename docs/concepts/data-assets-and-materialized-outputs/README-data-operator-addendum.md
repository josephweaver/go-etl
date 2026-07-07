# Data Asset Operator Addendum

Status: Proposed addendum  
Cadence: CSxIx  
Parent Strategic Concept: `docs/concepts/data-assets-and-materialized-outputs/README.md`

## Purpose

Add first-class data movement operators to the Data Assets and Materialized Outputs Strategic Concept so common input assets and published outputs can be scheduled, deduplicated, rate-limited, retried, and observed like ordinary GOET work.

This addendum intentionally preserves the existing data-provider, archive, cache, artifact, and published-asset vocabulary. It changes the execution shape from implicit worker-side data movement around every compute work item to explicit data movement work items when a workflow benefits from shared cache reuse, outbound publication, or resource-controlled network transfer.

## Revised strategic decision

Data assets remain logical declarations. Cached asset instances and published outputs are physical target-environment facts.

Data movement should be represented by explicit work-item operators when it crosses a shared boundary:

```text
cache_data:
  inbound operation
  external source / named location -> target-local cache or verified reference

compute:
  local operation
  target-local cached inputs -> attempt-local artifacts

commit_data:
  outbound operation
  promoted artifacts -> declared durable store / named publish location
```

Worker-internal materialization may remain as a compatibility path for tiny local fixtures, but the target state for shared CDL/Yan/Roy-style runs is:

```text
cache_data work items are generated and deduplicated before compute fan-out.
compute work items consume completed materialized-data manifests.
commit_data work items publish selected promoted outputs after compute succeeds.
```

## Why this addendum is needed

The current SC already models provider templates, cache immutability, archive extraction, materialized input assets, artifacts, and published data assets. The missing scheduling boundary is that data movement is still mostly described as a worker phase around plugin execution.

That creates two large-run risks:

1. Multiple compute jobs may independently materialize the same input asset.
2. A large fan-out may create a remote-source request storm against HTTP, Google Drive/rclone, or a shared filesystem.

Representing `cache_data` and `commit_data` as ordinary work-item operators lets the existing dependency and resource-constraint machinery solve both problems.

## Operator summary

### `cache_data`

`cache_data` makes one resolved bound data asset available inside one target environment. It may download, copy, verify, extract, or reference, depending on provider and materialization strategy.

It emits a compact materialized-data manifest that downstream compute work items consume.

### `commit_data`

`commit_data` publishes one selected promoted artifact or artifact directory to a declared durable store location.

It emits compact published-asset evidence and must not register a global catalog entry unless a later concept explicitly adds a data catalog.

## Required invariant

```text
Compute operators must not implicitly perform remote acquisition or durable publication.

They consume completed materialized-data manifests and produce attempt-local artifacts.
Inbound acquisition is represented by cache_data.
Outbound publication is represented by commit_data.
```

## Added operational slices

Add these after the current `013-concept-closure-and-documentation-sync.md`, or insert them before closure if the original concept has not yet been implemented.

| Slice | File | Purpose |
|---:|---|---|
| 014 | `014-data-operator-model-and-sc-decision-update.md` | Define `cache_data` and `commit_data` operators and update SC vocabulary/target state. |
| 015 | `015-cache-data-deduplicated-materialization-work-items.md` | Compile data bindings into deterministic, deduplicated `cache_data` work items and downstream compute dependencies. |
| 016 | `016-cache-data-provider-resource-admission-and-transfer-limits.md` | Attach provider/source/cache resource constraints and per-transfer throttles to `cache_data`. |
| 017 | `017-commit-data-published-output-work-items.md` | Compile publish bindings into `commit_data` work items with overwrite policy, target evidence, and upload/write constraints. |
| 018 | `018-data-operator-integration-smoke-and-documentation-sync.md` | Add fixture/fake-HPCC smoke coverage and sync docs/project state. |

## Implementation posture

Keep the implementation fixture-sized:

```text
No real CDL downloads.
No real Google Drive access.
No real Yan/Roy 7z archive.
No real HPCC dependency.
No credentials in tests.
```

Use:

```text
httptest or equivalent local HTTP fixtures
fake rclone executable
tiny ZIP fixtures
temporary filesystem roots
fake HPCC / Singularity smoke only after local path works
```

## Deferred work

Do not implement:

```text
token-bucket request-per-minute limits
global fair-share scheduling
data catalog registration
credential propagation
object-store native clients
destructive overwrite defaults
cache garbage collection beyond safe replacement/protection
```

The existing integer resource-constraint model can bound concurrent transfers and declared transfer units. True time-window rate limiting remains a later resource-admission extension.

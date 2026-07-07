# 015 Cache Data Deduplicated Materialization Work Items

Status: Proposed

## Objective

Compile resolved data bindings into deterministic, deduplicated `cache_data` work items.

If many compute work items require the same logical input asset in the same target environment, GOET should produce one inbound materialization work item and make all consumers depend on its completed materialized-data manifest.

## Current State

The Data Assets concept already defines:

```text
data provider templates
step data bindings
materialized input asset manifests
cache immutability
integrity checks
archive selection
GOET_DATA_ASSETS_JSON
```

The missing behavior is controller/planner-owned deduplication of the materialization work itself.

## Target State

For each resolved bound input asset, the planner derives a canonical asset identity:

```text
asset_key = sha256(canonical_json({
  provider_type,
  resolved_source_location,
  resolved_parameters,
  cache_strategy,
  cache_key,
  immutable,
  integrity_expectations,
  archive_selection,
  expose_mode,
  target_environment_id
}))
```

The planner then creates or reuses one `cache_data` work item:

```text
operator: cache_data
dedupe_key: cache_data:<target_environment_id>:<asset_key>
```

All compute work items that require that asset depend on the same completed `cache_data` work item.

## Work Item Payload Shape

The exact storage model may differ, but `cache_data` work item payloads must contain equivalent facts:

```json
{
  "operator": "cache_data",
  "target_environment_id": "target-local",
  "asset_key": "sha256:...",
  "binding_name": "cropland_year",
  "provider_name": "cdl_zip",
  "provider_type": "http",
  "kind": "raster_archive",
  "format": "geotiff_zip",
  "resolved_location": {
    "type": "http",
    "uri": "https://example.invalid/2023_30m_cdls.zip"
  },
  "cache": {
    "strategy": "worker_cache",
    "cache_key": "cdl/2023/30m/source.zip",
    "immutable": true
  },
  "integrity": {
    "sha256": "optional-lowercase-hex",
    "size_bytes": 123456789,
    "required": false
  },
  "archive": {
    "type": "zip",
    "select": [
      {
        "member": "2023_30m_cdls.tif",
        "as": "cdl.tif",
        "required": true
      }
    ],
    "expose": "selected_path"
  },
  "parameters": {
    "year": 2023
  }
}
```

A `registered_location` reference may also compile to `cache_data`, but its execution may be a verification-only operation rather than a copy/download.

## Completed Output Shape

A completed `cache_data` work item reports a materialized-data manifest equivalent to:

```json
{
  "schema": "goet/materialized-data-assets/v1",
  "asset_key": "sha256:...",
  "target_environment_id": "target-local",
  "assets": [
    {
      "binding_name": "cropland_year",
      "provider_name": "cdl_zip",
      "provider_type": "http",
      "kind": "raster_archive",
      "format": "geotiff_zip",
      "local_path": "/data/goetl/cache/assets/sha256.../selected/cdl.tif",
      "materialization_strategy": "worker_cache",
      "cache_key": "cdl/2023/30m/source.zip",
      "cache_immutable": true,
      "source_size_bytes": 123456789,
      "source_sha256": "...",
      "selected_size_bytes": 123000000,
      "selected_sha256": "..."
    }
  ]
}
```

Downstream compute assignments should receive this manifest or a controller-compiled projection of it. The controller must not assume the `local_path` is meaningful on the user's local machine.

## Deduplication Rules

- Two data bindings deduplicate only if their canonical materialization identity is equal.
- Provider name alone is insufficient.
- Source URI/path, target environment, cache key, archive selection, integrity expectations, and expose mode must participate in the key.
- Binding alias should not normally affect physical deduplication. Two aliases may consume the same underlying asset.
- The completed manifest may be projected under the consumer's binding alias when needed.
- If one asset is already ready in the target cache and its immutable evidence matches, `cache_data` may complete without re-downloading.
- If immutable cache evidence conflicts, `cache_data` fails before any compute consumer runs.

## Dependency Rules

The planner should transform:

```text
compute(field_cdl_composition)
  needs cdl_zip(year=2023)
  needs yanroy_release(year=2023,tile=h16v06)
```

into:

```text
cache_data(cdl_zip, year=2023)
cache_data(yanroy_release, year=2023,tile=h16v06)

compute(field_cdl_composition)
  depends_on cache_data(cdl_zip, year=2023)
  depends_on cache_data(yanroy_release, year=2023,tile=h16v06)
```

If 1,000 compute items use the same CDL year archive, they all depend on the same `cache_data(cdl_zip, year)` work item.

## Worker Execution Rules

A `cache_data` worker operation must:

```text
1. validate resolved provider payload
2. determine final cache path from worker/target config and safe cache key
3. check existing cache evidence
4. acquire into attempt-local or cache-temp staging
5. stream/copy/hash source bytes where applicable
6. extract selected archive members where applicable
7. verify expected and observed evidence
8. atomically promote to ready cache path when possible
9. emit compact manifest
```

It must not write directly into a ready cache path before verification.

## Required Context

Read these files first:

```text
docs/concepts/data-assets-and-materialized-outputs/README.md
docs/concepts/complete/dependency-aware-workflow-execution/README.md
docs/concepts/complete/workflow-execution-persistence/README.md
docs/concepts/complete/resource-constrained-work-admission/README.md
internal/model/work_item.go
internal/persistence/store.go
internal/persistence/db_adapter_sqlite.go
```

Also read the current workflow compiler/planner files that create work items and dependencies.

## Allowed Production Files

Use exact current paths if names differ.

```text
internal/model/work_item.go
internal/model/data_asset*.go
internal/model/artifact*.go
internal/workflow/*compile*.go
internal/workflow/*plan*.go
internal/persistence/store.go
internal/persistence/db_adapter_sqlite.go
internal/persistence/*workflow*.go
internal/worker/*data*.go
internal/worker/*assignment*.go
```

Prefer new focused files over large mixed edits.

## Allowed Test Files

```text
internal/model/*data*_test.go
internal/workflow/*data*_test.go
internal/persistence/*data*_test.go
internal/worker/*data*_test.go
```

## Out Of Scope

```text
rclone implementation
real HTTP downloads
real ZIP/7z archives larger than tiny fixtures
commit_data implementation
provider resource-admission defaults
transfer bandwidth throttling
data catalog registration
cache eviction
credential propagation
```

Use local fixture providers or fakes as needed.

## Acceptance Criteria

- A resolved bound data asset can compile to a `cache_data` work item.
- Multiple compute items requiring the same canonical asset reuse the same `cache_data` work item.
- Compute items depend on their required `cache_data` work items.
- `cache_data` work item payloads include enough resolved facts for worker execution without re-running user expressions at claim time.
- Existing immutable cache evidence can be reused when it matches.
- Conflicting immutable evidence fails materialization before compute runs.
- Completed `cache_data` output uses a compact materialized-data manifest.
- Unit tests prove deduplication for at least:
  - same provider/parameters used by two compute jobs;
  - same physical asset under two binding aliases;
  - different archive selection does not deduplicate;
  - different target environment does not deduplicate.

## Notes

Do not implement a mutable global data catalog in this slice. The deduplication scope is work planning plus target-environment cache identity.

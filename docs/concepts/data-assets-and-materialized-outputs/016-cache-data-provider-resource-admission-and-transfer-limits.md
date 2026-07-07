# 016 Cache Data Provider Resource Admission And Transfer Limits

Status: Proposed

## Objective

Attach resource constraints and per-transfer limits to `cache_data` work items so inbound data movement is scheduled and bounded.

This prevents a large fan-out from creating a download storm against HTTP, Google Drive/rclone, local shared filesystems, or cache disks.

## Current State

The completed Resource-Constrained Work Admission concept supports controller-owned claim-time admission using persisted integer facts:

```text
resource_key
requested_units
operator
target_units
```

It explicitly does not implement time-window rate limits such as requests per minute.

The Data Assets concept needs to use the existing resource model for practical concurrency limits and add provider-local transfer limits for bandwidth caps.

## Target State

Every `cache_data` work item may carry resolved resource constraints derived from:

```text
provider type
provider source / remote
target environment
cache root
archive extractor
declared transfer policy
```

Example constraints:

```json
{
  "resource_constraints": [
    {
      "resource_key": "provider:http:nass.usda.gov/download",
      "requested_units": 1,
      "operator": "<=",
      "target_units": 2
    },
    {
      "resource_key": "target:hpcc/asset-cache-write",
      "requested_units": 1,
      "operator": "<=",
      "target_units": 1
    }
  ]
}
```

For Google Drive / rclone:

```json
{
  "resource_constraints": [
    {
      "resource_key": "provider:gdrive-rclone:landcore/download",
      "requested_units": 1,
      "operator": "<=",
      "target_units": 1
    }
  ],
  "transfer_limits": {
    "max_bytes_per_second": 26214400
  }
}
```

The controller controls how many transfer work items can run. The worker/provider adapter controls how aggressive one transfer process may be.

## Transfer Policy Model

Add a small resolved transfer policy to `cache_data` payloads:

```json
{
  "transfer_policy": {
    "max_concurrent_source_transfers": 1,
    "requested_bandwidth_mib_per_second": 25,
    "max_bytes_per_second": 26214400,
    "provider_args": {
      "rclone_bwlimit": "25M"
    }
  }
}
```

Implementation may store only the fields it can enforce.

### Required distinction

Resource constraints are admission controls:

```text
How many cache_data items may run at once for this resource?
```

Transfer limits are worker/provider controls:

```text
How fast may this one transfer run?
```

Do not pretend the current integer resource model implements true request-per-minute limits.

## Resource Key Naming

Use explicit scope:

```text
provider:http:<host>/download
provider:gdrive-rclone:<remote>/download
provider:local-file:<root-name>/read
target:<target_id>/asset-cache-write
target:<target_id>/archive-extract
target:<target_id>/asset-cache-mibps
```

Examples:

```text
provider:http:www.nass.usda.gov/download
provider:gdrive-rclone:landcore/download
target:fake-hpcc/asset-cache-write
target:fake-hpcc/archive-extract
```

When a source identity contains private details, use a sanitized configured resource alias rather than committing private names.

## Provider Behavior

### HTTP

The HTTP provider should:

```text
stream bytes
hash while streaming
respect max size
optionally throttle read/write loop to max_bytes_per_second
write only to staging path before verification
```

Tests must use local HTTP fixtures, not real NASS URLs.

### gdrive_rclone

The rclone provider should:

```text
invoke configured executable with structured args
pass --bwlimit when transfer_policy requests it
never construct shell strings
never log credentials
use configured rclone environment/config from worker runtime
```

Tests must use a fake rclone executable that records arguments and copies a tiny local fixture.

### local_file / registered_location

Local providers may still need constraints:

```text
target:<target_id>/asset-cache-write
provider:local-file:<root>/read
```

because a shared filesystem can be overwhelmed even without internet downloads.

## Required Context

Read these files first:

```text
docs/concepts/data-assets-and-materialized-outputs/README.md
docs/concepts/complete/resource-constrained-work-admission/README.md
internal/model/resource_constraint.go
internal/model/work_item.go
internal/workflow/*compile*.go
internal/worker/*data*.go
internal/worker/*provider*.go
```

Also read current rclone/archive/provider slice docs from this concept if present.

## Allowed Production Files

```text
internal/model/data_asset*.go
internal/model/resource_constraint.go
internal/workflow/*compile*.go
internal/worker/*data*.go
internal/worker/*provider*.go
internal/worker/*http*.go
internal/worker/*rclone*.go
internal/config/*worker*.go
```

If the worker config currently lacks executable or transfer-limit fields, add focused config fields with safe defaults.

## Allowed Test Files

```text
internal/workflow/*data*_test.go
internal/worker/*data*_test.go
internal/worker/*provider*_test.go
internal/worker/*http*_test.go
internal/worker/*rclone*_test.go
```

## Out Of Scope

```text
true token-bucket request-per-minute scheduler
distributed bandwidth accounting across multiple controllers
real Google Drive access
real CDL downloads
credential propagation
OS-level cgroups/network shaping
data catalog registration
commit_data upload constraints
```

## Acceptance Criteria

- `cache_data` work items can carry resolved resource constraints.
- Provider/source-specific resource keys are deterministic and sanitized.
- The workflow/planner can apply default constraints for a provider/source.
- A fan-out of many work items using one source can be configured so only N `cache_data` work items for that source are claimable at once.
- HTTP fixture transfers stream through staging and can respect a configured per-transfer byte/sec cap in tests or through injectable clock/sleeper behavior.
- rclone fixture transfers pass a configured bandwidth flag to the fake executable when requested.
- Existing resource-admission tests still pass.
- Unit tests prove at least:
  - source mutex: target_units=1 admits only one matching cache_data item;
  - source capacity: target_units=2 admits two but not three;
  - independent sources do not block each other;
  - compute work with no matching data-transfer resource remains schedulable under existing rules.

## Notes

Sequential `cache_data` work items prevent request storms. Per-transfer throttles prevent one admitted transfer from consuming all available bandwidth.

If bandwidth units are modeled as resource constraints, they must also be enforced by the provider. Admission without provider throttling is only a scheduling hint.

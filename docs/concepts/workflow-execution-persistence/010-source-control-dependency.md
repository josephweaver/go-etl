# 010 Source-Control Dependency

Status: moved

## Decision

Source-control abstraction, GitHub resolution, local cache layout,
materialization, and cache pin behavior are split out of this epic into:

```text
docs/concepts/source-control-resolution-and-cache/README.md
```

## Reason

`workflow-execution-persistence` needs durable source-control references, but
source-control behavior is not only persistence. It has its own lifecycle:
mutable-ref resolution, exact commit retrieval, path safety, GitHub behavior,
credentials, local cache layout, cache pins, materialization, offline restart,
and cleanup coordination.

Keeping that work in this epic would expand the persistence epic beyond its
database lifecycle purpose. The persistence epic should store and query source
locator facts; the source-control epic should create, verify, cache, and
materialize those source identities.

## Persistence-Owned Contract

This epic continues to own database fields and methods that store source
identity facts:

- repository identity;
- resolved commit ID;
- repository-relative path;
- source object ID when available;
- canonical GOET SHA-256;
- schema/version metadata needed to reload the document.

It does not own GitHub API calls, local bare Git cache behavior, cache cleanup,
or file materialization.

## Next Persistence Slice

The next workflow-execution-persistence slice should remain database-owned.
Likely candidates include:

- restart reconstruction queries;
- list terminal attempts;
- running-attempt lookup;
- retry/requeue after failed attempt;
- stage failure transition policy.

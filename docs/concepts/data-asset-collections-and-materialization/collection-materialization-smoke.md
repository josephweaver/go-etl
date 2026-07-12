# Collection Materialization Smoke

Date: 2026-07-12

This file records the fixture-sized smoke evidence for
`asset.materialize` collection output and downstream member hydration.

## Controller Smoke

Test:

```text
cmd/controller/collection_materialization_smoke_test.go
```

Fixture domain:

```text
2008
2009
2010
```

The test verifies:

- collection aggregation emits one compact `goet/materialized-asset-collection/v1`
  object, not a JSON list of member outputs;
- the compact object preserves ordered `year` values and path template
  `/mnt/cache/assets/cdl/${year}.tif`;
- `workflow.step[0].dimensions.year.values[1]` resolves as typed integer `2009`;
- completed collection members survive through persisted controller records;
- downstream hydration selects the matching member for each year; and
- the worker-facing data projection exposes `data.cdl.path[0]` as the concrete
  path for that member.

Command:

```text
go test ./cmd/controller
```

## Worker Filesystem Evidence

Destination promotion, pinned evidence reuse, and conflict-safe failure are
covered by the focused worker tests in:

```text
cmd/worker/work_asset_materialize_test.go
```

Those tests verify deterministic destination writes, pinned destination reuse
without provider acquisition, missing destination evidence failure, byte
mismatch failure, materialization-key mismatch failure, and ZIP selected-member
promotion.

## Boundaries

This smoke uses synthetic controller and worker fixtures only. It does not prove
real CDL ingestion, real GeoTIFF validity, GDAL processing, HPCC execution, or
external network behavior.

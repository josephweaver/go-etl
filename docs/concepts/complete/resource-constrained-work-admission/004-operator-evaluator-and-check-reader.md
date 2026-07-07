# 004 Operator Evaluator And Check Reader

Status: implemented

## Objective

Add the Go-side resource predicate evaluator for all six operators and a persistence reader for candidate-check rows from the SQL view.

This slice should not yet replace the claim algorithm.

## Current State

The database can expose candidate-check rows through `queued_resource_constraint_checks`, but no Go code evaluates whether those rows pass.

## Target State

The model layer has an operator evaluator equivalent to:

```go
func ResourceConstraintAllows(totalUnits int64, requestedUnits int64, operator ResourceOperator, targetUnits int64) (bool, error) {
    candidateTotal, err := checkedAdd(totalUnits, requestedUnits)
    if err != nil {
        return false, err
    }

    switch operator {
    case ResourceOperatorEqual:
        return candidateTotal == targetUnits, nil
    case ResourceOperatorNotEqual:
        return candidateTotal != targetUnits, nil
    case ResourceOperatorLessThan:
        return candidateTotal < targetUnits, nil
    case ResourceOperatorGreaterThan:
        return candidateTotal > targetUnits, nil
    case ResourceOperatorLessThanOrEqual:
        return candidateTotal <= targetUnits, nil
    case ResourceOperatorGreaterThanOrEqual:
        return candidateTotal >= targetUnits, nil
    default:
        return false, fmt.Errorf("unsupported resource operator %q", operator)
    }
}
```

The store layer has a reader that returns rows equivalent to:

```text
work_item_id
queued_at
constraint_index
resource_key
total_units
requested_units
operator
target_units
```

The evaluator must treat all constraints for one candidate as an AND:

```text
candidate is eligible iff every constraint row passes
```

A candidate with zero constraint rows is eligible.

## Concept Decision

Operator evaluation belongs in Go code. SQL/view code only supplies data.

Overflow must be handled explicitly. If `total_units + requested_units` overflows, the candidate must not be claimed and the scheduler should return a clear error or treat the row as failed according to existing store error conventions.

## Required Context

Read these files first:

- `internal/model/resource_constraint.go`
- `internal/persistence/store.go`
- `internal/persistence/db_adapter_sqlite.go`
- `internal/persistence/*_test.go`
- `docs/concepts/resource-constrained-work-admission/README.md`

## Allowed Production Files

- `internal/model/resource_constraint.go`
- `internal/persistence/store.go`
- focused helper files under `internal/persistence/`

## Allowed Test Files

- `internal/model/*_test.go`
- `internal/persistence/*_test.go`

## Out Of Scope

- Replacing the claim algorithm.
- HTTP handler changes.
- Status/log changes.
- Workflow declaration parsing.

## Acceptance Criteria

- Unit tests cover `=` true and false cases.
- Unit tests cover `!=` true and false cases.
- Unit tests cover `<` true and false cases.
- Unit tests cover `>` true and false cases.
- Unit tests cover `<=` true and false cases, including equality.
- Unit tests cover `>=` true and false cases, including equality.
- Unit tests cover overflow in `total_units + requested_units`.
- Store tests can read candidate-check rows from the view.
- Store tests prove `total_units` reflects currently running work with the same `resource_key`.
- Store tests prove unrelated resource keys do not affect `total_units`.
- A candidate with no resource constraints is treated as eligible by the Go helper.

## Notes

- Keep the evaluator independent from SQL so it can be tested with a small table-driven test.
- Prefer `int64` in Go even if SQLite stores integer values, to make overflow handling explicit.
- The store reader should preserve deterministic ordering by queued time, work item ID, and constraint index.

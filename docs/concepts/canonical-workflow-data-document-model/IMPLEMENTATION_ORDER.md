# Implementation Order and Dependency Graph

## Required order

```text
001 -> 002 -> 003 -> 004 -> 005
                       |
                       v
006 -> 007 -> 008 -> 009 -> 010 -> 011 -> 012 -> 013

014 -> 015 -> 016 -> 017 -> 018 -> 019 -> 020
```

The expression-function chain may begin after OS-003 establishes canonical directive loading. OS-014 should not merge before the loader contract is stable.

## Recommended batches

### Batch A: document boundary

- OS-001
- OS-002
- OS-003
- OS-004
- OS-005

Review the normalized public workflow shape before starting data runtime migration.

### Batch B: data semantics

- OS-006
- OS-007
- OS-008

Do not start explicit operator migration until asset identity and materialization-scope validation are stable.

### Batch C: explicit operators

- OS-009
- OS-010
- OS-011
- OS-012
- OS-013

OS-013 is the breaking cleanup point that removes legacy implicit generation after all repository fixtures use the new shape.

### Batch D: expression functions

- OS-014
- OS-015
- OS-016 through OS-019 separately
- OS-020

Each list function remains one Operational Slice.

## Integration gates

1. JSON/YAML semantic-equivalence tests pass before workflow migration.
2. Project/workflow precedence tests pass before data asset migration.
3. `scope: worker` recognized-not-implemented tests pass before shared materialization changes.
4. Explicit `cache_data` smoke passes before legacy automatic cache planning is removed.
5. Explicit `commit_data` smoke passes before legacy publish planning is removed.
6. Full `go test ./...` passes at OS-013 and OS-020.

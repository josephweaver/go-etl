# internal/variable

This directory owns the typed variable model and resolver foundation.

It is not the workflow compiler, controller scheduler, ledger, client, or worker runtime. Its job is to define how named, typed values are represented, scoped, referenced, resolved, and accessed before other packages use those values to make runtime decisions.

## Files

- `namespace.go` owns variable namespaces and their precedence order.
- `name.go` owns the qualified variable name shape.
- `type.go` owns the supported variable type vocabulary.
- `variable.go` owns variable and resolved-value shapes.
- `scope.go` owns scoped collections and precedence-aware lookup.
- `reference.go` owns qualified and unqualified variable references.
- `literal.go` owns conversion from typed literal expressions into resolved values.
- `accessor.go` owns structured access into resolved object and list values.
- `resolver.go` owns recursive reference resolution and fan-out expression resolution.

Test files in this directory describe expected behavior but do not own production concepts.

## Owned Concepts

- Typed variables.
- Variable namespaces and precedence.
- Qualified and unqualified references.
- Resolved scalar, object, and list values.
- Recursive reference resolution with a depth limit.
- Small structured access rules for fields, indexes, and fan-out.
- Literal parsing for supported variable types.

## Concepts Owned Elsewhere

- Workflow definitions and fan-out compilation belong in `internal/workflow`.
- Work-item and controller status transport shapes belong in `internal/model`.
- Queue state, scheduling, worker startup decisions, completion handling, and skip decisions belong in the controller.
- Durable attempt snapshots belong in `internal/ledger`.
- Worker execution and filesystem output behavior belong in `cmd/worker`.
- Client submission and controller bootstrap belong in `internal/client`.

## Invariants

- Runtime configuration should enter the system as typed variables rather than as a parallel hidden config authority.
- Namespaces are meaningful lifecycle and ownership boundaries, not cosmetic prefixes.
- Unqualified references resolve through precedence; qualified references select one namespace explicitly.
- Resolved values carry type information alongside the value.
- Recursive resolution must remain bounded.
- Fan-out resolution produces a list of resolved values; it does not create work items.
- Structured access intentionally stays small until a concrete workflow need requires more expression power.

## Major Dependencies

- The Go standard library only.
- `encoding/json` for object and list literals.
- `time` for datetime literals.
- `strconv` and `math` for scalar parsing.
- `strings` for reference and accessor parsing.

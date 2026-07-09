# Expression Function Framework

Status: Proposed

## Purpose

Add a small, namespaced, pure-function expression layer to GOET's typed variable resolver so variables can be derived from other resolved variables at resolution time.

This Strategic Concept keeps `${name}` as the string/reference interpolation form and adds an explicit functional-expression value form for cases where the result is not necessarily a string.

## Problem

GOET currently supports typed literal values, whole-value references, structured object/list values, accessors, interpolation, and fan-out over resolved lists. That covers values such as:

```json
{
  "type": "string",
  "expression": "${greeting} world"
}
```

It does not yet cover derived values such as:

```text
list.crossproduct(years, regions)
```

where the output should remain a typed list and become available to the rest of the resolver exactly like any other resolved variable.

## Design Summary

A functional expression is an alternate expression payload inside an already typed variable node.

Preferred compact form:

```json
{
  "name": {"namespace": "workflow", "key": "pairs"},
  "type": "list",
  "expression": {
    "$expr": "list.crossproduct(A, B)"
  }
}
```

Supported verbose form:

```json
{
  "name": {"namespace": "workflow", "key": "pairs"},
  "type": "list",
  "expression": {
    "type": "expression",
    "value": "list.crossproduct(A, B)"
  }
}
```

The declared `type` remains the expected result type. The value `type: expression` is an expression-container marker, not a new resolved value type.

## Namespaced Function Names

Function names must be fully qualified:

```text
list.crossproduct
list.zip
list.flatten
list.length
```

The function namespace is separate from the variable namespace system. Variable namespaces still own lookup precedence and lifecycle ownership. Function namespaces own the allowed built-in expression surface.

A function name has this shape:

```text
<function-namespace>.<function-name>
```

Both parts must be simple identifiers. Unqualified function names are invalid.

## Phase 1 Function Grammar

Phase 1 intentionally supports only a small call grammar:

```text
call        := function_name "(" [argument ("," argument)*] ")"
function    := identifier "." identifier
argument    := reference_expression
```

An argument is resolved through the existing variable resolver and may use the existing qualified/unqualified reference and accessor syntax:

```text
A
workflow.A
project_config.years
C.prop1
items[0]
```

Out of scope for phase 1:

- nested function calls as arguments
- arithmetic operators
- boolean operators
- conditionals
- string literals inside function calls
- integer literals inside function calls
- map/filter/reduce
- arbitrary Go, Python, JavaScript, shell, or plugin execution

Those can be added later only when a concrete workflow need proves they are worth the added parser and review surface.

## Resolver Integration

Functional expressions evaluate JIT inside `internal/variable.Resolver` during normal variable resolution.

A function call:

1. is decoded as a `FunctionalExpression` payload inside `TypedExpression.Expression`;
2. validates as a parseable function call at definition-validation time;
3. resolves each argument using normal resolver lookup, namespace precedence, accessors, maximum depth, cycle detection, and sensitivity propagation;
4. calls a registered pure function;
5. verifies the returned `ResolvedValue.Type` matches the enclosing `TypedExpression.Type`;
6. returns the computed value as if it were any other resolved variable.

This keeps workers and workflow compilation downstream of already resolved values. Workers should continue to receive concrete parameters, not unresolved expression language.

## Function Contract

A function is pure and deterministic.

Recommended internal shape:

```go
type FunctionName struct {
    Namespace string
    Name      string
}

type Function interface {
    Name() FunctionName
    Evaluate(args []ResolvedValue) (ResolvedValue, error)
}

type FunctionRegistry interface {
    Lookup(name FunctionName) (Function, bool)
}
```

The exact Go names may change during implementation, but the semantics should not.

## Purity Boundary

Expression functions must not:

- read or write files;
- call the network;
- read environment variables directly;
- read secrets directly;
- mutate resolver state;
- create work items;
- call worker plugins;
- depend on wall-clock time;
- depend on map iteration order for semantic output.

Hard work belongs in worker plugins. Expression functions should only reshape already resolved values.

## Initial Function Library

Phase 1 should add one function per Operational Slice:

| Function | Result | Purpose |
|---|---:|---|
| `list.crossproduct(left, right)` | `list` | Produce left-major pair combinations. |
| `list.zip(left, right)` | `list` | Pair same-index values from equal-length lists. |
| `list.flatten(items)` | `list` | Flatten one level of a list of lists. |
| `list.length(items)` | `int` | Count resolved list items. |

No string, path, math, object, geospatial, or data-provider functions are part of this SC.

## Crossproduct Tuple Representation

Because JSON has arrays but not tuples, `list.crossproduct` returns each pair as a two-item list:

```json
[
  [1, 1],
  [1, 2],
  [1, 3],
  [2, 1]
]
```

Downstream accessors can use `[0]` and `[1]`.

## Example

Variables:

```json
[
  {
    "name": {"namespace": "workflow", "key": "A"},
    "type": "list",
    "expression": [
      {"type": "int", "expression": 1},
      {"type": "int", "expression": 2},
      {"type": "int", "expression": 3}
    ]
  },
  {
    "name": {"namespace": "workflow", "key": "pairs"},
    "type": "list",
    "expression": {"$expr": "list.crossproduct(A, A)"}
  }
]
```

Resolved `pairs`:

```json
[
  [1, 1], [1, 2], [1, 3],
  [2, 1], [2, 2], [2, 3],
  [3, 1], [3, 2], [3, 3]
]
```

Fan-out can remain reference driven:

```text
${pairs[*]}
```

The fan-out surface does not need to parse function calls directly.

## Compatibility

This SC adds new expression payload forms without changing existing literal, reference, interpolation, object, list, accessor, or fan-out behavior.

Existing valid variables should continue to decode, validate, and resolve unchanged.

The form below should remain string interpolation only and should not execute functions:

```json
{
  "type": "string",
  "expression": "${list.crossproduct(A, B)}"
}
```

`${...}` remains a variable-reference syntax. Functional expressions require an explicit expression container.

## Non-Goals

- Creating a full scripting language.
- Allowing function calls inside `${...}`.
- Adding worker-plugin calls to variable resolution.
- Adding domain-specific geospatial functions.
- Adding path normalization or target-specific path translation.
- Adding general arithmetic, conditionals, loops, lambdas, comprehensions, or reducers.
- Replacing the existing typed-expression tree.
- Changing workflow compiler ownership.
- Changing worker execution behavior.

## Proposed Operational Slices

Framework slices:

1. `001-expression-container-forms.md` — decode and validate `$expr` and verbose `type: expression` containers.
2. `002-namespaced-function-parser-and-registry.md` — parse namespaced calls and add a small pure-function registry.
3. `003-resolver-jit-function-evaluation.md` — evaluate functions during resolver execution.

Function slices:

4. `004-list-crossproduct-function.md` — add exactly `list.crossproduct`.
5. `005-list-zip-function.md` — add exactly `list.zip`.
6. `006-list-flatten-function.md` — add exactly `list.flatten`.
7. `007-list-length-function.md` — add exactly `list.length`.

Proof slice:

8. `008-fanout-integration-proof.md` — prove a functional-expression list can drive existing reference-based fan-out.

## Completion Criteria

- Both functional-expression container forms decode into the typed expression model.
- Existing variable definitions remain compatible.
- Function calls must be namespaced.
- Definition validation catches malformed call syntax before resolution.
- Unknown function names produce clear resolver errors.
- Function arguments resolve through normal variable resolver semantics.
- Maximum-depth and cycle detection still apply.
- Function results must match the declared enclosing variable type.
- The initial four functions exist, each added by its own OS.
- Fan-out can consume a variable whose value was produced by a function expression.
- Relevant `go test ./...` or narrower package tests pass.

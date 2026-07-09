# 002 Namespaced Function Parser and Registry

Status: proposed

## Objective

Add a small parser for namespaced function calls and a registry for pure expression functions.

The parser should recognize calls such as:

```text
list.crossproduct(A, B)
list.zip(workflow.A, workflow.B)
list.flatten(pairs)
list.length(pairs)
```

## Minimum Model

Codex 5.4, medium reasoning.

The surface is intentionally small, but parser boundaries and error messages are easier to get wrong than the later one-function slices.

## Required Context

Read these files first:

- `docs/concepts/expression-function-framework/README.md`
- `docs/concepts/expression-function-framework/001-expression-container-forms.md`
- `internal/variable/expression.go`
- `internal/variable/reference.go`
- `internal/variable/accessor.go`
- `internal/variable/name.go`
- `internal/variable/namespace.go`

## Allowed Production Files

- `internal/variable/expression_function.go`
- `internal/variable/function_call.go` new file allowed
- `internal/variable/function_registry.go` new file allowed

## Allowed Test Files

- `internal/variable/expression_function_test.go` new file allowed
- `internal/variable/function_call_test.go` new file allowed
- `internal/variable/function_registry_test.go` new file allowed

## Function Name Rules

A function name must have exactly one namespace separator:

```text
<namespace>.<name>
```

Examples:

- valid: `list.crossproduct`
- valid: `string.join` for future use, even if unregistered
- invalid: `crossproduct`
- invalid: `list.`
- invalid: `.crossproduct`
- invalid: `list.cross.product`

Use a simple identifier rule for both parts:

```text
[A-Za-z_][A-Za-z0-9_]*
```

Do not reuse `variable.Namespace` unless that type naturally fits. Function namespaces are a separate concept from variable namespaces.

## Argument Rules

Phase 1 arguments are existing reference expressions with optional accessors.

Valid examples:

```text
A
workflow.A
project_config.years
C.prop1
items[0]
```

Invalid examples:

```text
"literal"
1
true
list.length(A)
A + B
```

Nested function calls and literals are explicitly out of scope for phase 1.

## Registry Rules

Add a registry that can look up a function by namespaced function name.

Recommended shape:

```go
type FunctionName struct {
    Namespace string
    Name      string
}

type ExpressionFunction interface {
    Name() FunctionName
    Evaluate(args []ResolvedValue) (ResolvedValue, error)
}

type FunctionRegistry interface {
    Lookup(name FunctionName) (ExpressionFunction, bool)
}
```

The exact names may vary. Keep the registry immutable after construction.

This slice may add an empty default registry or a registry constructor, but it must not add concrete functions yet.

## Out Of Scope

- Resolver evaluation.
- Adding concrete built-in functions.
- Supporting literals, nested calls, operators, conditionals, or lambdas.
- Extending `${...}` interpolation syntax.
- Touching workflow compilation.

## Acceptance Criteria

- Valid namespaced calls parse into function name plus ordered argument list.
- Unqualified function names are rejected.
- Malformed calls have clear errors.
- Argument order is preserved.
- Argument reference text is validated using existing reference/accessor rules where possible.
- Unknown function names are not rejected by the parser; registry lookup is a separate concern.
- The registry can register and look up functions by namespace/name.
- Duplicate function registration is rejected or impossible by construction.
- No concrete built-in function is introduced in this slice.

## Suggested Tests

Add table tests for parser success:

- `list.crossproduct(A, B)`
- `list.zip(workflow.A, workflow.B)`
- `list.flatten(C.items)`
- `list.length(items[0])`

Add table tests for parser failure:

- `crossproduct(A, B)`
- `list.crossproduct()` if zero-argument calls are not required yet
- `list.crossproduct(A,,B)`
- `list.cross.product(A)`
- `list.crossproduct(list.length(A))`
- `list.crossproduct("A", B)`

Add registry tests for lookup hit, lookup miss, and duplicate registration behavior.

## Test Command

```bash
go test ./internal/variable
```

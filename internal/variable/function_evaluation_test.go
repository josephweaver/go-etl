package variable

import (
	"fmt"
	"strings"
	"testing"
)

type evaluationTestFunction struct {
	name     FunctionName
	evaluate func(args []ResolvedValue) (ResolvedValue, error)
}

func (function evaluationTestFunction) Name() FunctionName {
	return function.name
}

func (function evaluationTestFunction) Evaluate(args []ResolvedValue) (ResolvedValue, error) {
	return function.evaluate(args)
}

func TestResolverEvaluatesFunctionCallExpression(t *testing.T) {
	registry := functionRegistryForTest(t, "test.collect", func(args []ResolvedValue) (ResolvedValue, error) {
		return ResolvedList(args), nil
	})
	scope := scopeForFunctionEvaluationTest(t,
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "left"}, TypedExpression: TypedExpression{Type: TypeInt, Expression: 1}},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "right"}, TypedExpression: TypedExpression{Type: TypeInt, Expression: 2}},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "pairs"}, TypedExpression: functionCallExpressionForTest(t, "test.collect", TypeList, "left", "right")},
	)
	resolver := NewResolver(NewSet(scope), ResolverConfig{FunctionRegistry: registry})

	value, err := resolver.Resolve(Reference{Name: Name{Namespace: NamespaceWorkflow, Key: "pairs"}, Qualified: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if value.Type != TypeList || len(value.List) != 2 || value.List[0].Value != 1 || value.List[1].Value != 2 {
		t.Fatalf("resolved function value = %+v", value)
	}
}

func TestResolverEvaluatesFunctionArgumentsWithAccessors(t *testing.T) {
	registry := functionRegistryForTest(t, "test.identity", func(args []ResolvedValue) (ResolvedValue, error) {
		if len(args) != 1 {
			return ResolvedValue{}, fmt.Errorf("arity = %d, want 1", len(args))
		}
		return args[0], nil
	})
	scope := scopeForFunctionEvaluationTest(t,
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "records"}, TypedExpression: TypedExpression{Type: TypeList, Expression: []TypedExpression{
			{Type: TypeObject, Expression: map[string]TypedExpression{"tile": {Type: TypeString, Expression: "h18v07"}}},
		}}},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "tile"}, TypedExpression: functionCallExpressionForTest(t, "test.identity", TypeString, "records[0].tile")},
	)
	resolver := NewResolver(NewSet(scope), ResolverConfig{FunctionRegistry: registry})

	value, err := resolver.String("tile")
	if err != nil {
		t.Fatalf("String() error = %v", err)
	}
	if value != "h18v07" {
		t.Fatalf("tile = %q", value)
	}
}

func TestResolverRejectsUnknownFunction(t *testing.T) {
	scope := scopeForFunctionEvaluationTest(t,
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "source"}, TypedExpression: TypedExpression{Type: TypeString, Expression: "value"}},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "derived"}, TypedExpression: functionCallExpressionForTest(t, "test.missing", TypeString, "source")},
	)
	resolver := NewResolver(NewSet(scope), ResolverConfig{})

	_, err := resolver.String("derived")
	if err == nil || !strings.Contains(err.Error(), "unknown function: test.missing") {
		t.Fatalf("String() error = %v, want unknown function", err)
	}
}

func TestResolverRejectsFunctionResultTypeMismatch(t *testing.T) {
	registry := functionRegistryForTest(t, "test.bad", func(args []ResolvedValue) (ResolvedValue, error) {
		return ResolvedValue{Type: TypeString, Value: "wrong"}, nil
	})
	scope := scopeForFunctionEvaluationTest(t,
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "source"}, TypedExpression: TypedExpression{Type: TypeString, Expression: "value"}},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "derived"}, TypedExpression: functionCallExpressionForTest(t, "test.bad", TypeList, "source")},
	)
	resolver := NewResolver(NewSet(scope), ResolverConfig{FunctionRegistry: registry})

	_, err := resolver.Resolve(Reference{Name: Name{Namespace: NamespaceWorkflow, Key: "derived"}, Qualified: true})
	if err == nil || !strings.Contains(err.Error(), "returned type string, want list") {
		t.Fatalf("Resolve() error = %v, want result type mismatch", err)
	}
}

func TestResolverReportsFunctionArityAndArgumentTypeErrors(t *testing.T) {
	registry := functionRegistryForTest(t, "test.needs_two_strings", func(args []ResolvedValue) (ResolvedValue, error) {
		if len(args) != 2 {
			return ResolvedValue{}, fmt.Errorf("arity = %d, want 2", len(args))
		}
		for index, arg := range args {
			if arg.Type != TypeString {
				return ResolvedValue{}, fmt.Errorf("argument %d has type %s, want string", index, arg.Type)
			}
		}
		return ResolvedValue{Type: TypeString, Value: args[0].Value.(string) + args[1].Value.(string)}, nil
	})
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "arity", args: []string{"text"}, want: "arity = 1, want 2"},
		{name: "argument type", args: []string{"text", "count"}, want: "argument 1 has type int, want string"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scope := scopeForFunctionEvaluationTest(t,
				Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "text"}, TypedExpression: TypedExpression{Type: TypeString, Expression: "a"}},
				Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "count"}, TypedExpression: TypedExpression{Type: TypeInt, Expression: 1}},
				Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "derived"}, TypedExpression: functionCallExpressionForTest(t, "test.needs_two_strings", TypeString, test.args...)},
			)
			resolver := NewResolver(NewSet(scope), ResolverConfig{FunctionRegistry: registry})
			_, err := resolver.String("derived")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("String() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestResolverPropagatesSensitivityFromFunctionArguments(t *testing.T) {
	registry := functionRegistryForTest(t, "test.masked", func(args []ResolvedValue) (ResolvedValue, error) {
		return ResolvedValue{Type: TypeString, Value: "derived"}, nil
	})
	scope := scopeForFunctionEvaluationTest(t,
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "secret"}, TypedExpression: TypedExpression{Type: TypeString, Expression: "raw"}, Sensitive: true},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "derived"}, TypedExpression: functionCallExpressionForTest(t, "test.masked", TypeString, "secret")},
	)
	resolver := NewResolver(NewSet(scope), ResolverConfig{FunctionRegistry: registry})

	value, err := resolver.Resolve(Reference{Name: Name{Namespace: NamespaceWorkflow, Key: "derived"}, Qualified: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if !value.Sensitive || value.Provenance != "workflow.secret" {
		t.Fatalf("sensitivity = %+v, want workflow.secret provenance", value)
	}
}

func TestResolverFunctionArgumentsPreserveCycleDetection(t *testing.T) {
	registry := functionRegistryForTest(t, "test.collect", func(args []ResolvedValue) (ResolvedValue, error) {
		return ResolvedList(args), nil
	})
	scope := scopeForFunctionEvaluationTest(t,
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "a"}, TypedExpression: functionCallExpressionForTest(t, "test.collect", TypeList, "b")},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "b"}, TypedExpression: TypedExpression{Type: TypeList, Expression: "${a}"}},
	)
	resolver := NewResolver(NewSet(scope), ResolverConfig{FunctionRegistry: registry})

	_, err := resolver.Resolve(Reference{Name: Name{Namespace: NamespaceWorkflow, Key: "a"}, Qualified: true})
	if err == nil || !strings.Contains(err.Error(), "reference cycle") {
		t.Fatalf("Resolve() error = %v, want cycle", err)
	}
}

func TestResolverFunctionArgumentsCountTowardMaxDepth(t *testing.T) {
	registry := functionRegistryForTest(t, "test.collect", func(args []ResolvedValue) (ResolvedValue, error) {
		return ResolvedList(args), nil
	})
	scope := scopeForFunctionEvaluationTest(t,
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "a"}, TypedExpression: functionCallExpressionForTest(t, "test.collect", TypeList, "b")},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "b"}, TypedExpression: TypedExpression{Type: TypeInt, Expression: 1}},
	)
	resolver := NewResolver(NewSet(scope), ResolverConfig{FunctionRegistry: registry, MaxDepth: 1})

	_, err := resolver.Resolve(Reference{Name: Name{Namespace: NamespaceWorkflow, Key: "a"}, Qualified: true})
	if err == nil || !strings.Contains(err.Error(), "maximum variable resolution depth exceeded") {
		t.Fatalf("Resolve() error = %v, want max depth", err)
	}
}

func functionRegistryForTest(t *testing.T, name string, evaluate func(args []ResolvedValue) (ResolvedValue, error)) FunctionRegistry {
	t.Helper()
	functionName, err := ParseFunctionName(name)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewFunctionRegistry(evaluationTestFunction{name: functionName, evaluate: evaluate})
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func functionCallExpressionForTest(t *testing.T, name string, resultType Type, argRefs ...string) TypedExpression {
	t.Helper()
	functionName, err := ParseFunctionName(name)
	if err != nil {
		t.Fatal(err)
	}
	args := make([]FunctionArgumentReference, 0, len(argRefs))
	for _, argRef := range argRefs {
		arg, err := NewFunctionArgumentReference(argRef)
		if err != nil {
			t.Fatal(err)
		}
		args = append(args, arg)
	}
	call, err := NewFunctionCallExpression(functionName, resultType, args)
	if err != nil {
		t.Fatal(err)
	}
	return TypedExpression{Type: resultType, Expression: call}
}

func scopeForFunctionEvaluationTest(t *testing.T, variables ...Variable) Scope {
	t.Helper()
	scope, err := NewScope(variables...)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

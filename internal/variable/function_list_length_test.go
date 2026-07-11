package variable

import (
	"strings"
	"testing"
)

func TestListLengthFunctionReturnsNonemptyLength(t *testing.T) {
	result, err := listLengthFunction{}.Evaluate([]ResolvedValue{
		ResolvedList([]ResolvedValue{
			{Type: TypeString, Value: "a"},
			{Type: TypeInt, Value: 2},
			ResolvedList(nil),
		}),
	})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if result.Type != TypeInt || result.Value != 3 {
		t.Fatalf("result = %+v, want int 3", result)
	}
}

func TestListLengthFunctionReturnsZeroForEmptyList(t *testing.T) {
	result, err := listLengthFunction{}.Evaluate([]ResolvedValue{ResolvedList(nil)})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if result.Type != TypeInt || result.Value != 0 {
		t.Fatalf("result = %+v, want int 0", result)
	}
}

func TestListLengthFunctionRejectsArityAndTypes(t *testing.T) {
	tests := []struct {
		name string
		args []ResolvedValue
		want string
	}{
		{name: "arity", args: []ResolvedValue{}, want: "arity = 0, want 1"},
		{name: "type", args: []ResolvedValue{{Type: TypeString, Value: "abc"}}, want: "argument 0 has type string, want list"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := listLengthFunction{}.Evaluate(test.args)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Evaluate() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestResolverUsesDefaultListLengthFunction(t *testing.T) {
	scope, err := NewScope(
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "regions"}, TypedExpression: TypedExpression{Type: TypeList, Expression: []TypedExpression{
			{Type: TypeString, Expression: "north"},
			{Type: TypeString, Expression: "south"},
		}}},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "region_count"}, TypedExpression: functionCallExpressionForTest(t, "list.length", TypeInt, "regions")},
	)
	if err != nil {
		t.Fatal(err)
	}
	resolver := NewResolver(NewSet(scope), ResolverConfig{FunctionRegistry: DefaultFunctionRegistry()})

	value, err := resolver.Resolve(Reference{Name: Name{Namespace: NamespaceWorkflow, Key: "region_count"}, Qualified: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if value.Type != TypeInt || value.Value != 2 {
		t.Fatalf("value = %+v, want int 2", value)
	}
}

func TestResolverPropagatesSensitivityFromListLengthArgument(t *testing.T) {
	scope, err := NewScope(
		Variable{
			Name:      Name{Namespace: NamespaceWorkflow, Key: "secret_regions"},
			Sensitive: true,
			TypedExpression: TypedExpression{Type: TypeList, Expression: []TypedExpression{
				{Type: TypeString, Expression: "north"},
			}},
		},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "secret_region_count"}, TypedExpression: functionCallExpressionForTest(t, "list.length", TypeInt, "secret_regions")},
	)
	if err != nil {
		t.Fatal(err)
	}
	resolver := NewResolver(NewSet(scope), ResolverConfig{FunctionRegistry: DefaultFunctionRegistry()})

	value, err := resolver.Resolve(Reference{Name: Name{Namespace: NamespaceWorkflow, Key: "secret_region_count"}, Qualified: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if value.Type != TypeInt || value.Value != 1 {
		t.Fatalf("value = %+v, want int 1", value)
	}
	if !value.Sensitive || value.Provenance != "workflow.secret_regions" {
		t.Fatalf("value sensitivity = %+v", value)
	}
}

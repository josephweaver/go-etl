package variable

import (
	"strings"
	"testing"
)

func TestListCrossproductFunctionProducesLeftMajorPairs(t *testing.T) {
	result, err := listCrossproductFunction{}.Evaluate([]ResolvedValue{
		ResolvedList([]ResolvedValue{
			{Type: TypeInt, Value: 1},
			{Type: TypeInt, Value: 2},
		}),
		ResolvedList([]ResolvedValue{
			{Type: TypeString, Value: "a"},
			{Type: TypeString, Value: "b"},
		}),
	})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	got := crossproductPairStrings(t, result)
	want := []string{"1:a", "1:b", "2:a", "2:b"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("pairs = %#v, want %#v", got, want)
	}
}

func TestListCrossproductFunctionReturnsEmptyForEmptyInput(t *testing.T) {
	tests := []struct {
		name string
		args []ResolvedValue
	}{
		{
			name: "left empty",
			args: []ResolvedValue{
				ResolvedList(nil),
				ResolvedList([]ResolvedValue{{Type: TypeString, Value: "a"}}),
			},
		},
		{
			name: "right empty",
			args: []ResolvedValue{
				ResolvedList([]ResolvedValue{{Type: TypeInt, Value: 1}}),
				ResolvedList(nil),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := listCrossproductFunction{}.Evaluate(test.args)
			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}
			if result.Type != TypeList || len(result.List) != 0 {
				t.Fatalf("result = %+v, want empty list", result)
			}
		})
	}
}

func TestListCrossproductFunctionRejectsArityAndTypes(t *testing.T) {
	tests := []struct {
		name string
		args []ResolvedValue
		want string
	}{
		{name: "arity", args: []ResolvedValue{ResolvedList(nil)}, want: "arity = 1, want 2"},
		{name: "left type", args: []ResolvedValue{{Type: TypeString, Value: "a"}, ResolvedList(nil)}, want: "argument 0 has type string, want list"},
		{name: "right type", args: []ResolvedValue{ResolvedList(nil), {Type: TypeInt, Value: 1}}, want: "argument 1 has type int, want list"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := listCrossproductFunction{}.Evaluate(test.args)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Evaluate() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestListCrossproductFunctionPreservesItemSensitivity(t *testing.T) {
	secret := ResolvedValue{Type: TypeString, Value: "secret", Sensitive: true, RedactionLabel: "[REDACTED:secret]", Provenance: "workflow.secret"}
	result, err := listCrossproductFunction{}.Evaluate([]ResolvedValue{
		ResolvedList([]ResolvedValue{secret}),
		ResolvedList([]ResolvedValue{{Type: TypeString, Value: "public"}}),
	})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	pair := result.List[0]
	if !pair.Sensitive || pair.RedactionLabel != "[REDACTED:secret]" || pair.Provenance != "workflow.secret" {
		t.Fatalf("pair sensitivity = %+v", pair)
	}
	if !pair.List[0].Sensitive {
		t.Fatalf("left item sensitivity = %+v", pair.List[0])
	}
}

func TestListCrossproductFunctionDoesNotMutateInputs(t *testing.T) {
	left := ResolvedList([]ResolvedValue{{Type: TypeInt, Value: 1}})
	right := ResolvedList([]ResolvedValue{{Type: TypeString, Value: "a"}})
	result, err := listCrossproductFunction{}.Evaluate([]ResolvedValue{left, right})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	result.List[0].List[0].Value = 99
	if left.List[0].Value != 1 {
		t.Fatalf("left input mutated = %+v", left)
	}
}

func TestResolverUsesDefaultListCrossproductFunction(t *testing.T) {
	scope, err := NewScope(
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "years"}, TypedExpression: TypedExpression{Type: TypeList, Expression: []TypedExpression{
			{Type: TypeInt, Expression: 2024},
			{Type: TypeInt, Expression: 2025},
		}}},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "regions"}, TypedExpression: TypedExpression{Type: TypeList, Expression: []TypedExpression{
			{Type: TypeString, Expression: "north"},
			{Type: TypeString, Expression: "south"},
		}}},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "pairs"}, TypedExpression: functionCallExpressionForTest(t, "list.crossproduct", TypeList, "years", "regions")},
	)
	if err != nil {
		t.Fatal(err)
	}
	resolver := NewResolver(NewSet(scope), ResolverConfig{FunctionRegistry: DefaultFunctionRegistry()})

	value, err := resolver.Resolve(Reference{Name: Name{Namespace: NamespaceWorkflow, Key: "pairs"}, Qualified: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	got := crossproductPairStrings(t, value)
	want := []string{"2024:north", "2024:south", "2025:north", "2025:south"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("pairs = %#v, want %#v", got, want)
	}
}

func crossproductPairStrings(t *testing.T, value ResolvedValue) []string {
	t.Helper()
	if value.Type != TypeList {
		t.Fatalf("value type = %s, want list", value.Type)
	}
	result := make([]string, 0, len(value.List))
	for index, pair := range value.List {
		if pair.Type != TypeList || len(pair.List) != 2 {
			t.Fatalf("pair %d = %+v, want two-item list", index, pair)
		}
		result = append(result, pair.List[0].String()+":"+pair.List[1].String())
	}
	return result
}

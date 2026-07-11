package variable

import (
	"strings"
	"testing"
)

func TestListZipFunctionProducesIndexedPairs(t *testing.T) {
	result, err := listZipFunction{}.Evaluate([]ResolvedValue{
		ResolvedList([]ResolvedValue{
			{Type: TypeInt, Value: 1},
			{Type: TypeString, Value: "two"},
		}),
		ResolvedList([]ResolvedValue{
			{Type: TypeString, Value: "a"},
			{Type: TypeBool, Value: true},
		}),
	})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	got := twoItemPairStrings(t, result)
	want := []string{"1:a", "two:true"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("pairs = %#v, want %#v", got, want)
	}
}

func TestListZipFunctionReturnsEmptyForTwoEmptyLists(t *testing.T) {
	result, err := listZipFunction{}.Evaluate([]ResolvedValue{ResolvedList(nil), ResolvedList(nil)})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if result.Type != TypeList || len(result.List) != 0 {
		t.Fatalf("result = %+v, want empty list", result)
	}
}

func TestListZipFunctionRejectsArityTypesAndLengthMismatch(t *testing.T) {
	tests := []struct {
		name string
		args []ResolvedValue
		want string
	}{
		{name: "arity", args: []ResolvedValue{ResolvedList(nil)}, want: "arity = 1, want 2"},
		{name: "left type", args: []ResolvedValue{{Type: TypeString, Value: "a"}, ResolvedList(nil)}, want: "argument 0 has type string, want list"},
		{name: "right type", args: []ResolvedValue{ResolvedList(nil), {Type: TypeInt, Value: 1}}, want: "argument 1 has type int, want list"},
		{
			name: "length mismatch",
			args: []ResolvedValue{
				ResolvedList([]ResolvedValue{{Type: TypeInt, Value: 1}, {Type: TypeInt, Value: 2}}),
				ResolvedList([]ResolvedValue{{Type: TypeString, Value: "a"}}),
			},
			want: "list length mismatch: left=2 right=1",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := listZipFunction{}.Evaluate(test.args)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Evaluate() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestResolverUsesDefaultListZipFunction(t *testing.T) {
	scope, err := NewScope(
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "years"}, TypedExpression: TypedExpression{Type: TypeList, Expression: []TypedExpression{
			{Type: TypeInt, Expression: 2024},
			{Type: TypeInt, Expression: 2025},
		}}},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "regions"}, TypedExpression: TypedExpression{Type: TypeList, Expression: []TypedExpression{
			{Type: TypeString, Expression: "north"},
			{Type: TypeString, Expression: "south"},
		}}},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "pairs"}, TypedExpression: functionCallExpressionForTest(t, "list.zip", TypeList, "years", "regions")},
	)
	if err != nil {
		t.Fatal(err)
	}
	resolver := NewResolver(NewSet(scope), ResolverConfig{FunctionRegistry: DefaultFunctionRegistry()})

	value, err := resolver.Resolve(Reference{Name: Name{Namespace: NamespaceWorkflow, Key: "pairs"}, Qualified: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	got := twoItemPairStrings(t, value)
	want := []string{"2024:north", "2025:south"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("pairs = %#v, want %#v", got, want)
	}
}

func twoItemPairStrings(t *testing.T, value ResolvedValue) []string {
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

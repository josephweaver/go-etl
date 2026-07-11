package variable

import (
	"strings"
	"testing"
)

func TestListFlattenFunctionFlattensOneLevel(t *testing.T) {
	result, err := listFlattenFunction{}.Evaluate([]ResolvedValue{
		ResolvedList([]ResolvedValue{
			ResolvedList([]ResolvedValue{
				{Type: TypeInt, Value: 1},
				{Type: TypeString, Value: "two"},
			}),
			ResolvedList([]ResolvedValue{
				{Type: TypeBool, Value: true},
			}),
		}),
	})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	got := listItemStrings(t, result)
	want := []string{"1", "two", "true"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("items = %#v, want %#v", got, want)
	}
}

func TestListFlattenFunctionKeepsEmptyChildren(t *testing.T) {
	result, err := listFlattenFunction{}.Evaluate([]ResolvedValue{
		ResolvedList([]ResolvedValue{
			ResolvedList(nil),
			ResolvedList([]ResolvedValue{{Type: TypeString, Value: "a"}}),
			ResolvedList(nil),
		}),
	})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	got := listItemStrings(t, result)
	want := []string{"a"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("items = %#v, want %#v", got, want)
	}
}

func TestListFlattenFunctionDoesNotRecursivelyFlattenNestedLists(t *testing.T) {
	nested := ResolvedList([]ResolvedValue{{Type: TypeString, Value: "deep"}})
	result, err := listFlattenFunction{}.Evaluate([]ResolvedValue{
		ResolvedList([]ResolvedValue{
			ResolvedList([]ResolvedValue{nested}),
		}),
	})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if result.Type != TypeList || len(result.List) != 1 {
		t.Fatalf("result = %+v, want one-item list", result)
	}
	if result.List[0].Type != TypeList || len(result.List[0].List) != 1 || result.List[0].List[0].String() != "deep" {
		t.Fatalf("nested item = %+v, want preserved child list", result.List[0])
	}
}

func TestListFlattenFunctionRejectsArityAndTypes(t *testing.T) {
	tests := []struct {
		name string
		args []ResolvedValue
		want string
	}{
		{name: "arity", args: []ResolvedValue{}, want: "arity = 0, want 1"},
		{name: "argument type", args: []ResolvedValue{{Type: TypeString, Value: "a"}}, want: "argument 0 has type string, want list"},
		{
			name: "scalar child",
			args: []ResolvedValue{
				ResolvedList([]ResolvedValue{
					ResolvedList(nil),
					{Type: TypeInt, Value: 1},
				}),
			},
			want: "argument 0[1] has type int, want list",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := listFlattenFunction{}.Evaluate(test.args)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Evaluate() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestListFlattenFunctionPreservesItemSensitivity(t *testing.T) {
	secret := ResolvedValue{Type: TypeString, Value: "secret", Sensitive: true, RedactionLabel: "[REDACTED:secret]", Provenance: "workflow.secret"}
	result, err := listFlattenFunction{}.Evaluate([]ResolvedValue{
		ResolvedList([]ResolvedValue{
			ResolvedList([]ResolvedValue{secret}),
		}),
	})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !result.Sensitive || result.RedactionLabel != "[REDACTED:secret]" || result.Provenance != "workflow.secret" {
		t.Fatalf("result sensitivity = %+v", result)
	}
	if !result.List[0].Sensitive {
		t.Fatalf("flattened item sensitivity = %+v", result.List[0])
	}
}

func TestResolverUsesDefaultListFlattenFunction(t *testing.T) {
	scope, err := NewScope(
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "groups"}, TypedExpression: TypedExpression{Type: TypeList, Expression: []TypedExpression{
			{Type: TypeList, Expression: []TypedExpression{
				{Type: TypeString, Expression: "north"},
				{Type: TypeString, Expression: "south"},
			}},
			{Type: TypeList, Expression: []TypedExpression{
				{Type: TypeString, Expression: "west"},
			}},
		}}},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "regions"}, TypedExpression: functionCallExpressionForTest(t, "list.flatten", TypeList, "groups")},
	)
	if err != nil {
		t.Fatal(err)
	}
	resolver := NewResolver(NewSet(scope), ResolverConfig{FunctionRegistry: DefaultFunctionRegistry()})

	value, err := resolver.Resolve(Reference{Name: Name{Namespace: NamespaceWorkflow, Key: "regions"}, Qualified: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	got := listItemStrings(t, value)
	want := []string{"north", "south", "west"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("items = %#v, want %#v", got, want)
	}
}

func listItemStrings(t *testing.T, value ResolvedValue) []string {
	t.Helper()
	if value.Type != TypeList {
		t.Fatalf("value type = %s, want list", value.Type)
	}
	result := make([]string, 0, len(value.List))
	for _, item := range value.List {
		result = append(result, item.String())
	}
	return result
}

package variable

import (
	"encoding/json"
	"testing"
)

func TestVariableValidate(t *testing.T) {
	valid := Variable{Name: Name{
		Namespace: NamespaceProject,
		Key:       "data_dir",
	}, TypedExpression: TypedExpression{Type: TypePath, Expression: "/data/project"}}

	tests := []struct {
		name     string
		variable Variable
		wantErr  bool
	}{
		{name: "valid variable", variable: valid},
		{
			name: "invalid name",
			variable: Variable{
				TypedExpression: TypedExpression{Type: TypePath, Expression: "/data/project"},
			},
			wantErr: true,
		},
		{
			name:     "unsupported type",
			variable: Variable{Name: valid.Name, TypedExpression: TypedExpression{Type: Type{Kind: "unknown"}, Expression: "/data/project"}},

			wantErr: true,
		},
		{
			name: "missing expression",
			variable: Variable{
				Name:            valid.Name,
				TypedExpression: TypedExpression{Type: TypePath},
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.variable.Validate()

			if test.wantErr && err == nil {
				t.Fatal("expected an error")
			}

			if !test.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestResolvedObjectCarriesFields(t *testing.T) {
	value := ResolvedObject(map[string]ResolvedValue{
		"year": {Type: TypeInt, Value: 2025},
		"path": {Type: TypePath, Value: "/data/cdl/2025.tif"},
	})

	if value.Type != TypeObject {
		t.Fatalf("unexpected type: %s", value.Type)
	}

	if value.Object["year"].Value != 2025 {
		t.Fatalf("unexpected year field: %#v", value.Object["year"].Value)
	}
}

func TestResolvedListCarriesElements(t *testing.T) {
	value := ResolvedList([]ResolvedValue{
		{Type: TypeInt, Value: 2024},
		{Type: TypeString, Value: "2025"},
		ResolvedList([]ResolvedValue{{Type: TypeBool, Value: true}}),
	})

	if value.Type != TypeList {
		t.Fatalf("unexpected type: %s", value.Type)
	}

	if len(value.List) != 3 {
		t.Fatalf("unexpected list length: %d", len(value.List))
	}

	if value.List[2].Type != TypeList {
		t.Fatalf("unexpected nested type: %s", value.List[2].Type)
	}
}

func TestResolvedListAllowsEmptyList(t *testing.T) {
	value := ResolvedList(nil)
	if value.Type != TypeList || len(value.List) != 0 {
		t.Fatalf("unexpected empty list: %#v", value)
	}
}

func TestOptionalObjectFieldStringListRejectsNonStringItem(t *testing.T) {
	fields := map[string]ResolvedValue{
		"args": ResolvedList([]ResolvedValue{
			{Type: TypeString, Value: "--once"},
			{Type: TypeInt, Value: 2},
		}),
	}

	if _, _, err := OptionalObjectFieldStringList(fields, "args"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestVariableJSONRoundTripUsesFlatTypedExpression(t *testing.T) {
	text := `{
		"name":{"namespace":"workflow","key":"settings"},
		"type":"object",
		"expression":{
			"enabled":{"type":"bool","expression":true},
			"values":{"type":"list","expression":[{"type":"int","expression":2}]}
		}
	}`

	var value Variable
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if value.Name.Key != "settings" || value.Type != TypeObject {
		t.Fatalf("unexpected variable: %#v", value)
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	assertJSONEqual(t, encoded, []byte(text))
}

func TestVariableJSONRejectsLegacyStructuredExpression(t *testing.T) {
	legacy := `{
		"Name":{"Namespace":"workflow","Key":"settings"},
		"Type":{"Kind":"object"},
		"Expression":"{\"enabled\":true}"
	}`

	var value Variable
	if err := json.Unmarshal([]byte(legacy), &value); err == nil {
		t.Fatal("expected an error")
	}
}

func TestTypedExpressionFromResolvedConvertsStructuredValue(t *testing.T) {
	value := ResolvedObject(map[string]ResolvedValue{
		"answer": {Type: TypeInt, Value: 42},
		"label":  {Type: TypeString, Value: "done"},
		"items": ResolvedList([]ResolvedValue{
			ResolvedObject(map[string]ResolvedValue{"ok": {Type: TypeBool, Value: true}}),
		}),
	})

	expression, err := TypedExpressionFromResolved(value)
	if err != nil {
		t.Fatalf("TypedExpressionFromResolved() error = %v", err)
	}
	if err := expression.ValidateDefinition(); err != nil {
		t.Fatalf("ValidateDefinition() error = %v", err)
	}
	fields, ok := expression.Expression.(map[string]TypedExpression)
	if !ok {
		t.Fatalf("expression = %#v, want typed field map", expression.Expression)
	}
	if fields["answer"].Type != TypeInt || fields["answer"].Expression != 42 {
		t.Fatalf("answer expression = %#v, want int 42", fields["answer"])
	}
	items, ok := fields["items"].Expression.([]TypedExpression)
	if !ok || len(items) != 1 || items[0].Type != TypeObject {
		t.Fatalf("items expression = %#v, want object list", fields["items"].Expression)
	}
}

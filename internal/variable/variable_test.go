package variable

import "testing"

func TestVariableValidate(t *testing.T) {
	valid := Variable{
		Name: Name{
			Namespace: NamespaceProject,
			Key:       "data_dir",
		},
		Type:       TypePath,
		Expression: "/data/project",
	}

	tests := []struct {
		name     string
		variable Variable
		wantErr  bool
	}{
		{name: "valid variable", variable: valid},
		{
			name: "invalid name",
			variable: Variable{
				Type:       TypePath,
				Expression: "/data/project",
			},
			wantErr: true,
		},
		{
			name: "unsupported type",
			variable: Variable{
				Name:       valid.Name,
				Type:       Type{Kind: "unknown"},
				Expression: "/data/project",
			},
			wantErr: true,
		},
		{
			name: "missing expression",
			variable: Variable{
				Name: valid.Name,
				Type: TypePath,
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

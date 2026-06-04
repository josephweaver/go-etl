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
	value, err := ResolvedList(TypeInt, []ResolvedValue{
		{Type: TypeInt, Value: 2024},
		{Type: TypeInt, Value: 2025},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if value.Type.String() != TypeList(TypeInt).String() {
		t.Fatalf("unexpected type: %s", value.Type)
	}

	if len(value.List) != 2 {
		t.Fatalf("unexpected list length: %d", len(value.List))
	}
}

func TestResolvedListRejectsWrongElementType(t *testing.T) {
	_, err := ResolvedList(TypeInt, []ResolvedValue{
		{Type: TypeInt, Value: 2024},
		{Type: TypeString, Value: "2025"},
	})
	if err == nil {
		t.Fatal("expected an error")
	}
}

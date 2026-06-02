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

package variable

import (
	"testing"
	"time"
)

func TestParseLiteral(t *testing.T) {
	tests := []struct {
		name      string
		variable  Variable
		wantType  Type
		wantValue any
	}{
		{
			name: "string",
			variable: Variable{
				Name:       Name{Namespace: NamespaceWorkflow, Key: "crop"},
				Type:       TypeString,
				Expression: "corn",
			},
			wantType:  TypeString,
			wantValue: "corn",
		},
		{
			name: "int",
			variable: Variable{
				Name:       Name{Namespace: NamespaceWorkflow, Key: "year"},
				Type:       TypeInt,
				Expression: "2025",
			},
			wantType:  TypeInt,
			wantValue: 2025,
		},
		{
			name: "bool",
			variable: Variable{
				Name:       Name{Namespace: NamespaceWorkflow, Key: "enabled"},
				Type:       TypeBool,
				Expression: "true",
			},
			wantType:  TypeBool,
			wantValue: true,
		},
		{
			name: "datetime",
			variable: Variable{
				Name:       Name{Namespace: NamespaceWorkflow, Key: "start"},
				Type:       TypeDatetime,
				Expression: "2026-06-02T12:00:00Z",
			},
			wantType:  TypeDatetime,
			wantValue: time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC),
		},
		{
			name: "path",
			variable: Variable{
				Name:       Name{Namespace: NamespaceWorkflow, Key: "data_dir"},
				Type:       TypePath,
				Expression: "/data/project",
			},
			wantType:  TypePath,
			wantValue: "/data/project",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, err := ParseLiteral(test.variable)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if value.Type != test.wantType {
				t.Fatalf("unexpected type: %s", value.Type)
			}

			if value.Value != test.wantValue {
				t.Fatalf("unexpected value: %#v", value.Value)
			}
		})
	}
}

func TestParseLiteralRejectsInvalidValue(t *testing.T) {
	tests := []Variable{
		{
			Name:       Name{Namespace: NamespaceWorkflow, Key: "year"},
			Type:       TypeInt,
			Expression: "not-int",
		},
		{
			Name:       Name{Namespace: NamespaceWorkflow, Key: "enabled"},
			Type:       TypeBool,
			Expression: "not-bool",
		},
		{
			Name:       Name{Namespace: NamespaceWorkflow, Key: "start"},
			Type:       TypeDatetime,
			Expression: "not-datetime",
		},
	}

	for _, variable := range tests {
		t.Run(variable.Name.Key, func(t *testing.T) {
			if _, err := ParseLiteral(variable); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestParseLiteralRejectsStructuredTypes(t *testing.T) {
	tests := []Variable{
		{
			Name:       Name{Namespace: NamespaceWorkflow, Key: "years"},
			Type:       TypeList(TypeInt),
			Expression: "[2024, 2025]",
		},
		{
			Name:       Name{Namespace: NamespaceWorkflow, Key: "record"},
			Type:       TypeObject,
			Expression: `{"year": 2025}`,
		},
	}

	for _, variable := range tests {
		t.Run(variable.Name.Key, func(t *testing.T) {
			if _, err := ParseLiteral(variable); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

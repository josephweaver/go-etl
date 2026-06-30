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
			name:     "string",
			variable: Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "crop"}, TypedExpression: TypedExpression{Type: TypeString, Expression: "corn"}},

			wantType:  TypeString,
			wantValue: "corn",
		},
		{
			name:     "int",
			variable: Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "year"}, TypedExpression: TypedExpression{Type: TypeInt, Expression: 2025}},

			wantType:  TypeInt,
			wantValue: 2025,
		},
		{
			name:     "bool",
			variable: Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "enabled"}, TypedExpression: TypedExpression{Type: TypeBool, Expression: true}},

			wantType:  TypeBool,
			wantValue: true,
		},
		{
			name:     "datetime",
			variable: Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "start"}, TypedExpression: TypedExpression{Type: TypeDatetime, Expression: "2026-06-02T12:00:00Z"}},

			wantType:  TypeDatetime,
			wantValue: time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC),
		},
		{
			name:     "path",
			variable: Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "data_dir"}, TypedExpression: TypedExpression{Type: TypePath, Expression: "/data/project"}},

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
			Name:            Name{Namespace: NamespaceWorkflow, Key: "year"},
			TypedExpression: TypedExpression{Type: TypeInt, Expression: "not-int"},
		},
		{
			Name:            Name{Namespace: NamespaceWorkflow, Key: "enabled"},
			TypedExpression: TypedExpression{Type: TypeBool, Expression: "not-bool"},
		},
		{
			Name:            Name{Namespace: NamespaceWorkflow, Key: "start"},
			TypedExpression: TypedExpression{Type: TypeDatetime, Expression: "not-datetime"},
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

func TestParseLiteralParsesObject(t *testing.T) {
	value, err := ParseLiteral(Variable{
		Name: Name{Namespace: NamespaceWorkflow, Key: "record"},
		TypedExpression: TypedExpression{
			Type: TypeObject,
			Expression: map[string]TypedExpression{
				"year":    {Type: TypeInt, Expression: 2025},
				"path":    {Type: TypePath, Expression: "/data/cdl/2025.tif"},
				"enabled": {Type: TypeBool, Expression: true},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if value.Type != TypeObject {
		t.Fatalf("unexpected type: %s", value.Type)
	}

	if value.Object["year"].Value != 2025 {
		t.Fatalf("unexpected year: %#v", value.Object["year"].Value)
	}

	if value.Object["path"].Value != "/data/cdl/2025.tif" {
		t.Fatalf("unexpected path: %#v", value.Object["path"].Value)
	}
}

func TestParseLiteralParsesList(t *testing.T) {
	value, err := ParseLiteral(Variable{
		Name: Name{Namespace: NamespaceWorkflow, Key: "values"},
		TypedExpression: TypedExpression{
			Type: TypeList,
			Expression: []TypedExpression{
				{Type: TypeInt, Expression: 2024},
				{Type: TypeString, Expression: "ready"},
				{Type: TypeBool, Expression: true},
				{Type: TypeList, Expression: []TypedExpression{{Type: TypeInt, Expression: 2025}}},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if value.Type != TypeList {
		t.Fatalf("unexpected type: %s", value.Type)
	}

	if value.List[1].Value != "ready" {
		t.Fatalf("unexpected second value: %#v", value.List[1].Value)
	}

	if value.List[3].Type != TypeList || value.List[3].List[0].Value != 2025 {
		t.Fatalf("unexpected nested list: %#v", value.List[3])
	}
}

func TestParseLiteralParsesEmptyList(t *testing.T) {
	value, err := ParseLiteral(Variable{
		Name:            Name{Namespace: NamespaceWorkflow, Key: "values"},
		TypedExpression: TypedExpression{Type: TypeList, Expression: []TypedExpression{}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value.Type != TypeList || len(value.List) != 0 {
		t.Fatalf("unexpected empty list: %#v", value)
	}
}

func TestParseLiteralRejectsInvalidStructuredValue(t *testing.T) {
	tests := []Variable{
		{
			Name:            Name{Namespace: NamespaceWorkflow, Key: "record"},
			TypedExpression: TypedExpression{Type: TypeObject, Expression: []TypedExpression{}},
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

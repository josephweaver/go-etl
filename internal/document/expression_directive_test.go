package document

import (
	"strings"
	"testing"

	"goetl/internal/variable"
)

func TestLoadVariablesParsesStructuredFunctionCallDirective(t *testing.T) {
	variables, err := LoadVariables(map[string]any{
		"pairs": map[string]any{
			"$type": "list",
			"$call": "list.crossproduct",
			"$args": []any{
				map[string]any{"$ref": "workflow.years"},
				map[string]any{"$ref": "workflow.regions[0].code"},
			},
		},
	}, variable.NamespaceWorkflow)
	if err != nil {
		t.Fatalf("LoadVariables() error = %v", err)
	}

	expression := variables[0].TypedExpression
	if expression.Type != variable.TypeList {
		t.Fatalf("expression type = %s, want list", expression.Type)
	}
	call, ok := expression.Expression.(variable.FunctionCallExpression)
	if !ok {
		t.Fatalf("expression = %T, want FunctionCallExpression", expression.Expression)
	}
	if call.Name.String() != "list.crossproduct" || call.ResultType != variable.TypeList {
		t.Fatalf("call = %+v", call)
	}
	if got := call.Arguments[1].Expression; got != "workflow.regions[0].code" {
		t.Fatalf("second argument = %q", got)
	}
}

func TestLoadVariablesRejectsExpressionContainerDirective(t *testing.T) {
	_, err := LoadVariables(map[string]any{
		"pairs": map[string]any{"$expr": "list.crossproduct(A, B)"},
	}, variable.NamespaceWorkflow)
	if err == nil || !strings.Contains(err.Error(), "$expr is not supported") {
		t.Fatalf("LoadVariables() error = %v, want $expr rejection", err)
	}
}

func TestLoadVariablesRejectsAmbiguousDirectiveObjects(t *testing.T) {
	tests := []struct {
		name  string
		value map[string]any
		want  string
	}{
		{
			name:  "missing type",
			value: map[string]any{"$call": "list.crossproduct", "$args": []any{map[string]any{"$ref": "A"}}},
			want:  "expression directive must contain exactly",
		},
		{
			name:  "extra field",
			value: map[string]any{"$type": "list", "$call": "list.crossproduct", "$args": []any{map[string]any{"$ref": "A"}}, "note": "ordinary"},
			want:  "expression directive must contain exactly",
		},
		{
			name:  "ref outside args",
			value: map[string]any{"$ref": "workflow.years"},
			want:  "$ref is only valid inside $args",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadVariables(map[string]any{"value": test.value}, variable.NamespaceWorkflow)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("LoadVariables() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLoadVariablesKeepsNonReservedDollarObjectsOrdinary(t *testing.T) {
	variables, err := LoadVariables(map[string]any{
		"metadata": map[string]any{
			"$comment": "not a directive",
			"value":    "kept",
		},
	}, variable.NamespaceWorkflow)
	if err != nil {
		t.Fatalf("LoadVariables() error = %v", err)
	}
	fields := variables[0].Expression.(map[string]variable.TypedExpression)
	if fields["$comment"].Type != variable.TypeString || fields["value"].Type != variable.TypeString {
		t.Fatalf("fields = %+v", fields)
	}
}

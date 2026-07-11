package document

import (
	"encoding/json"
	"reflect"
	"testing"

	"goetl/internal/variable"
)

func TestLoadVariablesAppliesSourceNamespaceAndSorts(t *testing.T) {
	variables, err := LoadVariables(map[string]any{
		"zeta":    "last",
		"alpha":   int64(1),
		"enabled": true,
	}, variable.NamespaceWorkflow)
	if err != nil {
		t.Fatalf("LoadVariables() error = %v", err)
	}

	gotNames := []variable.Name{variables[0].Name, variables[1].Name, variables[2].Name}
	wantNames := []variable.Name{
		{Namespace: variable.NamespaceWorkflow, Key: "alpha"},
		{Namespace: variable.NamespaceWorkflow, Key: "enabled"},
		{Namespace: variable.NamespaceWorkflow, Key: "zeta"},
	}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("variable names = %+v, want %+v", gotNames, wantNames)
	}
}

func TestLoadVariablesPreservesRecursiveTypes(t *testing.T) {
	variables, err := LoadVariables(map[string]any{
		"settings": map[string]any{
			"script": "run.py",
			"count":  int64(2),
			"items": []any{
				"one",
				int64(3),
				map[string]any{"enabled": true},
			},
		},
	}, variable.NamespaceProjectConfig)
	if err != nil {
		t.Fatalf("LoadVariables() error = %v", err)
	}
	if len(variables) != 1 {
		t.Fatalf("variable count = %d, want 1", len(variables))
	}

	settings := variables[0].TypedExpression
	if settings.Type != variable.TypeObject {
		t.Fatalf("settings type = %s, want object", settings.Type)
	}
	fields := settings.Expression.(map[string]variable.TypedExpression)
	if fields["script"].Type != variable.TypeString || fields["script"].Expression != "run.py" {
		t.Fatalf("script field = %+v", fields["script"])
	}
	if fields["count"].Type != variable.TypeInt || fields["count"].Expression != 2 {
		t.Fatalf("count field = %+v", fields["count"])
	}
	items := fields["items"].Expression.([]variable.TypedExpression)
	if items[0].Type != variable.TypeString || items[1].Type != variable.TypeInt || items[2].Type != variable.TypeObject {
		t.Fatalf("items = %+v", items)
	}
}

func TestLoadVariablesSupportsCanonicalDecoderValues(t *testing.T) {
	value, err := DecodeSource([]byte(`
variables:
  tiles:
    - h18v07
    - h18v08
  year: 2026
  enabled: true
`), DecodeOptions{Path: "workflow.yaml"})
	if err != nil {
		t.Fatalf("DecodeSource() error = %v", err)
	}

	root := value.(map[string]any)
	variableValues := root["variables"].(map[string]any)
	variables, err := LoadVariables(variableValues, variable.NamespaceWorkflow)
	if err != nil {
		t.Fatalf("LoadVariables() error = %v", err)
	}

	if variables[0].Name != (variable.Name{Namespace: variable.NamespaceWorkflow, Key: "enabled"}) {
		t.Fatalf("first variable = %+v, want workflow.enabled", variables[0].Name)
	}
	if variables[1].Name != (variable.Name{Namespace: variable.NamespaceWorkflow, Key: "tiles"}) {
		t.Fatalf("second variable = %+v, want workflow.tiles", variables[1].Name)
	}
	if variables[2].Name != (variable.Name{Namespace: variable.NamespaceWorkflow, Key: "year"}) {
		t.Fatalf("third variable = %+v, want workflow.year", variables[2].Name)
	}
}

func TestLoadVariablesRejectsUnsupportedValues(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]any
	}{
		{name: "null", values: map[string]any{"value": nil}},
		{name: "float", values: map[string]any{"value": 1.5}},
		{name: "fractional json number", values: map[string]any{"value": json.Number("1.5")}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := LoadVariables(test.values, variable.NamespaceProjectConfig); err == nil {
				t.Fatal("LoadVariables() expected error")
			}
		})
	}
}

func TestLoadVariablesRejectsUnsupportedNamespace(t *testing.T) {
	_, err := LoadVariables(map[string]any{"value": "alpha"}, variable.NamespaceRuntime)
	if err == nil {
		t.Fatal("LoadVariables() expected unsupported namespace error")
	}
}

package variable

import (
	"encoding/json"
	"strings"
	"testing"
)

const sensitiveSentinel = "goet-secret-sentinel-001"
const controlledSinkSentinel007 = "goet-secret-sentinel-007-do-not-persist"

func TestVariableJSONRoundTripIncludesSensitiveFlag(t *testing.T) {
	text := `{
		"name":{"namespace":"workflow","key":"api_token"},
		"type":"string",
		"expression":"goet-secret-sentinel-001",
		"sensitive":true
	}`

	var value Variable
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !value.Sensitive {
		t.Fatal("expected variable to be sensitive")
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	assertJSONEqual(t, encoded, []byte(text))
}

func TestParseLiteralMarksSensitiveValue(t *testing.T) {
	value, err := ParseLiteral(Variable{
		Name:            Name{Namespace: NamespaceWorkflow, Key: "api_token"},
		TypedExpression: TypedExpression{Type: TypeString, Expression: sensitiveSentinel},
		Sensitive:       true,
	})
	if err != nil {
		t.Fatalf("ParseLiteral() error = %v", err)
	}
	if !value.Sensitive {
		t.Fatal("expected sensitive resolved value")
	}
	if value.Provenance != "workflow.api_token" {
		t.Fatalf("provenance = %q, want workflow.api_token", value.Provenance)
	}
	if value.RedactionLabel == "" {
		t.Fatal("expected redaction label")
	}
}

func TestResolvedValueSafeRenderingRedactsSensitiveScalar(t *testing.T) {
	value := ResolvedValue{
		Type:           TypeString,
		Value:          sensitiveSentinel,
		Sensitive:      true,
		RedactionLabel: "[REDACTED:workflow.api_token]",
		Provenance:     "workflow.api_token",
	}

	if got := value.String(); strings.Contains(got, sensitiveSentinel) {
		t.Fatalf("String() leaked sentinel: %q", got)
	}
	if got := value.GoString(); strings.Contains(got, sensitiveSentinel) {
		t.Fatalf("GoString() leaked sentinel: %q", got)
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}
	if strings.Contains(string(encoded), sensitiveSentinel) {
		t.Fatalf("MarshalJSON() leaked sentinel: %s", encoded)
	}
	if !strings.Contains(string(encoded), `"sensitive":true`) {
		t.Fatalf("MarshalJSON() = %s, want sensitive metadata", encoded)
	}
}

func TestResolvedValueSafeRenderingRedactsControlledSinkSentinel(t *testing.T) {
	value := ResolvedValue{
		Type:           TypeString,
		Value:          controlledSinkSentinel007,
		Sensitive:      true,
		RedactionLabel: "[REDACTED:workflow.api_token]",
		Provenance:     "workflow.api_token",
	}

	rendered := []string{value.String(), value.GoString()}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}
	rendered = append(rendered, string(encoded))

	for _, text := range rendered {
		if strings.Contains(text, controlledSinkSentinel007) {
			t.Fatalf("safe rendering leaked sentinel: %s", text)
		}
	}
}

func TestResolvedValueSafeRenderingRedactsSensitiveAggregate(t *testing.T) {
	value := ResolvedObject(map[string]ResolvedValue{
		"name": {Type: TypeString, Value: "orders"},
		"token": {
			Type:           TypeString,
			Value:          sensitiveSentinel,
			Sensitive:      true,
			RedactionLabel: "[REDACTED:workflow.api_token]",
			Provenance:     "workflow.api_token",
		},
	})

	if !value.Sensitive {
		t.Fatal("expected aggregate to be sensitive")
	}

	text, err := safeJSONText(value)
	if err != nil {
		t.Fatalf("safeJSONText() error = %v", err)
	}
	if strings.Contains(text, sensitiveSentinel) {
		t.Fatalf("safeJSONText() leaked sentinel: %s", text)
	}
	if !strings.Contains(text, `"name":{"type":"string","value":"orders"}`) {
		t.Fatalf("safeJSONText() = %s, want public field preserved", text)
	}
}

func TestResolverPropagatesSensitivityThroughReferenceWithoutDeclassification(t *testing.T) {
	scope, err := NewScope(
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "api_token"}, TypedExpression: TypedExpression{Type: TypeString, Expression: sensitiveSentinel}, Sensitive: true},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "alias"}, TypedExpression: TypedExpression{Type: TypeString, Expression: "${api_token}"}},
	)
	if err != nil {
		t.Fatal(err)
	}

	value, err := NewResolver(NewSet(scope), ResolverConfig{}).Resolve(Reference{
		Name: Name{Namespace: NamespaceWorkflow, Key: "alias"},
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if !value.Sensitive {
		t.Fatal("expected reference target to remain sensitive")
	}
	if value.Value != sensitiveSentinel {
		t.Fatalf("value = %#v, want sentinel plaintext in memory", value.Value)
	}
}

func TestResolverPropagatesSensitivityThroughObjectListAccessorAndInterpolation(t *testing.T) {
	scope, err := NewScope(
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "token"}, TypedExpression: TypedExpression{Type: TypeString, Expression: sensitiveSentinel}, Sensitive: true},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "record"}, TypedExpression: TypedExpression{Type: TypeObject, Expression: map[string]TypedExpression{
			"token": {Type: TypeString, Expression: "${token}"},
			"name":  {Type: TypeString, Expression: "orders"},
		}}},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "values"}, TypedExpression: TypedExpression{Type: TypeList, Expression: []TypedExpression{
			{Type: TypeString, Expression: "public"},
			{Type: TypeString, Expression: "${token}"},
		}}},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "token_copy"}, TypedExpression: TypedExpression{Type: TypeString, Expression: "${record.token}"}},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "list_copy"}, TypedExpression: TypedExpression{Type: TypeString, Expression: "${values[1]}"}},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "label"}, TypedExpression: TypedExpression{Type: TypeString, Expression: "token=${token}"}},
	)
	if err != nil {
		t.Fatal(err)
	}

	resolver := NewResolver(NewSet(scope), ResolverConfig{})

	record, err := resolver.Resolve(Reference{Name: Name{Namespace: NamespaceWorkflow, Key: "record"}, Qualified: true})
	if err != nil {
		t.Fatalf("record resolve error = %v", err)
	}
	if !record.Sensitive {
		t.Fatal("expected object aggregate to be sensitive")
	}
	if record.Object["name"].Sensitive {
		t.Fatal("public object field should remain non-sensitive")
	}
	if !record.Object["token"].Sensitive {
		t.Fatal("expected sensitive object field")
	}

	values, err := resolver.Resolve(Reference{Name: Name{Namespace: NamespaceWorkflow, Key: "values"}, Qualified: true})
	if err != nil {
		t.Fatalf("values resolve error = %v", err)
	}
	if !values.Sensitive {
		t.Fatal("expected list aggregate to be sensitive")
	}
	if values.List[0].Sensitive {
		t.Fatal("public list item should remain non-sensitive")
	}
	if !values.List[1].Sensitive {
		t.Fatal("expected sensitive list item")
	}

	tokenCopy, err := resolver.Resolve(Reference{Name: Name{Namespace: NamespaceWorkflow, Key: "token_copy"}, Qualified: true})
	if err != nil {
		t.Fatalf("token_copy resolve error = %v", err)
	}
	if !tokenCopy.Sensitive {
		t.Fatal("expected field accessor result to be sensitive")
	}

	listCopy, err := resolver.Resolve(Reference{Name: Name{Namespace: NamespaceWorkflow, Key: "list_copy"}, Qualified: true})
	if err != nil {
		t.Fatalf("list_copy resolve error = %v", err)
	}
	if !listCopy.Sensitive {
		t.Fatal("expected index accessor result to be sensitive")
	}

	label, err := resolver.Resolve(Reference{Name: Name{Namespace: NamespaceWorkflow, Key: "label"}, Qualified: true})
	if err != nil {
		t.Fatalf("label resolve error = %v", err)
	}
	if !label.Sensitive {
		t.Fatal("expected interpolation result to be sensitive")
	}
	if got, ok := label.Value.(string); !ok || got != "token="+sensitiveSentinel {
		t.Fatalf("label value = %#v, want interpolated plaintext in memory", label.Value)
	}
	if strings.Contains(label.String(), sensitiveSentinel) {
		t.Fatalf("label.String() leaked sentinel: %q", label.String())
	}
}

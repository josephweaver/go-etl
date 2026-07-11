package variable

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseFunctionNameRequiresNamespace(t *testing.T) {
	name, err := ParseFunctionName("list.crossproduct")
	if err != nil {
		t.Fatalf("ParseFunctionName() error = %v", err)
	}
	if name.Namespace != "list" || name.Name != "crossproduct" {
		t.Fatalf("function name = %+v", name)
	}

	tests := []string{"crossproduct", "list.", ".crossproduct", "list.cross.product", "list.1bad"}
	for _, text := range tests {
		t.Run(text, func(t *testing.T) {
			if _, err := ParseFunctionName(text); err == nil {
				t.Fatal("ParseFunctionName() expected error")
			}
		})
	}
}

func TestFunctionArgumentReferenceValidatesReferenceAccessors(t *testing.T) {
	arg, err := NewFunctionArgumentReference("workflow.records[0].tile")
	if err != nil {
		t.Fatalf("NewFunctionArgumentReference() error = %v", err)
	}
	if arg.Reference.Name.Namespace != NamespaceWorkflow || arg.Reference.Name.Key != "records" || arg.Accessor != "[0].tile" {
		t.Fatalf("argument = %+v", arg)
	}

	tests := []string{`"literal"`, "1", "true", "list.length(A)", "A + B", "items[*]"}
	for _, text := range tests {
		t.Run(text, func(t *testing.T) {
			if _, err := NewFunctionArgumentReference(text); err == nil {
				t.Fatal("NewFunctionArgumentReference() expected error")
			}
		})
	}
}

func TestFunctionCallExpressionValidation(t *testing.T) {
	name, err := ParseFunctionName("list.crossproduct")
	if err != nil {
		t.Fatal(err)
	}
	arg, err := NewFunctionArgumentReference("workflow.years")
	if err != nil {
		t.Fatal(err)
	}
	call, err := NewFunctionCallExpression(name, TypeList, []FunctionArgumentReference{arg})
	if err != nil {
		t.Fatalf("NewFunctionCallExpression() error = %v", err)
	}
	expression := TypedExpression{Type: TypeList, Expression: call}
	if err := expression.ValidateDefinition(); err != nil {
		t.Fatalf("ValidateDefinition() error = %v", err)
	}

	expression.Type = TypeString
	if err := expression.ValidateDefinition(); err == nil {
		t.Fatal("ValidateDefinition() expected result type mismatch")
	}
}

func TestTypedExpressionJSONDoesNotAcceptExpressionContainerPayload(t *testing.T) {
	var expression TypedExpression
	err := json.Unmarshal([]byte(`{"type":"list","expression":{"$expr":"list.crossproduct(A, B)"}}`), &expression)
	if err == nil {
		t.Fatal("Unmarshal() expected $expr payload rejection")
	}
}

func TestFunctionCallExpressionMarshalUsesSemanticShape(t *testing.T) {
	name, err := ParseFunctionName("list.crossproduct")
	if err != nil {
		t.Fatal(err)
	}
	arg, err := NewFunctionArgumentReference("workflow.years")
	if err != nil {
		t.Fatal(err)
	}
	call, err := NewFunctionCallExpression(name, TypeList, []FunctionArgumentReference{arg})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(call)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	text := string(data)
	if strings.Contains(text, "$call") || strings.Contains(text, "$ref") || strings.Contains(text, "$type") {
		t.Fatalf("semantic call JSON contains public directive keys: %s", text)
	}
	if !strings.Contains(text, `"name":"list.crossproduct"`) || !strings.Contains(text, `"result_type":"list"`) {
		t.Fatalf("semantic call JSON = %s", text)
	}
}

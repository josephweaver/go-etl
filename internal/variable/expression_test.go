package variable

import (
	"encoding/json"
	"testing"
)

func TestTypedExpressionRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{name: "string", text: `{"type":"string","expression":"alpha"}`},
		{name: "int", text: `{"type":"int","expression":2}`},
		{name: "bool", text: `{"type":"bool","expression":true}`},
		{name: "empty object", text: `{"type":"object","expression":{}}`},
		{name: "empty list", text: `{"type":"list","expression":[]}`},
		{
			name: "nested object and list",
			text: `{"type":"object","expression":{"values":{"type":"list","expression":[` +
				`{"type":"string","expression":"alpha"},` +
				`{"type":"int","expression":2},` +
				`{"type":"list","expression":[{"type":"bool","expression":true}]}` +
				`]}}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var expression TypedExpression
			if err := json.Unmarshal([]byte(test.text), &expression); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			encoded, err := json.Marshal(expression)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			assertJSONEqual(t, encoded, []byte(test.text))
		})
	}
}

func TestTypedExpressionDecodesRecursiveNodes(t *testing.T) {
	var expression TypedExpression
	err := json.Unmarshal([]byte(`{
		"type": "object",
		"expression": {
			"values": {
				"type": "list",
				"expression": [
					{"type": "string", "expression": "alpha"},
					{"type": "int", "expression": 2}
				]
			}
		}
	}`), &expression)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	fields := expression.Expression.(map[string]TypedExpression)
	items := fields["values"].Expression.([]TypedExpression)
	if items[0].Type != TypeString || items[1].Type != TypeInt {
		t.Fatalf("unexpected item types: %s, %s", items[0].Type, items[1].Type)
	}
}

func TestTypedExpressionRejectsInvalidJSONShape(t *testing.T) {
	tests := []string{
		`{`,
		`{"expression":"alpha"}`,
		`{"type":"string"}`,
		`{"type":"unknown","expression":"alpha"}`,
		`{"type":"object","expression":[]}`,
		`{"type":"object","expression":null}`,
		`{"type":"list","expression":{}}`,
		`{"type":"list","expression":null}`,
		`{"type":"string","expression":"alpha","extra":true}`,
	}

	for _, text := range tests {
		t.Run(text, func(t *testing.T) {
			var expression TypedExpression
			if err := json.Unmarshal([]byte(text), &expression); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestTypedExpressionMarshalRejectsInvalidContainer(t *testing.T) {
	tests := []TypedExpression{
		{Type: TypeObject, Expression: []TypedExpression{}},
		{Type: TypeList, Expression: map[string]TypedExpression{}},
		{Type: Type{Kind: "unknown"}, Expression: "alpha"},
	}

	for _, expression := range tests {
		if _, err := json.Marshal(expression); err == nil {
			t.Fatalf("expected an error for %#v", expression)
		}
	}
}

func assertJSONEqual(t *testing.T, got []byte, want []byte) {
	t.Helper()

	var gotValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("unmarshal got JSON: %v", err)
	}

	var wantValue any
	if err := json.Unmarshal(want, &wantValue); err != nil {
		t.Fatalf("unmarshal want JSON: %v", err)
	}

	gotJSON, err := json.Marshal(gotValue)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.Marshal(wantValue)
	if err != nil {
		t.Fatal(err)
	}

	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("JSON mismatch:\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
}

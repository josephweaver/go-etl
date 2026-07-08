package variable

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestVariableJSONRoundTripIncludesProtectedReference(t *testing.T) {
	text := `{
		"name":{"namespace":"workflow","key":"gdrive_token"},
		"type":"string",
		"protected_ref":{"provider":"worker_env","key":"GOET_GDRIVE_TOKEN"}
	}`

	var value Variable
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !value.Sensitive {
		t.Fatal("expected protected reference to be sensitive")
	}
	if value.ProtectedRef == nil {
		t.Fatal("expected protected reference")
	}
	if got, want := value.ProtectedRef.RedactionLabel, "${worker_env.GOET_GDRIVE_TOKEN}"; got != want {
		t.Fatalf("redaction label = %q, want %q", got, want)
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	expected := `{
		"name":{"namespace":"workflow","key":"gdrive_token"},
		"type":"string",
		"sensitive":true,
		"protected_ref":{
			"provider":"worker_env",
			"key":"GOET_GDRIVE_TOKEN",
			"redaction_label":"${worker_env.GOET_GDRIVE_TOKEN}"
		}
	}`
	assertJSONEqual(t, encoded, []byte(expected))
}

func TestProtectedReferenceJSONRejectsInvalidProviderAndKey(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{
			name: "missing provider",
			text: `{
				"name":{"namespace":"workflow","key":"gdrive_token"},
				"type":"string",
				"protected_ref":{"key":"GOET_GDRIVE_TOKEN"}
			}`,
		},
		{
			name: "missing key",
			text: `{
				"name":{"namespace":"workflow","key":"gdrive_token"},
				"type":"string",
				"protected_ref":{"provider":"worker_env"}
			}`,
		},
		{
			name: "unsupported provider",
			text: `{
				"name":{"namespace":"workflow","key":"gdrive_token"},
				"type":"string",
				"protected_ref":{"provider":"client_env","key":"GOET_GDRIVE_TOKEN"}
			}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var value Variable
			if err := json.Unmarshal([]byte(test.text), &value); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestProtectedReferenceAllowsTestProvider(t *testing.T) {
	ref := ProtectedRef{Provider: protectedRefProviderTest, Key: "TOKEN"}
	if err := ref.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if got, want := ref.Normalize().RedactionLabel, "${test.TOKEN}"; got != want {
		t.Fatalf("Normalize().RedactionLabel = %q, want %q", got, want)
	}
}

func TestParseLiteralPreservesProtectedReferenceWithoutPlaintext(t *testing.T) {
	value, err := ParseLiteral(Variable{
		Name:            Name{Namespace: NamespaceWorkflow, Key: "gdrive_token"},
		TypedExpression: TypedExpression{Type: TypeString},
		ProtectedRef:    &ProtectedRef{Provider: protectedRefProviderWorkerEnv, Key: "GOET_GDRIVE_TOKEN"},
	})
	if err != nil {
		t.Fatalf("ParseLiteral() error = %v", err)
	}

	if !value.Sensitive {
		t.Fatal("expected resolved value to be sensitive")
	}
	if value.ProtectedRef == nil {
		t.Fatal("expected protected reference on resolved value")
	}
	if value.Value != nil {
		t.Fatalf("value = %#v, want no plaintext", value.Value)
	}
	if got, want := value.String(), "${worker_env.GOET_GDRIVE_TOKEN}"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}

	encoded, err := safeJSONText(value)
	if err != nil {
		t.Fatalf("safeJSONText() error = %v", err)
	}
	if strings.Contains(encoded, `"value"`) {
		t.Fatalf("safeJSONText() unexpectedly included value field: %s", encoded)
	}
	if !strings.Contains(encoded, `"protected_ref"`) {
		t.Fatalf("safeJSONText() = %s, want protected_ref", encoded)
	}
}

func TestResolverPreservesProtectedReferenceWithoutPlaintext(t *testing.T) {
	scope, err := NewScope(Variable{
		Name:            Name{Namespace: NamespaceWorkflow, Key: "gdrive_token"},
		TypedExpression: TypedExpression{Type: TypeString},
		ProtectedRef:    &ProtectedRef{Provider: protectedRefProviderWorkerEnv, Key: "GOET_GDRIVE_TOKEN"},
	})
	if err != nil {
		t.Fatal(err)
	}

	value, err := NewResolver(NewSet(scope), ResolverConfig{}).Resolve(Reference{
		Name:      Name{Namespace: NamespaceWorkflow, Key: "gdrive_token"},
		Qualified: true,
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if !value.Sensitive {
		t.Fatal("expected resolved value to be sensitive")
	}
	if value.ProtectedRef == nil {
		t.Fatal("expected protected reference on resolved value")
	}
	if value.Value != nil {
		t.Fatalf("value = %#v, want no plaintext", value.Value)
	}
	if got, want := value.String(), "${worker_env.GOET_GDRIVE_TOKEN}"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}

	encoded, err := safeJSONText(value)
	if err != nil {
		t.Fatalf("safeJSONText() error = %v", err)
	}
	if strings.Contains(encoded, `"value"`) {
		t.Fatalf("safeJSONText() unexpectedly included value field: %s", encoded)
	}
	if !strings.Contains(encoded, `"protected_ref"`) {
		t.Fatalf("safeJSONText() = %s, want protected_ref", encoded)
	}
}

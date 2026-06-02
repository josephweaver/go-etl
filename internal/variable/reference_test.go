package variable

import "testing"

func TestParseReference(t *testing.T) {
	tests := []struct {
		name          string
		text          string
		wantNamespace Namespace
		wantKey       string
		wantQualified bool
	}{
		{
			name:    "unqualified",
			text:    "data_dir",
			wantKey: "data_dir",
		},
		{
			name:          "qualified",
			text:          "workflow.data_dir",
			wantNamespace: NamespaceWorkflow,
			wantKey:       "data_dir",
			wantQualified: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ref, err := ParseReference(test.text)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if ref.Name.Namespace != test.wantNamespace {
				t.Fatalf("unexpected namespace: %q", ref.Name.Namespace)
			}

			if ref.Name.Key != test.wantKey {
				t.Fatalf("unexpected key: %q", ref.Name.Key)
			}

			if ref.Qualified != test.wantQualified {
				t.Fatalf("unexpected qualified value: %t", ref.Qualified)
			}
		})
	}
}

func TestParseReferenceRejectsInvalidInput(t *testing.T) {
	tests := []string{
		"",
		"unknown.data_dir",
		"workflow.",
		"a.b.c",
	}

	for _, text := range tests {
		t.Run(text, func(t *testing.T) {
			if _, err := ParseReference(text); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

package variable

import "testing"

func TestTypeValid(t *testing.T) {
	tests := []struct {
		name  string
		typ   Type
		valid bool
	}{
		{name: "string", typ: TypeString, valid: true},
		{name: "int", typ: TypeInt, valid: true},
		{name: "bool", typ: TypeBool, valid: true},
		{name: "datetime", typ: TypeDatetime, valid: true},
		{name: "path", typ: TypePath, valid: true},
		{name: "list", typ: TypeList, valid: true},
		{name: "object", typ: TypeObject, valid: true},
		{name: "empty", typ: Type{}, valid: false},
		{name: "unknown", typ: Type{Kind: "unknown"}, valid: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.typ.Valid(); got != test.valid {
				t.Fatalf("unexpected validity for %s: %t", test.typ, got)
			}
		})
	}
}

func TestTypeString(t *testing.T) {
	tests := []struct {
		name string
		typ  Type
		want string
	}{
		{name: "scalar", typ: TypeString, want: "string"},
		{name: "object", typ: TypeObject, want: "object"},
		{name: "list", typ: TypeList, want: "list"},
		{name: "empty", typ: Type{}, want: "<empty>"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.typ.String(); got != test.want {
				t.Fatalf("unexpected string: %q", got)
			}
		})
	}
}

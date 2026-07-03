package fingerprint

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCanonicalJSONSortsObjectKeys(t *testing.T) {
	left := map[string]any{
		"z": "last",
		"a": "first",
		"m": map[string]any{
			"b": 2,
			"a": 1,
		},
	}
	right := map[string]any{
		"m": map[string]any{
			"a": 1,
			"b": 2,
		},
		"a": "first",
		"z": "last",
	}

	leftCanonical, leftHash, err := CanonicalJSONSHA256(left)
	if err != nil {
		t.Fatalf("CanonicalJSONSHA256(left) error = %v", err)
	}
	rightCanonical, rightHash, err := CanonicalJSONSHA256(right)
	if err != nil {
		t.Fatalf("CanonicalJSONSHA256(right) error = %v", err)
	}

	want := `{"a":"first","m":{"a":1,"b":2},"z":"last"}`
	if string(leftCanonical) != want {
		t.Fatalf("canonical = %s, want %s", leftCanonical, want)
	}
	if string(rightCanonical) != want {
		t.Fatalf("right canonical = %s, want %s", rightCanonical, want)
	}
	if leftHash != rightHash {
		t.Fatalf("hash mismatch: %s != %s", leftHash, rightHash)
	}
}

func TestCanonicalJSONPreservesListOrder(t *testing.T) {
	canonical, err := CanonicalJSON([]any{2, 1, 3})
	if err != nil {
		t.Fatalf("CanonicalJSON() error = %v", err)
	}
	if string(canonical) != `[2,1,3]` {
		t.Fatalf("canonical = %s, want [2,1,3]", canonical)
	}
}

func TestCanonicalJSONDistinguishesNullAndMissing(t *testing.T) {
	withNull, err := CanonicalJSON(map[string]any{"a": nil})
	if err != nil {
		t.Fatalf("CanonicalJSON(withNull) error = %v", err)
	}
	missing, err := CanonicalJSON(map[string]any{})
	if err != nil {
		t.Fatalf("CanonicalJSON(missing) error = %v", err)
	}

	if string(withNull) != `{"a":null}` {
		t.Fatalf("withNull = %s, want null field", withNull)
	}
	if string(missing) != `{}` {
		t.Fatalf("missing = %s, want empty object", missing)
	}
	if string(withNull) == string(missing) {
		t.Fatal("null and missing produced the same canonical JSON")
	}
}

func TestCanonicalJSONAcceptsIntegerJSONNumbers(t *testing.T) {
	value := decodeJSON(t, `{"negative":-7,"positive":42,"zero":0}`)

	canonical, err := CanonicalJSON(value)
	if err != nil {
		t.Fatalf("CanonicalJSON() error = %v", err)
	}
	want := `{"negative":-7,"positive":42,"zero":0}`
	if string(canonical) != want {
		t.Fatalf("canonical = %s, want %s", canonical, want)
	}
}

func TestCanonicalJSONRejectsDecimalAndExponentNumbers(t *testing.T) {
	tests := []string{
		`{"value":1.5}`,
		`{"value":1e3}`,
		`{"value":1E3}`,
	}

	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			value := decodeJSON(t, raw)
			_, err := CanonicalJSON(value)
			if err == nil || !strings.Contains(err.Error(), "integer numbers") {
				t.Fatalf("CanonicalJSON() error = %v, want integer rejection", err)
			}
		})
	}
}

func TestCanonicalJSONRejectsFloatingPointValues(t *testing.T) {
	_, err := CanonicalJSON(map[string]any{"value": 1.5})
	if err == nil || !strings.Contains(err.Error(), "floating-point") {
		t.Fatalf("CanonicalJSON() error = %v, want floating-point rejection", err)
	}
}

func TestCanonicalJSONRepresentsDecimalsAsStrings(t *testing.T) {
	canonical, err := CanonicalJSON(map[string]any{"threshold": "0.125"})
	if err != nil {
		t.Fatalf("CanonicalJSON() error = %v", err)
	}
	if string(canonical) != `{"threshold":"0.125"}` {
		t.Fatalf("canonical = %s, want decimal string", canonical)
	}
}

func TestSHA256Hex(t *testing.T) {
	hash := SHA256Hex([]byte("abc"))
	want := "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if hash != want {
		t.Fatalf("hash = %s, want %s", hash, want)
	}
}

func TestValidateSHA256Hex(t *testing.T) {
	valid := "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if err := ValidateSHA256Hex(valid); err != nil {
		t.Fatalf("ValidateSHA256Hex(valid) error = %v", err)
	}

	tests := []string{
		valid[:len(valid)-1],
		"Ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		"za7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
	}
	for _, value := range tests {
		t.Run(value, func(t *testing.T) {
			if err := ValidateSHA256Hex(value); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func decodeJSON(t *testing.T, raw string) any {
	t.Helper()

	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	return value
}

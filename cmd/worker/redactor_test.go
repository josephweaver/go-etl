package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactorReplacesExactMaterializedSecretLiterally(t *testing.T) {
	redactor := NewRedactor()
	redactor.Register(NewSensitiveValue("string", `token.a[1]`, "[TOKEN]"))

	input := `stdout token.a[1] tokenXa1 token.a[1]`
	got := redactor.RedactString(input)
	want := `stdout [TOKEN] tokenXa1 [TOKEN]`
	if got != want {
		t.Fatalf("RedactString() = %q, want %q", got, want)
	}
}

func TestRedactorHandlesMultipleSecretsDeterministically(t *testing.T) {
	redactor := NewRedactor()
	redactor.RegisterPlaintext("secret", "[SHORT]")
	redactor.RegisterPlaintext("secret-value", "[LONG]")
	redactor.RegisterPlaintext("alpha", "[ALPHA]")

	input := "secret-value secret alpha secret-value"
	want := "[LONG] [SHORT] [ALPHA] [LONG]"
	for i := 0; i < 3; i++ {
		if got := redactor.RedactString(input); got != want {
			t.Fatalf("run %d RedactString() = %q, want %q", i, got, want)
		}
	}
}

func TestRedactorRedactsByteStreams(t *testing.T) {
	redactor := NewRedactor()
	redactor.RegisterPlaintext("plain-secret", "[TOKEN]")

	got := string(redactor.RedactBytes([]byte("before plain-secret after")))
	want := "before [TOKEN] after"
	if got != want {
		t.Fatalf("RedactBytes() = %q, want %q", got, want)
	}
}

func TestRedactorDoesNotPersistRegistry(t *testing.T) {
	redactor := NewRedactor()
	redactor.RegisterPlaintext("plain-secret", "[TOKEN]")

	encoded, err := json.Marshal(redactor)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}
	if strings.Contains(string(encoded), "plain-secret") {
		t.Fatalf("encoded redactor contains plaintext secret: %s", encoded)
	}

	nextAttempt := NewRedactor()
	if got := nextAttempt.RedactString("plain-secret"); got != "plain-secret" {
		t.Fatalf("new redactor retained prior registry, got %q", got)
	}
}

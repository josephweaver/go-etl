package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestWorkerEnvProtectedValueResolverResolvesWorkerEnv(t *testing.T) {
	t.Setenv("GOET_TEST_TOKEN", "plain-secret")

	resolver := WorkerEnvProtectedValueResolver{}
	value, err := resolver.ResolveProtectedValue(context.Background(), ProtectedValueRef{
		Type:           "string",
		Provider:       "worker_env",
		Key:            "GOET_TEST_TOKEN",
		RedactionLabel: "${worker_env.GOET_TEST_TOKEN}",
	})
	if err != nil {
		t.Fatalf("ResolveProtectedValue() error = %v", err)
	}

	if value.Type() != "string" {
		t.Fatalf("type = %q, want string", value.Type())
	}
	if value.Plaintext() != "plain-secret" {
		t.Fatalf("plaintext = %q", value.Plaintext())
	}
	if fmt.Sprint(value) != "${worker_env.GOET_TEST_TOKEN}" {
		t.Fatalf("formatted value = %q", fmt.Sprint(value))
	}
}

func TestWorkerEnvProtectedValueResolverMissingEnvReturnsSanitizedError(t *testing.T) {
	resolver := WorkerEnvProtectedValueResolver{LookupEnv: func(string) (string, bool) {
		return "", false
	}}

	_, err := resolver.ResolveProtectedValue(context.Background(), ProtectedValueRef{
		Type:           "string",
		Provider:       "worker_env",
		Key:            "GOET_MISSING_TOKEN",
		RedactionLabel: "${worker_env.GOET_MISSING_TOKEN}",
	})
	if err == nil {
		t.Fatal("expected an error")
	}

	text := err.Error()
	if !strings.Contains(text, "${worker_env.GOET_MISSING_TOKEN}") {
		t.Fatalf("error does not include redaction label: %q", text)
	}
	if strings.Contains(text, "plain-secret") {
		t.Fatalf("error contains plaintext secret: %q", text)
	}
}

func TestWorkerEnvProtectedValueResolverUnsupportedProviderReturnsSanitizedError(t *testing.T) {
	resolver := WorkerEnvProtectedValueResolver{}

	_, err := resolver.ResolveProtectedValue(context.Background(), ProtectedValueRef{
		Type:     "string",
		Provider: "vault",
		Key:      "do-not-echo-this-key",
	})
	if err == nil {
		t.Fatal("expected an error")
	}

	text := err.Error()
	if !strings.Contains(text, `unsupported protected value provider "vault"`) {
		t.Fatalf("unexpected error: %q", text)
	}
	if strings.Contains(text, "do-not-echo-this-key") {
		t.Fatalf("error contains protected reference key: %q", text)
	}
}

func TestSensitiveValueDefaultFormattingAndJSONAreRedacted(t *testing.T) {
	value := NewSensitiveValue("string", "plain-secret", "[TOKEN]")

	formats := []string{
		fmt.Sprint(value),
		fmt.Sprintf("%v", value),
		fmt.Sprintf("%+v", value),
		fmt.Sprintf("%#v", value),
		fmt.Sprintf("%q", value),
		value.Error(),
	}
	for _, formatted := range formats {
		if strings.Contains(formatted, "plain-secret") {
			t.Fatalf("formatted value contains plaintext: %q", formatted)
		}
		if !strings.Contains(formatted, "[TOKEN]") {
			t.Fatalf("formatted value does not contain redaction label: %q", formatted)
		}
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}
	if strings.Contains(string(encoded), "plain-secret") {
		t.Fatalf("json contains plaintext: %s", encoded)
	}
	if !strings.Contains(string(encoded), "[TOKEN]") {
		t.Fatalf("json does not contain redaction label: %s", encoded)
	}

	if value.Plaintext() != "plain-secret" {
		t.Fatalf("plaintext accessor = %q", value.Plaintext())
	}
}

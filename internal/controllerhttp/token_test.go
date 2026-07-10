package controllerhttp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestSensitiveTokenRedactsFormatting(t *testing.T) {
	token, err := NewSensitiveToken("secret-sentinel")
	if err != nil {
		t.Fatalf("NewSensitiveToken() error = %v", err)
	}

	formatted := []string{
		token.String(),
		token.GoString(),
		token.Error(),
		fmt.Sprintf("%s", token),
		fmt.Sprintf("%#v", token),
	}
	for _, text := range formatted {
		if strings.Contains(text, "secret-sentinel") {
			t.Fatalf("formatted token leaked plaintext: %q", text)
		}
		if !strings.Contains(text, tokenRedactionLabel) {
			t.Fatalf("formatted token missing redaction label: %q", text)
		}
	}

	encoded, err := json.Marshal(token)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}
	if strings.Contains(string(encoded), "secret-sentinel") {
		t.Fatalf("encoded token leaked plaintext: %s", encoded)
	}
}

func TestNewSensitiveTokenRejectsInvalidBearerValues(t *testing.T) {
	for _, value := range []string{"", "two words", "tab\tvalue", "line\nvalue", "comma,value"} {
		t.Run(value, func(t *testing.T) {
			if _, err := NewSensitiveToken(value); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestStaticTokenProviderHonorsContextCancellation(t *testing.T) {
	token, err := NewSensitiveToken("secret-sentinel")
	if err != nil {
		t.Fatalf("NewSensitiveToken() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = NewStaticTokenProvider(token).Token(ctx)
	if err == nil {
		t.Fatal("expected canceled context error")
	}
}

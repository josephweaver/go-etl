package controllerhttp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const tokenRedactionLabel = "[REDACTED:controller-token]"

type TokenProvider interface {
	Token(context.Context) (SensitiveToken, error)
}

type SensitiveToken struct {
	value string
}

type StaticTokenProvider struct {
	token SensitiveToken
}

func NewSensitiveToken(value string) (SensitiveToken, error) {
	if value == "" {
		return SensitiveToken{}, fmt.Errorf("controller token is required")
	}
	if strings.ContainsAny(value, " \t\r\n,") {
		return SensitiveToken{}, fmt.Errorf("controller token contains characters that are not valid in bearer authorization")
	}
	return SensitiveToken{value: value}, nil
}

func NewStaticTokenProvider(token SensitiveToken) StaticTokenProvider {
	return StaticTokenProvider{token: token}
}

func (p StaticTokenProvider) Token(ctx context.Context) (SensitiveToken, error) {
	if err := ctx.Err(); err != nil {
		return SensitiveToken{}, err
	}
	if p.token.value == "" {
		return SensitiveToken{}, fmt.Errorf("controller token is required")
	}
	return p.token, nil
}

func (t SensitiveToken) Plaintext() string {
	return t.value
}

func (t SensitiveToken) String() string {
	return tokenRedactionLabel
}

func (t SensitiveToken) GoString() string {
	return tokenRedactionLabel
}

func (t SensitiveToken) Error() string {
	return tokenRedactionLabel
}

func (t SensitiveToken) MarshalJSON() ([]byte, error) {
	return json.Marshal(tokenRedactionLabel)
}

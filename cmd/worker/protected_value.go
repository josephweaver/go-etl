package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"goetl/internal/model"
)

const (
	protectedValueProviderWorkerEnv = "worker_env"
	sensitiveValueRedactedLabel     = "[REDACTED]"
)

type ProtectedValueRef = model.ExecutionEnvelopeProtectedReference

type ProtectedValueResolver interface {
	ResolveProtectedValue(ctx context.Context, ref ProtectedValueRef) (SensitiveValue, error)
}

type WorkerEnvProtectedValueResolver struct {
	LookupEnv func(string) (string, bool)
}

func (r WorkerEnvProtectedValueResolver) ResolveProtectedValue(ctx context.Context, ref ProtectedValueRef) (SensitiveValue, error) {
	select {
	case <-ctx.Done():
		return SensitiveValue{}, fmt.Errorf("resolve protected value: %w", ctx.Err())
	default:
	}

	provider := strings.TrimSpace(ref.Provider)
	if provider != protectedValueProviderWorkerEnv {
		return SensitiveValue{}, fmt.Errorf("unsupported protected value provider %q", provider)
	}

	key := strings.TrimSpace(ref.Key)
	if key == "" {
		return SensitiveValue{}, fmt.Errorf("protected value key is required")
	}

	lookupEnv := r.LookupEnv
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}

	plaintext, ok := lookupEnv(key)
	if !ok {
		return SensitiveValue{}, fmt.Errorf("protected value %s is unavailable in worker environment", protectedValueRedactionLabel(ref))
	}

	return NewSensitiveValue(ref.Type, plaintext, protectedValueRedactionLabel(ref)), nil
}

type SensitiveValue struct {
	typ            string
	plaintext      string
	redactionLabel string
}

func NewSensitiveValue(typ, plaintext, redactionLabel string) SensitiveValue {
	if redactionLabel == "" {
		redactionLabel = sensitiveValueRedactedLabel
	}
	return SensitiveValue{
		typ:            typ,
		plaintext:      plaintext,
		redactionLabel: redactionLabel,
	}
}

func (v SensitiveValue) Type() string {
	return v.typ
}

func (v SensitiveValue) Plaintext() string {
	return v.plaintext
}

func (v SensitiveValue) RedactionLabel() string {
	if v.redactionLabel == "" {
		return sensitiveValueRedactedLabel
	}
	return v.redactionLabel
}

func (v SensitiveValue) String() string {
	return v.RedactionLabel()
}

func (v SensitiveValue) GoString() string {
	return v.RedactionLabel()
}

func (v SensitiveValue) Error() string {
	return v.RedactionLabel()
}

func (v SensitiveValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type      string `json:"type,omitempty"`
		Sensitive bool   `json:"sensitive"`
		Value     string `json:"value"`
	}{
		Type:      v.typ,
		Sensitive: true,
		Value:     v.RedactionLabel(),
	})
}

func protectedValueRedactionLabel(ref ProtectedValueRef) string {
	if ref.RedactionLabel != "" {
		return ref.RedactionLabel
	}
	if ref.Provider != "" && ref.Key != "" {
		return "${" + ref.Provider + "." + ref.Key + "}"
	}
	return sensitiveValueRedactedLabel
}

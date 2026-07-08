package variable

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

const (
	protectedRefProviderWorkerEnv = "worker_env"
	protectedRefProviderTest      = "test"
)

type ProtectedRef struct {
	Provider       string `json:"provider"`
	Key            string `json:"key"`
	RedactionLabel string `json:"redaction_label,omitempty"`
}

func (r ProtectedRef) Validate() error {
	if r.Provider == "" {
		return fmt.Errorf("protected reference provider is required")
	}
	if r.Key == "" {
		return fmt.Errorf("protected reference key is required")
	}
	if !r.Valid() {
		return fmt.Errorf("unsupported protected reference provider: %s", r.Provider)
	}
	return nil
}

func (r ProtectedRef) Valid() bool {
	switch r.Provider {
	case protectedRefProviderWorkerEnv, protectedRefProviderTest:
		return true
	default:
		return false
	}
}

func (r ProtectedRef) Normalize() ProtectedRef {
	if r.RedactionLabel == "" {
		r.RedactionLabel = r.defaultRedactionLabel()
	}
	return r
}

func (r ProtectedRef) RedactionLabelValue() string {
	if r.RedactionLabel != "" {
		return r.RedactionLabel
	}
	return r.defaultRedactionLabel()
}

func (r ProtectedRef) Provenance() string {
	return r.Provider + "." + r.Key
}

func (r ProtectedRef) defaultRedactionLabel() string {
	return "${" + r.Provider + "." + r.Key + "}"
}

func (r ProtectedRef) MarshalJSON() ([]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}

	normalized := r.Normalize()
	return json.Marshal(struct {
		Provider       string `json:"provider"`
		Key            string `json:"key"`
		RedactionLabel string `json:"redaction_label,omitempty"`
	}{
		Provider:       normalized.Provider,
		Key:            normalized.Key,
		RedactionLabel: normalized.RedactionLabel,
	})
}

func (r *ProtectedRef) UnmarshalJSON(data []byte) error {
	var encoded struct {
		Provider       string `json:"provider"`
		Key            string `json:"key"`
		RedactionLabel string `json:"redaction_label,omitempty"`
	}
	if err := decodeProtectedRefJSON(data, &encoded); err != nil {
		return err
	}

	r.Provider = encoded.Provider
	r.Key = encoded.Key
	r.RedactionLabel = encoded.RedactionLabel

	if err := r.Validate(); err != nil {
		return err
	}

	*r = r.Normalize()
	return nil
}

func protectedReferenceValue(typ Type, ref ProtectedRef) (ResolvedValue, error) {
	if err := ref.Validate(); err != nil {
		return ResolvedValue{}, err
	}

	normalized := ref.Normalize()
	return ResolvedValue{
		Type:           typ,
		Sensitive:      true,
		ProtectedRef:   &normalized,
		RedactionLabel: normalized.RedactionLabel,
		Provenance:     normalized.Provenance(),
	}, nil
}

func decodeProtectedRefJSON(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode protected reference: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode protected reference: multiple JSON values")
		}
		return fmt.Errorf("decode protected reference: %w", err)
	}
	return nil
}

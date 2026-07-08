package variable

import (
	"bytes"
	"encoding/json"
	"fmt"
)

const redactedLabel = "[REDACTED]"

type resolvedValueJSON struct {
	Type           string                       `json:"type"`
	Sensitive      bool                         `json:"sensitive,omitempty"`
	RedactionLabel string                       `json:"redaction_label,omitempty"`
	Provenance     string                       `json:"provenance,omitempty"`
	ProtectedRef   *ProtectedRef                `json:"protected_ref,omitempty"`
	Value          any                          `json:"value,omitempty"`
	Object         map[string]resolvedValueJSON `json:"object,omitempty"`
	List           []resolvedValueJSON          `json:"list,omitempty"`
}

func (v ResolvedValue) String() string {
	if v.Type == TypeObject || v.Type == TypeList {
		encoded, err := json.Marshal(v)
		if err != nil {
			return v.redactionLabel()
		}
		return string(encoded)
	}
	if v.isSensitive() {
		return v.redactionLabel()
	}
	return fmt.Sprint(v.Value)
}

func (v ResolvedValue) GoString() string {
	return v.String()
}

func (v ResolvedValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.safeJSON())
}

func (v ResolvedValue) safeJSON() resolvedValueJSON {
	encoded := resolvedValueJSON{
		Type:         v.Type.String(),
		Sensitive:    v.isSensitive(),
		ProtectedRef: v.ProtectedRef,
	}
	if v.isSensitive() {
		encoded.RedactionLabel = v.redactionLabel()
		encoded.Provenance = v.resolvedProvenance()
	}

	switch v.Type {
	case TypeObject:
		encoded.Object = make(map[string]resolvedValueJSON, len(v.Object))
		for name, child := range v.Object {
			encoded.Object[name] = child.safeJSON()
		}
	case TypeList:
		encoded.List = make([]resolvedValueJSON, 0, len(v.List))
		for _, child := range v.List {
			encoded.List = append(encoded.List, child.safeJSON())
		}
	default:
		if v.ProtectedRef != nil {
			encoded.ProtectedRef = v.ProtectedRef
		} else if v.isSensitive() {
			encoded.Value = v.redactionLabel()
		} else {
			encoded.Value = v.Value
		}
	}

	return encoded
}

func (v ResolvedValue) redactionLabel() string {
	if v.RedactionLabel != "" {
		return v.RedactionLabel
	}
	if v.ProtectedRef != nil {
		return v.ProtectedRef.RedactionLabelValue()
	}
	return redactedLabel
}

func (v ResolvedValue) isSensitive() bool {
	return v.Sensitive || v.ProtectedRef != nil
}

func (v ResolvedValue) resolvedProvenance() string {
	if v.Provenance != "" {
		return v.Provenance
	}
	if v.ProtectedRef != nil {
		return v.ProtectedRef.Provenance()
	}
	return ""
}

func mergeSensitivity(value ResolvedValue, sensitive bool, provenance string) ResolvedValue {
	if value.ProtectedRef != nil {
		sensitive = true
		provenance = value.ProtectedRef.Provenance()
	}
	if !sensitive {
		return value
	}

	value.Sensitive = true
	if value.RedactionLabel == "" {
		if value.ProtectedRef != nil {
			value.RedactionLabel = value.ProtectedRef.RedactionLabelValue()
		} else {
			value.RedactionLabel = redactionLabelForProvenance(provenance)
		}
	}
	if value.ProtectedRef != nil {
		if value.Provenance == "" {
			value.Provenance = provenance
		}
		return value
	}
	if value.Provenance == "" {
		value.Provenance = provenance
	}
	return value
}

func aggregateSensitivity(fields map[string]ResolvedValue, values []ResolvedValue) (bool, string, string) {
	for _, child := range fields {
		if child.isSensitive() {
			return true, child.redactionLabel(), child.resolvedProvenance()
		}
	}
	for _, child := range values {
		if child.isSensitive() {
			return true, child.redactionLabel(), child.resolvedProvenance()
		}
	}
	return false, "", ""
}

func redactionLabelForProvenance(provenance string) string {
	if provenance == "" {
		return redactedLabel
	}
	return "[REDACTED:" + provenance + "]"
}

func safeJSONText(value ResolvedValue) (string, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return "", err
	}
	text := buffer.String()
	if len(text) > 0 && text[len(text)-1] == '\n' {
		text = text[:len(text)-1]
	}
	return text, nil
}

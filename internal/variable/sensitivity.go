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
	if v.Sensitive {
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
		Type:           v.Type.String(),
		Sensitive:      v.Sensitive,
		RedactionLabel: v.RedactionLabel,
		Provenance:     v.Provenance,
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
		if v.Sensitive {
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
	return redactedLabel
}

func mergeSensitivity(value ResolvedValue, sensitive bool, provenance string) ResolvedValue {
	if !sensitive {
		return value
	}

	if !value.Sensitive {
		value.Sensitive = true
	}
	if value.RedactionLabel == "" {
		value.RedactionLabel = redactionLabelForProvenance(provenance)
	}
	if value.Provenance == "" {
		value.Provenance = provenance
	}
	return value
}

func aggregateSensitivity(fields map[string]ResolvedValue, values []ResolvedValue) (bool, string, string) {
	for _, child := range fields {
		if child.Sensitive {
			return true, child.RedactionLabel, child.Provenance
		}
	}
	for _, child := range values {
		if child.Sensitive {
			return true, child.RedactionLabel, child.Provenance
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

package document

import (
	"reflect"
	"strings"
	"testing"

	"goetl/internal/fingerprint"
)

func TestDecodeSourceEquivalentJSONAndYAML(t *testing.T) {
	jsonValue, err := DecodeSource([]byte(`{
		"api_version": "goet/v1alpha1",
		"kind": "Workflow",
		"id": "yan-roy-header-analysis",
		"variables": {
			"tiles": ["h18v07", "h18v08"],
			"enabled": true,
			"count": 2,
			"run_day": "2026-07-11"
		}
	}`), DecodeOptions{Path: "workflow.json"})
	if err != nil {
		t.Fatalf("DecodeSource(JSON) error = %v", err)
	}

	yamlValue, err := DecodeSource([]byte(`
api_version: goet/v1alpha1
kind: Workflow
id: yan-roy-header-analysis
variables:
  tiles:
    - h18v07
    - h18v08
  enabled: true
  count: 2
  run_day: 2026-07-11
`), DecodeOptions{Path: "workflow.yaml"})
	if err != nil {
		t.Fatalf("DecodeSource(YAML) error = %v", err)
	}

	if !reflect.DeepEqual(jsonValue, yamlValue) {
		t.Fatalf("JSON/YAML trees differ:\nJSON: %#v\nYAML: %#v", jsonValue, yamlValue)
	}

	_, jsonHash, err := fingerprint.CanonicalJSONSHA256(jsonValue)
	if err != nil {
		t.Fatalf("canonical JSON hash for JSON value: %v", err)
	}
	_, yamlHash, err := fingerprint.CanonicalJSONSHA256(yamlValue)
	if err != nil {
		t.Fatalf("canonical JSON hash for YAML value: %v", err)
	}
	if jsonHash != yamlHash {
		t.Fatalf("canonical hashes differ: JSON %s YAML %s", jsonHash, yamlHash)
	}
}

func TestSourceFormatFor(t *testing.T) {
	tests := []struct {
		name    string
		options DecodeOptions
		want    SourceFormat
	}{
		{name: "explicit", options: DecodeOptions{Format: SourceFormatYAML, Path: "workflow.json"}, want: SourceFormatYAML},
		{name: "json media", options: DecodeOptions{MediaType: "application/json; charset=utf-8"}, want: SourceFormatJSON},
		{name: "yaml media", options: DecodeOptions{MediaType: "application/x-yaml"}, want: SourceFormatYAML},
		{name: "json path", options: DecodeOptions{Path: "workflow.json"}, want: SourceFormatJSON},
		{name: "yaml path", options: DecodeOptions{Path: "workflow.yml"}, want: SourceFormatYAML},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := SourceFormatFor(test.options)
			if err != nil {
				t.Fatalf("SourceFormatFor() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("SourceFormatFor() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSourceFormatForDoesNotGuessFromContent(t *testing.T) {
	_, err := DecodeSource([]byte(`{"id":"workflow-001"}`), DecodeOptions{Path: "workflow"})
	if err == nil || !strings.Contains(err.Error(), "requires an explicit format") {
		t.Fatalf("DecodeSource() error = %v, want explicit format requirement", err)
	}
}

func TestDecodeJSONRejectsDuplicateKeys(t *testing.T) {
	_, err := DecodeSource([]byte(`{"id":"one","id":"two"}`), DecodeOptions{Path: "workflow.json"})
	if err == nil || !strings.Contains(err.Error(), `duplicate object key "id"`) {
		t.Fatalf("DecodeSource() error = %v, want duplicate key", err)
	}
	if !strings.Contains(err.Error(), "workflow.json:") {
		t.Fatalf("DecodeSource() error = %v, want source path", err)
	}
}

func TestDecodeJSONRejectsNullAndFractions(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "null", raw: `{"value": null}`, want: "null values are unsupported"},
		{name: "fraction", raw: `{"value": 1.25}`, want: "only JSON integer numbers are supported"},
		{name: "exponent", raw: `{"value": 1e3}`, want: "only JSON integer numbers are supported"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeSource([]byte(test.raw), DecodeOptions{Path: "document.json"})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("DecodeSource() error = %v, want %q", err, test.want)
			}
		})
	}
}

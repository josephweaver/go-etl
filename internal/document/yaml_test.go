package document

import (
	"strings"
	"testing"
)

func TestDecodeYAMLNormalizesBooleansIntegersAndTimestamps(t *testing.T) {
	value, err := DecodeSource([]byte(`
enabled: true
disabled: false
count: 42
run_day: 2026-07-11
`), DecodeOptions{Path: "workflow.yaml"})
	if err != nil {
		t.Fatalf("DecodeSource() error = %v", err)
	}

	root := value.(map[string]any)
	if root["enabled"] != true {
		t.Fatalf("enabled = %#v, want true", root["enabled"])
	}
	if root["disabled"] != false {
		t.Fatalf("disabled = %#v, want false", root["disabled"])
	}
	if root["count"] != int64(42) {
		t.Fatalf("count = %#v, want int64(42)", root["count"])
	}
	if root["run_day"] != "2026-07-11" {
		t.Fatalf("run_day = %#v, want string timestamp", root["run_day"])
	}
}

func TestDecodeYAMLRejectsDuplicateKeys(t *testing.T) {
	_, err := DecodeSource([]byte(`
id: one
id: two
`), DecodeOptions{Path: "workflow.yaml"})
	if err == nil || !strings.Contains(err.Error(), `duplicate mapping key "id"`) {
		t.Fatalf("DecodeSource() error = %v, want duplicate key", err)
	}
	if !strings.Contains(err.Error(), "workflow.yaml:") {
		t.Fatalf("DecodeSource() error = %v, want source path", err)
	}
}

func TestDecodeYAMLRejectsUnsupportedValues(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "null", raw: "value: null\n", want: "null values are unsupported"},
		{name: "empty null", raw: "value:\n", want: "null values are unsupported"},
		{name: "fraction", raw: "value: 1.25\n", want: "fractional numbers are unsupported"},
		{name: "non-string key", raw: "1: one\n", want: "mapping keys must be strings"},
		{name: "tag", raw: "value: !custom tagged\n", want: "unsupported YAML tag"},
		{name: "anchor", raw: "value: &shared one\n", want: "YAML anchors are unsupported"},
		{name: "alias", raw: "first: &shared one\nsecond: *shared\n", want: "YAML anchors are unsupported"},
		{name: "hex integer", raw: "value: 0x10\n", want: "only JSON-compatible decimal integers are supported"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeSource([]byte(test.raw), DecodeOptions{Path: "workflow.yaml"})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("DecodeSource() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestDecodeYAMLRejectsMultipleDocuments(t *testing.T) {
	_, err := DecodeSource([]byte("---\nid: one\n---\nid: two\n"), DecodeOptions{Path: "workflow.yaml"})
	if err == nil || !strings.Contains(err.Error(), "multiple YAML documents") {
		t.Fatalf("DecodeSource() error = %v, want multiple documents", err)
	}
}

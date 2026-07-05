package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goetl/internal/model"
)

func TestCanonicalJSONDocumentCanonicalizesEquivalentLogicalOutput(t *testing.T) {
	left := []byte(`{"b":2,"a":1,"nested":{"y":2,"x":1}}`)
	right := []byte("{\n  \"nested\": {\n    \"x\": 1,\n    \"y\": 2\n  },\n  \"a\": 1,\n  \"b\": 2\n}")

	leftCanonical, leftHash, _, err := canonicalJSONDocument(left, "GOET_OUTPUT_JSON")
	if err != nil {
		t.Fatalf("canonicalJSONDocument(left) error = %v", err)
	}
	rightCanonical, rightHash, _, err := canonicalJSONDocument(right, "GOET_OUTPUT_JSON")
	if err != nil {
		t.Fatalf("canonicalJSONDocument(right) error = %v", err)
	}

	if leftHash != rightHash {
		t.Fatalf("hashes differ: %s vs %s", leftHash, rightHash)
	}
	if !bytes.Equal(leftCanonical, rightCanonical) {
		t.Fatalf("canonical bytes differ:\nleft=%s\nright=%s", leftCanonical, rightCanonical)
	}
}

func TestCanonicalJSONDocumentRejectsInvalidMultipleAndTrailingContent(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{name: "invalid", raw: "{", wantErr: "decode GOET_OUTPUT_JSON"},
		{name: "multiple", raw: `{"a":1} {"b":2}`, wantErr: "one JSON document"},
		{name: "trailing", raw: `{"a":1} trailing`, wantErr: "one JSON document"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := canonicalJSONDocument([]byte(tt.raw), "GOET_OUTPUT_JSON")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestAtomicWriteFilePromotesCanonicalOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "result.json")

	if err := os.WriteFile(path, []byte("stale"), 0644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}

	output := []byte(`{"b":2,"a":1}`)
	canonical, _, _, err := canonicalJSONDocument(output, "GOET_OUTPUT_JSON")
	if err != nil {
		t.Fatalf("canonicalJSONDocument() error = %v", err)
	}

	if err := atomicWriteFile(path, canonical, 0644); err != nil {
		t.Fatalf("atomicWriteFile() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read promoted output: %v", err)
	}
	if !bytes.Equal(data, canonical) {
		t.Fatalf("promoted output = %s, want %s", data, canonical)
	}
}

func TestPythonOutputEvidenceJSONTextWrapsLogicalOutput(t *testing.T) {
	item := model.WorkItem{
		ID:        "python-001",
		AttemptID: "attempt-001",
		Parameters: model.Parameters{
			"python_args": model.Parameter{Type: "list", Value: []any{"alpha"}},
		},
		OutputFilename: "result.json",
	}

	wrapperJSON, err := pythonOutputEvidenceJSONText(
		item,
		"/tmp/source/scripts/run.py",
		"/tmp/source/config/env.json",
		0,
		[]byte(`{"value":1}`),
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
	)
	if err != nil {
		t.Fatalf("pythonOutputEvidenceJSONText() error = %v", err)
	}

	var wrapper struct {
		Schema          string         `json:"schema"`
		WorkItemID      string         `json:"work_item_id"`
		Operation       string         `json:"operation"`
		Entrypoint      string         `json:"entrypoint"`
		Environment     string         `json:"environment"`
		ExitCode        int            `json:"exit_code"`
		LogicalOutput   map[string]any `json:"logical_output"`
		InputSHA256     string         `json:"input_sha256"`
		OutputSHA256    string         `json:"output_sha256"`
		PreStateSHA256  string         `json:"pre_state_sha256"`
		PostStateSHA256 string         `json:"post_state_sha256"`
		StdoutSHA256    string         `json:"stdout_sha256"`
		StderrSHA256    string         `json:"stderr_sha256"`
	}
	if err := json.Unmarshal([]byte(wrapperJSON), &wrapper); err != nil {
		t.Fatalf("decode wrapper: %v", err)
	}

	if wrapper.Schema != pythonOutputEvidenceSchema {
		t.Fatalf("schema = %q, want %q", wrapper.Schema, pythonOutputEvidenceSchema)
	}
	if wrapper.WorkItemID != item.ID {
		t.Fatalf("work_item_id = %q, want %q", wrapper.WorkItemID, item.ID)
	}
	if wrapper.Operation != pythonScriptOperation {
		t.Fatalf("operation = %q, want %q", wrapper.Operation, pythonScriptOperation)
	}
	if wrapper.Entrypoint != "/tmp/source/scripts/run.py" {
		t.Fatalf("entrypoint = %q", wrapper.Entrypoint)
	}
	if wrapper.Environment != "/tmp/source/config/env.json" {
		t.Fatalf("environment = %q", wrapper.Environment)
	}
	if wrapper.ExitCode != 0 {
		t.Fatalf("exit_code = %d, want 0", wrapper.ExitCode)
	}
	if wrapper.LogicalOutput["value"] != float64(1) {
		t.Fatalf("logical output = %+v", wrapper.LogicalOutput)
	}
	if wrapper.InputSHA256 == "" || wrapper.OutputSHA256 == "" || wrapper.PreStateSHA256 == "" || wrapper.PostStateSHA256 == "" {
		t.Fatalf("missing hash fields: %+v", wrapper)
	}
	if wrapper.StdoutSHA256 == "" || wrapper.StderrSHA256 == "" {
		t.Fatalf("missing log hashes: %+v", wrapper)
	}
	if strings.Contains(wrapperJSON, "python stdout") || strings.Contains(wrapperJSON, "python stderr") {
		t.Fatalf("wrapper should not embed log contents: %s", wrapperJSON)
	}
}

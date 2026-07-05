package main

import (
	"encoding/json"
	"goetl/internal/model"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendFallbackLogObservationWritesStructuredJSONL(t *testing.T) {
	root := t.TempDir()
	observation := model.LogObservation{
		Component: "worker",
		Level:     model.LogLevelInfo,
		Timestamp: "2026-07-05T11:00:00Z",
		Message:   "fallback test",
		Stream:    model.LogStreamSystem,
	}

	if err := appendFallbackLogObservation(root, observation); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	path := filepath.Join(root, fallbackObservationsFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected fallback file %q: %v", path, err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	var parsed model.LogObservation
	if err := json.Unmarshal([]byte(lines[0]), &parsed); err != nil {
		t.Fatalf("expected valid jsonl: %v", err)
	}

	if parsed.Message != observation.Message {
		t.Fatalf("unexpected message: %q", parsed.Message)
	}
}

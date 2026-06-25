package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalTransportCopy(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	destination := filepath.Join(root, "nested", "destination.txt")

	if err := os.WriteFile(source, []byte("goetl"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	if err := (LocalTransport{}).Copy(context.Background(), source, destination); err != nil {
		t.Fatalf("copy: %v", err)
	}

	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}
	if string(content) != "goetl" {
		t.Fatalf("destination content = %q, want goetl", string(content))
	}
}

func TestLocalTransportExec(t *testing.T) {
	output, err := (LocalTransport{}).Exec(context.Background(), "go", "env", "GOVERSION")
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !strings.HasPrefix(string(output), "go") {
		t.Fatalf("output = %q, want Go version", string(output))
	}
}

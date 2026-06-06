package main

import (
	"os"
	"path/filepath"
	"testing"

	"goetl/internal/variable"
)

func TestLoadControllerConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controller-config.json")
	content := []byte(`{
		"variables": [
			{
				"Name": {"Namespace": "backend", "Key": "controller_url"},
				"Type": {"Kind": "string"},
				"Expression": "http://localhost:8080"
			},
			{
				"Name": {"Namespace": "runtime", "Key": "ledger_db_path"},
				"Type": {"Kind": "path"},
				"Expression": ".run/controller/ledger.sqlite"
			}
		]
	}`)

	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	config, err := loadControllerConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(config.Variables) != 2 {
		t.Fatalf("unexpected variable count: %d", len(config.Variables))
	}

	for _, item := range config.Variables {
		if item.Name.Namespace != variable.NamespaceControllerConfig {
			t.Fatalf("namespace = %q, want %q", item.Name.Namespace, variable.NamespaceControllerConfig)
		}
	}
}

func TestLoadControllerConfigRejectsMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")

	if _, err := loadControllerConfig(path); err == nil {
		t.Fatal("expected an error")
	}
}

func TestLoadControllerConfigRejectsMalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controller-config.json")

	if err := os.WriteFile(path, []byte(`{"variables":`), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := loadControllerConfig(path); err == nil {
		t.Fatal("expected an error")
	}
}

func TestControllerConfigRejectsNoVariables(t *testing.T) {
	config := ControllerConfig{}

	if err := config.Validate(); err == nil {
		t.Fatal("expected an error")
	}
}

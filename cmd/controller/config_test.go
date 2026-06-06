package main

import (
	"context"
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

func TestControllerConfigFromArgsReturnsEmptyWithoutPath(t *testing.T) {
	config, err := controllerConfigFromArgs([]string{"controller"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(config.Variables) != 0 {
		t.Fatalf("unexpected variable count: %d", len(config.Variables))
	}
}

func TestControllerConfigFromArgsLoadsPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controller-config.json")
	content := []byte(`{
		"variables": [
			{
				"Name": {"Namespace": "controller_config", "Key": "controller_url"},
				"Type": {"Kind": "string"},
				"Expression": "http://localhost:8080"
			}
		]
	}`)

	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	config, err := controllerConfigFromArgs([]string{"controller", path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(config.Variables) != 1 {
		t.Fatalf("unexpected variable count: %d", len(config.Variables))
	}
}

func TestInitConfiguredLedgerReturnsNilWithoutPath(t *testing.T) {
	db, err := initConfiguredLedger(context.Background(), ControllerConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if db != nil {
		t.Fatal("expected no database")
	}
}

func TestInitConfiguredLedgerCreatesSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ledger.sqlite")
	config := ControllerConfig{Variables: []variable.Variable{
		{
			Name:       variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "ledger_db_path"},
			Type:       variable.TypePath,
			Expression: dbPath,
		},
	}}

	db, err := initConfiguredLedger(context.Background(), config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer db.Close()

	var version int
	if err := db.QueryRowContext(context.Background(), `SELECT version FROM schema_version`).Scan(&version); err != nil {
		t.Fatalf("query schema version: %v", err)
	}
	if version != 1 {
		t.Fatalf("schema version = %d, want 1", version)
	}
}

func TestInitConfiguredLedgerRejectsWrongPathType(t *testing.T) {
	config := ControllerConfig{Variables: []variable.Variable{
		{
			Name:       variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "ledger_db_path"},
			Type:       variable.TypeString,
			Expression: "ledger.sqlite",
		},
	}}

	if _, err := initConfiguredLedger(context.Background(), config); err == nil {
		t.Fatal("expected an error")
	}
}

func TestControllerOwnsConfiguredLedger(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ledger.sqlite")
	config := ControllerConfig{Variables: []variable.Variable{
		{
			Name:       variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "ledger_db_path"},
			Type:       variable.TypePath,
			Expression: dbPath,
		},
	}}

	db, err := initConfiguredLedger(context.Background(), config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer db.Close()

	controller := newController(nil)
	controller.ledger = db

	if controller.ledger == nil {
		t.Fatal("expected controller ledger")
	}
}

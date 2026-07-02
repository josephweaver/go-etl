package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"goetl/internal/ledger"
	"goetl/internal/variable"
)

func TestLoadControllerConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controller-config.json")
	content := []byte(`{
		"api_version": "goet/v1alpha1",
		"kind": "Controller",
		"variables": [
			{
				"name": {"namespace": "backend", "key": "controller_url"},
				"type": "string",
				"expression": "http://localhost:8080"
			},
			{
				"name": {"namespace": "runtime", "key": "ledger_db_path"},
				"type": "path",
				"expression": ".run/controller/ledger.sqlite"
			}
		],
		"execution_environment": {
			"name": "dockerized-slurm",
			"transports": [
				{"name": "control", "type": "docker"}
			],
			"dialect": {"type": "bash"},
			"scheduler": {"type": "slurm"},
			"runtime": {"type": "worker"}
		}
	}`)

	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	config, err := loadControllerConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.APIVersion != controllerAPIVersion {
		t.Fatalf("api version = %q, want %q", config.APIVersion, controllerAPIVersion)
	}
	if config.Kind != controllerKind {
		t.Fatalf("kind = %q, want %q", config.Kind, controllerKind)
	}
	if len(config.Variables) != 2 {
		t.Fatalf("unexpected variable count: %d", len(config.Variables))
	}

	for _, item := range config.Variables {
		if item.Name.Namespace != variable.NamespaceControllerConfig {
			t.Fatalf("namespace = %q, want %q", item.Name.Namespace, variable.NamespaceControllerConfig)
		}
	}
	if config.ExecutionEnvironment.Name != "dockerized-slurm" {
		t.Fatalf("execution environment = %q, want dockerized-slurm", config.ExecutionEnvironment.Name)
	}
}

func TestLoadControllerConfigSupportsSSHTransportSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controller-config.json")
	content := []byte(`{
		"api_version": "goet/v1alpha1",
		"kind": "Controller",
		"variables": [
			{
				"name": {"namespace": "controller_config", "key": "controller_url"},
				"type": "string",
				"expression": "http://localhost:8080"
			}
		],
		"execution_environment": {
			"name": "ssh-slurm",
			"transports": [
				{
					"name": "login",
					"type": "ssh",
					"settings": {
						"host": "hpcc.example.edu",
						"port": "2222",
						"user": "researcher",
						"identity_file": "/home/researcher/.ssh/id_ed25519",
						"host_key_policy": "pinned",
						"pinned_host_key": "ssh-ed25519 AAAATESTKEY"
					}
				}
			],
			"dialect": {"type": "bash"},
			"scheduler": {"type": "slurm"},
			"runtime": {"type": "worker"}
		}
	}`)

	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	config, err := loadControllerConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	env, err := NewExecutionEnvironment(config.ExecutionEnvironment)
	if err != nil {
		t.Fatalf("unexpected environment error: %v", err)
	}
	transport, ok := env.Transports[0].(*SSHTransport)
	if !ok {
		t.Fatalf("transport type = %T, want *SSHTransport", env.Transports[0])
	}
	if transport.Config.IdentityFile != "/home/researcher/.ssh/id_ed25519" {
		t.Fatalf("identity file = %q, want configured path", transport.Config.IdentityFile)
	}
}

func TestFakeHPCCSSHConfigBuildsSSHTransport(t *testing.T) {
	config, err := loadControllerConfig("fake-hpcc-ssh-config.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	env, err := NewExecutionEnvironment(config.ExecutionEnvironment)
	if err != nil {
		t.Fatalf("unexpected environment error: %v", err)
	}

	if env.Config.Name != "fake-hpcc-ssh" {
		t.Fatalf("environment name = %q, want fake-hpcc-ssh", env.Config.Name)
	}
	transport, ok := env.Transports[0].(*SSHTransport)
	if !ok {
		t.Fatalf("transport type = %T, want *SSHTransport", env.Transports[0])
	}
	if transport.Config.Host != "127.0.0.1" {
		t.Fatalf("host = %q, want 127.0.0.1", transport.Config.Host)
	}
	if transport.Config.Port != 2222 {
		t.Fatalf("port = %d, want 2222", transport.Config.Port)
	}
	if transport.Config.IdentityFile != ".run/fake-hpcc-ssh/id_ed25519" {
		t.Fatalf("identity file = %q, want fake key path", transport.Config.IdentityFile)
	}
	if _, ok := env.Scheduler.(SlurmScheduler); !ok {
		t.Fatalf("scheduler type = %T, want SlurmScheduler", env.Scheduler)
	}
	runtime, ok := env.Runtime.(WorkerRuntime)
	if !ok {
		t.Fatalf("runtime type = %T, want WorkerRuntime", env.Runtime)
	}
	if runtime.Root != "/data/goetl" {
		t.Fatalf("runtime root = %q, want /data/goetl", runtime.Root)
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

func TestLoadControllerConfigRejectsInvalidEnvelopeBeforeVariables(t *testing.T) {
	tests := []struct {
		name      string
		document  string
		errorText string
	}{
		{name: "missing api version", document: `{"kind":"Controller"}`, errorText: "api_version"},
		{name: "empty api version", document: `{"api_version":"","kind":"Controller"}`, errorText: "api_version"},
		{name: "unsupported api version", document: `{"api_version":"goet/v2","kind":"Controller"}`, errorText: "api_version"},
		{name: "incorrectly cased api version", document: `{"api_version":"GOET/v1alpha1","kind":"Controller"}`, errorText: "api_version"},
		{name: "missing kind", document: `{"api_version":"goet/v1alpha1"}`, errorText: "kind"},
		{name: "empty kind", document: `{"api_version":"goet/v1alpha1","kind":""}`, errorText: "kind"},
		{name: "unsupported kind", document: `{"api_version":"goet/v1alpha1","kind":"Project"}`, errorText: "kind"},
		{name: "incorrectly cased kind", document: `{"api_version":"goet/v1alpha1","kind":"controller"}`, errorText: "kind"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "controller-config.json")
			if err := os.WriteFile(path, []byte(test.document), 0644); err != nil {
				t.Fatal(err)
			}

			_, err := loadControllerConfig(path)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), test.errorText) {
				t.Fatalf("error = %q, want it to identify %q", err, test.errorText)
			}
			if strings.Contains(err.Error(), "variables are required") {
				t.Fatalf("envelope error occurred after variable validation: %v", err)
			}
		})
	}
}

func TestControllerConfigRejectsNoVariables(t *testing.T) {
	config := ControllerConfig{}

	if err := config.Validate(); err == nil {
		t.Fatal("expected an error")
	}
}

func TestControllerConfigFromArgsLoadsDefaultWithoutPath(t *testing.T) {
	config, err := controllerConfigFromArgs([]string{"controller"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(config.Variables) == 0 {
		t.Fatal("expected default variables")
	}
	if config.ExecutionEnvironment.Name != "dockerized-slurm" {
		t.Fatalf("execution environment = %q, want dockerized-slurm", config.ExecutionEnvironment.Name)
	}
}

func TestControllerConfigFromArgsLoadsPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controller-config.json")
	content := []byte(`{
		"api_version": "goet/v1alpha1",
		"kind": "Controller",
		"variables": [
			{
				"name": {"namespace": "controller_config", "key": "controller_url"},
				"type": "string",
				"expression": "http://localhost:8080"
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

func TestInitConfiguredExecutionEnvironmentReturnsNilWhenMissing(t *testing.T) {
	env, err := initConfiguredExecutionEnvironment(ControllerConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env != nil {
		t.Fatal("expected no execution environment")
	}
}

func TestInitConfiguredExecutionEnvironmentBuildsConfiguredEnvironment(t *testing.T) {
	env, err := initConfiguredExecutionEnvironment(ControllerConfig{
		ExecutionEnvironment: ExecutionEnvironmentConfig{
			Name: "dockerized-slurm",
			Transports: []ExecutionComponentConfig{
				{Type: "docker", Settings: map[string]string{"container": "slurmctld"}},
			},
			Dialect:   ExecutionComponentConfig{Type: "bash"},
			Scheduler: ExecutionComponentConfig{Type: "slurm"},
			Runtime:   ExecutionComponentConfig{Type: "worker", Settings: map[string]string{"root": "/data/goetl"}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env == nil {
		t.Fatal("expected execution environment")
	}
	if env.Config.Name != "dockerized-slurm" {
		t.Fatalf("environment name = %q, want dockerized-slurm", env.Config.Name)
	}
	if _, ok := env.Scheduler.(SlurmScheduler); !ok {
		t.Fatalf("scheduler type = %T, want SlurmScheduler", env.Scheduler)
	}
}

func TestInitConfiguredExecutionEnvironmentRejectsInvalidEnvironment(t *testing.T) {
	_, err := initConfiguredExecutionEnvironment(ControllerConfig{
		ExecutionEnvironment: ExecutionEnvironmentConfig{
			Name:       "bad-env",
			Transports: []ExecutionComponentConfig{{Type: "docker"}},
			Dialect:    ExecutionComponentConfig{Type: "bash"},
			Scheduler:  ExecutionComponentConfig{Type: "slurm"},
			Runtime:    ExecutionComponentConfig{Type: "worker"},
		},
	})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestInitConfiguredLedgerCreatesSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ledger.sqlite")
	config := ControllerConfig{Variables: []variable.Variable{{Name: variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "ledger_db_path"}, TypedExpression: variable.TypedExpression{Type: variable.TypePath, Expression: dbPath}}}}

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
	config := ControllerConfig{Variables: []variable.Variable{{Name: variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "ledger_db_path"}, TypedExpression: variable.TypedExpression{Type: variable.TypeString, Expression: "ledger.sqlite"}}}}

	if _, err := initConfiguredLedger(context.Background(), config); err == nil {
		t.Fatal("expected an error")
	}
}

func TestControllerOwnsConfiguredLedger(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ledger.sqlite")
	config := ControllerConfig{Variables: []variable.Variable{{Name: variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "ledger_db_path"}, TypedExpression: variable.TypedExpression{Type: variable.TypePath, Expression: dbPath}}}}

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

func TestControllerRecordAttemptNoopsWithoutLedger(t *testing.T) {
	controller := newController(nil)

	if err := controller.recordAttempt(context.Background(), ledger.Attempt{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestControllerRecordAttemptWritesConfiguredLedger(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ledger.sqlite")
	config := ControllerConfig{Variables: []variable.Variable{{Name: variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "ledger_db_path"}, TypedExpression: variable.TypedExpression{Type: variable.TypePath, Expression: dbPath}}}}

	db, err := initConfiguredLedger(context.Background(), config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer db.Close()

	controller := newController(nil)
	controller.ledger = db
	attempt := ledger.Attempt{
		ID:                  "attempt-001",
		WorkflowInstanceID:  "workflow-instance-001",
		StepInstanceID:      "step-instance-001",
		WorkItemID:          "work-item-001",
		WorkItemFingerprint: "work-item-fingerprint",
		InputFingerprint:    "input-fingerprint",
		OutputFingerprint:   "output-fingerprint",
		CodeVersion:         "code-version",
		Status:              ledger.AttemptStatusCompleted,
		StartedAt:           time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC),
	}

	if err := controller.recordAttempt(context.Background(), attempt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var count int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM attempts`).Scan(&count); err != nil {
		t.Fatalf("query attempt count: %v", err)
	}
	if count != 1 {
		t.Fatalf("attempt count = %d, want 1", count)
	}
}

package main

import (
	"context"
	"fmt"
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
				"name": {"namespace": "controller_config", "key": "controller_url"},
				"type": "string",
				"expression": "http://localhost:8080"
			},
			{
				"name": {"namespace": "controller_config", "key": "ledger_db_path"},
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

func TestDefaultsPathForControllerConfig(t *testing.T) {
	tests := []struct {
		controllerPath string
		want           string
	}{
		{controllerPath: filepath.Join("configs", "controller.json"), want: filepath.Join("configs", defaultsFilename)},
		{controllerPath: "controller.json", want: defaultsFilename},
		{controllerPath: filepath.Join(string(filepath.Separator), "opt", "goet", "controller.json"), want: filepath.Join(string(filepath.Separator), "opt", "goet", defaultsFilename)},
	}

	for _, test := range tests {
		if got := defaultsPathForControllerConfig(test.controllerPath); got != test.want {
			t.Errorf("defaults path for %q = %q, want %q", test.controllerPath, got, test.want)
		}
	}
}

func TestLoadDefaultsDocument(t *testing.T) {
	document, err := loadDefaultsDocument("defaults.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if document.APIVersion != controllerAPIVersion {
		t.Fatalf("api version = %q, want %q", document.APIVersion, controllerAPIVersion)
	}
	if document.Kind != defaultsKind {
		t.Fatalf("kind = %q, want %q", document.Kind, defaultsKind)
	}
	expectedKeys := []string{
		"controller_listen_host",
		"controller_listen_port",
		"controller_root_dir",
		"controller_git_cache_path",
		"controller_temp_path",
		"controller_artifact_cache_path",
		"caretaker_interval_schedule_milliseconds",
		"caretaker_missed_interval_limit",
		"resolver_max_depth",
		"controller_log_root_path",
		"controller_filesystem_logging_enabled",
		"controller_log_level",
		"controller_read_header_timeout_milliseconds",
		"controller_read_timeout_milliseconds",
		"controller_write_timeout_milliseconds",
		"controller_idle_timeout_milliseconds",
		"controller_shutdown_timeout_milliseconds",
		"controller_max_request_bytes",
		"controller_max_header_bytes",
		"controller_git_cache_max_size_mb",
		"controller_git_cache_retention_milliseconds",
		"controller_git_fetch_timeout_milliseconds",
		"controller_git_fetch_concurrency",
		"controller_temp_cleanup_age_milliseconds",
		"controller_artifact_cache_max_size_mb",
		"controller_artifact_cache_retention_milliseconds",
		"controller_storage_min_free_mb",
	}
	actualKeys := make(map[string]bool, len(document.Variables))
	for _, item := range document.Variables {
		if item.Name.Namespace != variable.NamespaceControllerConfig {
			t.Fatalf("checked-in default %s uses namespace %q", item.Name, item.Name.Namespace)
		}
		actualKeys[item.Name.Key] = true
	}
	if len(actualKeys) != len(expectedKeys) {
		t.Fatalf("checked-in default count = %d, want %d", len(actualKeys), len(expectedKeys))
	}
	for _, key := range expectedKeys {
		if !actualKeys[key] {
			t.Errorf("checked-in defaults missing controller_config.%s", key)
		}
	}
}

func TestLoadDefaultsDocumentRejectsInvalidEnvelopeBeforeVariables(t *testing.T) {
	tests := []struct {
		name      string
		document  string
		errorText string
	}{
		{name: "missing api version", document: `{"kind":"Defaults"}`, errorText: "api_version"},
		{name: "empty api version", document: `{"api_version":"","kind":"Defaults"}`, errorText: "api_version"},
		{name: "unsupported api version", document: `{"api_version":"goet/v2","kind":"Defaults"}`, errorText: "api_version"},
		{name: "incorrectly cased api version", document: `{"api_version":"GOET/v1alpha1","kind":"Defaults"}`, errorText: "api_version"},
		{name: "missing kind", document: `{"api_version":"goet/v1alpha1"}`, errorText: "kind"},
		{name: "empty kind", document: `{"api_version":"goet/v1alpha1","kind":""}`, errorText: "kind"},
		{name: "unsupported kind", document: `{"api_version":"goet/v1alpha1","kind":"Controller"}`, errorText: "kind"},
		{name: "incorrectly cased kind", document: `{"api_version":"goet/v1alpha1","kind":"defaults"}`, errorText: "kind"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), defaultsFilename)
			if err := os.WriteFile(path, []byte(test.document), 0644); err != nil {
				t.Fatal(err)
			}

			_, err := loadDefaultsDocument(path)
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

func TestDefaultsDocumentAllowsConfigurationNamespaces(t *testing.T) {
	document := DefaultsDocument{Variables: []variable.Variable{
		testDefaultVariable(variable.NamespaceClientConfig, "shared"),
		testDefaultVariable(variable.NamespaceControllerConfig, "shared"),
		testDefaultVariable(variable.NamespaceWorkerConfig, "shared"),
		testDefaultVariable(variable.NamespaceProjectConfig, "shared"),
	}}
	if err := document.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDefaultsDocumentRejectsDisallowedNamespaces(t *testing.T) {
	namespaces := []variable.Namespace{
		variable.NamespaceClientEnvironment,
		variable.NamespaceControllerEnvironment,
		variable.NamespaceWorkerEnvironment,
		variable.NamespaceOverride,
		variable.NamespaceRuntime,
		variable.NamespaceWorkflow,
		variable.NamespaceStep,
		variable.NamespaceWorkItem,
		variable.NamespaceGlobalConfig,
		variable.NamespaceGlobal,
		variable.NamespaceBackend,
		variable.NamespaceProject,
	}

	for _, namespace := range namespaces {
		t.Run(string(namespace), func(t *testing.T) {
			document := DefaultsDocument{Variables: []variable.Variable{testDefaultVariable(namespace, "value")}}
			err := document.Validate()
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), string(namespace)) {
				t.Fatalf("error = %q, want namespace %q", err, namespace)
			}
		})
	}
}

func TestDefaultsDocumentRejectsDuplicateKeyWithinNamespace(t *testing.T) {
	document := DefaultsDocument{Variables: []variable.Variable{
		testDefaultVariable(variable.NamespaceControllerConfig, "duplicate"),
		testDefaultVariable(variable.NamespaceControllerConfig, "duplicate"),
	}}
	err := document.Validate()
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "controller_config") || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("error = %q, want namespace and duplicate key", err)
	}
}

func TestLoadDefaultsDocumentErrorsContainPath(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), defaultsFilename)
	if _, err := loadDefaultsDocument(missingPath); err == nil || !strings.Contains(err.Error(), missingPath) {
		t.Fatalf("missing-file error = %v, want path %q", err, missingPath)
	}

	malformedPath := filepath.Join(t.TempDir(), defaultsFilename)
	if err := os.WriteFile(malformedPath, []byte(`{"variables":`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadDefaultsDocument(malformedPath); err == nil || !strings.Contains(err.Error(), malformedPath) {
		t.Fatalf("decode error = %v, want path %q", err, malformedPath)
	}

	invalidPath := filepath.Join(t.TempDir(), defaultsFilename)
	invalid := `{"api_version":"goet/v1alpha1","kind":"Defaults","variables":[{"name":{"namespace":"runtime","key":"bad"},"type":"string","expression":"value"}]}`
	if err := os.WriteFile(invalidPath, []byte(invalid), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadDefaultsDocument(invalidPath); err == nil || !strings.Contains(err.Error(), invalidPath) {
		t.Fatalf("validation error = %v, want path %q", err, invalidPath)
	}
}

func testDefaultVariable(namespace variable.Namespace, key string) variable.Variable {
	return variable.Variable{
		Name:            variable.Name{Namespace: namespace, Key: key},
		TypedExpression: variable.TypedExpression{Type: variable.TypeString, Expression: "default"},
	}
}

func TestControllerConfigRejectsNoVariables(t *testing.T) {
	config := ControllerConfig{}

	if err := config.Validate(); err == nil {
		t.Fatal("expected an error")
	}
}

func TestLoadControllerConfigRejectsNonCanonicalNamespace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controller.json")
	document := `{
		"api_version":"goet/v1alpha1",
		"kind":"Controller",
		"variables":[
			{"name":{"namespace":"backend","key":"controller_url"},"type":"string","expression":"http://localhost:8080"}
		]
	}`
	if err := os.WriteFile(path, []byte(document), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := loadControllerConfig(path)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "backend") || !strings.Contains(err.Error(), "controller_config") {
		t.Fatalf("error = %q, want actual and required namespaces", err)
	}
}

func TestControllerStartupSourcesRetainAndLayerDocuments(t *testing.T) {
	directory := t.TempDir()
	controllerPath := filepath.Join(directory, "selected-controller.json")
	defaultsPath := filepath.Join(directory, defaultsFilename)
	controllerDocument := `{
		"api_version":"goet/v1alpha1",
		"kind":"Controller",
		"variables":[
			{"name":{"namespace":"controller_config","key":"shared"},"type":"string","expression":"explicit"},
			{"name":{"namespace":"controller_config","key":"explicit_only"},"type":"string","expression":"explicit"}
		]
	}`
	defaultsDocument := `{
		"api_version":"goet/v1alpha1",
		"kind":"Defaults",
		"variables":[
			{"name":{"namespace":"controller_config","key":"shared"},"type":"string","expression":"default"},
			{"name":{"namespace":"controller_config","key":"default_only"},"type":"string","expression":"default"},
			{"name":{"namespace":"worker_config","key":"worker_only"},"type":"string","expression":"retained"}
		]
	}`
	if err := os.WriteFile(controllerPath, []byte(controllerDocument), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defaultsPath, []byte(defaultsDocument), 0644); err != nil {
		t.Fatal(err)
	}

	sources, err := loadControllerStartupSources(controllerPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sources.ControllerPath != controllerPath {
		t.Fatalf("controller path = %q, want %q", sources.ControllerPath, controllerPath)
	}
	if sources.DefaultsPath != defaultsPath {
		t.Fatalf("defaults path = %q, want %q", sources.DefaultsPath, defaultsPath)
	}
	if len(sources.Defaults.Variables) != 3 {
		t.Fatalf("retained default count = %d, want 3", len(sources.Defaults.Variables))
	}

	defaultScope, controllerScope, err := sources.controllerScopes()
	if err != nil {
		t.Fatalf("unexpected scope error: %v", err)
	}
	if _, ok := defaultScope["worker_only"]; ok {
		t.Fatal("worker_config default entered controller startup scope")
	}
	set := variable.NewSet(defaultScope, controllerScope)
	assertVariableExpression(t, set, "shared", "explicit")
	assertVariableExpression(t, set, "default_only", "default")
	assertVariableExpression(t, set, "explicit_only", "explicit")

	if sources.Defaults.Variables[0].TypedExpression.Expression != "default" {
		t.Fatalf("retained default was mutated: %#v", sources.Defaults.Variables[0])
	}
	if sources.Controller.Variables[0].TypedExpression.Expression != "explicit" {
		t.Fatalf("retained controller declaration was mutated: %#v", sources.Controller.Variables[0])
	}
}

func TestLoadControllerStartupSourcesRequiresAdjacentDefaults(t *testing.T) {
	directory := t.TempDir()
	controllerPath := filepath.Join(directory, "controller.json")
	writeTestControllerConfig(t, controllerPath)

	_, err := loadControllerStartupSources(controllerPath)
	if err == nil {
		t.Fatal("expected an error")
	}
	wantPath := filepath.Join(directory, defaultsFilename)
	if !strings.Contains(err.Error(), wantPath) {
		t.Fatalf("error = %q, want defaults path %q", err, wantPath)
	}
}

func assertVariableExpression(t *testing.T, set variable.Set, key string, want any) {
	t.Helper()
	item, ok := set.LookupName(variable.Name{Namespace: variable.NamespaceControllerConfig, Key: key})
	if !ok {
		t.Fatalf("controller_config.%s is missing", key)
	}
	if item.TypedExpression.Expression != want {
		t.Fatalf("controller_config.%s expression = %#v, want %#v", key, item.TypedExpression.Expression, want)
	}
}

func TestControllerConfigFromArgsLoadsDefaultWithoutPath(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, defaultControllerConfigFilename)
	writeTestControllerConfig(t, path)

	config, err := controllerConfigFromArgs([]string{"controller"}, func() (string, error) {
		return filepath.Join(directory, "controller.exe"), nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(config.Variables) == 0 {
		t.Fatal("expected default variables")
	}
}

func TestControllerConfigFromArgsLoadsPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controller-config.json")
	writeTestControllerConfig(t, path)

	config, err := controllerConfigFromArgs([]string{"controller", "--config", path}, func() (string, error) {
		return "", fmt.Errorf("explicit config must not inspect the executable")
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(config.Variables) != 1 {
		t.Fatalf("unexpected variable count: %d", len(config.Variables))
	}
}

func TestControllerConfigFromArgsRejectsExecutablePathFailure(t *testing.T) {
	_, err := controllerConfigFromArgs([]string{"controller"}, func() (string, error) {
		return "", fmt.Errorf("executable unavailable")
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "determine controller executable path") {
		t.Fatalf("error = %q, want executable path context", err)
	}
}

func TestControllerConfigFromArgsRejectsUnappliedOverride(t *testing.T) {
	_, err := controllerConfigFromArgs([]string{"controller", "--override", `{}`}, func() (string, error) {
		return "", fmt.Errorf("override rejection must occur first")
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "overrides are not supported yet") {
		t.Fatalf("error = %q, want unsupported override context", err)
	}
}

func writeTestControllerConfig(t *testing.T, path string) {
	t.Helper()
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
}

func TestParseControllerStartupOptions(t *testing.T) {
	options, err := parseControllerStartupOptions([]string{
		"controller",
		"--config", "controller.json",
		`--override={"name":{"namespace":"override","key":"log_level"},"type":"string","expression":"debug"}`,
		"--override", `{"name":{"namespace":"override","key":"port"},"type":"int","expression":8081}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if options.ConfigPath != "controller.json" {
		t.Fatalf("config path = %q, want controller.json", options.ConfigPath)
	}
	if len(options.OverrideJSON) != 2 {
		t.Fatalf("override count = %d, want 2", len(options.OverrideJSON))
	}
	if !strings.Contains(options.OverrideJSON[0], `"key":"log_level"`) {
		t.Fatalf("first override was not preserved: %q", options.OverrideJSON[0])
	}
	if !strings.Contains(options.OverrideJSON[1], `"key":"port"`) {
		t.Fatalf("second override was not preserved: %q", options.OverrideJSON[1])
	}
}

func TestParseControllerStartupOptionsWithoutFlags(t *testing.T) {
	options, err := parseControllerStartupOptions([]string{"controller"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if options.ConfigPath != "" {
		t.Fatalf("config path = %q, want empty", options.ConfigPath)
	}
	if len(options.OverrideJSON) != 0 {
		t.Fatalf("override count = %d, want 0", len(options.OverrideJSON))
	}
}

func TestParseControllerStartupOptionsRejectsInvalidArguments(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "duplicate config", args: []string{"controller", "--config", "one.json", "--config=two.json"}},
		{name: "missing config value", args: []string{"controller", "--config"}},
		{name: "missing override value", args: []string{"controller", "--override"}},
		{name: "unknown flag", args: []string{"controller", "--unknown", "value"}},
		{name: "positional argument", args: []string{"controller", "controller.json"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseControllerStartupOptions(test.args)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), "parse controller startup arguments") {
				t.Fatalf("error = %q, want argument parsing context", err)
			}
		})
	}
}

func TestParseControllerStartupOverrides(t *testing.T) {
	raw := []string{
		`{"name":{"namespace":"override","key":"level"},"type":"string","expression":"debug"}`,
		`{"name":{"namespace":"override","key":"ports"},"type":"list","expression":[{"type":"int","expression":8080}]}`,
		`{"name":{"namespace":"override","key":"logging"},"type":"object","expression":{"enabled":{"type":"bool","expression":true}}}`,
	}

	scope, err := parseControllerStartupOverrides(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scope) != 3 {
		t.Fatalf("scope length = %d, want 3", len(scope))
	}
	for _, key := range []string{"level", "ports", "logging"} {
		if _, ok := scope[key]; !ok {
			t.Fatalf("override.%s is missing", key)
		}
	}
}

func TestParseControllerStartupOverridesAllowsEmptyScope(t *testing.T) {
	scope, err := parseControllerStartupOverrides(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scope) != 0 {
		t.Fatalf("scope length = %d, want 0", len(scope))
	}
}

func TestParseControllerStartupOverridesRejectsInvalidDeclarations(t *testing.T) {
	tests := []struct {
		name    string
		raw     []string
		want    []string
		missing string
	}{
		{
			name: "malformed recursive expression",
			raw: []string{
				`{"name":{"namespace":"override","key":"first"},"type":"string","expression":"ok"}`,
				`{"name":{"namespace":"override","key":"secret"},"type":"list","expression":[{"type":"invalid","expression":"do-not-repeat"}]}`,
			},
			want:    []string{"override argument 2"},
			missing: "do-not-repeat",
		},
		{
			name: "missing namespace",
			raw:  []string{`{"name":{"key":"port"},"type":"int","expression":8080}`},
			want: []string{"override argument 1"},
		},
		{
			name: "different namespace",
			raw:  []string{`{"name":{"namespace":"runtime","key":"port"},"type":"int","expression":8080}`},
			want: []string{"override argument 1", "runtime.port", "namespace must be override"},
		},
		{
			name:    "controller environment namespace",
			raw:     []string{`{"name":{"namespace":"controller_env","key":"PASSWORD"},"type":"string","expression":"hidden"}`},
			want:    []string{"override argument 1", "controller_env.PASSWORD", "namespace must be override"},
			missing: "hidden",
		},
		{
			name: "duplicate key",
			raw: []string{
				`{"name":{"namespace":"override","key":"port"},"type":"int","expression":8080}`,
				`{"name":{"namespace":"override","key":"port"},"type":"int","expression":9090}`,
			},
			want: []string{"override argument 2", "override.port", "duplicate"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseControllerStartupOverrides(test.raw)
			if err == nil {
				t.Fatal("expected an error")
			}
			for _, text := range test.want {
				if !strings.Contains(err.Error(), text) {
					t.Fatalf("error = %q, want %q", err, text)
				}
			}
			if test.missing != "" && strings.Contains(err.Error(), test.missing) {
				t.Fatalf("error reproduces expression value: %q", err)
			}
		})
	}
}

func TestControllerStartupOverridePrecedence(t *testing.T) {
	defaultScope, err := variable.NewScope(testDefaultVariable(variable.NamespaceControllerConfig, "level"))
	if err != nil {
		t.Fatal(err)
	}
	controllerScope, err := variable.NewScope(variable.Variable{
		Name:            variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "level"},
		TypedExpression: variable.TypedExpression{Type: variable.TypeString, Expression: "info"},
	})
	if err != nil {
		t.Fatal(err)
	}
	overrideScope, err := parseControllerStartupOverrides([]string{
		`{"name":{"namespace":"override","key":"level"},"type":"string","expression":"debug"}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	set := variable.NewSet(defaultScope, controllerScope, overrideScope)
	unqualified, ok := set.Lookup("level")
	if !ok || unqualified.TypedExpression.Expression != "debug" {
		t.Fatalf("unqualified level = %#v, want override", unqualified)
	}
	controller, ok := set.LookupName(variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "level"})
	if !ok || controller.TypedExpression.Expression != "info" {
		t.Fatalf("qualified controller_config.level = %#v, want controller", controller)
	}
	override, ok := set.LookupName(variable.Name{Namespace: variable.NamespaceOverride, Key: "level"})
	if !ok || override.TypedExpression.Expression != "debug" {
		t.Fatalf("qualified override.level = %#v, want override", override)
	}
}

func TestNewStartupRuntimeScope(t *testing.T) {
	startedAt := time.Date(2026, time.July, 2, 20, 15, 16, 123456789, time.FixedZone("test", -4*60*60))
	scope, err := newStartupRuntimeScope(1234, "instance-001", startedAt, "v0.8.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scope) != 4 {
		t.Fatalf("scope length = %d, want 4", len(scope))
	}

	tests := []struct {
		key       string
		wantType  variable.Type
		wantValue any
	}{
		{key: "controller_process_id", wantType: variable.TypeInt, wantValue: 1234},
		{key: "controller_instance_id", wantType: variable.TypeString, wantValue: "instance-001"},
		{key: "controller_started_at", wantType: variable.TypeDatetime, wantValue: startedAt.UTC().Format(time.RFC3339Nano)},
		{key: "controller_build_version", wantType: variable.TypeString, wantValue: "v0.8.0"},
	}
	for _, test := range tests {
		declaration, ok := scope[test.key]
		if !ok {
			t.Fatalf("runtime.%s is missing", test.key)
		}
		if declaration.Name.Namespace != variable.NamespaceRuntime {
			t.Fatalf("%s namespace = %q, want runtime", test.key, declaration.Name.Namespace)
		}
		if declaration.TypedExpression.Type != test.wantType {
			t.Fatalf("%s type = %s, want %s", test.key, declaration.TypedExpression.Type, test.wantType)
		}
		if declaration.TypedExpression.Expression != test.wantValue {
			t.Fatalf("%s value = %#v, want %#v", test.key, declaration.TypedExpression.Expression, test.wantValue)
		}
	}

	first := scope["controller_instance_id"].TypedExpression.Expression
	second := scope["controller_instance_id"].TypedExpression.Expression
	if first != second {
		t.Fatalf("instance ID changed from %#v to %#v", first, second)
	}
}

func TestNewStartupRuntimeScopeRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name         string
		processID    int
		instanceID   string
		buildVersion string
		want         string
	}{
		{name: "zero process ID", processID: 0, instanceID: "instance-001", buildVersion: "v0.8.0", want: "runtime.controller_process_id"},
		{name: "negative process ID", processID: -1, instanceID: "instance-001", buildVersion: "v0.8.0", want: "runtime.controller_process_id"},
		{name: "empty instance ID", processID: 1234, instanceID: "", buildVersion: "v0.8.0", want: "runtime.controller_instance_id"},
		{name: "empty build version", processID: 1234, instanceID: "instance-001", buildVersion: "", want: "runtime.controller_build_version"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := newStartupRuntimeScope(test.processID, test.instanceID, time.Unix(0, 0), test.buildVersion)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %q, want %q", err, test.want)
			}
		})
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

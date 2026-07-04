package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"
	"time"

	"goetl/internal/ledger"
	"goetl/internal/model"
	"goetl/internal/persistence"
	"goetl/internal/variable"
	"goetl/internal/workflow"
)

func TestBuildControllerServerUsesStartupPrecedenceAndRecoveryMode(t *testing.T) {
	dir := t.TempDir()
	controllerPath, defaultDBPath, overrideDBPath := writeControllerStartupFiles(t, dir)
	overrideDBJSON := filepath.ToSlash(overrideDBPath)

	server, release, err := buildControllerServer(
		[]string{"controller", "--config", controllerPath, "--override", `{"name":{"namespace":"override","key":"main_database_connection_string"},"type":"string","expression":"` + overrideDBJSON + `"}`},
		func() (string, error) { return filepath.Join(dir, "controller-binary"), nil },
		func(string) (string, bool) { return "", false },
		func() (string, error) { return dir, nil },
		func() int { return 1234 },
		func() time.Time { return time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC) },
		func(int) string { return "cafebabe" },
		func() string { return "test-version" },
	)
	if err != nil {
		t.Fatalf("buildControllerServer() error = %v", err)
	}
	t.Cleanup(func() {
		if release != nil {
			if err := release(); err != nil {
				t.Fatalf("release controller startup resources: %v", err)
			}
		}
	})

	if server.Addr != "localhost:9091" {
		t.Fatalf("server addr = %q, want localhost:9091", server.Addr)
	}

	statusResp := httptest.NewRecorder()
	server.Handler.ServeHTTP(statusResp, httptest.NewRequest(http.MethodGet, "/status", nil))
	if statusResp.Code != http.StatusServiceUnavailable {
		t.Fatalf("status code = %d, want 503 in recovery mode", statusResp.Code)
	}

	healthResp := httptest.NewRecorder()
	server.Handler.ServeHTTP(healthResp, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if healthResp.Code != http.StatusNoContent {
		t.Fatalf("health code = %d, want 204", healthResp.Code)
	}

	if _, err := os.Stat(defaultDBPath); err != nil {
		t.Fatalf("default database path missing: %v", err)
	}
	if _, err := os.Stat(overrideDBPath); !os.IsNotExist(err) {
		t.Fatalf("override database path exists or unreadable: %v", err)
	}
}

func TestBuildControllerServerFailsClosedBeforeBind(t *testing.T) {
	dir := t.TempDir()
	_, _, _ = writeControllerStartupFiles(t, dir)
	badControllerPath := filepath.Join(dir, "bad-controller.json")
	if err := os.WriteFile(badControllerPath, []byte(`{"api_version":"goet/v1alpha1","kind":"Controller","variables":[{"name":{"namespace":"controller_config","key":"main_database_driver"},"type":"string","expression":"bad"}]}`), 0600); err != nil {
		t.Fatalf("write bad controller config: %v", err)
	}

	server, release, err := buildControllerServer(
		[]string{"controller", "--config", badControllerPath},
		func() (string, error) { return filepath.Join(dir, "controller-binary"), nil },
		func(string) (string, bool) { return "", false },
		func() (string, error) { return dir, nil },
		func() int { return 1234 },
		func() time.Time { return time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC) },
		func(int) string { return "cafebabe" },
		func() string { return "test-version" },
	)
	if err == nil || !strings.Contains(err.Error(), "controller database failed") {
		t.Fatalf("error = %v, want controller database failure", err)
	}
	if server != nil || release != nil {
		t.Fatalf("server = %#v release = %T, want nils on startup failure", server, release)
	}

	if _, err := os.Stat(filepath.Join(dir, "controller-startup.sqlite")); !os.IsNotExist(err) {
		t.Fatalf("unexpected database file created for failing startup: %v", err)
	}
}

func TestControllerCanHoldWorkflowExecutionStore(t *testing.T) {
	ctx := context.Background()
	store, err := persistence.OpenStore(ctx, persistence.Config{
		Driver:           persistence.DriverSQLite,
		ConnectionString: filepath.Join(t.TempDir(), "workflow-execution.sqlite"),
	})
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	controller := newController(nil)
	controller.workflowStore = store

	version, err := controller.workflowStore.CurrentSchemaVersion(ctx)
	if err != nil {
		t.Fatalf("CurrentSchemaVersion() error = %v", err)
	}
	if version != persistence.SupportedSchemaVersion {
		t.Fatalf("schema version = %d, want %d", version, persistence.SupportedSchemaVersion)
	}
}

func writeControllerStartupFiles(t *testing.T, dir string) (string, string, string) {
	t.Helper()

	defaultDBPath := filepath.Join(dir, "controller-startup.sqlite")
	overrideDBPath := filepath.Join(dir, "override-startup.sqlite")
	defaultDBJSON := filepath.ToSlash(defaultDBPath)
	controllerPath := filepath.Join(dir, "controller.json")
	defaultsPath := filepath.Join(dir, "defaults.json")

	controllerJSON := `{
  "api_version": "goet/v1alpha1",
  "kind": "Controller",
  "variables": [
    {
      "name": {"namespace": "controller_config", "key": "main_database_driver"},
      "type": "string",
      "expression": "sqlite"
    },
    {
      "name": {"namespace": "controller_config", "key": "main_database_connection_string"},
      "type": "string",
      "expression": "` + defaultDBJSON + `"
    },
    {
      "name": {"namespace": "controller_config", "key": "controller_url"},
      "type": "string",
      "expression": "http://localhost:9091"
    }
  ]
}`

	defaultsJSON := `{
  "api_version": "goet/v1alpha1",
  "kind": "Defaults",
  "variables": [
    {"name": {"namespace": "controller_config", "key": "controller_listen_host"}, "type": "string", "expression": "localhost"},
    {"name": {"namespace": "controller_config", "key": "controller_listen_port"}, "type": "int", "expression": 9091},
    {"name": {"namespace": "controller_config", "key": "controller_root_dir"}, "type": "path", "expression": "./.run"},
    {"name": {"namespace": "controller_config", "key": "controller_git_cache_path"}, "type": "path", "expression": "${controller_root_dir}/git_cache"},
    {"name": {"namespace": "controller_config", "key": "controller_temp_path"}, "type": "path", "expression": "${controller_root_dir}/temp"},
    {"name": {"namespace": "controller_config", "key": "controller_artifact_cache_path"}, "type": "path", "expression": "${controller_root_dir}/artifacts"},
    {"name": {"namespace": "controller_config", "key": "caretaker_interval_schedule_milliseconds"}, "type": "int", "expression": 60000},
    {"name": {"namespace": "controller_config", "key": "caretaker_missed_interval_limit"}, "type": "int", "expression": 1},
    {"name": {"namespace": "controller_config", "key": "resolver_max_depth"}, "type": "int", "expression": 10},
    {"name": {"namespace": "controller_config", "key": "controller_log_root_path"}, "type": "path", "expression": "${controller_root_dir}/logs"},
    {"name": {"namespace": "controller_config", "key": "controller_filesystem_logging_enabled"}, "type": "bool", "expression": true},
    {"name": {"namespace": "controller_config", "key": "controller_log_level"}, "type": "string", "expression": "info"},
    {"name": {"namespace": "controller_config", "key": "controller_read_header_timeout_milliseconds"}, "type": "int", "expression": 5000},
    {"name": {"namespace": "controller_config", "key": "controller_read_timeout_milliseconds"}, "type": "int", "expression": 30000},
    {"name": {"namespace": "controller_config", "key": "controller_write_timeout_milliseconds"}, "type": "int", "expression": 30000},
    {"name": {"namespace": "controller_config", "key": "controller_idle_timeout_milliseconds"}, "type": "int", "expression": 120000},
    {"name": {"namespace": "controller_config", "key": "controller_shutdown_timeout_milliseconds"}, "type": "int", "expression": 30000},
    {"name": {"namespace": "controller_config", "key": "controller_max_request_bytes"}, "type": "int", "expression": 16777216},
    {"name": {"namespace": "controller_config", "key": "controller_max_header_bytes"}, "type": "int", "expression": 1048576},
    {"name": {"namespace": "controller_config", "key": "controller_git_cache_max_size_mb"}, "type": "int", "expression": 10240},
    {"name": {"namespace": "controller_config", "key": "controller_git_cache_retention_milliseconds"}, "type": "int", "expression": 604800000},
    {"name": {"namespace": "controller_config", "key": "controller_git_fetch_timeout_milliseconds"}, "type": "int", "expression": 300000},
    {"name": {"namespace": "controller_config", "key": "controller_git_fetch_concurrency"}, "type": "int", "expression": 4},
    {"name": {"namespace": "controller_config", "key": "controller_temp_cleanup_age_milliseconds"}, "type": "int", "expression": 86400000},
    {"name": {"namespace": "controller_config", "key": "controller_artifact_cache_max_size_mb"}, "type": "int", "expression": 10240},
    {"name": {"namespace": "controller_config", "key": "controller_artifact_cache_retention_milliseconds"}, "type": "int", "expression": 604800000},
    {"name": {"namespace": "controller_config", "key": "controller_storage_min_free_mb"}, "type": "int", "expression": 1024}
  ]
}`

	if err := os.WriteFile(controllerPath, []byte(controllerJSON), 0600); err != nil {
		t.Fatalf("write controller config: %v", err)
	}
	if err := os.WriteFile(defaultsPath, []byte(defaultsJSON), 0600); err != nil {
		t.Fatalf("write defaults config: %v", err)
	}

	return controllerPath, defaultDBPath, overrideDBPath
}

func TestResolveControllerFilesystemPaths(t *testing.T) {
	workingDirectory := filepath.Join(t.TempDir(), "working")
	outsideRoot := filepath.Join(t.TempDir(), "shared", "artifacts")
	controllerScope := testStartupScope(t,
		testStartupVariable(variable.NamespaceControllerConfig, "controller_root_dir", variable.TypePath, "./state"),
		testStartupVariable(variable.NamespaceControllerConfig, "controller_git_cache_path", variable.TypePath, "${controller_root_dir}/git/../git-cache"),
		testStartupVariable(variable.NamespaceControllerConfig, "controller_temp_path", variable.TypePath, "controller-temp"),
		testStartupVariable(variable.NamespaceControllerConfig, "controller_artifact_cache_path", variable.TypePath, "ignored"),
	)
	overrideScope := testStartupScope(t,
		testStartupVariable(variable.NamespaceOverride, "controller_artifact_cache_path", variable.TypePath, outsideRoot),
	)
	resolver := variable.NewResolver(variable.NewSet(controllerScope, overrideScope), variable.ResolverConfig{})

	paths, err := resolveControllerFilesystemPaths(resolver, workingDirectory)
	if err != nil {
		t.Fatalf("resolveControllerFilesystemPaths() error = %v", err)
	}

	want := controllerFilesystemPaths{
		Root:          filepath.Join(workingDirectory, "state"),
		GitCache:      filepath.Join(workingDirectory, "state", "git-cache"),
		Temp:          filepath.Join(workingDirectory, "controller-temp"),
		ArtifactCache: filepath.Clean(outsideRoot),
	}
	if paths != want {
		t.Fatalf("resolveControllerFilesystemPaths() = %#v, want %#v", paths, want)
	}
}

func TestResolveControllerFilesystemPathsRejectsInvalidWorkingDirectory(t *testing.T) {
	resolver := testMainDatabaseResolver(t)
	for _, workingDirectory := range []string{"", "relative"} {
		_, err := resolveControllerFilesystemPaths(resolver, workingDirectory)
		if err == nil || !strings.Contains(err.Error(), "working directory") {
			t.Fatalf("working directory %q error = %v, want working-directory context", workingDirectory, err)
		}
	}
}

func TestResolveControllerFilesystemPathsReportsInvalidVariable(t *testing.T) {
	tests := []struct {
		name      string
		variables []variable.Variable
		want      string
	}{
		{name: "missing", want: "controller_root_dir"},
		{
			name: "wrong type",
			variables: []variable.Variable{
				testStartupVariable(variable.NamespaceControllerConfig, "controller_root_dir", variable.TypeString, "state"),
			},
			want: "controller_root_dir has type string, want path",
		},
		{
			name: "empty",
			variables: []variable.Variable{
				testStartupVariable(variable.NamespaceControllerConfig, "controller_root_dir", variable.TypePath, ""),
			},
			want: "controller_root_dir is required",
		},
		{
			name: "missing dependency",
			variables: []variable.Variable{
				testStartupVariable(variable.NamespaceControllerConfig, "controller_root_dir", variable.TypePath, "${missing_root}"),
			},
			want: "missing_root",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := testMainDatabaseResolver(t, tt.variables...)
			_, err := resolveControllerFilesystemPaths(resolver, t.TempDir())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			if !strings.Contains(err.Error(), "controller startup filesystem") {
				t.Fatalf("error = %v, want filesystem consumer context", err)
			}
		})
	}
}

func TestResolveControllerOperationalPolicy(t *testing.T) {
	workingDirectory := filepath.Join(t.TempDir(), "working")
	resolver := variable.NewResolver(variable.NewSet(testStartupScope(t,
		testStartupVariable(variable.NamespaceControllerConfig, "resolver_max_depth", variable.TypeInt, 12),
		testStartupVariable(variable.NamespaceControllerConfig, "caretaker_interval_schedule_milliseconds", variable.TypeInt, 60000),
		testStartupVariable(variable.NamespaceControllerConfig, "caretaker_missed_interval_limit", variable.TypeInt, 2),
		testStartupVariable(variable.NamespaceControllerConfig, "controller_git_cache_max_size_mb", variable.TypeInt, 10240),
		testStartupVariable(variable.NamespaceControllerConfig, "controller_git_cache_retention_milliseconds", variable.TypeInt, 604800000),
		testStartupVariable(variable.NamespaceControllerConfig, "controller_git_fetch_timeout_milliseconds", variable.TypeInt, 300000),
		testStartupVariable(variable.NamespaceControllerConfig, "controller_git_fetch_concurrency", variable.TypeInt, 4),
		testStartupVariable(variable.NamespaceControllerConfig, "controller_temp_cleanup_age_milliseconds", variable.TypeInt, 86400000),
		testStartupVariable(variable.NamespaceControllerConfig, "controller_artifact_cache_max_size_mb", variable.TypeInt, 20480),
		testStartupVariable(variable.NamespaceControllerConfig, "controller_artifact_cache_retention_milliseconds", variable.TypeInt, 604800000),
		testStartupVariable(variable.NamespaceControllerConfig, "controller_storage_min_free_mb", variable.TypeInt, 1024),
		testStartupVariable(variable.NamespaceControllerConfig, "controller_filesystem_logging_enabled", variable.TypeBool, true),
		testStartupVariable(variable.NamespaceControllerConfig, "controller_log_root_path", variable.TypePath, "./logs"),
		testStartupVariable(variable.NamespaceControllerConfig, "controller_log_level", variable.TypeString, "debug"),
	)), variable.ResolverConfig{})

	policy, err := resolveControllerOperationalPolicy(resolver, workingDirectory)
	if err != nil {
		t.Fatalf("resolveControllerOperationalPolicy() error = %v", err)
	}

	if policy.ResolverMaxDepth != 12 {
		t.Fatalf("resolver max depth = %d, want 12", policy.ResolverMaxDepth)
	}
	if policy.CaretakerIntervalScheduleMillis != 60000 || policy.CaretakerMissedIntervalLimit != 2 {
		t.Fatalf("caretaker policy = %+v", policy)
	}
	if policy.GitCacheMaxSizeMB != 10240 || policy.GitFetchConcurrency != 4 {
		t.Fatalf("git policy = %+v", policy)
	}
	if policy.TempCleanupAgeMillis != 86400000 || policy.StorageMinFreeMB != 1024 {
		t.Fatalf("storage policy = %+v", policy)
	}
	if !policy.FilesystemLoggingEnabled {
		t.Fatal("filesystem logging should be enabled")
	}
	if policy.LogRootPath != filepath.Join(workingDirectory, "logs") {
		t.Fatalf("log root path = %q, want joined working directory", policy.LogRootPath)
	}
	if policy.LogLevel != "debug" {
		t.Fatalf("log level = %q, want debug", policy.LogLevel)
	}
}

func TestResolveControllerOperationalPolicyRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name      string
		variables []variable.Variable
		want      string
	}{
		{
			name: "missing",
			want: "resolver_max_depth",
		},
		{
			name: "wrong type",
			variables: []variable.Variable{
				testStartupVariable(variable.NamespaceControllerConfig, "resolver_max_depth", variable.TypeString, "ten"),
			},
			want: "resolver_max_depth has type string, want int",
		},
		{
			name: "not positive",
			variables: []variable.Variable{
				testStartupVariable(variable.NamespaceControllerConfig, "resolver_max_depth", variable.TypeInt, 0),
			},
			want: "resolver_max_depth must be greater than zero",
		},
		{
			name: "log level missing",
			variables: []variable.Variable{
				testStartupVariable(variable.NamespaceControllerConfig, "resolver_max_depth", variable.TypeInt, 1),
				testStartupVariable(variable.NamespaceControllerConfig, "caretaker_interval_schedule_milliseconds", variable.TypeInt, 1),
				testStartupVariable(variable.NamespaceControllerConfig, "caretaker_missed_interval_limit", variable.TypeInt, 1),
				testStartupVariable(variable.NamespaceControllerConfig, "controller_git_cache_max_size_mb", variable.TypeInt, 1),
				testStartupVariable(variable.NamespaceControllerConfig, "controller_git_cache_retention_milliseconds", variable.TypeInt, 1),
				testStartupVariable(variable.NamespaceControllerConfig, "controller_git_fetch_timeout_milliseconds", variable.TypeInt, 1),
				testStartupVariable(variable.NamespaceControllerConfig, "controller_git_fetch_concurrency", variable.TypeInt, 1),
				testStartupVariable(variable.NamespaceControllerConfig, "controller_temp_cleanup_age_milliseconds", variable.TypeInt, 1),
				testStartupVariable(variable.NamespaceControllerConfig, "controller_artifact_cache_max_size_mb", variable.TypeInt, 1),
				testStartupVariable(variable.NamespaceControllerConfig, "controller_artifact_cache_retention_milliseconds", variable.TypeInt, 1),
				testStartupVariable(variable.NamespaceControllerConfig, "controller_storage_min_free_mb", variable.TypeInt, 1),
				testStartupVariable(variable.NamespaceControllerConfig, "controller_filesystem_logging_enabled", variable.TypeBool, true),
				testStartupVariable(variable.NamespaceControllerConfig, "controller_log_root_path", variable.TypePath, "./logs"),
			},
			want: "controller_log_level",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := variable.NewResolver(variable.NewSet(testStartupScope(t, tt.variables...)), variable.ResolverConfig{})
			_, err := resolveControllerOperationalPolicy(resolver, t.TempDir())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			if !strings.Contains(err.Error(), "controller startup policy") {
				t.Fatalf("error = %v, want policy consumer context", err)
			}
		})
	}
}

func TestResolveControllerHTTPSettings(t *testing.T) {
	resolver := variable.NewResolver(variable.NewSet(testStartupScope(t,
		testStartupVariable(variable.NamespaceControllerConfig, "controller_listen_host", variable.TypeString, "0.0.0.0"),
		testStartupVariable(variable.NamespaceControllerConfig, "controller_listen_port", variable.TypeInt, 9090),
		testStartupVariable(variable.NamespaceControllerConfig, "controller_url", variable.TypeString, "http://controller.example"),
		testStartupVariable(variable.NamespaceControllerConfig, "controller_read_header_timeout_milliseconds", variable.TypeInt, 1000),
		testStartupVariable(variable.NamespaceControllerConfig, "controller_read_timeout_milliseconds", variable.TypeInt, 2000),
		testStartupVariable(variable.NamespaceControllerConfig, "controller_write_timeout_milliseconds", variable.TypeInt, 3000),
		testStartupVariable(variable.NamespaceControllerConfig, "controller_idle_timeout_milliseconds", variable.TypeInt, 4000),
		testStartupVariable(variable.NamespaceControllerConfig, "controller_shutdown_timeout_milliseconds", variable.TypeInt, 5000),
		testStartupVariable(variable.NamespaceControllerConfig, "controller_max_request_bytes", variable.TypeInt, 6000),
		testStartupVariable(variable.NamespaceControllerConfig, "controller_max_header_bytes", variable.TypeInt, 7000),
	)), variable.ResolverConfig{})

	settings, err := resolveControllerHTTPSettings(resolver)
	if err != nil {
		t.Fatalf("resolveControllerHTTPSettings() error = %v", err)
	}

	if settings.ListenHost != "0.0.0.0" || settings.ListenPort != 9090 {
		t.Fatalf("listen settings = %+v", settings)
	}
	if settings.AdvertisedURL != "http://controller.example" {
		t.Fatalf("advertised url = %q, want http://controller.example", settings.AdvertisedURL)
	}
	if settings.ReadHeaderTimeoutMillis != 1000 || settings.ReadTimeoutMillis != 2000 {
		t.Fatalf("read timeouts = %+v", settings)
	}
	if settings.WriteTimeoutMillis != 3000 || settings.IdleTimeoutMillis != 4000 {
		t.Fatalf("write/idle timeouts = %+v", settings)
	}
	if settings.ShutdownTimeoutMillis != 5000 {
		t.Fatalf("shutdown timeout = %d, want 5000", settings.ShutdownTimeoutMillis)
	}
	if settings.MaxRequestBytes != 6000 || settings.MaxHeaderBytes != 7000 {
		t.Fatalf("size limits = %+v", settings)
	}
}

func TestResolveControllerHTTPSettingsRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name      string
		variables []variable.Variable
		want      string
	}{
		{
			name: "missing",
			want: "controller_listen_host",
		},
		{
			name: "wrong type",
			variables: []variable.Variable{
				testStartupVariable(variable.NamespaceControllerConfig, "controller_listen_host", variable.TypeInt, 8080),
			},
			want: "controller_listen_host has type int, want string",
		},
		{
			name: "empty string",
			variables: []variable.Variable{
				testStartupVariable(variable.NamespaceControllerConfig, "controller_listen_host", variable.TypeString, ""),
			},
			want: "controller_listen_host is required",
		},
		{
			name: "invalid port",
			variables: []variable.Variable{
				testStartupVariable(variable.NamespaceControllerConfig, "controller_listen_host", variable.TypeString, "localhost"),
				testStartupVariable(variable.NamespaceControllerConfig, "controller_listen_port", variable.TypeInt, 0),
			},
			want: "controller_listen_port must be greater than zero",
		},
		{
			name: "missing url",
			variables: []variable.Variable{
				testStartupVariable(variable.NamespaceControllerConfig, "controller_listen_host", variable.TypeString, "localhost"),
				testStartupVariable(variable.NamespaceControllerConfig, "controller_listen_port", variable.TypeInt, 8080),
			},
			want: "controller_url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := variable.NewResolver(variable.NewSet(testStartupScope(t, tt.variables...)), variable.ResolverConfig{})
			_, err := resolveControllerHTTPSettings(resolver)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			if !strings.Contains(err.Error(), "controller startup http") {
				t.Fatalf("error = %v, want HTTP consumer context", err)
			}
		})
	}
}

func TestNextWorkHandler(t *testing.T) {
	controller := newTestController()
	request := httptest.NewRequest(http.MethodGet, "/work/next", nil)
	response := httptest.NewRecorder()

	controller.nextWorkHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status code: %d", response.Code)
	}

	var item model.WorkItem
	if err := json.NewDecoder(response.Body).Decode(&item); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if item.ID != "test-001" {
		t.Fatalf("unexpected id: %q", item.ID)
	}
}

func TestNextWorkHandlerReturnsNoContentWhenQueueIsEmpty(t *testing.T) {
	controller := newController(nil)
	request := httptest.NewRequest(http.MethodGet, "/work/next", nil)
	response := httptest.NewRecorder()

	controller.nextWorkHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestNextWorkHandlerSkipsReusablePendingWork(t *testing.T) {
	controller := newControllerWithCompletedAttempt(t, model.WorkCompletion{
		ID:                   "test-001",
		AttemptID:            "attempt-001",
		WorkflowDefinitionID: "workflow-definition-001",
		WorkflowFingerprint:  "workflow-fingerprint",
		WorkflowInstanceID:   "workflow-instance-001",
		StepDefinitionID:     "step-definition-001",
		StepFingerprint:      "step-fingerprint",
		StepInstanceID:       "step-instance-001",
		WorkItemFingerprint:  "work-item-fingerprint",
		InputFingerprint:     "input-fingerprint",
		OutputFingerprint:    "output-fingerprint",
		CodeVersion:          "code-version",
		StartedAt:            "2026-06-06T12:00:00Z",
		CompletedAt:          "2026-06-06T12:01:00Z",
	})
	controller.pending = []model.WorkItem{
		reusableTestWorkItem("test-001"),
		{
			ID:             "test-002",
			Type:           model.WorkItemTypeWriteDemoOutput,
			OutputFilename: "result-2.txt",
		},
	}
	request := httptest.NewRequest(http.MethodGet, "/work/next", nil)
	response := httptest.NewRecorder()

	controller.nextWorkHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status code: %d", response.Code)
	}

	var item model.WorkItem
	if err := json.NewDecoder(response.Body).Decode(&item); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if item.ID != "test-002" {
		t.Fatalf("assigned item id = %q, want test-002", item.ID)
	}
	if _, ok := controller.assigned["test-001"]; ok {
		t.Fatal("skipped item should not be assigned")
	}

	var skippedCount int
	if err := controller.ledger.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM attempts WHERE status = ?`, string(ledger.AttemptStatusSkipped)).Scan(&skippedCount); err != nil {
		t.Fatalf("query skipped count: %v", err)
	}
	if skippedCount != 1 {
		t.Fatalf("skipped count = %d, want 1", skippedCount)
	}
}

func TestNextWorkHandlerReturnsNoContentWhenAllPendingWorkIsReusable(t *testing.T) {
	controller := newControllerWithCompletedAttempt(t, model.WorkCompletion{
		ID:                   "test-001",
		AttemptID:            "attempt-001",
		WorkflowDefinitionID: "workflow-definition-001",
		WorkflowFingerprint:  "workflow-fingerprint",
		WorkflowInstanceID:   "workflow-instance-001",
		StepDefinitionID:     "step-definition-001",
		StepFingerprint:      "step-fingerprint",
		StepInstanceID:       "step-instance-001",
		WorkItemFingerprint:  "work-item-fingerprint",
		InputFingerprint:     "input-fingerprint",
		OutputFingerprint:    "output-fingerprint",
		CodeVersion:          "code-version",
		StartedAt:            "2026-06-06T12:00:00Z",
		CompletedAt:          "2026-06-06T12:01:00Z",
	})
	controller.pending = []model.WorkItem{reusableTestWorkItem("test-001")}
	request := httptest.NewRequest(http.MethodGet, "/work/next", nil)
	response := httptest.NewRecorder()

	controller.nextWorkHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
	if len(controller.assigned) != 0 {
		t.Fatalf("assigned count = %d, want 0", len(controller.assigned))
	}
}

func TestNextWorkHandlerRejectsPost(t *testing.T) {
	controller := newTestController()
	request := httptest.NewRequest(http.MethodPost, "/work/next", nil)
	response := httptest.NewRecorder()

	controller.nextWorkHandler(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestCompleteWorkHandler(t *testing.T) {
	controller := newTestController()
	assignNextWork(t, controller)

	request := httptest.NewRequest(http.MethodPost, "/work/complete", bytes.NewBufferString(`{"id":"test-001"}`))
	response := httptest.NewRecorder()

	controller.completeWorkHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestCompleteWorkHandlerRecordsAttemptWhenMetadataPresent(t *testing.T) {
	controller := newTestController()
	db := testSQLiteMainDatabase(t)
	defer db.Close()
	controller.ledger = db
	assignNextWork(t, controller)

	request := httptest.NewRequest(http.MethodPost, "/work/complete", bytes.NewBufferString(`{
		"id":"test-001",
		"attempt_id":"attempt-001",
		"workflow_definition_id":"workflow-definition-001",
		"workflow_fingerprint":"workflow-fingerprint",
		"workflow_instance_id":"workflow-instance-001",
		"step_definition_id":"step-definition-001",
		"step_fingerprint":"step-fingerprint",
		"step_instance_id":"step-instance-001",
		"work_item_fingerprint":"work-item-fingerprint",
		"input_fingerprint":"input-fingerprint",
		"output_fingerprint":"output-fingerprint",
		"code_version":"code-version",
		"started_at":"2026-06-06T12:00:00Z",
		"completed_at":"2026-06-06T12:01:00Z",
		"parameters": {
			"input_path": {
				"type": "path",
				"value": "demo-summary-input.txt"
			}
		}
	}`))
	response := httptest.NewRecorder()

	controller.completeWorkHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", response.Code)
	}

	var count int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM attempts`).Scan(&count); err != nil {
		t.Fatalf("query attempt count: %v", err)
	}
	if count != 1 {
		t.Fatalf("attempt count = %d, want 1", count)
	}

	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM attempt_variables WHERE namespace = 'runtime'`).Scan(&count); err != nil {
		t.Fatalf("query attempt variable count: %v", err)
	}
	if count != 14 {
		t.Fatalf("runtime attempt variable count = %d, want 14", count)
	}

	var valueJSON string
	if err := db.QueryRowContext(context.Background(), `SELECT value_json FROM attempt_variables WHERE namespace = 'runtime' AND name = 'workflow_definition_id'`).Scan(&valueJSON); err != nil {
		t.Fatalf("query workflow definition variable: %v", err)
	}
	if valueJSON != `"workflow-definition-001"` {
		t.Fatalf("workflow_definition_id value_json = %q", valueJSON)
	}

	if err := db.QueryRowContext(context.Background(), `SELECT value_json FROM attempt_variables WHERE namespace = 'runtime' AND name = 'workflow_fingerprint'`).Scan(&valueJSON); err != nil {
		t.Fatalf("query workflow fingerprint variable: %v", err)
	}
	if valueJSON != `"workflow-fingerprint"` {
		t.Fatalf("workflow_fingerprint value_json = %q", valueJSON)
	}

	if err := db.QueryRowContext(context.Background(), `SELECT value_json FROM attempt_variables WHERE namespace = 'runtime' AND name = 'workflow_instance_id'`).Scan(&valueJSON); err != nil {
		t.Fatalf("query workflow instance variable: %v", err)
	}
	if valueJSON != `"workflow-instance-001"` {
		t.Fatalf("workflow_instance_id value_json = %q", valueJSON)
	}

	if err := db.QueryRowContext(context.Background(), `SELECT value_json FROM attempt_variables WHERE namespace = 'work_item' AND name = 'input_path'`).Scan(&valueJSON); err != nil {
		t.Fatalf("query input path variable: %v", err)
	}
	if valueJSON != `"demo-summary-input.txt"` {
		t.Fatalf("input_path value_json = %q", valueJSON)
	}
}

func TestPriorCompletedAttemptFindsMatchingFingerprint(t *testing.T) {
	controller := newTestController()
	db := testSQLiteMainDatabase(t)
	defer db.Close()
	controller.ledger = db

	completion := model.WorkCompletion{
		ID:                   "test-001",
		AttemptID:            "attempt-001",
		WorkflowDefinitionID: "workflow-definition-001",
		WorkflowFingerprint:  "workflow-fingerprint",
		WorkflowInstanceID:   "workflow-instance-001",
		StepDefinitionID:     "step-definition-001",
		StepFingerprint:      "step-fingerprint",
		StepInstanceID:       "step-instance-001",
		WorkItemFingerprint:  "work-item-fingerprint",
		InputFingerprint:     "input-fingerprint",
		OutputFingerprint:    "output-fingerprint",
		CodeVersion:          "code-version",
		StartedAt:            "2026-06-06T12:00:00Z",
		CompletedAt:          "2026-06-06T12:01:00Z",
	}
	attempt, _, err := attemptFromCompletion(completion)
	if err != nil {
		t.Fatalf("build attempt: %v", err)
	}
	if err := controller.recordAttempt(context.Background(), attempt); err != nil {
		t.Fatalf("record attempt: %v", err)
	}

	found, ok, err := controller.priorCompletedAttempt(context.Background(), model.WorkItem{
		WorkItemFingerprint: "work-item-fingerprint",
	})
	if err != nil {
		t.Fatalf("priorCompletedAttempt() error = %v", err)
	}
	if !ok {
		t.Fatal("expected a prior attempt")
	}
	if found.ID != "attempt-001" {
		t.Fatalf("attempt id = %q, want attempt-001", found.ID)
	}
}

func TestPriorCompletedAttemptReturnsMissingWithoutLedgerOrFingerprint(t *testing.T) {
	controller := newTestController()

	if attempt, ok, err := controller.priorCompletedAttempt(context.Background(), model.WorkItem{
		WorkItemFingerprint: "work-item-fingerprint",
	}); err != nil || ok {
		t.Fatalf("priorCompletedAttempt() = %+v, %v, %v; want missing nil error", attempt, ok, err)
	}

	db := testSQLiteMainDatabase(t)
	defer db.Close()
	controller.ledger = db

	if attempt, ok, err := controller.priorCompletedAttempt(context.Background(), model.WorkItem{}); err != nil || ok {
		t.Fatalf("priorCompletedAttempt() = %+v, %v, %v; want missing nil error", attempt, ok, err)
	}
}

func TestPriorCompletedAttemptMatchesWorkItem(t *testing.T) {
	item := model.WorkItem{
		WorkItemFingerprint: "work-item-fingerprint",
		InputFingerprint:    "input-fingerprint",
		OutputFingerprint:   "output-fingerprint",
		CodeVersion:         "code-version",
	}
	attempt := ledger.Attempt{
		WorkItemFingerprint: "work-item-fingerprint",
		InputFingerprint:    "input-fingerprint",
		OutputFingerprint:   "output-fingerprint",
		CodeVersion:         "code-version",
		Status:              ledger.AttemptStatusCompleted,
	}

	if !priorCompletedAttemptMatchesWorkItem(item, attempt) {
		t.Fatal("expected matching prior attempt")
	}
}

func TestPriorCompletedAttemptMatchesWorkItemRejectsMismatch(t *testing.T) {
	baseItem := model.WorkItem{
		WorkItemFingerprint: "work-item-fingerprint",
		InputFingerprint:    "input-fingerprint",
		OutputFingerprint:   "output-fingerprint",
		CodeVersion:         "code-version",
	}
	baseAttempt := ledger.Attempt{
		WorkItemFingerprint: "work-item-fingerprint",
		InputFingerprint:    "input-fingerprint",
		OutputFingerprint:   "output-fingerprint",
		CodeVersion:         "code-version",
		Status:              ledger.AttemptStatusCompleted,
	}

	tests := []struct {
		name    string
		item    model.WorkItem
		attempt ledger.Attempt
	}{
		{
			name:    "failed prior attempt",
			item:    baseItem,
			attempt: withAttemptStatus(baseAttempt, ledger.AttemptStatusFailed),
		},
		{
			name:    "work item fingerprint",
			item:    withWorkItemFingerprint(baseItem, "changed"),
			attempt: baseAttempt,
		},
		{
			name:    "input fingerprint",
			item:    withInputFingerprint(baseItem, "changed"),
			attempt: baseAttempt,
		},
		{
			name:    "output fingerprint",
			item:    withOutputFingerprint(baseItem, "changed"),
			attempt: baseAttempt,
		},
		{
			name:    "code version",
			item:    withCodeVersion(baseItem, "changed"),
			attempt: baseAttempt,
		},
		{
			name:    "missing current fingerprint",
			item:    withInputFingerprint(baseItem, ""),
			attempt: baseAttempt,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if priorCompletedAttemptMatchesWorkItem(test.item, test.attempt) {
				t.Fatal("expected prior attempt mismatch")
			}
		})
	}
}

func TestReusablePriorAttemptFindsMatchingAttempt(t *testing.T) {
	controller := newControllerWithCompletedAttempt(t, model.WorkCompletion{
		ID:                   "test-001",
		AttemptID:            "attempt-001",
		WorkflowDefinitionID: "workflow-definition-001",
		WorkflowFingerprint:  "workflow-fingerprint",
		WorkflowInstanceID:   "workflow-instance-001",
		StepDefinitionID:     "step-definition-001",
		StepFingerprint:      "step-fingerprint",
		StepInstanceID:       "step-instance-001",
		WorkItemFingerprint:  "work-item-fingerprint",
		InputFingerprint:     "input-fingerprint",
		OutputFingerprint:    "output-fingerprint",
		CodeVersion:          "code-version",
		StartedAt:            "2026-06-06T12:00:00Z",
		CompletedAt:          "2026-06-06T12:01:00Z",
	})

	attempt, ok, err := controller.reusablePriorAttempt(context.Background(), model.WorkItem{
		WorkItemFingerprint: "work-item-fingerprint",
		InputFingerprint:    "input-fingerprint",
		OutputFingerprint:   "output-fingerprint",
		CodeVersion:         "code-version",
	})
	if err != nil {
		t.Fatalf("reusablePriorAttempt() error = %v", err)
	}
	if !ok {
		t.Fatal("expected reusable prior attempt")
	}
	if attempt.ID != "attempt-001" {
		t.Fatalf("attempt id = %q, want attempt-001", attempt.ID)
	}
}

func TestReusablePriorAttemptRejectsMismatchedAttempt(t *testing.T) {
	controller := newControllerWithCompletedAttempt(t, model.WorkCompletion{
		ID:                   "test-001",
		AttemptID:            "attempt-001",
		WorkflowDefinitionID: "workflow-definition-001",
		WorkflowFingerprint:  "workflow-fingerprint",
		WorkflowInstanceID:   "workflow-instance-001",
		StepDefinitionID:     "step-definition-001",
		StepFingerprint:      "step-fingerprint",
		StepInstanceID:       "step-instance-001",
		WorkItemFingerprint:  "work-item-fingerprint",
		InputFingerprint:     "input-fingerprint",
		OutputFingerprint:    "output-fingerprint",
		CodeVersion:          "old-code-version",
		StartedAt:            "2026-06-06T12:00:00Z",
		CompletedAt:          "2026-06-06T12:01:00Z",
	})

	attempt, ok, err := controller.reusablePriorAttempt(context.Background(), model.WorkItem{
		WorkItemFingerprint: "work-item-fingerprint",
		InputFingerprint:    "input-fingerprint",
		OutputFingerprint:   "output-fingerprint",
		CodeVersion:         "new-code-version",
	})
	if err != nil {
		t.Fatalf("reusablePriorAttempt() error = %v", err)
	}
	if ok {
		t.Fatalf("unexpected reusable attempt: %+v", attempt)
	}
}

func TestWorkReuseDecisionReportsReusableAttempt(t *testing.T) {
	controller := newControllerWithCompletedAttempt(t, model.WorkCompletion{
		ID:                   "test-001",
		AttemptID:            "attempt-001",
		WorkflowDefinitionID: "workflow-definition-001",
		WorkflowFingerprint:  "workflow-fingerprint",
		WorkflowInstanceID:   "workflow-instance-001",
		StepDefinitionID:     "step-definition-001",
		StepFingerprint:      "step-fingerprint",
		StepInstanceID:       "step-instance-001",
		WorkItemFingerprint:  "work-item-fingerprint",
		InputFingerprint:     "input-fingerprint",
		OutputFingerprint:    "output-fingerprint",
		CodeVersion:          "code-version",
		StartedAt:            "2026-06-06T12:00:00Z",
		CompletedAt:          "2026-06-06T12:01:00Z",
	})

	decision, err := controller.workReuseDecision(context.Background(), model.WorkItem{
		WorkItemFingerprint: "work-item-fingerprint",
		InputFingerprint:    "input-fingerprint",
		OutputFingerprint:   "output-fingerprint",
		CodeVersion:         "code-version",
	})
	if err != nil {
		t.Fatalf("workReuseDecision() error = %v", err)
	}

	if !decision.Reusable || decision.Reason != "matched_prior_completed_attempt" || decision.PriorAttemptID != "attempt-001" {
		t.Fatalf("unexpected decision: %+v", decision)
	}
}

func TestWorkReuseDecisionReportsMismatchedAttempt(t *testing.T) {
	controller := newControllerWithCompletedAttempt(t, model.WorkCompletion{
		ID:                   "test-001",
		AttemptID:            "attempt-001",
		WorkflowDefinitionID: "workflow-definition-001",
		WorkflowFingerprint:  "workflow-fingerprint",
		WorkflowInstanceID:   "workflow-instance-001",
		StepDefinitionID:     "step-definition-001",
		StepFingerprint:      "step-fingerprint",
		StepInstanceID:       "step-instance-001",
		WorkItemFingerprint:  "work-item-fingerprint",
		InputFingerprint:     "input-fingerprint",
		OutputFingerprint:    "output-fingerprint",
		CodeVersion:          "old-code-version",
		StartedAt:            "2026-06-06T12:00:00Z",
		CompletedAt:          "2026-06-06T12:01:00Z",
	})

	decision, err := controller.workReuseDecision(context.Background(), model.WorkItem{
		WorkItemFingerprint: "work-item-fingerprint",
		InputFingerprint:    "input-fingerprint",
		OutputFingerprint:   "output-fingerprint",
		CodeVersion:         "new-code-version",
	})
	if err != nil {
		t.Fatalf("workReuseDecision() error = %v", err)
	}

	if decision.Reusable || decision.Reason != "prior_attempt_mismatch" || decision.PriorAttemptID != "attempt-001" {
		t.Fatalf("unexpected decision: %+v", decision)
	}
}

func TestWorkReuseDecisionReportsMissingAttempt(t *testing.T) {
	controller := newController(nil)

	decision, err := controller.workReuseDecision(context.Background(), model.WorkItem{
		WorkItemFingerprint: "work-item-fingerprint",
		InputFingerprint:    "input-fingerprint",
		OutputFingerprint:   "output-fingerprint",
		CodeVersion:         "code-version",
	})
	if err != nil {
		t.Fatalf("workReuseDecision() error = %v", err)
	}

	if decision.Reusable || decision.Reason != "no_prior_completed_attempt" || decision.PriorAttemptID != "" {
		t.Fatalf("unexpected decision: %+v", decision)
	}
}

func TestWorkSkipForReuseDecisionBuildsSkip(t *testing.T) {
	skip, ok, err := workSkipForReuseDecision(model.WorkItem{ID: "work-item-001"}, WorkReuseDecision{
		Reusable:       true,
		Reason:         "matched_prior_completed_attempt",
		PriorAttemptID: "attempt-001",
	})
	if err != nil {
		t.Fatalf("workSkipForReuseDecision() error = %v", err)
	}
	if !ok {
		t.Fatal("expected skip marker")
	}
	if skip.ID != "work-item-001" || skip.PriorAttemptID != "attempt-001" || skip.Reason != "matched_prior_completed_attempt" {
		t.Fatalf("unexpected skip marker: %+v", skip)
	}
}

func TestWorkSkipForReuseDecisionReturnsMissingForNonReusableDecision(t *testing.T) {
	skip, ok, err := workSkipForReuseDecision(model.WorkItem{ID: "work-item-001"}, WorkReuseDecision{
		Reason: "prior_attempt_mismatch",
	})
	if err != nil {
		t.Fatalf("workSkipForReuseDecision() error = %v", err)
	}
	if ok {
		t.Fatalf("unexpected skip marker: %+v", skip)
	}
}

func TestWorkSkipForReuseDecisionRejectsInvalidSkip(t *testing.T) {
	if _, _, err := workSkipForReuseDecision(model.WorkItem{}, WorkReuseDecision{
		Reusable:       true,
		Reason:         "matched_prior_completed_attempt",
		PriorAttemptID: "attempt-001",
	}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestSkippedAttemptFromWorkSkip(t *testing.T) {
	skippedAt := mustParseTime(t, "2026-06-06T12:00:00Z")
	item := model.WorkItem{
		ID:                   "work-item-001",
		WorkflowDefinitionID: "workflow-definition-001",
		WorkflowFingerprint:  "workflow-fingerprint",
		WorkflowInstanceID:   "workflow-instance-001",
		StepDefinitionID:     "step-definition-001",
		StepFingerprint:      "step-fingerprint",
		StepInstanceID:       "step-instance-001",
		WorkItemFingerprint:  "work-item-fingerprint",
		InputFingerprint:     "input-fingerprint",
		OutputFingerprint:    "output-fingerprint",
		CodeVersion:          "code-version",
	}
	skip := model.WorkSkip{
		ID:             "work-item-001",
		PriorAttemptID: "attempt-001",
		Reason:         "matched_prior_completed_attempt",
	}

	attempt, err := skippedAttemptFromWorkSkip(item, skip, skippedAt)
	if err != nil {
		t.Fatalf("skippedAttemptFromWorkSkip() error = %v", err)
	}

	if !strings.HasPrefix(attempt.ID, "work-item-001-skip-") {
		t.Fatalf("unexpected attempt id: %q", attempt.ID)
	}
	if attempt.Status != ledger.AttemptStatusSkipped {
		t.Fatalf("status = %q, want skipped", attempt.Status)
	}
	if attempt.WorkItemFingerprint != item.WorkItemFingerprint {
		t.Fatalf("work item fingerprint = %q, want %q", attempt.WorkItemFingerprint, item.WorkItemFingerprint)
	}
	if !attempt.StartedAt.Equal(skippedAt) || !attempt.CompletedAt.Equal(skippedAt) {
		t.Fatalf("unexpected timestamps: started=%s completed=%s", attempt.StartedAt, attempt.CompletedAt)
	}

	variables := attemptVariablesByName(attempt.Variables)
	if variables["prior_attempt_id"].Value != "attempt-001" {
		t.Fatalf("prior_attempt_id = %+v", variables["prior_attempt_id"])
	}
	if variables["skip_reason"].Value != "matched_prior_completed_attempt" {
		t.Fatalf("skip_reason = %+v", variables["skip_reason"])
	}
}

func TestSkippedAttemptFromWorkSkipRejectsInvalidSkip(t *testing.T) {
	if _, err := skippedAttemptFromWorkSkip(model.WorkItem{}, model.WorkSkip{}, time.Time{}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestRecordSkippedAttemptStoresSkippedAttempt(t *testing.T) {
	controller := newControllerWithCompletedAttempt(t, model.WorkCompletion{
		ID:                   "test-001",
		AttemptID:            "attempt-001",
		WorkflowDefinitionID: "workflow-definition-001",
		WorkflowFingerprint:  "workflow-fingerprint",
		WorkflowInstanceID:   "workflow-instance-001",
		StepDefinitionID:     "step-definition-001",
		StepFingerprint:      "step-fingerprint",
		StepInstanceID:       "step-instance-001",
		WorkItemFingerprint:  "work-item-fingerprint",
		InputFingerprint:     "input-fingerprint",
		OutputFingerprint:    "output-fingerprint",
		CodeVersion:          "code-version",
		StartedAt:            "2026-06-06T12:00:00Z",
		CompletedAt:          "2026-06-06T12:01:00Z",
	})
	item := model.WorkItem{
		ID:                  "test-001",
		WorkflowInstanceID:  "workflow-instance-002",
		StepInstanceID:      "step-instance-002",
		WorkItemFingerprint: "work-item-fingerprint",
		InputFingerprint:    "input-fingerprint",
		OutputFingerprint:   "output-fingerprint",
		CodeVersion:         "code-version",
	}

	skip, ok, err := controller.recordSkippedAttempt(context.Background(), item, mustParseTime(t, "2026-06-06T12:02:00Z"))
	if err != nil {
		t.Fatalf("recordSkippedAttempt() error = %v", err)
	}
	if !ok {
		t.Fatal("expected skipped attempt")
	}
	if skip.PriorAttemptID != "attempt-001" {
		t.Fatalf("prior attempt id = %q, want attempt-001", skip.PriorAttemptID)
	}

	var status string
	if err := controller.ledger.QueryRowContext(context.Background(), `SELECT status FROM attempts WHERE status = ?`, string(ledger.AttemptStatusSkipped)).Scan(&status); err != nil {
		t.Fatalf("query skipped attempt: %v", err)
	}
	if status != string(ledger.AttemptStatusSkipped) {
		t.Fatalf("status = %q, want skipped", status)
	}
}

func TestRecordSkippedAttemptReturnsMissingForMismatchedAttempt(t *testing.T) {
	controller := newControllerWithCompletedAttempt(t, model.WorkCompletion{
		ID:                   "test-001",
		AttemptID:            "attempt-001",
		WorkflowDefinitionID: "workflow-definition-001",
		WorkflowFingerprint:  "workflow-fingerprint",
		WorkflowInstanceID:   "workflow-instance-001",
		StepDefinitionID:     "step-definition-001",
		StepFingerprint:      "step-fingerprint",
		StepInstanceID:       "step-instance-001",
		WorkItemFingerprint:  "work-item-fingerprint",
		InputFingerprint:     "input-fingerprint",
		OutputFingerprint:    "output-fingerprint",
		CodeVersion:          "old-code-version",
		StartedAt:            "2026-06-06T12:00:00Z",
		CompletedAt:          "2026-06-06T12:01:00Z",
	})

	skip, ok, err := controller.recordSkippedAttempt(context.Background(), model.WorkItem{
		ID:                  "test-001",
		WorkItemFingerprint: "work-item-fingerprint",
		InputFingerprint:    "input-fingerprint",
		OutputFingerprint:   "output-fingerprint",
		CodeVersion:         "new-code-version",
	}, mustParseTime(t, "2026-06-06T12:02:00Z"))
	if err != nil {
		t.Fatalf("recordSkippedAttempt() error = %v", err)
	}
	if ok {
		t.Fatalf("unexpected skip marker: %+v", skip)
	}
}

func TestRecordSkippedAttemptReturnsMissingWithoutLedger(t *testing.T) {
	controller := newController(nil)

	skip, ok, err := controller.recordSkippedAttempt(context.Background(), model.WorkItem{
		ID:                  "test-001",
		WorkItemFingerprint: "work-item-fingerprint",
		InputFingerprint:    "input-fingerprint",
		OutputFingerprint:   "output-fingerprint",
		CodeVersion:         "code-version",
	}, mustParseTime(t, "2026-06-06T12:02:00Z"))
	if err != nil {
		t.Fatalf("recordSkippedAttempt() error = %v", err)
	}
	if ok {
		t.Fatalf("unexpected skip marker: %+v", skip)
	}
}

func TestCompleteWorkHandlerRejectsInvalidAttemptMetadata(t *testing.T) {
	controller := newTestController()
	assignNextWork(t, controller)

	request := httptest.NewRequest(http.MethodPost, "/work/complete", bytes.NewBufferString(`{
		"id":"test-001",
		"attempt_id":"attempt-001",
		"started_at":"not-a-time",
		"completed_at":"2026-06-06T12:01:00Z"
	}`))
	response := httptest.NewRecorder()

	controller.completeWorkHandler(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestCompleteWorkHandlerRejectsUnassignedItem(t *testing.T) {
	controller := newTestController()
	request := httptest.NewRequest(http.MethodPost, "/work/complete", bytes.NewBufferString(`{"id":"test-001"}`))
	response := httptest.NewRecorder()

	controller.completeWorkHandler(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestCompleteWorkHandlerRejectsGet(t *testing.T) {
	controller := newTestController()
	request := httptest.NewRequest(http.MethodGet, "/work/complete", nil)
	response := httptest.NewRecorder()

	controller.completeWorkHandler(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestCompleteWorkHandlerRejectsMissingID(t *testing.T) {
	controller := newTestController()
	request := httptest.NewRequest(http.MethodPost, "/work/complete", bytes.NewBufferString(`{}`))
	response := httptest.NewRecorder()

	controller.completeWorkHandler(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestFailWorkHandler(t *testing.T) {
	controller := newTestController()
	assignNextWork(t, controller)

	request := httptest.NewRequest(http.MethodPost, "/work/fail", bytes.NewBufferString(`{"id":"test-001","error":"failed"}`))
	response := httptest.NewRecorder()

	controller.failWorkHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", response.Code)
	}

	if controller.failed["test-001"].Error != "failed" {
		t.Fatalf("unexpected failure: %+v", controller.failed["test-001"])
	}
}

func TestFailWorkHandlerRejectsUnassignedItem(t *testing.T) {
	controller := newTestController()
	request := httptest.NewRequest(http.MethodPost, "/work/fail", bytes.NewBufferString(`{"id":"test-001","error":"failed"}`))
	response := httptest.NewRecorder()

	controller.failWorkHandler(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestFailWorkHandlerRejectsMissingError(t *testing.T) {
	controller := newTestController()
	request := httptest.NewRequest(http.MethodPost, "/work/fail", bytes.NewBufferString(`{"id":"test-001"}`))
	response := httptest.NewRecorder()

	controller.failWorkHandler(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestStatusHandler(t *testing.T) {
	controller := newTestController()

	status := getStatus(t, controller)

	if status.Pending != 1 || status.Assigned != 0 || status.Failed != 0 {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestStatusHandlerReportsAssignedWork(t *testing.T) {
	controller := newTestController()
	assignNextWork(t, controller)

	status := getStatus(t, controller)

	if status.Pending != 0 || status.Assigned != 1 || status.Failed != 0 {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestStatusHandlerReportsFailedWork(t *testing.T) {
	controller := newTestController()
	assignNextWork(t, controller)

	request := httptest.NewRequest(http.MethodPost, "/work/fail", bytes.NewBufferString(`{"id":"test-001","error":"failed"}`))
	response := httptest.NewRecorder()
	controller.failWorkHandler(response, request)

	status := getStatus(t, controller)

	if status.Pending != 0 || status.Assigned != 0 || status.Failed != 1 {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestStatusHandlerUsesWorkflowExecutionStoreWhenConfigured(t *testing.T) {
	ctx := context.Background()
	store := openTestWorkflowExecutionStore(t)
	defer store.Close()
	run := insertTestPersistenceRunWithStage(t, ctx, store)
	queued := testPersistenceWorkItem("persisted-queued", run.ID, 0, 0)
	running := testPersistenceWorkItem("persisted-running", run.ID, 0, 1)
	failed := testPersistenceWorkItem("persisted-failed", run.ID, 0, 2)
	if err := store.InsertWorkItems(ctx, []persistence.WorkItemRecord{queued, running, failed}); err != nil {
		t.Fatalf("InsertWorkItems() error = %v", err)
	}
	if err := store.EnqueueWorkItems(ctx, []persistence.QueuedWorkRecord{
		{WorkItemRecord: queued, QueuedAt: "2026-07-03T00:00:00Z"},
		{WorkItemRecord: running, QueuedAt: "2026-07-03T00:00:01Z"},
		{WorkItemRecord: failed, QueuedAt: "2026-07-03T00:00:02Z"},
	}); err != nil {
		t.Fatalf("EnqueueWorkItems() error = %v", err)
	}
	if _, found, err := store.ClaimNextWork(ctx, persistence.ClaimWorkRequest{
		AttemptID:    "attempt-running",
		ExecutorType: persistence.ExecutorTypeWorker,
		StartedAt:    "2026-07-03T00:00:03Z",
	}); err != nil || !found {
		t.Fatalf("ClaimNextWork(running) found = %v error = %v, want success", found, err)
	}
	if _, found, err := store.ClaimNextWork(ctx, persistence.ClaimWorkRequest{
		AttemptID:    "attempt-failed",
		ExecutorType: persistence.ExecutorTypeWorker,
		StartedAt:    "2026-07-03T00:00:04Z",
	}); err != nil || !found {
		t.Fatalf("ClaimNextWork(failed) found = %v error = %v, want success", found, err)
	}
	if _, found, err := store.FailAttempt(ctx, persistence.FailAttemptRequest{
		AttemptID: "attempt-failed",
		Error:     "failed",
		FailedAt:  "2026-07-03T00:00:05Z",
	}); err != nil || !found {
		t.Fatalf("FailAttempt() found = %v error = %v, want success", found, err)
	}

	controller := newController([]model.WorkItem{testWorkItem("memory-pending")})
	controller.assigned["memory-assigned"] = testWorkItem("memory-assigned")
	controller.failed["memory-failed"] = model.WorkFailure{ID: "memory-failed", Error: "memory failed"}
	controller.workflowStore = store

	status := getStatus(t, controller)

	if status.Pending != 1 || status.Assigned != 1 || status.Failed != 1 {
		t.Fatalf("unexpected persisted status: %+v", status)
	}
}

func TestStatusHandlerReportsLedgerCounts(t *testing.T) {
	controller := newTestController()
	db := testSQLiteMainDatabase(t)
	defer db.Close()
	controller.ledger = db
	assignNextWork(t, controller)

	request := httptest.NewRequest(http.MethodPost, "/work/complete", bytes.NewBufferString(`{
		"id":"test-001",
		"attempt_id":"attempt-001",
		"workflow_definition_id":"workflow-definition-001",
		"workflow_fingerprint":"workflow-fingerprint",
		"workflow_instance_id":"workflow-instance-001",
		"step_definition_id":"step-definition-001",
		"step_fingerprint":"step-fingerprint",
		"step_instance_id":"step-instance-001",
		"work_item_fingerprint":"work-item-fingerprint",
		"input_fingerprint":"input-fingerprint",
		"output_fingerprint":"output-fingerprint",
		"code_version":"code-version",
		"started_at":"2026-06-06T12:00:00Z",
		"completed_at":"2026-06-06T12:01:00Z"
	}`))
	response := httptest.NewRecorder()

	controller.completeWorkHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected completion status code: %d", response.Code)
	}

	status := getStatus(t, controller)

	if status.Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", status.Attempts)
	}

	if status.AttemptVariables != 14 {
		t.Fatalf("attempt_variables = %d, want 14", status.AttemptVariables)
	}
}

func TestStatusHandlerReportsPendingReuseCandidates(t *testing.T) {
	controller := newControllerWithCompletedAttempt(t, model.WorkCompletion{
		ID:                   "test-001",
		AttemptID:            "attempt-001",
		WorkflowDefinitionID: "workflow-definition-001",
		WorkflowFingerprint:  "workflow-fingerprint",
		WorkflowInstanceID:   "workflow-instance-001",
		StepDefinitionID:     "step-definition-001",
		StepFingerprint:      "step-fingerprint",
		StepInstanceID:       "step-instance-001",
		WorkItemFingerprint:  "work-item-fingerprint",
		InputFingerprint:     "input-fingerprint",
		OutputFingerprint:    "output-fingerprint",
		CodeVersion:          "code-version",
		StartedAt:            "2026-06-06T12:00:00Z",
		CompletedAt:          "2026-06-06T12:01:00Z",
	})
	controller.pending = append(controller.pending, model.WorkItem{
		ID:                  "test-001",
		Type:                model.WorkItemTypeWriteDemoOutput,
		OutputFilename:      "result.txt",
		WorkItemFingerprint: "work-item-fingerprint",
		InputFingerprint:    "input-fingerprint",
		OutputFingerprint:   "output-fingerprint",
		CodeVersion:         "code-version",
	})

	status := getStatus(t, controller)

	if status.PendingReuseCandidates != 1 {
		t.Fatalf("pending_reuse_candidates = %d, want 1", status.PendingReuseCandidates)
	}
}

func TestPendingReuseDecisionReasonsCountsReasons(t *testing.T) {
	controller := newControllerWithCompletedAttempt(t, model.WorkCompletion{
		ID:                   "test-001",
		AttemptID:            "attempt-001",
		WorkflowDefinitionID: "workflow-definition-001",
		WorkflowFingerprint:  "workflow-fingerprint",
		WorkflowInstanceID:   "workflow-instance-001",
		StepDefinitionID:     "step-definition-001",
		StepFingerprint:      "step-fingerprint",
		StepInstanceID:       "step-instance-001",
		WorkItemFingerprint:  "work-item-fingerprint",
		InputFingerprint:     "input-fingerprint",
		OutputFingerprint:    "output-fingerprint",
		CodeVersion:          "code-version",
		StartedAt:            "2026-06-06T12:00:00Z",
		CompletedAt:          "2026-06-06T12:01:00Z",
	})
	items := []model.WorkItem{
		{
			WorkItemFingerprint: "work-item-fingerprint",
			InputFingerprint:    "input-fingerprint",
			OutputFingerprint:   "output-fingerprint",
			CodeVersion:         "code-version",
		},
		{
			WorkItemFingerprint: "work-item-fingerprint",
			InputFingerprint:    "input-fingerprint",
			OutputFingerprint:   "output-fingerprint",
			CodeVersion:         "new-code-version",
		},
		{
			WorkItemFingerprint: "missing-fingerprint",
			InputFingerprint:    "input-fingerprint",
			OutputFingerprint:   "output-fingerprint",
			CodeVersion:         "code-version",
		},
	}

	reasons, err := controller.pendingReuseDecisionReasons(context.Background(), items)
	if err != nil {
		t.Fatalf("pendingReuseDecisionReasons() error = %v", err)
	}

	if reasons["matched_prior_completed_attempt"] != 1 {
		t.Fatalf("matched count = %d, want 1", reasons["matched_prior_completed_attempt"])
	}
	if reasons["prior_attempt_mismatch"] != 1 {
		t.Fatalf("mismatch count = %d, want 1", reasons["prior_attempt_mismatch"])
	}
	if reasons["no_prior_completed_attempt"] != 1 {
		t.Fatalf("missing count = %d, want 1", reasons["no_prior_completed_attempt"])
	}
}

func TestStatusHandlerRejectsPost(t *testing.T) {
	controller := newTestController()
	request := httptest.NewRequest(http.MethodPost, "/status", nil)
	response := httptest.NewRecorder()

	controller.statusHandler(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestSubmitWorkHandler(t *testing.T) {
	controller := newController(nil)
	request := httptest.NewRequest(http.MethodPost, "/work", bytes.NewBufferString(`{
		"id":"test-001",
		"type":"write_demo_output",
		"output_filename":"result.txt"
	}`))
	response := httptest.NewRecorder()

	controller.submitWorkHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", response.Code)
	}

	status := getStatus(t, controller)
	if status.Pending != 1 {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestSubmitWorkHandlerPersistsRawWorkWhenWorkflowStoreConfigured(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	defer store.Close()
	controller := newController(nil)
	controller.workflowStore = store
	request := httptest.NewRequest(http.MethodPost, "/work", bytes.NewBufferString(`{
		"id":"test-001",
		"type":"write_demo_output",
		"output_filename":"result.txt",
		"parameters":{"year":{"type":"int","value":2026}}
	}`))
	response := httptest.NewRecorder()

	controller.submitWorkHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
	if len(controller.pending) != 0 {
		t.Fatalf("in-memory pending count = %d, want 0 when workflow store is configured", len(controller.pending))
	}
	status := getStatus(t, controller)
	if status.Pending != 1 {
		t.Fatalf("unexpected status: %+v", status)
	}
	queued, err := store.ListQueuedWorkItems(context.Background())
	if err != nil {
		t.Fatalf("ListQueuedWorkItems() error = %v", err)
	}
	if len(queued) != 1 {
		t.Fatalf("queued count = %d, want 1: %+v", len(queued), queued)
	}
	if queued[0].RunID != rawPersistenceRunID || queued[0].StageIndex != rawPersistenceStageIndex {
		t.Fatalf("queued raw location = run %q stage %d, want raw run/stage", queued[0].RunID, queued[0].StageIndex)
	}
	var persisted model.WorkItem
	if err := json.Unmarshal([]byte(queued[0].WorkerPayloadJSON), &persisted); err != nil {
		t.Fatalf("decode worker payload json: %v", err)
	}
	if persisted.ID != "test-001" || persisted.OutputFilename != "result.txt" {
		t.Fatalf("persisted payload = %+v, want submitted work item", persisted)
	}
	if persisted.Parameters["year"].Value == nil {
		t.Fatalf("persisted parameters = %+v, want year parameter", persisted.Parameters)
	}
}

func TestSubmitWorkHandlerRejectsDuplicatePersistedRawWork(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	defer store.Close()
	controller := newController(nil)
	controller.workflowStore = store
	body := `{
		"id":"test-001",
		"type":"write_demo_output",
		"output_filename":"result.txt"
	}`
	first := httptest.NewRecorder()
	controller.submitWorkHandler(first, httptest.NewRequest(http.MethodPost, "/work", bytes.NewBufferString(body)))
	if first.Code != http.StatusNoContent {
		t.Fatalf("first status code = %d, want 204", first.Code)
	}

	second := httptest.NewRecorder()
	controller.submitWorkHandler(second, httptest.NewRequest(http.MethodPost, "/work", bytes.NewBufferString(body)))

	if second.Code != http.StatusConflict {
		t.Fatalf("second status code = %d, want 409", second.Code)
	}
}

func TestSubmitWorkHandlerRejectsInvalidItem(t *testing.T) {
	controller := newController(nil)
	request := httptest.NewRequest(http.MethodPost, "/work", bytes.NewBufferString(`{"id":"test-001"}`))
	response := httptest.NewRecorder()

	controller.submitWorkHandler(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestSubmitWorkHandlerRejectsDuplicateID(t *testing.T) {
	controller := newTestController()
	request := httptest.NewRequest(http.MethodPost, "/work", bytes.NewBufferString(`{
		"id":"test-001",
		"type":"write_demo_output",
		"output_filename":"duplicate.txt"
	}`))
	response := httptest.NewRecorder()

	controller.submitWorkHandler(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestSubmitWorkHandlerRejectsGet(t *testing.T) {
	controller := newController(nil)
	request := httptest.NewRequest(http.MethodGet, "/work", nil)
	response := httptest.NewRecorder()

	controller.submitWorkHandler(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestNextWorkHandlerClaimsPersistedWorkWhenWorkflowStoreConfigured(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	defer store.Close()
	controller := newController(nil)
	controller.workflowStore = store
	submitReq := httptest.NewRequest(http.MethodPost, "/work", bytes.NewBufferString(`{
		"id":"test-001",
		"type":"write_demo_output",
		"output_filename":"result.txt"
	}`))
	submitResp := httptest.NewRecorder()
	controller.submitWorkHandler(submitResp, submitReq)
	if submitResp.Code != http.StatusNoContent {
		t.Fatalf("submit status code = %d, want 204", submitResp.Code)
	}

	nextReq := httptest.NewRequest(http.MethodGet, "/work/next", nil)
	nextResp := httptest.NewRecorder()
	controller.nextWorkHandler(nextResp, nextReq)

	if nextResp.Code != http.StatusOK {
		t.Fatalf("next status code = %d, want 200", nextResp.Code)
	}
	var item model.WorkItem
	if err := json.NewDecoder(nextResp.Body).Decode(&item); err != nil {
		t.Fatalf("decode assigned work: %v", err)
	}
	if item.ID != "test-001" || item.OutputFilename != "result.txt" {
		t.Fatalf("assigned item = %+v, want submitted item", item)
	}
	if item.AttemptID == "" {
		t.Fatal("assigned item attempt_id is required for persisted claim")
	}
	if len(controller.assigned) != 0 {
		t.Fatalf("assigned map count = %d, want 0 for persisted claim", len(controller.assigned))
	}
	queued, err := store.ListQueuedWorkItems(context.Background())
	if err != nil {
		t.Fatalf("ListQueuedWorkItems() error = %v", err)
	}
	if len(queued) != 0 {
		t.Fatalf("queued count = %d, want 0 after persisted claim", len(queued))
	}
	running, err := store.ListRunningWork(context.Background())
	if err != nil {
		t.Fatalf("ListRunningWork() error = %v", err)
	}
	if len(running) != 1 {
		t.Fatalf("running count = %d, want 1: %+v", len(running), running)
	}
	if running[0].WorkItem.ID != "test-001" {
		t.Fatalf("running work item id = %q, want test-001", running[0].WorkItem.ID)
	}
	if running[0].AttemptID != item.AttemptID {
		t.Fatalf("running attempt id = %q, want assigned attempt %q", running[0].AttemptID, item.AttemptID)
	}
}

func TestCompleteWorkHandlerCompletesPersistedAttemptWhenWorkflowStoreConfigured(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	defer store.Close()
	controller := newController(nil)
	controller.workflowStore = store
	item := submitAndClaimPersistedWork(t, controller)
	controller.assigned[item.ID] = item

	request := httptest.NewRequest(http.MethodPost, "/work/complete", bytes.NewBufferString(`{
		"id":"test-001",
		"attempt_id":"`+item.AttemptID+`",
		"output_json":"{\"work_item_id\":\"test-001\",\"status\":\"ok\"}",
		"pre_state_json":"{\"output_exists\":false}",
		"post_state_json":"{\"bytes_written\":19,\"output_exists\":true}",
		"completed_at":"2026-07-03T12:00:00Z"
	}`))
	response := httptest.NewRecorder()

	controller.completeWorkHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("complete status code = %d, want 204: %s", response.Code, response.Body.String())
	}
	if _, ok := controller.assigned[item.ID]; !ok {
		t.Fatal("persisted completion should not mutate in-memory assigned map")
	}
	running, err := store.ListRunningWork(context.Background())
	if err != nil {
		t.Fatalf("ListRunningWork() error = %v", err)
	}
	if len(running) != 0 {
		t.Fatalf("running count = %d, want 0: %+v", len(running), running)
	}
	terminal, err := store.ListTerminalAttemptsForRun(context.Background(), rawPersistenceRunID)
	if err != nil {
		t.Fatalf("ListTerminalAttemptsForRun() error = %v", err)
	}
	if len(terminal) != 1 {
		t.Fatalf("terminal count = %d, want 1: %+v", len(terminal), terminal)
	}
	if terminal[0].TerminalState != "completed" || terminal[0].AttemptID != item.AttemptID {
		t.Fatalf("terminal attempt = %+v, want completed assigned attempt", terminal[0])
	}
	if terminal[0].OutputJSON != `{"status":"ok","work_item_id":"test-001"}` {
		t.Fatalf("canonical output json = %q", terminal[0].OutputJSON)
	}
	if terminal[0].OutputJSONSHA256 == "" || terminal[0].PreStateSHA256 == "" || terminal[0].PostStateSHA256 == "" {
		t.Fatalf("missing completion hashes: %+v", terminal[0])
	}
}

func TestCompleteAttemptRequestFromCompletionMapsWorkerObservedSkipEvidence(t *testing.T) {
	preStateSHA256 := strings.Repeat("a", 64)
	postStateSHA256 := strings.Repeat("b", 64)
	request, err := completeAttemptRequestFromCompletion(model.WorkCompletion{
		ID:              "test-001",
		AttemptID:       "attempt-001",
		Skipped:         true,
		SkippedParentID: "attempt-prior",
		SkipReason:      "matched_worker_observed_state",
		InputSHA256:     strings.Repeat("c", 64),
		OutputSHA256:    strings.Repeat("d", 64),
		PreStateSHA256:  preStateSHA256,
		PostStateSHA256: postStateSHA256,
		OutputJSON:      `{"input_sha256":"` + strings.Repeat("c", 64) + `","output_sha256":"` + strings.Repeat("d", 64) + `"}`,
		PreStateJSON:    `{"output_exists":true}`,
		PostStateJSON:   `{"output_exists":true}`,
		CompletedAt:     "2026-07-03T12:00:00Z",
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("build complete request: %v", err)
	}

	if request.SkippedParentID != "attempt-prior" {
		t.Fatalf("skipped parent = %q, want attempt-prior", request.SkippedParentID)
	}
	if request.PreStateSHA256 != preStateSHA256 || request.PostStateSHA256 != postStateSHA256 {
		t.Fatalf("state hashes = %q/%q, want reported worker hashes", request.PreStateSHA256, request.PostStateSHA256)
	}
}

func TestCompleteWorkHandlerRejectsPersistedCompletionMissingAttemptID(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	defer store.Close()
	controller := newController(nil)
	controller.workflowStore = store
	submitAndClaimPersistedWork(t, controller)

	request := httptest.NewRequest(http.MethodPost, "/work/complete", bytes.NewBufferString(`{
		"id":"test-001",
		"output_json":"{}",
		"pre_state_json":"{}",
		"post_state_json":"{}"
	}`))
	response := httptest.NewRecorder()

	controller.completeWorkHandler(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want 400", response.Code)
	}
}

func TestCompleteWorkHandlerReturnsNotFoundForMissingPersistedAttempt(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	defer store.Close()
	controller := newController(nil)
	controller.workflowStore = store

	request := httptest.NewRequest(http.MethodPost, "/work/complete", bytes.NewBufferString(`{
		"id":"test-001",
		"attempt_id":"missing-attempt",
		"output_json":"{}",
		"pre_state_json":"{}",
		"post_state_json":"{}"
	}`))
	response := httptest.NewRecorder()

	controller.completeWorkHandler(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status code = %d, want 404", response.Code)
	}
}

func TestFailWorkHandlerFailsPersistedAttemptWhenWorkflowStoreConfigured(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	defer store.Close()
	controller := newController(nil)
	controller.workflowStore = store
	item := submitAndClaimPersistedWork(t, controller)
	controller.assigned[item.ID] = item

	body := `{
		"id":"test-001",
		"attempt_id":"` + item.AttemptID + `",
		"failed_at":"2026-07-03T12:01:00Z",
		"error":"boom"
	}`
	request := httptest.NewRequest(http.MethodPost, "/work/fail", bytes.NewBufferString(body))
	response := httptest.NewRecorder()

	controller.failWorkHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("fail status code = %d, want 204: %s", response.Code, response.Body.String())
	}
	if _, ok := controller.assigned[item.ID]; !ok {
		t.Fatal("persisted failure should not mutate in-memory assigned map")
	}
	running, err := store.ListRunningWork(context.Background())
	if err != nil {
		t.Fatalf("ListRunningWork() error = %v", err)
	}
	if len(running) != 0 {
		t.Fatalf("running count = %d, want 0: %+v", len(running), running)
	}
	terminal, err := store.ListTerminalAttemptsForRun(context.Background(), rawPersistenceRunID)
	if err != nil {
		t.Fatalf("ListTerminalAttemptsForRun() error = %v", err)
	}
	if len(terminal) != 1 {
		t.Fatalf("terminal count = %d, want 1: %+v", len(terminal), terminal)
	}
	if terminal[0].TerminalState != "failed" || terminal[0].AttemptID != item.AttemptID || terminal[0].Error != "boom" {
		t.Fatalf("terminal attempt = %+v, want failed assigned attempt", terminal[0])
	}

	duplicate := httptest.NewRecorder()
	controller.failWorkHandler(duplicate, httptest.NewRequest(http.MethodPost, "/work/fail", bytes.NewBufferString(body)))
	if duplicate.Code != http.StatusNoContent {
		t.Fatalf("duplicate fail status code = %d, want 204: %s", duplicate.Code, duplicate.Body.String())
	}
}

func TestFailWorkHandlerRejectsPersistedFailureMissingAttemptID(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	defer store.Close()
	controller := newController(nil)
	controller.workflowStore = store

	request := httptest.NewRequest(http.MethodPost, "/work/fail", bytes.NewBufferString(`{"id":"test-001","error":"boom"}`))
	response := httptest.NewRecorder()

	controller.failWorkHandler(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want 400", response.Code)
	}
}

func submitAndClaimPersistedWork(t *testing.T, controller *Controller) model.WorkItem {
	t.Helper()

	submitReq := httptest.NewRequest(http.MethodPost, "/work", bytes.NewBufferString(`{
		"id":"test-001",
		"type":"write_demo_output",
		"output_filename":"result.txt"
	}`))
	submitResp := httptest.NewRecorder()
	controller.submitWorkHandler(submitResp, submitReq)
	if submitResp.Code != http.StatusNoContent {
		t.Fatalf("submit status code = %d, want 204: %s", submitResp.Code, submitResp.Body.String())
	}

	nextReq := httptest.NewRequest(http.MethodGet, "/work/next", nil)
	nextResp := httptest.NewRecorder()
	controller.nextWorkHandler(nextResp, nextReq)
	if nextResp.Code != http.StatusOK {
		t.Fatalf("next status code = %d, want 200: %s", nextResp.Code, nextResp.Body.String())
	}

	var item model.WorkItem
	if err := json.NewDecoder(nextResp.Body).Decode(&item); err != nil {
		t.Fatalf("decode assigned work: %v", err)
	}
	if item.AttemptID == "" {
		t.Fatal("assigned item attempt_id is required")
	}
	return item
}

func TestNextWorkHandlerReturnsNoContentForEmptyPersistedQueue(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	defer store.Close()
	controller := newController([]model.WorkItem{testWorkItem("memory-pending")})
	controller.workflowStore = store
	request := httptest.NewRequest(http.MethodGet, "/work/next", nil)
	response := httptest.NewRecorder()

	controller.nextWorkHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status code = %d, want 204", response.Code)
	}
	if len(controller.pending) != 1 {
		t.Fatalf("in-memory pending count = %d, want unchanged fallback state", len(controller.pending))
	}
}

func TestSubmitWorkflowHandler(t *testing.T) {
	controller := newController(nil)
	request := httptest.NewRequest(http.MethodPost, "/workflow", bytes.NewBufferString(`{
		"workflow": {
			"ID": "cdl",
			"Variables": [
				{
					"name": {"namespace": "workflow", "key": "years"},
					"type": "list",
					"expression": [{"type": "int", "expression": 2024}, {"type": "int", "expression": 2025}]
				}
			],
			"Steps": [
				{
					"ID": "download",
					"FanOut": {
						"WorkItem": {
							"FanOutExpression": "${years[*]}",
							"Type": "write_demo_output",
							"OutputPrefix": "cdl",
							"OutputExtension": ".txt"
						}
					}
				}
			]
		}
	}`))
	response := httptest.NewRecorder()

	controller.submitWorkflowHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", response.Code)
	}

	status := getStatus(t, controller)
	if status.Pending != 2 {
		t.Fatalf("unexpected status: %+v", status)
	}

	nextRequest := httptest.NewRequest(http.MethodGet, "/work/next", nil)
	nextResponse := httptest.NewRecorder()
	controller.nextWorkHandler(nextResponse, nextRequest)

	if nextResponse.Code != http.StatusOK {
		t.Fatalf("unexpected next work status code: %d", nextResponse.Code)
	}

	var item model.WorkItem
	if err := json.NewDecoder(nextResponse.Body).Decode(&item); err != nil {
		t.Fatalf("decode next work item: %v", err)
	}

	if !strings.HasPrefix(item.WorkflowInstanceID, "cdl-instance-") {
		t.Fatalf("unexpected workflow instance id: %q", item.WorkflowInstanceID)
	}

	if item.WorkflowDefinitionID != "cdl" {
		t.Fatalf("unexpected workflow definition id: %q", item.WorkflowDefinitionID)
	}

	if !strings.HasPrefix(item.WorkflowFingerprint, "workflow:sha256:") {
		t.Fatalf("unexpected workflow fingerprint: %q", item.WorkflowFingerprint)
	}

	if item.StepDefinitionID != "download" {
		t.Fatalf("unexpected step definition id: %q", item.StepDefinitionID)
	}

	if !strings.HasPrefix(item.StepFingerprint, "step:sha256:") {
		t.Fatalf("unexpected step fingerprint: %q", item.StepFingerprint)
	}

	if item.StepInstanceID != item.WorkflowInstanceID+"-step-download" {
		t.Fatalf("unexpected step instance id: %q", item.StepInstanceID)
	}

	if !strings.HasPrefix(item.WorkItemFingerprint, "work-item:sha256:") {
		t.Fatalf("unexpected work item fingerprint: %q", item.WorkItemFingerprint)
	}

	if !strings.HasPrefix(item.InputFingerprint, "input:sha256:") {
		t.Fatalf("unexpected input fingerprint: %q", item.InputFingerprint)
	}

	if !strings.HasPrefix(item.OutputFingerprint, "output:sha256:") {
		t.Fatalf("unexpected output fingerprint: %q", item.OutputFingerprint)
	}

	if item.CodeVersion == "" || item.CodeVersion == "demo" {
		t.Fatalf("unexpected code version: %q", item.CodeVersion)
	}
}

func TestSubmitWorkflowHandlerRejectsInlinePayloadWhenWorkflowStoreConfigured(t *testing.T) {
	store := openTestWorkflowExecutionStore(t)
	defer store.Close()
	controller := newController([]model.WorkItem{testWorkItem("memory-pending")})
	controller.workflowStore = store
	controller.assigned["memory-assigned"] = testWorkItem("memory-assigned")
	controller.failed["memory-failed"] = model.WorkFailure{ID: "memory-failed", Error: "failed"}

	request := httptest.NewRequest(http.MethodPost, "/workflow", bytes.NewBufferString(`{
		"workflow": {
			"ID": "cdl",
			"Steps": []
		}
	}`))
	response := httptest.NewRecorder()

	controller.submitWorkflowHandler(response, request)

	if response.Code != http.StatusNotImplemented {
		t.Fatalf("status code = %d, want 501", response.Code)
	}
	if len(controller.pending) != 1 {
		t.Fatalf("pending count = %d, want unchanged 1", len(controller.pending))
	}
	if len(controller.assigned) != 1 {
		t.Fatalf("assigned count = %d, want unchanged 1", len(controller.assigned))
	}
	if len(controller.failed) != 1 {
		t.Fatalf("failed count = %d, want unchanged 1", len(controller.failed))
	}
}

func TestSubmitWorkflowHandlerUsesConfiguredCodeVersion(t *testing.T) {
	controller := newController(nil)
	request := httptest.NewRequest(http.MethodPost, "/workflow", bytes.NewBufferString(`{
		"workflow": {
			"ID": "cdl",
			"Variables": [
				{
					"name": {"namespace": "workflow", "key": "years"},
					"type": "list",
					"expression": [{"type": "int", "expression": 2024}]
				}
			],
			"Steps": [
				{
					"ID": "download",
					"FanOut": {
						"WorkItem": {
							"FanOutExpression": "${years[*]}",
							"Type": "write_demo_output",
							"OutputPrefix": "cdl",
							"OutputExtension": ".txt"
						}
					}
				}
			]
		},
		"variables": [
			{
				"name": {"namespace": "override", "key": "code_version"},
				"type": "string",
				"expression": "test-version"
			}
		]
	}`))
	response := httptest.NewRecorder()

	controller.submitWorkflowHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", response.Code)
	}

	nextRequest := httptest.NewRequest(http.MethodGet, "/work/next", nil)
	nextResponse := httptest.NewRecorder()
	controller.nextWorkHandler(nextResponse, nextRequest)

	if nextResponse.Code != http.StatusOK {
		t.Fatalf("unexpected next work status code: %d", nextResponse.Code)
	}

	var item model.WorkItem
	if err := json.NewDecoder(nextResponse.Body).Decode(&item); err != nil {
		t.Fatalf("decode next work item: %v", err)
	}

	if item.CodeVersion != "test-version" {
		t.Fatalf("code version = %q, want test-version", item.CodeVersion)
	}
}

func TestWorkItemsWithRuntimeMetadataFingerprintsParameters(t *testing.T) {
	items := workItemsWithRuntimeMetadata("summary", []workflow.CompiledWorkItem{
		{
			WorkflowID: "summary",
			StepID:     "summarize",
			WorkItem: model.WorkItem{
				ID:             "summary-a",
				Type:           model.WorkItemTypeSummarizeInputFile,
				OutputFilename: "summary-a.txt",
				Parameters: model.Parameters{
					"input_path": {Type: "path", Value: "a.txt"},
				},
			},
		},
		{
			WorkflowID: "summary",
			StepID:     "summarize",
			WorkItem: model.WorkItem{
				ID:             "summary-b",
				Type:           model.WorkItemTypeSummarizeInputFile,
				OutputFilename: "summary-b.txt",
				Parameters: model.Parameters{
					"input_path": {Type: "path", Value: "b.txt"},
				},
			},
		},
	}, "test-version")

	if items[0].InputFingerprint == items[1].InputFingerprint {
		t.Fatalf("input fingerprints should differ: %s", items[0].InputFingerprint)
	}

	if items[0].OutputFingerprint == items[1].OutputFingerprint {
		t.Fatalf("output fingerprints should differ: %s", items[0].OutputFingerprint)
	}

	if items[0].WorkflowDefinitionID != "summary" {
		t.Fatalf("workflow definition id = %q, want summary", items[0].WorkflowDefinitionID)
	}

	if !strings.HasPrefix(items[0].WorkflowFingerprint, "workflow:sha256:") {
		t.Fatalf("unexpected workflow fingerprint: %q", items[0].WorkflowFingerprint)
	}

	if items[0].StepDefinitionID != "summarize" {
		t.Fatalf("step definition id = %q, want summarize", items[0].StepDefinitionID)
	}

	if !strings.HasPrefix(items[0].StepFingerprint, "step:sha256:") {
		t.Fatalf("unexpected step fingerprint: %q", items[0].StepFingerprint)
	}

	if !strings.HasPrefix(items[0].WorkItemFingerprint, "work-item:sha256:") {
		t.Fatalf("unexpected work item fingerprint: %q", items[0].WorkItemFingerprint)
	}

	if items[0].CodeVersion != "test-version" {
		t.Fatalf("code version = %q, want test-version", items[0].CodeVersion)
	}
}

func TestBuildSetting(t *testing.T) {
	info := &debug.BuildInfo{
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abc123"},
		},
	}

	if got := buildSetting(info, "vcs.revision"); got != "abc123" {
		t.Fatalf("build setting = %q, want abc123", got)
	}

	if got := buildSetting(info, "missing"); got != "" {
		t.Fatalf("missing build setting = %q, want empty", got)
	}
}

const testSlurmWorkerVariables = `
			{
				"name": {"namespace": "worker_config", "key": "scheduler"},
				"type": "object",
				"expression": {
					"type": {"type": "string", "expression": "slurm"},
					"settings": {"type": "object", "expression": {
						"script_path": {"type": "path", "expression": "/data/goetl/scripts/worker.slurm"},
						"job_name": {"type": "string", "expression": "goetl-worker"}
					}}
				}
			},
			{
				"name": {"namespace": "worker_config", "key": "runtime"},
				"type": "object",
				"expression": {
					"type": {"type": "string", "expression": "worker"},
					"settings": {"type": "object", "expression": {
						"executable": {"type": "path", "expression": "/data/goetl/artifacts/goetl-worker"},
						"config_path": {"type": "path", "expression": "/data/goetl/config/worker.json"},
						"log_dir": {"type": "path", "expression": "/data/goetl/logs"}
					}}
				}
			}`

func TestSubmitWorkflowHandlerStartsConfiguredWorker(t *testing.T) {
	scheduler := &testScheduler{}
	controller := newControllerWithTestEnvironment(scheduler)
	request := httptest.NewRequest(http.MethodPost, "/workflow", bytes.NewBufferString(`{
		"workflow": {
			"ID": "cdl",
			"Variables": [
				{
					"name": {"namespace": "workflow", "key": "years"},
					"type": "list",
					"expression": [{"type": "int", "expression": 2024}]
				}
			],
			"Steps": [
				{
					"ID": "download",
					"FanOut": {
						"WorkItem": {
							"FanOutExpression": "${years[*]}",
							"Type": "write_demo_output",
							"OutputPrefix": "cdl",
							"OutputExtension": ".txt"
						}
					}
				}
			]
		},
		"variables": [
`+testSlurmWorkerVariables+`
		]
	}`))
	response := httptest.NewRecorder()

	controller.submitWorkflowHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", response.Code)
	}

	if scheduler.calls != 1 {
		t.Fatalf("unexpected scheduler calls: %d", scheduler.calls)
	}
	if scheduler.jobs[0].RemoteScriptPath != "/data/goetl/scripts/worker.slurm" {
		t.Fatalf("remote script path = %q, want configured path", scheduler.jobs[0].RemoteScriptPath)
	}
	if scheduler.jobs[0].WorkerScript.WorkerExecutable != "/data/goetl/artifacts/goetl-worker" {
		t.Fatalf("worker executable = %q, want configured executable", scheduler.jobs[0].WorkerScript.WorkerExecutable)
	}
}

func TestSubmitWorkflowHandlerUsesConfiguredSlurmJob(t *testing.T) {
	scheduler := &testScheduler{}
	controller := newControllerWithTestEnvironment(scheduler)
	request := httptest.NewRequest(http.MethodPost, "/workflow", bytes.NewBufferString(`{
		"workflow": {
			"ID": "cdl",
			"Variables": [
				{
					"name": {"namespace": "workflow", "key": "years"},
					"type": "list",
					"expression": [{"type": "int", "expression": 2024}]
				}
			],
			"Steps": [
				{
					"ID": "download",
					"FanOut": {
						"WorkItem": {
							"FanOutExpression": "${years[*]}",
							"Type": "write_demo_output",
							"OutputPrefix": "cdl",
							"OutputExtension": ".txt"
						}
					}
				}
			]
		},
		"variables": [
`+testSlurmWorkerVariables+`
		]
	}`))
	response := httptest.NewRecorder()

	controller.submitWorkflowHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
	if scheduler.calls != 1 {
		t.Fatalf("unexpected scheduler calls: %d", scheduler.calls)
	}
	if scheduler.jobs[0].WorkerScript.WorkerConfigPath != "/data/goetl/config/worker.json" {
		t.Fatalf("worker config path = %q, want configured path", scheduler.jobs[0].WorkerScript.WorkerConfigPath)
	}
}

func TestSubmitWorkflowHandlerUsesSingularityWorkerRuntime(t *testing.T) {
	scheduler := &testScheduler{}
	controller := newControllerWithTestEnvironment(scheduler)
	controller.env.Transports = []Transport{&recordingTransport{}}
	controller.env.Runtime = SingularityWorkerRuntime{
		SingularityExecutable:     "singularity",
		ImagePath:                 "/data/goetl/images/goetl-worker.sif",
		ContainerWorkerExecutable: "/goetl/goetl-worker",
		Bind:                      "/data/goetl:/data/goetl",
	}
	request := httptest.NewRequest(http.MethodPost, "/workflow", bytes.NewBufferString(`{
		"workflow": {
			"ID": "cdl",
			"Variables": [
				{
					"name": {"namespace": "workflow", "key": "years"},
					"type": "list",
					"expression": [{"type": "int", "expression": 2024}]
				}
			],
			"Steps": [
				{
					"ID": "download",
					"FanOut": {
						"WorkItem": {
							"FanOutExpression": "${years[*]}",
							"Type": "write_demo_output",
							"OutputPrefix": "cdl",
							"OutputExtension": ".txt"
						}
					}
				}
			]
		},
		"variables": [
`+testSlurmWorkerVariables+`
		]
	}`))
	response := httptest.NewRecorder()

	controller.submitWorkflowHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
	if scheduler.calls != 1 {
		t.Fatalf("unexpected scheduler calls: %d", scheduler.calls)
	}
	script := scheduler.jobs[0].WorkerScript
	if script.WorkerExecutable != "singularity" {
		t.Fatalf("worker executable = %q, want singularity", script.WorkerExecutable)
	}
	wantArgs := []string{
		"exec",
		"--bind",
		"/data/goetl:/data/goetl",
		"/data/goetl/images/goetl-worker.sif",
		"/goetl/goetl-worker",
	}
	if !stringSlicesEqual(script.WorkerArgs, wantArgs) {
		t.Fatalf("worker args = %#v, want %#v", script.WorkerArgs, wantArgs)
	}
	if script.WorkerConfigPath != "/data/goetl/config/worker.json" {
		t.Fatalf("worker config path = %q, want original worker config path", script.WorkerConfigPath)
	}
}

func TestSubmitWorkflowHandlerStartsPlannedWorkerCount(t *testing.T) {
	scheduler := &testScheduler{}
	controller := newControllerWithTestEnvironment(scheduler)
	controller.scaleCfg = WorkerScaleConfig{MinCount: 2, MaxCount: 2, CountPerStart: 2}
	request := httptest.NewRequest(http.MethodPost, "/workflow", bytes.NewBufferString(`{
		"workflow": {
			"ID": "cdl",
			"Variables": [
				{
					"name": {"namespace": "workflow", "key": "years"},
					"type": "list",
					"expression": [{"type": "int", "expression": 2024}, {"type": "int", "expression": 2025}]
				}
			],
			"Steps": [
				{
					"ID": "download",
					"FanOut": {
						"WorkItem": {
							"FanOutExpression": "${years[*]}",
							"Type": "write_demo_output",
							"OutputPrefix": "cdl",
							"OutputExtension": ".txt"
						}
					}
				}
			]
		},
		"variables": [
`+testSlurmWorkerVariables+`
		]
	}`))
	response := httptest.NewRecorder()

	controller.submitWorkflowHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", response.Code)
	}

	if scheduler.calls != 2 {
		t.Fatalf("unexpected scheduler calls: %d", scheduler.calls)
	}
}

func TestSubmitWorkflowHandlerUsesSubmittedWorkerScaleConfig(t *testing.T) {
	scheduler := &testScheduler{}
	controller := newControllerWithTestEnvironment(scheduler)
	request := httptest.NewRequest(http.MethodPost, "/workflow", bytes.NewBufferString(`{
		"workflow": {
			"ID": "cdl",
			"Variables": [
				{
					"name": {"namespace": "workflow", "key": "years"},
					"type": "list",
					"expression": [{"type": "int", "expression": 2024}, {"type": "int", "expression": 2025}]
				}
			],
			"Steps": [
				{
					"ID": "download",
					"FanOut": {
						"WorkItem": {
							"FanOutExpression": "${years[*]}",
							"Type": "write_demo_output",
							"OutputPrefix": "cdl",
							"OutputExtension": ".txt"
						}
					}
				}
			]
		},
		"variables": [
`+testSlurmWorkerVariables+`,
			{
				"name": {"namespace": "worker_config", "key": "worker_min_count"},
				"type": "int",
				"expression": 2
			},
			{
				"name": {"namespace": "worker_config", "key": "worker_max_count"},
				"type": "int",
				"expression": 2
			},
			{
				"name": {"namespace": "worker_config", "key": "worker_count_per_start"},
				"type": "int",
				"expression": 2
			},
			{
				"name": {"namespace": "worker_config", "key": "worker_min_elapsed_time_between_starts"},
				"type": "string",
				"expression": "0s"
			}
		]
	}`))
	response := httptest.NewRecorder()

	controller.submitWorkflowHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", response.Code)
	}

	if scheduler.calls != 2 {
		t.Fatalf("unexpected scheduler calls: %d", scheduler.calls)
	}
}

func TestSubmitWorkflowHandlerRejectsInvalidWorkerScaleConfig(t *testing.T) {
	controller := newController(nil)
	request := httptest.NewRequest(http.MethodPost, "/workflow", bytes.NewBufferString(`{
		"workflow": {
			"ID": "cdl",
			"Variables": [
				{
					"name": {"namespace": "workflow", "key": "years"},
					"type": "list",
					"expression": [{"type": "int", "expression": 2024}]
				}
			],
			"Steps": [
				{
					"ID": "download",
					"FanOut": {
						"WorkItem": {
							"FanOutExpression": "${years[*]}",
							"Type": "write_demo_output",
							"OutputPrefix": "cdl",
							"OutputExtension": ".txt"
						}
					}
				}
			]
		},
		"variables": [
			{
				"name": {"namespace": "worker_config", "key": "worker_target_environment"},
				"type": "string",
				"expression": "local"
			},
			{
				"name": {"namespace": "worker_config", "key": "worker_max_count"},
				"type": "string",
				"expression": "two"
			}
		]
	}`))
	response := httptest.NewRecorder()

	controller.submitWorkflowHandler(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestSubmitWorkflowHandlerWaitsForWorkerClaimBeforeOrganicScaleUp(t *testing.T) {
	scheduler := &testScheduler{}
	controller := newControllerWithTestEnvironment(scheduler)
	controller.scaleCfg = WorkerScaleConfig{MaxCount: 2, CountPerStart: 1}

	submitWorkflowYears(t, controller, 2024)
	submitWorkflowYears(t, controller, 2025)

	if scheduler.calls != 1 {
		t.Fatalf("unexpected scheduler calls before claim: %d", scheduler.calls)
	}

	assignNextWork(t, controller)
	submitWorkflowYears(t, controller, 2026)

	if scheduler.calls != 2 {
		t.Fatalf("unexpected scheduler calls after claim: %d", scheduler.calls)
	}
}

func TestSubmitWorkflowHandlerRejectsDuplicateGeneratedID(t *testing.T) {
	controller := newTestController()
	request := httptest.NewRequest(http.MethodPost, "/workflow", bytes.NewBufferString(`{
		"workflow": {
			"ID": "cdl",
			"Variables": [
				{
					"name": {"namespace": "workflow", "key": "years"},
					"type": "list",
					"expression": [{"type": "string", "expression": "001"}]
				}
			],
			"Steps": [
				{
					"ID": "test",
					"FanOut": {
						"WorkItem": {
							"FanOutExpression": "${years[*]}",
							"Type": "write_demo_output",
							"IDPrefix": "test",
							"OutputPrefix": "cdl",
							"OutputExtension": ".txt"
						}
					}
				}
			]
		}
	}`))
	response := httptest.NewRecorder()

	controller.submitWorkflowHandler(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func submitWorkflowYears(t *testing.T, controller *Controller, year int) {
	t.Helper()

	request := httptest.NewRequest(http.MethodPost, "/workflow", bytes.NewBufferString(`{
		"workflow": {
			"ID": "cdl",
			"Variables": [
				{
					"name": {"namespace": "workflow", "key": "years"},
					"type": "list",
					"expression": [{"type": "int", "expression": `+strconv.Itoa(year)+`}]
				}
			],
			"Steps": [
				{
					"ID": "download",
					"FanOut": {
						"WorkItem": {
							"FanOutExpression": "${years[*]}",
							"Type": "write_demo_output",
							"OutputPrefix": "cdl",
							"OutputExtension": ".txt"
						}
					}
				}
			]
		},
		"variables": [
`+testSlurmWorkerVariables+`
		]
	}`))
	response := httptest.NewRecorder()

	controller.submitWorkflowHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

type testScheduler struct {
	calls int
	jobs  []JobSpec
}

func (s *testScheduler) Submit(ctx context.Context, job JobSpec) (JobHandle, error) {
	s.calls++
	s.jobs = append(s.jobs, job)
	return JobHandle{ID: strconv.Itoa(s.calls)}, nil
}

func newControllerWithTestEnvironment(scheduler Scheduler) *Controller {
	controller := newController(nil)
	controller.env = &ExecutionEnvironment{
		Dialect:   BashShellPlatform{},
		Scheduler: scheduler,
	}
	return controller
}

func TestSubmitWorkflowHandlerRejectsInvalidPayload(t *testing.T) {
	controller := newController(nil)
	request := httptest.NewRequest(http.MethodPost, "/workflow", bytes.NewBufferString(`{"workflow": {}}`))
	response := httptest.NewRecorder()

	controller.submitWorkflowHandler(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestSubmitWorkflowHandlerRejectsGet(t *testing.T) {
	controller := newController(nil)
	request := httptest.NewRequest(http.MethodGet, "/workflow", nil)
	response := httptest.NewRecorder()

	controller.submitWorkflowHandler(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestShutdownHandler(t *testing.T) {
	called := make(chan struct{}, 1)
	controller := newController(nil)
	controller.shutdown = func(context.Context) error {
		called <- struct{}{}
		return nil
	}

	request := httptest.NewRequest(http.MethodPost, "/shutdown", nil)
	response := httptest.NewRecorder()

	controller.shutdownHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", response.Code)
	}

	<-called
}

func TestShutdownHandlerRejectsGet(t *testing.T) {
	controller := newController(nil)
	request := httptest.NewRequest(http.MethodGet, "/shutdown", nil)
	response := httptest.NewRecorder()

	controller.shutdownHandler(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestShutdownHandlerRejectsUnavailableShutdown(t *testing.T) {
	controller := newController(nil)
	request := httptest.NewRequest(http.MethodPost, "/shutdown", nil)
	response := httptest.NewRecorder()

	controller.shutdownHandler(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestRecoveryModeBlocksNormalAdmission(t *testing.T) {
	controller := newController(nil)
	controller.enterRecoveryMode()

	request := httptest.NewRequest(http.MethodPost, "/workflow", bytes.NewBufferString(`{"workflow":{"ID":"cdl"}}`))
	response := httptest.NewRecorder()

	controller.submitWorkflowHandler(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), "recovery mode") {
		t.Fatalf("response body = %q, want recovery-mode context", response.Body.String())
	}
}

func TestRecoveryModeBlocksStatusAndShutdown(t *testing.T) {
	controller := newController(nil)
	controller.enterRecoveryMode()

	statusReq := httptest.NewRequest(http.MethodGet, "/status", nil)
	statusResp := httptest.NewRecorder()
	controller.statusHandler(statusResp, statusReq)
	if statusResp.Code != http.StatusServiceUnavailable {
		t.Fatalf("status code = %d, want 503", statusResp.Code)
	}

	shutdownReq := httptest.NewRequest(http.MethodPost, "/shutdown", nil)
	shutdownResp := httptest.NewRecorder()
	controller.shutdownHandler(shutdownResp, shutdownReq)
	if shutdownResp.Code != http.StatusServiceUnavailable {
		t.Fatalf("shutdown code = %d, want 503", shutdownResp.Code)
	}
}

func TestRecoveryModeAllowsWorkerReportEndpoints(t *testing.T) {
	controller := newController(nil)
	controller.enterRecoveryMode()
	controller.assigned["test-001"] = model.WorkItem{
		ID:             "test-001",
		Type:           model.WorkItemTypeWriteDemoOutput,
		OutputFilename: "result.txt",
	}

	completeReq := httptest.NewRequest(http.MethodPost, "/work/complete", bytes.NewBufferString(`{"id":"test-001"}`))
	completeResp := httptest.NewRecorder()
	controller.completeWorkHandler(completeResp, completeReq)
	if completeResp.Code != http.StatusNoContent {
		t.Fatalf("complete status = %d, want 204", completeResp.Code)
	}

	controller.assigned["test-002"] = model.WorkItem{
		ID:             "test-002",
		Type:           model.WorkItemTypeWriteDemoOutput,
		OutputFilename: "result-2.txt",
	}
	failReq := httptest.NewRequest(http.MethodPost, "/work/fail", bytes.NewBufferString(`{"id":"test-002","error":"boom"}`))
	failResp := httptest.NewRecorder()
	controller.failWorkHandler(failResp, failReq)
	if failResp.Code != http.StatusNoContent {
		t.Fatalf("fail status = %d, want 204", failResp.Code)
	}
}

func TestHealthHandler(t *testing.T) {
	controller := newController(nil)
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	controller.healthHandler(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", response.Code)
	}
}

func TestEnterRecoveryModeStampsRecoveryStartedAt(t *testing.T) {
	controller := newController(nil)
	if !controller.recoveryStartedAt.IsZero() {
		t.Fatal("new controller should not start with recovery timestamp")
	}

	before := time.Now().UTC()
	controller.enterRecoveryMode()

	if controller.normalAdmission {
		t.Fatal("recovery mode should close normal admission")
	}
	if controller.recoveryStartedAt.IsZero() {
		t.Fatal("recovery timestamp should be set")
	}
	if controller.recoveryStartedAt.Before(before) {
		t.Fatalf("recovery timestamp = %v, want not before %v", controller.recoveryStartedAt, before)
	}
}

func newTestController() *Controller {
	return newController([]model.WorkItem{
		{
			ID:             "test-001",
			Type:           model.WorkItemTypeWriteDemoOutput,
			OutputFilename: "result.txt",
		},
	})
}

func testWorkItem(id string) model.WorkItem {
	return model.WorkItem{
		ID:             id,
		Type:           model.WorkItemTypeWriteDemoOutput,
		OutputFilename: id + ".txt",
	}
}

func openTestWorkflowExecutionStore(t *testing.T) *persistence.Store {
	t.Helper()

	store, err := persistence.OpenStore(context.Background(), persistence.Config{
		Driver:           persistence.DriverSQLite,
		ConnectionString: filepath.Join(t.TempDir(), "workflow-execution.sqlite"),
	})
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	return store
}

func insertTestPersistenceRunWithStage(t *testing.T, ctx context.Context, store *persistence.Store) persistence.WorkflowRunRecord {
	t.Helper()

	project := persistence.ProjectRecord{
		ID:                 "project-001",
		Name:               "Project",
		RepositoryIdentity: "repo",
		SourceCommit:       "commit",
		ConfigPath:         "project.json",
		SourceObjectID:     "object",
		ConfigSHA256:       strings.Repeat("a", 64),
		CreatedAt:          "2026-07-03T00:00:00Z",
	}
	if err := store.UpsertProject(ctx, project); err != nil {
		t.Fatalf("UpsertProject() error = %v", err)
	}
	workflow := persistence.WorkflowRecord{
		ID:                 "workflow-001",
		ProjectID:          project.ID,
		Name:               "Workflow",
		RepositoryIdentity: "repo",
		SourceCommit:       "commit",
		WorkflowPath:       "workflow.json",
		SourceObjectID:     "object",
		WorkflowSHA256:     strings.Repeat("b", 64),
		CreatedAt:          "2026-07-03T00:00:00Z",
	}
	if err := store.UpsertWorkflow(ctx, workflow); err != nil {
		t.Fatalf("UpsertWorkflow() error = %v", err)
	}
	run := persistence.WorkflowRunRecord{
		ID:                    "run-001",
		ProjectID:             project.ID,
		WorkflowID:            workflow.ID,
		SubmissionContextJSON: `{"variables":[]}`,
		CreatedAt:             "2026-07-03T00:00:00Z",
	}
	if err := store.CreateWorkflowRun(ctx, run); err != nil {
		t.Fatalf("CreateWorkflowRun() error = %v", err)
	}
	if err := store.InsertStagePlan(ctx, run.ID, []persistence.WorkflowStageRecord{
		{
			RunID:                run.ID,
			StageIndex:           0,
			StepID:               "step-001",
			StageSourceReference: "workflow.json#/steps/0",
			State:                "ready",
			CreatedAt:            "2026-07-03T00:00:00Z",
			ReadyAt:              "2026-07-03T00:00:00Z",
		},
	}); err != nil {
		t.Fatalf("InsertStagePlan() error = %v", err)
	}
	return run
}

func testPersistenceWorkItem(id string, runID string, stageIndex int, workItemIndex int) persistence.WorkItemRecord {
	return persistence.WorkItemRecord{
		ID:                   id,
		RunID:                runID,
		StageIndex:           stageIndex,
		WorkItemIndex:        workItemIndex,
		WorkerPayloadJSON:    `{"plugin":"plugin-name","parameters":{}}`,
		ResolvedInputsSHA256: strings.Repeat("c", 64),
		CreatedAt:            "2026-07-03T00:00:00Z",
	}
}

func reusableTestWorkItem(id string) model.WorkItem {
	return model.WorkItem{
		ID:                   id,
		Type:                 model.WorkItemTypeWriteDemoOutput,
		OutputFilename:       id + ".txt",
		WorkflowDefinitionID: "workflow-definition-002",
		WorkflowFingerprint:  "workflow-fingerprint",
		WorkflowInstanceID:   "workflow-instance-002",
		StepDefinitionID:     "step-definition-002",
		StepFingerprint:      "step-fingerprint",
		StepInstanceID:       "step-instance-002",
		WorkItemFingerprint:  "work-item-fingerprint",
		InputFingerprint:     "input-fingerprint",
		OutputFingerprint:    "output-fingerprint",
		CodeVersion:          "code-version",
	}
}

func newControllerWithCompletedAttempt(t *testing.T, completion model.WorkCompletion) *Controller {
	t.Helper()

	controller := newController(nil)
	db := testSQLiteMainDatabase(t)
	t.Cleanup(func() {
		db.Close()
	})
	controller.ledger = db

	attempt, _, err := attemptFromCompletion(completion)
	if err != nil {
		t.Fatalf("build attempt: %v", err)
	}
	if err := controller.recordAttempt(context.Background(), attempt); err != nil {
		t.Fatalf("record attempt: %v", err)
	}

	return controller
}

func getStatus(t *testing.T, controller *Controller) model.ControllerStatus {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, "/status", nil)
	response := httptest.NewRecorder()
	controller.statusHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status code: %d", response.Code)
	}

	var status model.ControllerStatus
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		t.Fatalf("decode status: %v", err)
	}

	return status
}

func withAttemptStatus(attempt ledger.Attempt, status ledger.AttemptStatus) ledger.Attempt {
	attempt.Status = status
	return attempt
}

func withWorkItemFingerprint(item model.WorkItem, fingerprint string) model.WorkItem {
	item.WorkItemFingerprint = fingerprint
	return item
}

func withInputFingerprint(item model.WorkItem, fingerprint string) model.WorkItem {
	item.InputFingerprint = fingerprint
	return item
}

func withOutputFingerprint(item model.WorkItem, fingerprint string) model.WorkItem {
	item.OutputFingerprint = fingerprint
	return item
}

func withCodeVersion(item model.WorkItem, codeVersion string) model.WorkItem {
	item.CodeVersion = codeVersion
	return item
}

func mustParseTime(t *testing.T, text string) time.Time {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339, text)
	if err != nil {
		t.Fatalf("parse time %q: %v", text, err)
	}
	return parsed
}

func attemptVariablesByName(variables []ledger.AttemptVariable) map[string]ledger.AttemptVariable {
	byName := make(map[string]ledger.AttemptVariable, len(variables))
	for _, variable := range variables {
		byName[variable.Name] = variable
	}
	return byName
}

func assignNextWork(t *testing.T, controller *Controller) {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, "/work/next", nil)
	response := httptest.NewRecorder()
	controller.nextWorkHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected assignment status code: %d", response.Code)
	}
}

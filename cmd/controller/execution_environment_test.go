package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestExecutionEnvironmentConfigValidate(t *testing.T) {
	cfg := ExecutionEnvironmentConfig{
		Name: "dockerized-slurm",
		Transports: []ExecutionComponentConfig{
			{
				Name: "control",
				Type: "docker",
				Settings: ExecutionComponentSettings{
					"container": "slurmctld",
				},
			},
		},
		Dialect:   ExecutionComponentConfig{Type: "bash"},
		Scheduler: ExecutionComponentConfig{Type: "slurm"},
		Runtime:   ExecutionComponentConfig{Type: "worker"},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecutionEnvironmentConfigValidateRejectsMissingTransport(t *testing.T) {
	cfg := ExecutionEnvironmentConfig{
		Name:      "dockerized-slurm",
		Dialect:   ExecutionComponentConfig{Type: "bash"},
		Scheduler: ExecutionComponentConfig{Type: "slurm"},
		Runtime:   ExecutionComponentConfig{Type: "worker"},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected an error")
	}
}

func TestNewExecutionEnvironmentStoresValidatedConfig(t *testing.T) {
	env, err := NewExecutionEnvironment(ExecutionEnvironmentConfig{
		Name: "dockerized-slurm",
		Transports: []ExecutionComponentConfig{{
			Type: "docker",
			Settings: ExecutionComponentSettings{
				"container":  "slurmctld",
				"executable": "podman",
			},
		}},
		Dialect:   ExecutionComponentConfig{Type: "bash"},
		Scheduler: ExecutionComponentConfig{Type: "slurm"},
		Runtime:   ExecutionComponentConfig{Type: "worker", Settings: ExecutionComponentSettings{"root": "/data/goetl"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if env.Config.Name != "dockerized-slurm" {
		t.Fatalf("environment name = %q, want dockerized-slurm", env.Config.Name)
	}
	if len(env.Transports) != 1 {
		t.Fatalf("transport count = %d, want 1", len(env.Transports))
	}
	transport, ok := env.Transports[0].(DockerContainerTransport)
	if !ok {
		t.Fatalf("transport type = %T, want DockerContainerTransport", env.Transports[0])
	}
	if transport.Container != "slurmctld" {
		t.Fatalf("container = %q, want slurmctld", transport.Container)
	}
	if transport.Docker.Executable != "podman" {
		t.Fatalf("docker executable = %q, want podman", transport.Docker.Executable)
	}
	if _, ok := env.Dialect.(BashShellPlatform); !ok {
		t.Fatalf("dialect type = %T, want BashShellPlatform", env.Dialect)
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

func TestNewExecutionEnvironmentSupportsWorkerRuntimeControllerTokenFile(t *testing.T) {
	env, err := NewExecutionEnvironment(ExecutionEnvironmentConfig{
		Name:       "local-direct",
		Transports: []ExecutionComponentConfig{{Type: "local"}},
		Dialect:    ExecutionComponentConfig{Type: "bash"},
		Scheduler:  ExecutionComponentConfig{Type: "direct_process"},
		Runtime: ExecutionComponentConfig{Type: "worker", Settings: ExecutionComponentSettings{
			"root":                  "/tmp/goetl",
			"controller_url":        "https://controller.example.org",
			"controller_token_file": "/tmp/goetl/secrets/controller-worker-token",
			"controller_insecure_external_http_allowed": true,
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runtime, ok := env.Runtime.(WorkerRuntime)
	if !ok {
		t.Fatalf("runtime type = %T, want WorkerRuntime", env.Runtime)
	}
	if runtime.ControllerTokenFile != "/tmp/goetl/secrets/controller-worker-token" {
		t.Fatalf("controller token file = %q", runtime.ControllerTokenFile)
	}
	if !runtime.ControllerInsecureExternalHTTPAllowed {
		t.Fatal("expected insecure external HTTP to be allowed")
	}
}

func TestNewExecutionEnvironmentRejectsNonBooleanWorkerRuntimeInsecureHTTPSetting(t *testing.T) {
	_, err := NewExecutionEnvironment(ExecutionEnvironmentConfig{
		Name:       "local-direct",
		Transports: []ExecutionComponentConfig{{Type: "local"}},
		Dialect:    ExecutionComponentConfig{Type: "bash"},
		Scheduler:  ExecutionComponentConfig{Type: "direct_process"},
		Runtime: ExecutionComponentConfig{Type: "worker", Settings: ExecutionComponentSettings{
			"controller_insecure_external_http_allowed": "true",
		}},
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "controller_insecure_external_http_allowed must be a boolean") {
		t.Fatalf("error = %v, want boolean setting error", err)
	}
}

func TestNewExecutionEnvironmentSupportsLocalDirectProcess(t *testing.T) {
	env, err := NewExecutionEnvironment(ExecutionEnvironmentConfig{
		Name:       "local-direct",
		Transports: []ExecutionComponentConfig{{Type: "local"}},
		Dialect:    ExecutionComponentConfig{Type: "bash"},
		Scheduler:  ExecutionComponentConfig{Type: "direct_process"},
		Runtime: ExecutionComponentConfig{Type: "worker", Settings: ExecutionComponentSettings{
			"root":              "/tmp/goetl",
			"python_executable": "python3",
			"max_asset_bytes":   float64(20000000000),
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := env.Transports[0].(LocalTransport); !ok {
		t.Fatalf("transport type = %T, want LocalTransport", env.Transports[0])
	}
	if _, ok := env.Scheduler.(DirectProcessScheduler); !ok {
		t.Fatalf("scheduler type = %T, want DirectProcessScheduler", env.Scheduler)
	}
	runtime, ok := env.Runtime.(WorkerRuntime)
	if !ok {
		t.Fatalf("runtime type = %T, want WorkerRuntime", env.Runtime)
	}
	if runtime.PythonExecutable != "python3" {
		t.Fatalf("python executable = %q, want python3", runtime.PythonExecutable)
	}
	if runtime.MaxAssetBytes != 20000000000 {
		t.Fatalf("max asset bytes = %d, want 20000000000", runtime.MaxAssetBytes)
	}
}

func TestNewExecutionEnvironmentSupportsRemoteProcess(t *testing.T) {
	env, err := NewExecutionEnvironment(ExecutionEnvironmentConfig{
		Name:       "ssh-remote-process",
		Transports: []ExecutionComponentConfig{{Type: "local"}},
		Dialect:    ExecutionComponentConfig{Type: "bash"},
		Scheduler:  ExecutionComponentConfig{Type: "remote_process"},
		Runtime: ExecutionComponentConfig{Type: "worker", Settings: ExecutionComponentSettings{
			"root": "/tmp/goetl",
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	scheduler, ok := env.Scheduler.(RemoteProcessScheduler)
	if !ok {
		t.Fatalf("scheduler type = %T, want RemoteProcessScheduler", env.Scheduler)
	}
	if scheduler.Transport == nil {
		t.Fatal("remote process scheduler transport is nil")
	}
}

func TestNewExecutionEnvironmentSupportsSingularityWorkerRuntime(t *testing.T) {
	env, err := NewExecutionEnvironment(ExecutionEnvironmentConfig{
		Name:       "local-singularity",
		Transports: []ExecutionComponentConfig{{Type: "local"}},
		Dialect:    ExecutionComponentConfig{Type: "bash"},
		Scheduler:  ExecutionComponentConfig{Type: "direct_process"},
		Runtime: ExecutionComponentConfig{
			Type: "singularity_worker",
			Settings: ExecutionComponentSettings{
				"root":                        "/tmp/goetl",
				"controller_url":              "http://localhost:8080",
				"image_path":                  "/tmp/goetl/images/goetl-worker.sif",
				"container_worker_executable": "/goetl/goetl-worker",
				"singularity_executable":      "singularity",
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runtime, ok := env.Runtime.(SingularityWorkerRuntime)
	if !ok {
		t.Fatalf("runtime type = %T, want SingularityWorkerRuntime", env.Runtime)
	}
	if runtime.Root != "/tmp/goetl" {
		t.Fatalf("runtime root = %q, want /tmp/goetl", runtime.Root)
	}
	if runtime.ImagePath != "/tmp/goetl/images/goetl-worker.sif" {
		t.Fatalf("image path = %q, want configured image path", runtime.ImagePath)
	}
	script, err := runtime.WorkerScript(SlurmWorkerScriptConfig{
		JobName:          "goetl-worker",
		WorkerExecutable: "/tmp/goetl/artifacts/goetl-worker",
		WorkerConfigPath: "/tmp/goetl/config/worker.json",
		LogDir:           "/tmp/goetl/logs",
	})
	if err != nil {
		t.Fatalf("worker script: %v", err)
	}
	wantArgs := []string{
		"exec",
		"--bind",
		"/tmp/goetl:/tmp/goetl",
		"/tmp/goetl/images/goetl-worker.sif",
		"/goetl/goetl-worker",
	}
	if !stringSlicesEqual(script.WorkerArgs, wantArgs) {
		t.Fatalf("worker args = %#v, want %#v", script.WorkerArgs, wantArgs)
	}
}

func TestNewExecutionEnvironmentSupportsWorkerRuntimeDataLocationRoots(t *testing.T) {
	env, err := NewExecutionEnvironment(ExecutionEnvironmentConfig{
		Name:       "fake-hpcc-data-assets",
		Transports: []ExecutionComponentConfig{{Type: "local"}},
		Dialect:    ExecutionComponentConfig{Type: "bash"},
		Scheduler:  ExecutionComponentConfig{Type: "slurm"},
		Runtime: ExecutionComponentConfig{
			Type: "worker",
			Settings: ExecutionComponentSettings{
				"root":            ".run/fake-hpcc-data-assets/runtime",
				"controller_url":  "http://localhost:8080",
				"data_dir":        ".run/fake-hpcc-data-assets/worker-data",
				"asset_cache_dir": ".run/fake-hpcc-data-assets/cache/assets",
				"data_location_roots": map[string]any{
					"fixture_data":   ".run/fake-hpcc-data-assets/fixture-data",
					"published_data": ".run/fake-hpcc-data-assets/published-data",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runtime, ok := env.Runtime.(WorkerRuntime)
	if !ok {
		t.Fatalf("runtime type = %T, want WorkerRuntime", env.Runtime)
	}
	if runtime.DataDir != ".run/fake-hpcc-data-assets/worker-data" {
		t.Fatalf("data dir = %q", runtime.DataDir)
	}
	if runtime.AssetCacheDir != ".run/fake-hpcc-data-assets/cache/assets" {
		t.Fatalf("asset cache dir = %q", runtime.AssetCacheDir)
	}
	if runtime.DataLocationRoots["fixture_data"] != ".run/fake-hpcc-data-assets/fixture-data" ||
		runtime.DataLocationRoots["published_data"] != ".run/fake-hpcc-data-assets/published-data" {
		t.Fatalf("data location roots = %#v", runtime.DataLocationRoots)
	}
}

func TestNewExecutionEnvironmentSupportsSSHTransport(t *testing.T) {
	env, err := NewExecutionEnvironment(ExecutionEnvironmentConfig{
		Name: "ssh-slurm",
		Transports: []ExecutionComponentConfig{{
			Type: "ssh",
			Settings: ExecutionComponentSettings{
				"host":            "hpcc.example.edu",
				"port":            "2222",
				"user":            "researcher",
				"identity_file":   "/home/researcher/.ssh/id_ed25519",
				"host_key_policy": "pinned",
				"pinned_host_key": "ssh-ed25519 AAAATESTKEY",
				"jump_hosts": []any{
					map[string]any{
						"host":            "gateway.example.edu",
						"port":            "22",
						"user":            "researcher",
						"host_key_policy": "pinned",
						"pinned_host_key": "ssh-ed25519 AAAAGATEWAY",
					},
				},
			},
		}},
		Dialect:   ExecutionComponentConfig{Type: "bash"},
		Scheduler: ExecutionComponentConfig{Type: "slurm"},
		Runtime:   ExecutionComponentConfig{Type: "worker"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	transport, ok := env.Transports[0].(*SSHTransport)
	if !ok {
		t.Fatalf("transport type = %T, want *SSHTransport", env.Transports[0])
	}
	if transport.Config.Host != "hpcc.example.edu" {
		t.Fatalf("ssh host = %q, want hpcc.example.edu", transport.Config.Host)
	}
	if transport.Config.Port != 2222 {
		t.Fatalf("ssh port = %d, want 2222", transport.Config.Port)
	}
	if len(transport.Config.JumpHosts) != 1 {
		t.Fatalf("jump host count = %d, want 1", len(transport.Config.JumpHosts))
	}
	if transport.Config.JumpHosts[0].Host != "gateway.example.edu" {
		t.Fatalf("jump host = %q, want gateway.example.edu", transport.Config.JumpHosts[0].Host)
	}
}

func TestNewExecutionEnvironmentSupportsSSHReverseCallbackTunnel(t *testing.T) {
	env, err := NewExecutionEnvironment(ExecutionEnvironmentConfig{
		Name: "ssh-slurm",
		Transports: []ExecutionComponentConfig{{
			Name: "login",
			Type: "ssh",
			Settings: ExecutionComponentSettings{
				"host":            "dev.example.edu",
				"user":            "researcher",
				"identity_env":    "GOETL_SSH_KEY",
				"host_key_policy": "pinned",
				"pinned_host_key": "ssh-ed25519 AAAATARGET",
				"jump_hosts": []any{
					map[string]any{
						"host":            "gateway.example.edu",
						"user":            "researcher",
						"host_key_policy": "pinned",
						"pinned_host_key": "ssh-ed25519 AAAAGATEWAY",
					},
				},
			},
		}},
		Dialect:   ExecutionComponentConfig{Type: "bash"},
		Scheduler: ExecutionComponentConfig{Type: "slurm"},
		Runtime: ExecutionComponentConfig{
			Type: "worker",
			Settings: ExecutionComponentSettings{
				"root":           "/data/goetl",
				"controller_url": "http://hpcc.msu.edu:18080",
			},
		},
		CallbackTunnel: CallbackTunnelConfig{
			Type:                "ssh_reverse",
			Transport:           "login",
			BindHop:             "jump_hosts[0]",
			RemoteBindHost:      "0.0.0.0",
			RemoteBindPort:      18080,
			LocalHost:           "127.0.0.1",
			LocalPort:           8080,
			WorkerControllerURL: "http://hpcc.msu.edu:18080",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if env.CallbackTunnel == nil {
		t.Fatal("expected callback tunnel")
	}
	if env.CallbackTunnel.Config.BindHop != "jump_hosts[0]" {
		t.Fatalf("bind hop = %q, want jump_hosts[0]", env.CallbackTunnel.Config.BindHop)
	}
	if env.CallbackTunnel.transport == nil {
		t.Fatal("expected callback tunnel ssh transport")
	}
}

func TestNewExecutionEnvironmentRejectsCallbackTunnelControllerURLMismatch(t *testing.T) {
	_, err := NewExecutionEnvironment(ExecutionEnvironmentConfig{
		Name: "ssh-slurm",
		Transports: []ExecutionComponentConfig{{
			Name: "login",
			Type: "ssh",
			Settings: ExecutionComponentSettings{
				"host":            "dev.example.edu",
				"user":            "researcher",
				"identity_env":    "GOETL_SSH_KEY",
				"host_key_policy": "pinned",
				"pinned_host_key": "ssh-ed25519 AAAATARGET",
			},
		}},
		Dialect:   ExecutionComponentConfig{Type: "bash"},
		Scheduler: ExecutionComponentConfig{Type: "slurm"},
		Runtime: ExecutionComponentConfig{
			Type: "worker",
			Settings: ExecutionComponentSettings{
				"controller_url": "http://hpcc.msu.edu:18080",
			},
		},
		CallbackTunnel: CallbackTunnelConfig{
			Type:                "ssh_reverse",
			Transport:           "login",
			RemoteBindHost:      "127.0.0.1",
			RemoteBindPort:      18080,
			LocalHost:           "127.0.0.1",
			LocalPort:           8080,
			WorkerControllerURL: "http://different.example:18080",
		},
	})
	if err == nil {
		t.Fatal("expected callback tunnel URL mismatch")
	}
	if !strings.Contains(err.Error(), "must match runtime controller_url") {
		t.Fatalf("error = %v, want runtime URL mismatch", err)
	}
}

func TestNewExecutionEnvironmentRejectsInvalidSSHTransportConfig(t *testing.T) {
	_, err := NewExecutionEnvironment(ExecutionEnvironmentConfig{
		Name:       "bad-env",
		Transports: []ExecutionComponentConfig{{Type: "ssh"}},
		Dialect:    ExecutionComponentConfig{Type: "bash"},
		Scheduler:  ExecutionComponentConfig{Type: "slurm"},
		Runtime:    ExecutionComponentConfig{Type: "worker"},
	})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestNewExecutionEnvironmentRejectsUnsupportedComponentType(t *testing.T) {
	_, err := NewExecutionEnvironment(ExecutionEnvironmentConfig{
		Name:       "bad-env",
		Transports: []ExecutionComponentConfig{{Type: "telepathy"}},
		Dialect:    ExecutionComponentConfig{Type: "bash"},
		Scheduler:  ExecutionComponentConfig{Type: "slurm"},
		Runtime:    ExecutionComponentConfig{Type: "worker"},
	})
	if err == nil {
		t.Fatal("expected an error")
	}
}

type prepareTransport struct {
	calls int
	err   error
}

func (t *prepareTransport) Prepare(ctx context.Context) error {
	t.calls++
	return t.err
}

func (t *prepareTransport) Copy(ctx context.Context, localPath string, remotePath string) error {
	return nil
}

func (t *prepareTransport) Exec(ctx context.Context, args ...string) ([]byte, error) {
	return nil, nil
}

func TestExecutionEnvironmentPrepareCallsSupportedComponents(t *testing.T) {
	transport := &prepareTransport{}
	env := ExecutionEnvironment{
		Transports: []Transport{transport},
		Dialect:    BashShellPlatform{},
		Runtime:    WorkerRuntime{},
	}

	if err := env.Prepare(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if transport.calls != 1 {
		t.Fatalf("transport prepare calls = %d, want 1", transport.calls)
	}
}

func TestExecutionEnvironmentPrepareReportsTransportError(t *testing.T) {
	env := ExecutionEnvironment{
		Transports: []Transport{&prepareTransport{err: fmt.Errorf("docker unavailable")}},
	}

	err := env.Prepare(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "prepare transport[0]") {
		t.Fatalf("error = %v, want transport context", err)
	}
}

type closeTransport struct {
	prepareTransport
	closed bool
	err    error
}

func (t *closeTransport) Close() error {
	t.closed = true
	return t.err
}

func TestExecutionEnvironmentCloseClosesTransportsInReverseOrder(t *testing.T) {
	first := &closeTransport{}
	second := &closeTransport{}
	env := ExecutionEnvironment{
		Transports: []Transport{first, second},
	}

	if err := env.Close(); err != nil {
		t.Fatalf("unexpected close error: %v", err)
	}
	if !first.closed || !second.closed {
		t.Fatalf("closed first=%v second=%v, want both closed", first.closed, second.closed)
	}
}

func TestExecutionEnvironmentCloseReturnsFirstCloseError(t *testing.T) {
	env := ExecutionEnvironment{
		Transports: []Transport{
			&closeTransport{},
			&closeTransport{err: fmt.Errorf("close failed")},
		},
	}

	err := env.Close()
	if err == nil || !strings.Contains(err.Error(), "close failed") {
		t.Fatalf("error = %v, want close failed", err)
	}
}

type preflightTransport struct {
	prepareTransport
	issues []PreflightIssue
}

func (t *preflightTransport) Preflight(ctx context.Context) []PreflightIssue {
	return t.issues
}

type preflightScheduler struct {
	issues []PreflightIssue
}

func (s preflightScheduler) Submit(ctx context.Context, job JobSpec) (JobHandle, error) {
	return JobHandle{}, nil
}

func (s preflightScheduler) Preflight(ctx context.Context) []PreflightIssue {
	return s.issues
}

type preflightRuntime struct {
	issues []PreflightIssue
}

func (r preflightRuntime) Prepare(ctx context.Context, transport Transport, dialect ShellDialect) error {
	return nil
}

func (r preflightRuntime) Preflight(ctx context.Context) []PreflightIssue {
	return r.issues
}

type contextAwarePreflightRuntime struct {
	transport Transport
	dialect   ShellDialect
}

func (r *contextAwarePreflightRuntime) Prepare(ctx context.Context, transport Transport, dialect ShellDialect) error {
	return nil
}

func (r *contextAwarePreflightRuntime) RuntimePreflight(ctx context.Context, transport Transport, dialect ShellDialect) []PreflightIssue {
	r.transport = transport
	r.dialect = dialect
	return []PreflightIssue{{Severity: PreflightSeverityWarning, Code: "checked_with_context"}}
}

func TestExecutionEnvironmentPreflightNoComponentsReturnsNoIssues(t *testing.T) {
	env := ExecutionEnvironment{
		Transports: []Transport{LocalTransport{}},
		Scheduler:  DirectProcessScheduler{},
		Runtime:    WorkerRuntime{},
	}

	issues := env.Preflight(context.Background())
	if len(issues) != 0 {
		t.Fatalf("issues = %#v, want none", issues)
	}
}

func TestExecutionEnvironmentPreflightReturnsBlockingIssue(t *testing.T) {
	env := ExecutionEnvironment{
		Transports: []Transport{&preflightTransport{issues: []PreflightIssue{{
			Type:        "ssh",
			Severity:    PreflightSeverityError,
			Code:        "ssh_auth_failed",
			Message:     "SSH authentication failed.",
			Remediation: "Check the configured identity file.",
		}}}},
	}

	issues := env.Preflight(context.Background())
	if len(issues) != 1 {
		t.Fatalf("issue count = %d, want 1", len(issues))
	}
	if issues[0].Component != "transport[0]" {
		t.Fatalf("component = %q, want transport[0]", issues[0].Component)
	}
	if issues[0].Severity != PreflightSeverityError {
		t.Fatalf("severity = %q, want error", issues[0].Severity)
	}
}

func TestExecutionEnvironmentPreflightAggregatesMultipleComponents(t *testing.T) {
	env := ExecutionEnvironment{
		Transports: []Transport{
			&preflightTransport{issues: []PreflightIssue{{Component: "control transport", Severity: PreflightSeverityWarning, Code: "slow_connect"}}},
			&preflightTransport{issues: []PreflightIssue{{Severity: PreflightSeverityError, Code: "missing_route"}}},
		},
		Scheduler: preflightScheduler{issues: []PreflightIssue{{Severity: PreflightSeverityError, Code: "missing_sbatch"}}},
		Runtime:   preflightRuntime{issues: []PreflightIssue{{Severity: PreflightSeverityWarning, Code: "artifact_missing"}}},
	}

	issues := env.Preflight(context.Background())
	if len(issues) != 4 {
		t.Fatalf("issue count = %d, want 4", len(issues))
	}
	if issues[0].Component != "control transport" {
		t.Fatalf("component[0] = %q, want preserved component", issues[0].Component)
	}
	if issues[1].Component != "transport[1]" {
		t.Fatalf("component[1] = %q, want transport[1]", issues[1].Component)
	}
	if issues[2].Component != "scheduler" {
		t.Fatalf("component[2] = %q, want scheduler", issues[2].Component)
	}
	if issues[3].Component != "runtime" {
		t.Fatalf("component[3] = %q, want runtime", issues[3].Component)
	}
}

func TestExecutionEnvironmentPreflightPassesPrimaryTransportAndDialectToRuntime(t *testing.T) {
	transport := LocalTransport{}
	runtime := &contextAwarePreflightRuntime{}
	env := ExecutionEnvironment{
		Transports: []Transport{transport},
		Dialect:    BashShellPlatform{},
		Runtime:    runtime,
	}

	issues := env.Preflight(context.Background())
	if len(issues) != 1 {
		t.Fatalf("issue count = %d, want 1", len(issues))
	}
	if issues[0].Component != "runtime" {
		t.Fatalf("component = %q, want runtime", issues[0].Component)
	}
	if _, ok := runtime.transport.(LocalTransport); !ok {
		t.Fatalf("runtime transport = %T, want LocalTransport", runtime.transport)
	}
	if _, ok := runtime.dialect.(BashShellPlatform); !ok {
		t.Fatalf("runtime dialect = %T, want BashShellPlatform", runtime.dialect)
	}
}

func TestBlockingPreflightIssuesExcludesWarnings(t *testing.T) {
	issues := []PreflightIssue{
		{Severity: PreflightSeverityWarning, Code: "slow_connect"},
		{Severity: PreflightSeverityError, Code: "missing_sbatch"},
	}

	blocking := blockingPreflightIssues(issues)
	if len(blocking) != 1 {
		t.Fatalf("blocking count = %d, want 1", len(blocking))
	}
	if blocking[0].Code != "missing_sbatch" {
		t.Fatalf("blocking code = %q, want missing_sbatch", blocking[0].Code)
	}
}

func TestPreflightIssueJSONShape(t *testing.T) {
	issue := PreflightIssue{
		Component:   "transport[0]",
		Type:        "ssh",
		Severity:    PreflightSeverityError,
		Code:        "ssh_unknown_host_key",
		Message:     "Host is not trusted.",
		Remediation: "Configure a pinned host key.",
	}

	data, err := json.Marshal(issue)
	if err != nil {
		t.Fatalf("marshal preflight issue: %v", err)
	}
	got := string(data)
	for _, want := range []string{
		`"component":"transport[0]"`,
		`"type":"ssh"`,
		`"severity":"error"`,
		`"code":"ssh_unknown_host_key"`,
		`"message":"Host is not trusted."`,
		`"remediation":"Configure a pinned host key."`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("json = %s, want field %s", got, want)
		}
	}
}

func TestExecutionEnvironmentPrepareRemainsDistinctFromPreflight(t *testing.T) {
	transport := &preflightTransport{
		prepareTransport: prepareTransport{err: fmt.Errorf("prepare failed")},
		issues:           []PreflightIssue{{Severity: PreflightSeverityError, Code: "preflight_failed"}},
	}
	env := ExecutionEnvironment{
		Transports: []Transport{transport},
	}

	issues := env.Preflight(context.Background())
	if len(issues) != 1 {
		t.Fatalf("issue count = %d, want 1", len(issues))
	}

	err := env.Prepare(context.Background())
	if err == nil {
		t.Fatal("expected prepare error")
	}
	if !strings.Contains(err.Error(), "prepare transport[0]") {
		t.Fatalf("error = %v, want prepare context", err)
	}
}

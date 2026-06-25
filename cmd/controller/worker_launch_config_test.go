package main

import (
	"encoding/json"
	"os"
	"testing"

	"goetl/internal/variable"
)

func TestWorkerLaunchConfigResolvesStructuredWorkerConfig(t *testing.T) {
	cfg, err := workerLaunchConfig(testControllerResolver(t,
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "transport"},
			Type:       variable.TypeObject,
			Expression: `{"type":"docker","settings":{"executable":"docker","container":"slurmctld"}}`,
		},
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "scheduler"},
			Type:       variable.TypeObject,
			Expression: `{"type":"slurm","settings":{"script_path":"/tmp/goetl-worker.slurm","job_name":"goetl-worker"}}`,
		},
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "runtime"},
			Type:       variable.TypeObject,
			Expression: `{"type":"worker","settings":{"executable":"/opt/goetl/worker","args":["--mode","worker"],"config_path":"/shared/goetl/config/worker.json","log_dir":"/shared/goetl/logs"}}`,
		},
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.scriptPath != "/tmp/goetl-worker.slurm" {
		t.Fatalf("script path = %q, want /tmp/goetl-worker.slurm", cfg.scriptPath)
	}
	if cfg.dockerExecutable != "docker" {
		t.Fatalf("docker executable = %q, want docker", cfg.dockerExecutable)
	}
	if cfg.slurm.WorkerExecutable != "/opt/goetl/worker" {
		t.Fatalf("worker executable = %q, want /opt/goetl/worker", cfg.slurm.WorkerExecutable)
	}
	if len(cfg.slurm.WorkerArgs) != 2 || cfg.slurm.WorkerArgs[0] != "--mode" || cfg.slurm.WorkerArgs[1] != "worker" {
		t.Fatalf("worker args = %#v, want mode args", cfg.slurm.WorkerArgs)
	}
}

func TestWorkerLaunchConfigSupportsFlatWorkerConfig(t *testing.T) {
	cfg, err := workerLaunchConfig(testControllerResolver(t,
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "worker_script_path"},
			Type:       variable.TypePath,
			Expression: "/tmp/goetl-worker.slurm",
		},
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "worker_start_executable"},
			Type:       variable.TypeString,
			Expression: "/opt/goetl/worker",
		},
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "worker_config_path"},
			Type:       variable.TypePath,
			Expression: "/shared/goetl/config/worker.json",
		},
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "worker_log_dir"},
			Type:       variable.TypePath,
			Expression: "/shared/goetl/logs",
		},
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.scriptPath != "/tmp/goetl-worker.slurm" {
		t.Fatalf("script path = %q, want /tmp/goetl-worker.slurm", cfg.scriptPath)
	}
}

func TestWorkerLaunchConfigSupportsLegacyScriptPath(t *testing.T) {
	cfg, err := workerLaunchConfig(testControllerResolver(t,
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "docker_slurm_script_path"},
			Type:       variable.TypePath,
			Expression: "/tmp/legacy-goetl-worker.slurm",
		},
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "worker_start_executable"},
			Type:       variable.TypeString,
			Expression: "/opt/goetl/worker",
		},
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "worker_config_path"},
			Type:       variable.TypePath,
			Expression: "/shared/goetl/config/worker.json",
		},
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "worker_log_dir"},
			Type:       variable.TypePath,
			Expression: "/shared/goetl/logs",
		},
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.scriptPath != "/tmp/legacy-goetl-worker.slurm" {
		t.Fatalf("script path = %q, want legacy path", cfg.scriptPath)
	}
}

func TestWorkerLaunchConfigRejectsMissingScriptPath(t *testing.T) {
	_, err := workerLaunchConfig(testControllerResolver(t,
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "worker_start_executable"},
			Type:       variable.TypeString,
			Expression: "/opt/goetl/worker",
		},
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "worker_config_path"},
			Type:       variable.TypePath,
			Expression: "/shared/goetl/config/worker.json",
		},
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "worker_log_dir"},
			Type:       variable.TypePath,
			Expression: "/shared/goetl/logs",
		},
	))
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestDockerSlurmWorkflowFixtureResolvesWorkerConfig(t *testing.T) {
	data, err := os.ReadFile("../../demo-docker-slurm-workflow.json")
	if err != nil {
		t.Fatal(err)
	}

	var submission WorkflowSubmission
	if err := json.Unmarshal(data, &submission); err != nil {
		t.Fatal(err)
	}

	scope, err := variable.NewScope(submission.Variables...)
	if err != nil {
		t.Fatal(err)
	}
	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})

	target, err := workerTargetEnvironment(resolver)
	if err != nil {
		t.Fatal(err)
	}
	if target != "docker_slurm" {
		t.Fatalf("target = %q, want docker_slurm", target)
	}

	cfg, err := workerLaunchConfig(resolver)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.scriptPath != "/data/goetl/scripts/worker.slurm" {
		t.Fatalf("script path = %q, want fixture path", cfg.scriptPath)
	}
	if cfg.slurm.WorkerExecutable != "/data/goetl/artifacts/goetl-worker" {
		t.Fatalf("worker executable = %q, want fixture executable", cfg.slurm.WorkerExecutable)
	}
	if cfg.slurm.WorkerConfigPath != "/data/goetl/config/worker.json" {
		t.Fatalf("worker config path = %q, want fixture config path", cfg.slurm.WorkerConfigPath)
	}
	if cfg.slurm.LogDir != "/data/goetl/logs" {
		t.Fatalf("log dir = %q, want fixture log dir", cfg.slurm.LogDir)
	}
}

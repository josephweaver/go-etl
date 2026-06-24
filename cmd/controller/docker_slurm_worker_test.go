package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"goetl/internal/variable"
)

func TestDockerSlurmWorkerStarterBuildsAndSubmitsScript(t *testing.T) {
	var submitted DockerSlurmScriptConfig
	starter := DockerSlurmWorkerStarter{
		Submit: func(ctx context.Context, cfg DockerSlurmScriptConfig) (string, error) {
			submitted = cfg
			return "42", nil
		},
	}

	err := starter.StartWorker("docker_slurm", testControllerResolver(t,
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "docker_slurm_script_path"},
			Type:       variable.TypePath,
			Expression: "/tmp/goetl-worker.slurm",
		},
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "worker_start_executable"},
			Type:       variable.TypeString,
			Expression: "/opt/goetl/worker",
		},
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "worker_start_args"},
			Type:       variable.TypeList(variable.TypeString),
			Expression: `["--mode", "worker"]`,
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

	if submitted.ScriptPath != "/tmp/goetl-worker.slurm" {
		t.Fatalf("script path = %q, want /tmp/goetl-worker.slurm", submitted.ScriptPath)
	}
	if !strings.Contains(submitted.Script, "#SBATCH --job-name=goetl-worker") {
		t.Fatalf("script missing default job name:\n%s", submitted.Script)
	}
	if !strings.Contains(submitted.Script, "'/opt/goetl/worker' '--mode' 'worker' '/shared/goetl/config/worker.json'") {
		t.Fatalf("script missing worker command:\n%s", submitted.Script)
	}
}

func TestDockerSlurmWorkerStarterRejectsMissingScriptPath(t *testing.T) {
	starter := DockerSlurmWorkerStarter{
		Submit: func(ctx context.Context, cfg DockerSlurmScriptConfig) (string, error) {
			t.Fatal("submit should not be called")
			return "", nil
		},
	}

	err := starter.StartWorker("docker_slurm", testControllerResolver(t,
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

func TestDefaultWorkerStarterRoutesDockerSlurmTarget(t *testing.T) {
	var submitted DockerSlurmScriptConfig
	starter := DefaultWorkerStarter{
		DockerSlurm: DockerSlurmWorkerStarter{
			Submit: func(ctx context.Context, cfg DockerSlurmScriptConfig) (string, error) {
				submitted = cfg
				return "42", nil
			},
		},
	}

	err := starter.StartWorker("docker_slurm", testControllerResolver(t,
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "docker_slurm_script_path"},
			Type:       variable.TypePath,
			Expression: "/tmp/goetl-worker.slurm",
		},
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "worker_start_executable"},
			Type:       variable.TypeString,
			Expression: "/bin/echo",
		},
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "worker_config_path"},
			Type:       variable.TypeString,
			Expression: "worker-config",
		},
		variable.Variable{
			Name:       variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "worker_log_dir"},
			Type:       variable.TypePath,
			Expression: "/tmp/goetl-logs",
		},
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if submitted.ScriptPath != "/tmp/goetl-worker.slurm" {
		t.Fatalf("script path = %q, want /tmp/goetl-worker.slurm", submitted.ScriptPath)
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

	cfg, err := dockerSlurmWorkerScriptConfig(resolver)
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

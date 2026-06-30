package main

import (
	"encoding/json"
	"os"
	"testing"

	"goetl/internal/variable"
)

func TestWorkerLaunchConfigResolvesStructuredWorkerConfig(t *testing.T) {
	cfg, err := workerLaunchConfig(testControllerResolver(t,
		testWorkerConfigVariable("transport", "docker", map[string]variable.TypedExpression{
			"executable": {Type: variable.TypeString, Expression: "docker"},
			"container":  {Type: variable.TypeString, Expression: "slurmctld"},
		}),
		testWorkerConfigVariable("scheduler", "slurm", map[string]variable.TypedExpression{
			"script_path": {Type: variable.TypePath, Expression: "/tmp/goetl-worker.slurm"},
			"job_name":    {Type: variable.TypeString, Expression: "goetl-worker"},
		}),
		testWorkerConfigVariable("runtime", "worker", map[string]variable.TypedExpression{
			"executable":  {Type: variable.TypePath, Expression: "/opt/goetl/worker"},
			"args":        {Type: variable.TypeList, Expression: []variable.TypedExpression{{Type: variable.TypeString, Expression: "--mode"}, {Type: variable.TypeString, Expression: "worker"}}},
			"config_path": {Type: variable.TypePath, Expression: "/shared/goetl/config/worker.json"},
			"log_dir":     {Type: variable.TypePath, Expression: "/shared/goetl/logs"},
		}),
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
		variable.Variable{Name: variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "worker_script_path"}, TypedExpression: variable.TypedExpression{Type: variable.TypePath, Expression: "/tmp/goetl-worker.slurm"}},

		variable.Variable{Name: variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "worker_start_executable"}, TypedExpression: variable.TypedExpression{Type: variable.TypeString, Expression: "/opt/goetl/worker"}},

		variable.Variable{Name: variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "worker_config_path"}, TypedExpression: variable.TypedExpression{Type: variable.TypePath, Expression: "/shared/goetl/config/worker.json"}},

		variable.Variable{Name: variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "worker_log_dir"}, TypedExpression: variable.TypedExpression{Type: variable.TypePath, Expression: "/shared/goetl/logs"}},
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
		variable.Variable{Name: variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "docker_slurm_script_path"}, TypedExpression: variable.TypedExpression{Type: variable.TypePath, Expression: "/tmp/legacy-goetl-worker.slurm"}},

		variable.Variable{Name: variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "worker_start_executable"}, TypedExpression: variable.TypedExpression{Type: variable.TypeString, Expression: "/opt/goetl/worker"}},

		variable.Variable{Name: variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "worker_config_path"}, TypedExpression: variable.TypedExpression{Type: variable.TypePath, Expression: "/shared/goetl/config/worker.json"}},

		variable.Variable{Name: variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "worker_log_dir"}, TypedExpression: variable.TypedExpression{Type: variable.TypePath, Expression: "/shared/goetl/logs"}},
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
		variable.Variable{Name: variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "worker_start_executable"}, TypedExpression: variable.TypedExpression{Type: variable.TypeString, Expression: "/opt/goetl/worker"}},

		variable.Variable{Name: variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "worker_config_path"}, TypedExpression: variable.TypedExpression{Type: variable.TypePath, Expression: "/shared/goetl/config/worker.json"}},

		variable.Variable{Name: variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: "worker_log_dir"}, TypedExpression: variable.TypedExpression{Type: variable.TypePath, Expression: "/shared/goetl/logs"}},
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

func testWorkerConfigVariable(key string, kind string, settings map[string]variable.TypedExpression) variable.Variable {
	return variable.Variable{
		Name: variable.Name{Namespace: variable.NamespaceWorkerConfig, Key: key},
		TypedExpression: variable.TypedExpression{
			Type: variable.TypeObject,
			Expression: map[string]variable.TypedExpression{
				"type": {Type: variable.TypeString, Expression: kind},
				"settings": {
					Type:       variable.TypeObject,
					Expression: settings,
				},
			},
		},
	}
}

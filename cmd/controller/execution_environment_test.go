package main

import (
	"context"
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
				Settings: map[string]string{
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
			Settings: map[string]string{
				"container":  "slurmctld",
				"executable": "podman",
			},
		}},
		Dialect:   ExecutionComponentConfig{Type: "bash"},
		Scheduler: ExecutionComponentConfig{Type: "slurm"},
		Runtime:   ExecutionComponentConfig{Type: "worker", Settings: map[string]string{"root": "/data/goetl"}},
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

func TestNewExecutionEnvironmentSupportsLocalDirectProcess(t *testing.T) {
	env, err := NewExecutionEnvironment(ExecutionEnvironmentConfig{
		Name:       "local-direct",
		Transports: []ExecutionComponentConfig{{Type: "local"}},
		Dialect:    ExecutionComponentConfig{Type: "bash"},
		Scheduler:  ExecutionComponentConfig{Type: "direct_process"},
		Runtime:    ExecutionComponentConfig{Type: "worker", Settings: map[string]string{"root": "/tmp/goetl"}},
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
}

func TestNewExecutionEnvironmentSupportsSingularityWorkerRuntime(t *testing.T) {
	env, err := NewExecutionEnvironment(ExecutionEnvironmentConfig{
		Name:       "local-singularity",
		Transports: []ExecutionComponentConfig{{Type: "local"}},
		Dialect:    ExecutionComponentConfig{Type: "bash"},
		Scheduler:  ExecutionComponentConfig{Type: "direct_process"},
		Runtime: ExecutionComponentConfig{
			Type: "singularity_worker",
			Settings: map[string]string{
				"root":                        "/tmp/goetl",
				"controller_url":              "http://localhost:8080",
				"image_path":                  "/tmp/goetl/images/goetl-worker.sif",
				"container_worker_executable": "/goetl/goetl-worker",
				"singularity_executable":      "singularity",
				"bind":                        "/tmp/goetl:/data/goetl",
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
}

func TestNewExecutionEnvironmentRejectsUnsupportedComponentType(t *testing.T) {
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

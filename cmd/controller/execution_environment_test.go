package main

import "testing"

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
		Runtime:   ExecutionComponentConfig{Type: "shared_filesystem_worker"},
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
		Runtime:   ExecutionComponentConfig{Type: "shared_filesystem_worker"},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected an error")
	}
}

func TestNewExecutionEnvironmentStoresValidatedConfig(t *testing.T) {
	env, err := NewExecutionEnvironment(ExecutionEnvironmentConfig{
		Name:       "dockerized-slurm",
		Transports: []ExecutionComponentConfig{{
			Type: "docker",
			Settings: map[string]string{
				"container":  "slurmctld",
				"executable": "podman",
			},
		}},
		Dialect:    ExecutionComponentConfig{Type: "bash"},
		Scheduler:  ExecutionComponentConfig{Type: "slurm"},
		Runtime:    ExecutionComponentConfig{Type: "shared_filesystem_worker", Settings: map[string]string{"root": "/data/goetl"}},
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
	runtime, ok := env.Runtime.(SharedFilesystemWorkerRuntime)
	if !ok {
		t.Fatalf("runtime type = %T, want SharedFilesystemWorkerRuntime", env.Runtime)
	}
	if runtime.Root != "/data/goetl" {
		t.Fatalf("runtime root = %q, want /data/goetl", runtime.Root)
	}
}

func TestNewExecutionEnvironmentRejectsUnsupportedComponentType(t *testing.T) {
	_, err := NewExecutionEnvironment(ExecutionEnvironmentConfig{
		Name:       "bad-env",
		Transports: []ExecutionComponentConfig{{Type: "ssh"}},
		Dialect:    ExecutionComponentConfig{Type: "bash"},
		Scheduler:  ExecutionComponentConfig{Type: "slurm"},
		Runtime:    ExecutionComponentConfig{Type: "shared_filesystem_worker"},
	})
	if err == nil {
		t.Fatal("expected an error")
	}
}

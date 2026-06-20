package main

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"
)

func TestDefaultDockerSlurmRuntimePathsUseSharedDataRoot(t *testing.T) {
	paths := DefaultDockerSlurmRuntimePaths()

	if paths.Root != "/data/goetl" {
		t.Fatalf("root = %q, want /data/goetl", paths.Root)
	}
	if paths.WorkerExecutable != "/data/goetl/artifacts/goetl-worker" {
		t.Fatalf("worker executable = %q, want shared artifact path", paths.WorkerExecutable)
	}
	if paths.WorkerConfigPath != "/data/goetl/config/worker.json" {
		t.Fatalf("worker config = %q, want shared config path", paths.WorkerConfigPath)
	}
	if paths.WorkerScriptPath != "/data/goetl/scripts/worker.slurm" {
		t.Fatalf("worker script = %q, want shared script path", paths.WorkerScriptPath)
	}
}

func TestGenerateDockerSlurmWorkerConfig(t *testing.T) {
	paths := DefaultDockerSlurmRuntimePaths()

	text, err := GenerateDockerSlurmWorkerConfig("http://host.docker.internal:8080", paths)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var cfg DockerSlurmWorkerConfig
	if err := json.Unmarshal([]byte(text), &cfg); err != nil {
		t.Fatalf("decode generated config: %v", err)
	}
	if cfg.ControllerURL != "http://host.docker.internal:8080" {
		t.Fatalf("controller url = %q, want submitted URL", cfg.ControllerURL)
	}
	if cfg.LogDir != "/data/goetl/logs" || cfg.TmpDir != "/data/goetl/tmp" || cfg.DataDir != "/data/goetl/data" {
		t.Fatalf("unexpected runtime dirs: %+v", cfg)
	}
}

func TestGenerateDockerSlurmWorkerConfigRejectsMissingControllerURL(t *testing.T) {
	if _, err := GenerateDockerSlurmWorkerConfig("", DefaultDockerSlurmRuntimePaths()); err == nil {
		t.Fatal("expected an error")
	}
}

func TestDockerSlurmMkdirCommandUsesDefaults(t *testing.T) {
	executable, args, err := dockerSlurmMkdirCommand(DockerSlurmRuntimeConfig{}, DefaultDockerSlurmRuntimePaths())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if executable != "docker" {
		t.Fatalf("executable = %q, want docker", executable)
	}
	want := []string{
		"exec", "slurmctld", "mkdir", "-p",
		"/data/goetl/artifacts",
		"/data/goetl/config",
		"/data/goetl/scripts",
		"/data/goetl/logs",
		"/data/goetl/tmp",
		"/data/goetl/data",
	}
	if !stringSlicesEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestPrepareDockerSlurmRuntimeIntegration(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is required for Dockerized Slurm integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), dockerSlurmIntegrationTimeout)
	defer cancel()

	if err := dockerExec(ctx, "slurmctld", "test", "-d", "/data"); err != nil {
		t.Skipf("slurmctld container with /data is required: %v", err)
	}

	err := PrepareDockerSlurmRuntime(ctx, DockerSlurmRuntimeConfig{
		ControllerURL: "http://host.docker.internal:8080",
		Paths:         DockerSlurmRuntimePathsForRoot("/data/goetl-test-runtime"),
	})
	if err != nil {
		t.Fatalf("prepare runtime: %v", err)
	}

	if err := dockerExec(ctx, "slurmctld", "test", "-f", "/data/goetl-test-runtime/config/worker.json"); err != nil {
		t.Fatalf("worker config was not written: %v", err)
	}
	if err := dockerExec(ctx, "slurm-cpu-worker-1", "test", "-d", "/data/goetl-test-runtime/logs"); err != nil {
		t.Fatalf("runtime logs dir is not visible on worker: %v", err)
	}
}

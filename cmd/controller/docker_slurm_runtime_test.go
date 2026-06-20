package main

import (
	"encoding/json"
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

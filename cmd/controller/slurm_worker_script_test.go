package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateSlurmWorkerScript(t *testing.T) {
	script, err := GenerateSlurmWorkerScript(SlurmWorkerScriptConfig{
		JobName:          "goetl-worker",
		WorkerExecutable: "/fake-hpcc/shared/goetl/artifacts/goetl-worker",
		WorkerConfigPath: "/fake-hpcc/shared/goetl/configs/worker.json",
		LogDir:           "/fake-hpcc/shared/goetl/logs",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{
		"#!/usr/bin/env bash\n",
		"#SBATCH --job-name=goetl-worker\n",
		"#SBATCH --output=/fake-hpcc/shared/goetl/logs/%x-%j.out\n",
		"#SBATCH --error=/fake-hpcc/shared/goetl/logs/%x-%j.err\n",
		"set -euo pipefail\n",
		"mkdir -p '/fake-hpcc/shared/goetl/logs'\n",
		"'/fake-hpcc/shared/goetl/artifacts/goetl-worker' '/fake-hpcc/shared/goetl/configs/worker.json'\n",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q:\n%s", want, script)
		}
	}
}

func TestGenerateSlurmWorkerScriptQuotesShellValues(t *testing.T) {
	script, err := GenerateSlurmWorkerScript(SlurmWorkerScriptConfig{
		JobName:          "goetl-worker",
		WorkerExecutable: "/fake path/goetl-worker",
		WorkerConfigPath: "/fake path/worker's config.json",
		LogDir:           "/fake path/logs",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "'/fake path/goetl-worker' '/fake path/worker'\"'\"'s config.json'"
	if !strings.Contains(script, want) {
		t.Fatalf("script missing quoted command %q:\n%s", want, script)
	}
}

func TestGenerateSlurmWorkerScriptRejectsInvalidConfig(t *testing.T) {
	_, err := GenerateSlurmWorkerScript(SlurmWorkerScriptConfig{
		JobName:          "goetl worker",
		WorkerExecutable: "/fake/worker",
		WorkerConfigPath: "/fake/config.json",
		LogDir:           "/fake/logs",
	})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestWriteSlurmWorkerScript(t *testing.T) {
	scriptPath := filepath.Join(".run", "fake-hpcc", "worker.slurm")
	if err := os.RemoveAll(filepath.Join(".run", "fake-hpcc")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(filepath.Join(".run", "fake-hpcc"))
	})

	err := WriteSlurmWorkerScript(scriptPath, SlurmWorkerScriptConfig{
		JobName:          "goetl-worker",
		WorkerExecutable: "/bin/echo",
		WorkerConfigPath: "fake-worker-config",
		LogDir:           filepath.ToSlash(filepath.Join(".run", "fake-hpcc", "logs")),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(script), "'/bin/echo' 'fake-worker-config'") {
		t.Fatalf("script missing worker command:\n%s", script)
	}
}

func TestGeneratedSlurmWorkerScriptRunsThroughFakeSbatch(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is required for fake sbatch integration test")
	}

	testRoot := filepath.Join(".run", "slurm-worker-script-test")
	if err := os.RemoveAll(testRoot); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(testRoot)
	})
	if err := os.MkdirAll(testRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	script, err := GenerateSlurmWorkerScript(SlurmWorkerScriptConfig{
		JobName:          "goetl-worker",
		WorkerExecutable: "/bin/echo",
		WorkerConfigPath: "fake-worker-config",
		LogDir:           filepath.ToSlash(filepath.Join(testRoot, "logs")),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	scriptPath := filepath.ToSlash(filepath.Join(testRoot, "worker.slurm"))
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}

	fakeRunRoot := filepath.ToSlash(filepath.Join(testRoot, "state"))
	command := "export FAKE_SLURM_RUN_ROOT=" + shellQuote(fakeRunRoot) + "; " +
		"export FAKE_SLURM_FOREGROUND=1; " +
		"bash ../../scripts/fake-hpcc/sbatch " + shellQuote(scriptPath)
	output, err := exec.Command("bash", "-lc", command).CombinedOutput()
	if err != nil {
		t.Fatalf("fake sbatch failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "Submitted batch job 1000") {
		t.Fatalf("unexpected fake sbatch output: %s", output)
	}

	jobOutput, err := os.ReadFile(filepath.Join(testRoot, "state", "job-1000.out"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(jobOutput)) != "fake-worker-config" {
		t.Fatalf("unexpected job output: %q", jobOutput)
	}
}

package main

import (
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

package main

import (
	"context"
	"os"
	"strings"
	"testing"
)

type fakeSlurmConnector struct {
	copiedLocalPath  string
	copiedRemotePath string
	execArgs         []string
	output           []byte
}

func (c *fakeSlurmConnector) Copy(ctx context.Context, localPath string, remotePath string) error {
	c.copiedLocalPath = localPath
	c.copiedRemotePath = remotePath
	return nil
}

func (c *fakeSlurmConnector) Exec(ctx context.Context, args ...string) ([]byte, error) {
	c.execArgs = append([]string(nil), args...)
	return c.output, nil
}

func TestSlurmSchedulerExecuteCopiesAndSubmitsGeneratedScript(t *testing.T) {
	connector := &fakeSlurmConnector{output: []byte("Submitted batch job 42\n")}
	scheduler := SlurmScheduler{
		Transport: connector,
		TempDir:   t.TempDir(),
	}

	jobID, err := scheduler.Execute(context.Background(), SlurmExecutionConfig{
		RemoteScriptPath: "/data/goetl/scripts/worker.slurm",
		WorkerScript: SlurmWorkerScriptConfig{
			JobName:          "goetl-worker",
			WorkerExecutable: "/data/goetl/artifacts/goetl-worker",
			WorkerConfigPath: "/data/goetl/config/worker.json",
			LogDir:           "/data/goetl/logs",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if jobID != "42" {
		t.Fatalf("job id = %q, want 42", jobID)
	}
	if connector.copiedRemotePath != "/data/goetl/scripts/worker.slurm" {
		t.Fatalf("remote script path = %q, want configured path", connector.copiedRemotePath)
	}
	if !stringSlicesEqual(connector.execArgs, []string{"sbatch", "/data/goetl/scripts/worker.slurm"}) {
		t.Fatalf("exec args = %#v, want sbatch command", connector.execArgs)
	}
	if _, err := os.Stat(connector.copiedLocalPath); !os.IsNotExist(err) {
		t.Fatalf("temp script still exists or stat failed unexpectedly: %v", err)
	}
}

func TestSlurmSchedulerWriteTempScriptWritesScriptContent(t *testing.T) {
	scheduler := SlurmScheduler{TempDir: t.TempDir()}

	path, err := scheduler.writeTempScript("#!/usr/bin/env bash\nhostname\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "hostname") {
		t.Fatalf("script missing content: %s", data)
	}
}

func TestSlurmSchedulerExecuteRejectsMissingConnector(t *testing.T) {
	_, err := (SlurmScheduler{}).Execute(context.Background(), SlurmExecutionConfig{
		RemoteScriptPath: "/data/goetl/scripts/worker.slurm",
		WorkerScript: SlurmWorkerScriptConfig{
			JobName:          "goetl-worker",
			WorkerExecutable: "/bin/echo",
			WorkerConfigPath: "worker-config",
			LogDir:           "/tmp/logs",
		},
	})
	if err == nil {
		t.Fatal("expected an error")
	}
}

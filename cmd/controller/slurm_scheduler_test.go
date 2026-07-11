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
	copiedContent    []byte
	execArgs         []string
	output           []byte
}

func (c *fakeSlurmConnector) Copy(ctx context.Context, localPath string, remotePath string) error {
	c.copiedLocalPath = localPath
	c.copiedRemotePath = remotePath
	data, err := os.ReadFile(localPath)
	if err == nil {
		c.copiedContent = data
	}
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
		MemoryMB:  8192,
		TimeLimit: "01:00:00",
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
	if !strings.Contains(string(connector.copiedContent), "#SBATCH --mem=8192M") {
		t.Fatalf("copied script missing memory directive:\n%s", connector.copiedContent)
	}
	if !strings.Contains(string(connector.copiedContent), "#SBATCH --time=01:00:00") {
		t.Fatalf("copied script missing time directive:\n%s", connector.copiedContent)
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

func TestRemoteProcessSchedulerCopiesAndStartsGeneratedScript(t *testing.T) {
	connector := &fakeSlurmConnector{output: []byte("12345\n")}
	scheduler := RemoteProcessScheduler{
		Transport: connector,
		TempDir:   t.TempDir(),
	}

	pid, err := scheduler.Execute(context.Background(), JobSpec{
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

	if pid != "12345" {
		t.Fatalf("pid = %q, want 12345", pid)
	}
	if connector.copiedRemotePath != "/data/goetl/scripts/worker.slurm" {
		t.Fatalf("remote script path = %q, want configured path", connector.copiedRemotePath)
	}
	wantCommand := "mkdir -p '/data/goetl/logs' && nohup bash '/data/goetl/scripts/worker.slurm' > '/data/goetl/logs/remote-process.out' 2> '/data/goetl/logs/remote-process.err' < /dev/null & echo $!"
	if !stringSlicesEqual(connector.execArgs, []string{"sh", "-c", wantCommand}) {
		t.Fatalf("exec args = %#v, want remote process command", connector.execArgs)
	}
	if !strings.Contains(string(connector.copiedContent), "/data/goetl/artifacts/goetl-worker") {
		t.Fatalf("copied script missing worker executable:\n%s", connector.copiedContent)
	}
	if _, err := os.Stat(connector.copiedLocalPath); !os.IsNotExist(err) {
		t.Fatalf("temp script still exists or stat failed unexpectedly: %v", err)
	}
}

func TestRemoteProcessSchedulerRejectsInvalidPID(t *testing.T) {
	connector := &fakeSlurmConnector{output: []byte("not-a-pid\n")}
	scheduler := RemoteProcessScheduler{
		Transport: connector,
		TempDir:   t.TempDir(),
	}

	_, err := scheduler.Execute(context.Background(), JobSpec{
		RemoteScriptPath: "/data/goetl/scripts/worker.slurm",
		WorkerScript: SlurmWorkerScriptConfig{
			JobName:          "goetl-worker",
			WorkerExecutable: "/data/goetl/artifacts/goetl-worker",
			WorkerConfigPath: "/data/goetl/config/worker.json",
			LogDir:           "/data/goetl/logs",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid pid") {
		t.Fatalf("error = %v, want invalid pid", err)
	}
}

func TestSSHReverseCallbackTunnelSlurmPreflightSubmitsWaitScript(t *testing.T) {
	connector := &fakeSlurmConnector{output: []byte("Submitted batch job 77\n")}
	tunnel := &SSHReverseCallbackTunnel{
		Config: CallbackTunnelConfig{
			WorkerControllerURL: "http://hpcc.example.edu:18080",
		},
		scheduler:                SlurmScheduler{Transport: connector},
		slurmPreflightScriptPath: "/data/goetl/scripts/callback-tunnel-preflight.slurm",
	}

	if err := tunnel.checkSlurmWorkerControllerURL(context.Background()); err != nil {
		t.Fatalf("unexpected preflight error: %v", err)
	}

	if connector.copiedRemotePath != "/data/goetl/scripts/callback-tunnel-preflight.slurm" {
		t.Fatalf("remote script path = %q", connector.copiedRemotePath)
	}
	if !stringSlicesEqual(connector.execArgs, []string{"sbatch", "--wait", "/data/goetl/scripts/callback-tunnel-preflight.slurm"}) {
		t.Fatalf("exec args = %#v, want sbatch --wait", connector.execArgs)
	}
	script := string(connector.copiedContent)
	for _, want := range []string{
		"#SBATCH --job-name=goetl-callback-preflight",
		"curl --fail --silent --show-error --max-time 10",
		"'http://hpcc.example.edu:18080/healthz'",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q:\n%s", want, script)
		}
	}
	if _, err := os.Stat(connector.copiedLocalPath); !os.IsNotExist(err) {
		t.Fatalf("temp preflight script still exists or stat failed unexpectedly: %v", err)
	}
}

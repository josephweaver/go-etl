package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestDockerSlurmSbatchCommandUsesDefaults(t *testing.T) {
	executable, args, err := dockerSlurmSbatchCommand(DockerSlurmSubmitConfig{
		ScriptPath: "/shared/goetl/worker.slurm",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if executable != "docker" {
		t.Fatalf("executable = %q, want docker", executable)
	}
	want := []string{"exec", "slurmctld", "sbatch", "/shared/goetl/worker.slurm"}
	if !stringSlicesEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestDockerSlurmSbatchCommandUsesConfiguredContainer(t *testing.T) {
	executable, args, err := dockerSlurmSbatchCommand(DockerSlurmSubmitConfig{
		DockerExecutable: "podman",
		SlurmContainer:   "test-slurmctld",
		ScriptPath:       "/work/worker.slurm",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if executable != "podman" {
		t.Fatalf("executable = %q, want podman", executable)
	}
	want := []string{"exec", "test-slurmctld", "sbatch", "/work/worker.slurm"}
	if !stringSlicesEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestDockerSlurmSbatchCommandRejectsMissingScript(t *testing.T) {
	if _, _, err := dockerSlurmSbatchCommand(DockerSlurmSubmitConfig{}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestDockerSlurmWriteScriptCommandUsesDefaults(t *testing.T) {
	executable, args, err := dockerSlurmWriteScriptCommand(DockerSlurmScriptConfig{
		ScriptPath: "/tmp/goetl-worker.slurm",
		Script:     "#!/usr/bin/env bash\nhostname\n",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if executable != "docker" {
		t.Fatalf("executable = %q, want docker", executable)
	}
	want := []string{"exec", "-i", "slurmctld", "tee", "/tmp/goetl-worker.slurm"}
	if !stringSlicesEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestDockerSlurmWriteScriptCommandRejectsMissingContent(t *testing.T) {
	if _, _, err := dockerSlurmWriteScriptCommand(DockerSlurmScriptConfig{ScriptPath: "/tmp/goetl-worker.slurm"}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestParseSubmittedSlurmJobID(t *testing.T) {
	jobID, err := parseSubmittedSlurmJobID("Submitted batch job 42\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if jobID != "42" {
		t.Fatalf("job id = %q, want 42", jobID)
	}
}

func TestParseSubmittedSlurmJobIDRejectsUnexpectedOutput(t *testing.T) {
	if _, err := parseSubmittedSlurmJobID("sbatch: error: unable to open file"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestSubmitDockerSlurmScriptIntegration(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is required for Dockerized Slurm integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := dockerExec(ctx, "slurmctld", "test", "-x", "/usr/bin/sbatch"); err != nil {
		t.Skipf("slurmctld container with sbatch is required: %v", err)
	}

	const scriptPath = "/tmp/goetl-integration.slurm"
	script := "#!/usr/bin/env bash\nset -euo pipefail\nhostname\n"
	if err := dockerExec(ctx, "slurmctld", "bash", "-lc", "cat > "+shellQuote(scriptPath)+" <<'EOF'\n"+script+"EOF\n"); err != nil {
		t.Fatalf("write integration script: %v", err)
	}

	jobID, err := SubmitDockerSlurmScript(ctx, DockerSlurmSubmitConfig{
		ScriptPath: scriptPath,
	})
	if err != nil {
		t.Fatalf("submit script: %v", err)
	}
	if jobID == "" {
		t.Fatal("expected a job id")
	}

	state, err := waitDockerSlurmJobState(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if state != "COMPLETED" {
		t.Fatalf("job state = %q, want COMPLETED", state)
	}
}

func TestWriteAndSubmitDockerSlurmScriptIntegration(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is required for Dockerized Slurm integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := dockerExec(ctx, "slurmctld", "test", "-x", "/usr/bin/sbatch"); err != nil {
		t.Skipf("slurmctld container with sbatch is required: %v", err)
	}

	jobID, err := WriteAndSubmitDockerSlurmScript(ctx, DockerSlurmScriptConfig{
		ScriptPath: "/tmp/goetl-write-submit.slurm",
		Script:     "#!/usr/bin/env bash\nset -euo pipefail\nhostname\n",
	})
	if err != nil {
		t.Fatalf("write and submit script: %v", err)
	}
	if jobID == "" {
		t.Fatal("expected a job id")
	}

	state, err := waitDockerSlurmJobState(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if state != "COMPLETED" {
		t.Fatalf("job state = %q, want COMPLETED", state)
	}
}

func TestGeneratedWorkerScriptSubmitsToDockerSlurmIntegration(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is required for Dockerized Slurm integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := dockerExec(ctx, "slurmctld", "test", "-x", "/usr/bin/sbatch"); err != nil {
		t.Skipf("slurmctld container with sbatch is required: %v", err)
	}

	script, err := GenerateSlurmWorkerScript(SlurmWorkerScriptConfig{
		JobName:          "goetl-generated",
		WorkerExecutable: "/bin/echo",
		WorkerConfigPath: "generated-worker-config",
		LogDir:           "/tmp/goetl-generated-logs",
	})
	if err != nil {
		t.Fatalf("generate script: %v", err)
	}

	jobID, err := WriteAndSubmitDockerSlurmScript(ctx, DockerSlurmScriptConfig{
		ScriptPath: "/tmp/goetl-generated-worker.slurm",
		Script:     script,
	})
	if err != nil {
		t.Fatalf("write and submit generated script: %v", err)
	}

	state, err := waitDockerSlurmJobState(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if state != "COMPLETED" {
		t.Fatalf("job state = %q, want COMPLETED", state)
	}
}

func stringSlicesEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func dockerExec(ctx context.Context, container string, args ...string) error {
	commandArgs := append([]string{"exec", container}, args...)
	output, err := exec.CommandContext(ctx, "docker", commandArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func waitDockerSlurmJobState(ctx context.Context, jobID string) (string, error) {
	for {
		state, err := dockerSlurmJobState(ctx, jobID)
		if err != nil {
			return "", err
		}
		if state != "" && state != "PENDING" && state != "RUNNING" && state != "CONFIGURING" {
			return state, nil
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func dockerSlurmJobState(ctx context.Context, jobID string) (string, error) {
	output, err := exec.CommandContext(ctx, "docker", "exec", "slurmctld", "sacct", "-j", jobID, "--format=State", "--noheader", "--parsable2").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("query slurm job state: %w: %s", err, strings.TrimSpace(string(output)))
	}

	for _, line := range strings.Split(string(output), "\n") {
		state := strings.TrimSpace(line)
		if state != "" {
			return strings.Split(state, "|")[0], nil
		}
	}
	return "", nil
}

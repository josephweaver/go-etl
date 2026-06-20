package main

import "testing"

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

package main

import "testing"

func TestDockerTransportExecCommandUsesDefaults(t *testing.T) {
	executable, args, err := (DockerTransport{}).execCommand("slurmctld", "sbatch", "/data/goetl/scripts/worker.slurm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if executable != "docker" {
		t.Fatalf("executable = %q, want docker", executable)
	}
	want := []string{"exec", "slurmctld", "sbatch", "/data/goetl/scripts/worker.slurm"}
	if !stringSlicesEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestDockerTransportExecCommandUsesConfiguredExecutable(t *testing.T) {
	executable, args, err := (DockerTransport{Executable: "podman"}).execCommand("slurmctld", "mkdir", "-p", "/data/goetl")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if executable != "podman" {
		t.Fatalf("executable = %q, want podman", executable)
	}
	want := []string{"exec", "slurmctld", "mkdir", "-p", "/data/goetl"}
	if !stringSlicesEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestDockerTransportCopyToContainerCommand(t *testing.T) {
	executable, args, err := (DockerTransport{}).copyToContainerCommand("./goetl-worker", "slurmctld", "/data/goetl/artifacts/goetl-worker")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if executable != "docker" {
		t.Fatalf("executable = %q, want docker", executable)
	}
	want := []string{"cp", "./goetl-worker", "slurmctld:/data/goetl/artifacts/goetl-worker"}
	if !stringSlicesEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestDockerTransportCommandsRejectNewlines(t *testing.T) {
	if _, _, err := (DockerTransport{}).execCommand("slurmctld\n", "hostname"); err == nil {
		t.Fatal("expected exec command to reject newline")
	}
	if _, _, err := (DockerTransport{}).copyToContainerCommand("./worker", "slurmctld", "/data/goetl\n/worker"); err == nil {
		t.Fatal("expected copy command to reject newline")
	}
}

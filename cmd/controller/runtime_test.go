package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

type recordingTransport struct {
	execArgs []string
	copies   []recordedCopy
}

type recordedCopy struct {
	localPath  string
	remotePath string
	content    []byte
}

func (t *recordingTransport) Copy(ctx context.Context, localPath string, remotePath string) error {
	content, _ := os.ReadFile(localPath)
	t.copies = append(t.copies, recordedCopy{localPath: localPath, remotePath: remotePath, content: content})
	return nil
}

func (t *recordingTransport) Exec(ctx context.Context, args ...string) ([]byte, error) {
	t.execArgs = append([]string(nil), args...)
	return nil, nil
}

func TestSharedFilesystemWorkerRuntimePathsDefaultRoot(t *testing.T) {
	paths, err := (SharedFilesystemWorkerRuntime{}).paths()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if paths.Root != "/data/goetl" {
		t.Fatalf("root = %q, want /data/goetl", paths.Root)
	}
	if paths.WorkerExecutable != "/data/goetl/artifacts/goetl-worker" {
		t.Fatalf("worker executable = %q, want shared artifact path", paths.WorkerExecutable)
	}
	if paths.WorkerConfigPath != "/data/goetl/config/worker.json" {
		t.Fatalf("worker config = %q, want shared config path", paths.WorkerConfigPath)
	}
}

func TestSharedFilesystemWorkerRuntimePrepareCreatesDirectories(t *testing.T) {
	transport := &recordingTransport{}
	runtime := SharedFilesystemWorkerRuntime{Root: "/data/goetl-test"}

	if err := runtime.Prepare(context.Background(), transport, BashShellPlatform{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{
		"mkdir", "-p",
		"/data/goetl-test/artifacts",
		"/data/goetl-test/config",
		"/data/goetl-test/scripts",
		"/data/goetl-test/logs",
		"/data/goetl-test/tmp",
		"/data/goetl-test/data",
	}
	if !stringSlicesEqual(transport.execArgs, want) {
		t.Fatalf("exec args = %#v, want %#v", transport.execArgs, want)
	}
}

func TestSharedFilesystemWorkerRuntimePrepareWritesWorkerConfig(t *testing.T) {
	transport := &recordingTransport{}
	runtime := SharedFilesystemWorkerRuntime{
		Root:          "/data/goetl-test",
		ControllerURL: "http://host.docker.internal:8080",
	}

	if err := runtime.Prepare(context.Background(), transport, BashShellPlatform{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(transport.copies) != 1 {
		t.Fatalf("copy count = %d, want 1", len(transport.copies))
	}
	if transport.copies[0].remotePath != "/data/goetl-test/config/worker.json" {
		t.Fatalf("remote worker config = %q, want config path", transport.copies[0].remotePath)
	}

	var cfg SharedFilesystemWorkerConfig
	if err := json.Unmarshal(transport.copies[0].content, &cfg); err != nil {
		t.Fatalf("decode copied worker config: %v", err)
	}
	if cfg.ControllerURL != "http://host.docker.internal:8080" {
		t.Fatalf("controller url = %q, want configured URL", cfg.ControllerURL)
	}
	if cfg.LogDir != "/data/goetl-test/logs" || cfg.TmpDir != "/data/goetl-test/tmp" || cfg.DataDir != "/data/goetl-test/data" {
		t.Fatalf("unexpected runtime dirs: %+v", cfg)
	}
	if _, err := os.Stat(transport.copies[0].localPath); !os.IsNotExist(err) {
		t.Fatalf("temp worker config still exists or stat failed unexpectedly: %v", err)
	}
}

func TestSharedFilesystemWorkerRuntimePrepareUploadsArtifact(t *testing.T) {
	transport := &recordingTransport{}
	runtime := SharedFilesystemWorkerRuntime{
		Root:                "/data/goetl-test",
		LocalWorkerArtifact: "goetl-worker.exe",
	}

	if err := runtime.Prepare(context.Background(), transport, BashShellPlatform{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(transport.copies) != 1 {
		t.Fatalf("copy count = %d, want 1", len(transport.copies))
	}
	if transport.copies[0].remotePath != "/data/goetl-test/artifacts/goetl-worker" {
		t.Fatalf("remote artifact = %q, want worker executable path", transport.copies[0].remotePath)
	}
	want := []string{"chmod", "0755", "/data/goetl-test/artifacts/goetl-worker"}
	if !stringSlicesEqual(transport.execArgs, want) {
		t.Fatalf("exec args = %#v, want chmod command", transport.execArgs)
	}
}

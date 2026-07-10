package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

func TestWorkerRuntimePathsDefaultRoot(t *testing.T) {
	paths, err := (WorkerRuntime{}).paths()
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

func TestWorkerRuntimePrepareCreatesDirectories(t *testing.T) {
	transport := &recordingTransport{}
	runtime := WorkerRuntime{Root: "/data/goetl-test"}

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

func TestWorkerRuntimePrepareCreatesLocalDirectoriesWithoutShellMkdir(t *testing.T) {
	root := filepath.ToSlash(filepath.Join(t.TempDir(), "runtime"))
	runtime := WorkerRuntime{
		Root:          root,
		AssetCacheDir: filepath.ToSlash(filepath.Join(t.TempDir(), "asset-cache")),
		DataLocationRoots: map[string]string{
			"fixture_data": filepath.ToSlash(filepath.Join(t.TempDir(), "fixture-data")),
		},
	}

	if err := runtime.Prepare(context.Background(), LocalTransport{}, BashShellPlatform{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, dir := range []string{
		filepath.FromSlash(root + "/artifacts"),
		filepath.FromSlash(root + "/config"),
		filepath.FromSlash(root + "/scripts"),
		filepath.FromSlash(root + "/logs"),
		filepath.FromSlash(root + "/tmp"),
		filepath.FromSlash(root + "/data"),
		filepath.FromSlash(runtime.AssetCacheDir),
		filepath.FromSlash(runtime.DataLocationRoots["fixture_data"]),
	} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("runtime dir %s missing or not a dir: info=%v err=%v", dir, info, err)
		}
	}
}

func TestWorkerRuntimePrepareWritesWorkerConfig(t *testing.T) {
	transport := &recordingTransport{}
	runtime := WorkerRuntime{
		Root:                "/data/goetl-test",
		ControllerURL:       "http://host.docker.internal:8080",
		ControllerTokenFile: "/data/goetl-test/secrets/controller-worker-token",
		AssetCacheDir:       "/data/goetl-test/cache/assets",
		PythonExecutable:    "python3",
		MaxAssetBytes:       20000000000,
		DataLocationRoots: map[string]string{
			"fixture_data":   "/data/goetl-test/fixtures",
			"published_data": "/data/goetl-test/published",
		},
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

	var cfg WorkerConfig
	if err := json.Unmarshal(transport.copies[0].content, &cfg); err != nil {
		t.Fatalf("decode copied worker config: %v", err)
	}
	if cfg.ControllerURL != "http://host.docker.internal:8080" {
		t.Fatalf("controller url = %q, want configured URL", cfg.ControllerURL)
	}
	if cfg.ControllerTokenFile != "/data/goetl-test/secrets/controller-worker-token" {
		t.Fatalf("controller token file = %q, want configured token file", cfg.ControllerTokenFile)
	}
	if cfg.LogDir != "/data/goetl-test/logs" || cfg.TmpDir != "/data/goetl-test/tmp" || cfg.DataDir != "/data/goetl-test/data" {
		t.Fatalf("unexpected runtime dirs: %+v", cfg)
	}
	if cfg.AssetCacheDir != "/data/goetl-test/cache/assets" {
		t.Fatalf("asset cache dir = %q, want configured cache dir", cfg.AssetCacheDir)
	}
	if cfg.PythonExecutable != "python3" {
		t.Fatalf("python executable = %q, want python3", cfg.PythonExecutable)
	}
	if cfg.MaxAssetBytes != 20000000000 {
		t.Fatalf("max asset bytes = %d, want 20000000000", cfg.MaxAssetBytes)
	}
	if cfg.DataLocationRoots["fixture_data"] != "/data/goetl-test/fixtures" ||
		cfg.DataLocationRoots["published_data"] != "/data/goetl-test/published" {
		t.Fatalf("data location roots = %#v", cfg.DataLocationRoots)
	}
	if _, err := os.Stat(transport.copies[0].localPath); !os.IsNotExist(err) {
		t.Fatalf("temp worker config still exists or stat failed unexpectedly: %v", err)
	}
}

func TestWorkerRuntimePrepareRequiresControllerTokenFileForExternalControllerURL(t *testing.T) {
	runtime := WorkerRuntime{
		Root:          "/data/goetl-test",
		ControllerURL: "https://controller.example.org",
	}

	err := runtime.Prepare(context.Background(), &recordingTransport{}, BashShellPlatform{})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "worker controller token file is required") {
		t.Fatalf("error = %v, want missing token file", err)
	}
}

func TestWorkerRuntimePrepareWritesWorkerConfigWithoutTokenMaterial(t *testing.T) {
	sentinel := "goetl-worker-controller-token-sentinel-005-do-not-serialize"
	root := filepath.Join(t.TempDir(), "runtime")
	tokenFile := filepath.Join(t.TempDir(), "controller-worker-token")
	if err := os.WriteFile(tokenFile, []byte(sentinel), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	runtime := WorkerRuntime{
		Root:                filepath.ToSlash(root),
		ControllerURL:       "https://controller.example.org",
		ControllerTokenFile: filepath.ToSlash(tokenFile),
	}

	if err := runtime.Prepare(context.Background(), LocalTransport{}, BashShellPlatform{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "config", "worker.json"))
	if err != nil {
		t.Fatalf("read worker config: %v", err)
	}
	if strings.Contains(string(data), sentinel) {
		t.Fatalf("worker config contains token sentinel")
	}

	var cfg WorkerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("decode worker config: %v", err)
	}
	if cfg.ControllerTokenFile != tokenFile {
		t.Fatalf("controller token file = %q, want %q", cfg.ControllerTokenFile, tokenFile)
	}
}

func TestWorkerRuntimePrepareWritesAbsoluteLocalWorkerConfigPaths(t *testing.T) {
	root := filepath.Join(t.TempDir(), "runtime")
	tokenFile := filepath.Join(t.TempDir(), "controller-worker-token")
	if err := os.WriteFile(tokenFile, []byte("local-token"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	runtime := WorkerRuntime{
		Root:                filepath.ToSlash(root),
		ControllerURL:       "https://controller.example.org",
		ControllerTokenFile: filepath.ToSlash(tokenFile),
		DataLocationRoots: map[string]string{
			"fixture_data": filepath.ToSlash(filepath.Join(root, "fixtures")),
		},
	}

	if err := runtime.Prepare(context.Background(), LocalTransport{}, BashShellPlatform{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, "config", "worker.json"))
	if err != nil {
		t.Fatalf("read worker config: %v", err)
	}
	var cfg WorkerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("decode worker config: %v", err)
	}

	for name, path := range map[string]string{
		"log_dir":                   cfg.LogDir,
		"tmp_dir":                   cfg.TmpDir,
		"data_dir":                  cfg.DataDir,
		"controller_token_file":     cfg.ControllerTokenFile,
		"data_location_roots.value": cfg.DataLocationRoots["fixture_data"],
	} {
		if !filepath.IsAbs(path) {
			t.Fatalf("%s = %q, want absolute path", name, path)
		}
	}
}

func TestWorkerRuntimePreflightAllowsLoopbackWithoutTokenFile(t *testing.T) {
	issues := (WorkerRuntime{
		ControllerURL: "http://localhost:8080",
	}).RuntimePreflight(context.Background(), LocalTransport{}, BashShellPlatform{})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v, want none", issues)
	}
}

func TestWorkerRuntimePreflightRejectsUnusableLocalControllerTokenFiles(t *testing.T) {
	root := t.TempDir()
	emptyTokenFile := filepath.Join(root, "empty-token")
	if err := os.WriteFile(emptyTokenFile, nil, 0o600); err != nil {
		t.Fatalf("write empty token file: %v", err)
	}
	oversizedTokenFile := filepath.Join(root, "oversized-token")
	if err := os.WriteFile(oversizedTokenFile, []byte(strings.Repeat("x", maxWorkerControllerTokenBytes+1)), 0o600); err != nil {
		t.Fatalf("write oversized token file: %v", err)
	}

	tests := []struct {
		name      string
		tokenFile string
	}{
		{name: "missing", tokenFile: filepath.Join(root, "missing-token")},
		{name: "empty", tokenFile: emptyTokenFile},
		{name: "oversized", tokenFile: oversizedTokenFile},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			issues := (WorkerRuntime{
				ControllerURL:       "https://controller.example.org",
				ControllerTokenFile: filepath.ToSlash(test.tokenFile),
			}).RuntimePreflight(context.Background(), LocalTransport{}, BashShellPlatform{})
			if len(issues) != 1 {
				t.Fatalf("issue count = %d, want 1", len(issues))
			}
			if issues[0].Severity != PreflightSeverityError {
				t.Fatalf("severity = %q, want error", issues[0].Severity)
			}
			if strings.Contains(issues[0].Message, "goetl-worker-controller-token-sentinel") {
				t.Fatalf("issue message contains token sentinel")
			}
		})
	}
}

func TestWorkerRuntimePreflightAcceptsRestrictiveLocalControllerTokenFile(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "controller-worker-token")
	if err := os.WriteFile(tokenFile, []byte("goetl-worker-token"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	issues := (WorkerRuntime{
		ControllerURL:       "https://controller.example.org",
		ControllerTokenFile: filepath.ToSlash(tokenFile),
	}).RuntimePreflight(context.Background(), LocalTransport{}, BashShellPlatform{})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v, want none", issues)
	}
}

func TestWorkerRuntimePathsUseConfiguredDataDir(t *testing.T) {
	paths, err := (WorkerRuntime{
		Root:    "/data/goetl-test",
		DataDir: "/data/goetl-worker-data",
	}).paths()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if paths.DataDir != "/data/goetl-worker-data" {
		t.Fatalf("data dir = %q, want configured data dir", paths.DataDir)
	}
}

func TestWorkerRuntimePrepareUploadsArtifact(t *testing.T) {
	transport := &recordingTransport{}
	runtime := WorkerRuntime{
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

func TestWorkerRuntimePrepareSkipsMatchingLocalArtifactCopy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("local artifact upload uses chmod")
	}

	root := t.TempDir()
	source := filepath.Join(root, "source-worker")
	destination := filepath.Join(root, "runtime", "artifacts", "goetl-worker")
	if err := os.WriteFile(source, []byte("same-worker"), 0o755); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatalf("create destination dir: %v", err)
	}
	if err := os.WriteFile(destination, []byte("same-worker"), 0o755); err != nil {
		t.Fatalf("write destination: %v", err)
	}
	before, err := os.Stat(destination)
	if err != nil {
		t.Fatalf("stat destination: %v", err)
	}

	runtime := WorkerRuntime{
		Root:                filepath.ToSlash(filepath.Join(root, "runtime")),
		LocalWorkerArtifact: source,
	}
	if err := runtime.Prepare(context.Background(), LocalTransport{}, BashShellPlatform{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	after, err := os.Stat(destination)
	if err != nil {
		t.Fatalf("stat destination after prepare: %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("destination artifact was rewritten: before=%s after=%s", before.ModTime(), after.ModTime())
	}
}

func TestWorkerRuntimePrepareIntegration(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is required for Dockerized Slurm integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), dockerSlurmIntegrationTimeout)
	defer cancel()

	if err := dockerExec(ctx, "slurmctld", "test", "-d", "/data"); err != nil {
		t.Skipf("slurmctld container with /data is required: %v", err)
	}

	runtime := WorkerRuntime{
		Root:                "/data/goetl-test-runtime",
		ControllerURL:       "http://host.docker.internal:8080",
		ControllerTokenFile: "/data/goetl-test-runtime/secrets/controller-worker-token",
	}
	transport := DockerContainerTransport{Container: "slurmctld"}
	if err := runtime.Prepare(ctx, transport, BashShellPlatform{}); err != nil {
		t.Fatalf("prepare runtime: %v", err)
	}

	if err := dockerExec(ctx, "slurmctld", "test", "-f", "/data/goetl-test-runtime/config/worker.json"); err != nil {
		t.Fatalf("worker config was not written: %v", err)
	}
	if err := dockerExec(ctx, "slurm-cpu-worker-1", "test", "-d", "/data/goetl-test-runtime/logs"); err != nil {
		t.Fatalf("runtime logs dir is not visible on worker: %v", err)
	}
}

func TestWorkerRuntimePrepareUploadsArtifactIntegration(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is required for Dockerized Slurm integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), dockerSlurmIntegrationTimeout)
	defer cancel()

	if err := dockerExec(ctx, "slurmctld", "test", "-d", "/data"); err != nil {
		t.Skipf("slurmctld container with /data is required: %v", err)
	}

	localPath := filepath.Join(t.TempDir(), "goetl-worker")
	if err := os.WriteFile(localPath, []byte("#!/usr/bin/env bash\necho goetl-artifact\n"), 0o644); err != nil {
		t.Fatalf("write local artifact: %v", err)
	}

	runtime := WorkerRuntime{
		Root:                "/data/goetl-test-artifact",
		ControllerURL:       "http://host.docker.internal:8080",
		ControllerTokenFile: "/data/goetl-test-artifact/secrets/controller-worker-token",
		LocalWorkerArtifact: localPath,
	}
	transport := DockerContainerTransport{Container: "slurmctld"}
	if err := runtime.Prepare(ctx, transport, BashShellPlatform{}); err != nil {
		t.Fatalf("prepare runtime: %v", err)
	}

	if err := dockerExec(ctx, "slurm-cpu-worker-1", "test", "-x", "/data/goetl-test-artifact/artifacts/goetl-worker"); err != nil {
		t.Fatalf("artifact is not executable on worker: %v", err)
	}
}

func TestSingularityWorkerRuntimeWorkerScript(t *testing.T) {
	runtime := SingularityWorkerRuntime{
		SingularityExecutable:     "singularity",
		ImagePath:                 "/data/goetl/images/goetl-worker.sif",
		ContainerWorkerExecutable: "/goetl/goetl-worker",
		Bind:                      "/data/goetl:/data/goetl",
	}

	cfg, err := runtime.WorkerScript(SlurmWorkerScriptConfig{
		JobName:          "goetl-worker",
		WorkerExecutable: "/data/goetl/artifacts/goetl-worker",
		WorkerArgs:       []string{"--poll-once"},
		WorkerConfigPath: "/data/goetl/config/worker.json",
		LogDir:           "/data/goetl/logs",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.WorkerExecutable != "singularity" {
		t.Fatalf("worker executable = %q, want singularity", cfg.WorkerExecutable)
	}
	wantArgs := []string{
		"exec",
		"--bind",
		"/data/goetl:/data/goetl",
		"/data/goetl/images/goetl-worker.sif",
		"/goetl/goetl-worker",
		"--poll-once",
	}
	if !stringSlicesEqual(cfg.WorkerArgs, wantArgs) {
		t.Fatalf("worker args = %#v, want %#v", cfg.WorkerArgs, wantArgs)
	}
	if cfg.WorkerConfigPath != "/data/goetl/config/worker.json" {
		t.Fatalf("worker config path = %q, want original config path", cfg.WorkerConfigPath)
	}
}

func TestSingularityWorkerRuntimeWorkerScriptRequiresImage(t *testing.T) {
	_, err := (SingularityWorkerRuntime{
		ContainerWorkerExecutable: "/goetl/goetl-worker",
	}).WorkerScript(SlurmWorkerScriptConfig{
		JobName:          "goetl-worker",
		WorkerExecutable: "/data/goetl/artifacts/goetl-worker",
		WorkerConfigPath: "/data/goetl/config/worker.json",
		LogDir:           "/data/goetl/logs",
	})
	if err == nil {
		t.Fatal("expected an error")
	}
}

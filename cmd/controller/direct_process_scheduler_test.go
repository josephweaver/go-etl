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
	"time"
)

func TestDirectProcessSchedulerSubmitStartsProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("direct process scheduler test uses a bash script")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is required")
	}

	root := t.TempDir()
	marker := filepath.Join(root, "started.txt")
	script := filepath.Join(root, "worker.sh")
	if err := os.WriteFile(script, []byte("#!/usr/bin/env bash\nprintf '%s' \"$2\" > \"$1\"\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	handle, err := (DirectProcessScheduler{}).Submit(context.Background(), JobSpec{
		WorkerScript: SlurmWorkerScriptConfig{
			JobName:          "goetl-worker",
			WorkerExecutable: script,
			WorkerArgs:       []string{marker},
			WorkerConfigPath: "worker-config.json",
			LogDir:           root,
		},
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if handle.ID == "" {
		t.Fatal("job handle id is required")
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		content, err := os.ReadFile(marker)
		if err == nil {
			if string(content) != "worker-config.json" {
				t.Fatalf("marker = %q, want worker-config.json", string(content))
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("marker was not written: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestLocalDirectSingularityWorkerRuntimeStartsWrappedProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("local direct singularity runtime test uses a bash script")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is required")
	}

	root := t.TempDir()
	marker := filepath.Join(root, "singularity-args.txt")
	fakeSingularity := filepath.Join(root, "singularity")
	if err := os.WriteFile(fakeSingularity, []byte("#!/usr/bin/env bash\nprintf '%s\\n' \"$@\" > '"+marker+"'\n"), 0o755); err != nil {
		t.Fatalf("write fake singularity: %v", err)
	}

	env := ExecutionEnvironment{
		Transports: []Transport{LocalTransport{}},
		Dialect:    BashShellPlatform{},
		Scheduler:  DirectProcessScheduler{},
		Runtime: SingularityWorkerRuntime{
			WorkerRuntime: WorkerRuntime{
				Root:          root,
				ControllerURL: "http://localhost:8080",
			},
			SingularityExecutable:     fakeSingularity,
			ImagePath:                 filepath.Join(root, "images", "goetl-worker.sif"),
			ContainerWorkerExecutable: "/goetl/goetl-worker",
			Bind:                      root + ":/data/goetl",
		},
	}

	if err := env.Prepare(context.Background()); err != nil {
		t.Fatalf("prepare environment: %v", err)
	}

	workerCfgPath := filepath.Join(root, "config", "worker.json")
	var workerCfg WorkerConfig
	data, err := os.ReadFile(workerCfgPath)
	if err != nil {
		t.Fatalf("read worker config: %v", err)
	}
	if err := json.Unmarshal(data, &workerCfg); err != nil {
		t.Fatalf("decode worker config: %v", err)
	}
	if workerCfg.ControllerURL != "http://localhost:8080" {
		t.Fatalf("controller url = %q, want configured URL", workerCfg.ControllerURL)
	}

	script, err := env.Runtime.(WorkerScriptRuntime).WorkerScript(SlurmWorkerScriptConfig{
		JobName:          "goetl-worker",
		WorkerExecutable: filepath.Join(root, "artifacts", "goetl-worker"),
		WorkerConfigPath: workerCfgPath,
		LogDir:           filepath.Join(root, "logs"),
	})
	if err != nil {
		t.Fatalf("build worker script config: %v", err)
	}
	if _, err := env.Scheduler.Submit(context.Background(), JobSpec{WorkerScript: script}); err != nil {
		t.Fatalf("submit direct process: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		content, err := os.ReadFile(marker)
		if err == nil {
			got := strings.Split(strings.TrimSpace(string(content)), "\n")
			want := []string{
				"exec",
				"--bind",
				root + ":/data/goetl",
				filepath.Join(root, "images", "goetl-worker.sif"),
				"/goetl/goetl-worker",
				workerCfgPath,
			}
			if !stringSlicesEqual(got, want) {
				t.Fatalf("singularity args = %#v, want %#v", got, want)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("fake singularity was not called: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

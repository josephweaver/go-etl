package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"goetl/internal/model"
)

func TestDMTCPAdapterStartFreshUsesIsolatedDirectInterpreterLaunch(t *testing.T) {
	adapter, runner, process, item := newDMTCPLaunchTest(t)
	execution, err := adapter.StartFresh(context.Background(), item)
	if err != nil {
		t.Fatalf("StartFresh() error = %v", err)
	}
	if runner.starts != 1 {
		t.Fatalf("runner starts = %d, want 1", runner.starts)
	}
	spec := runner.spec
	if spec.Executable != "/opt/dmtcp/bin/dmtcp_launch" {
		t.Fatalf("executable = %q", spec.Executable)
	}
	if !containsDMTCPArg(spec.Args, "--new-coordinator") {
		t.Fatalf("args do not isolate the coordinator: %q", spec.Args)
	}
	wantRoot := filepath.Join(adapter.Profile.SharedTmpRoot, "dmtcp", item.AttemptID)
	assertDMTCPFlagPath(t, spec.Args, "--port-file", filepath.Join(wantRoot, "coordinator", "coordinator.port"))
	assertDMTCPFlagPath(t, spec.Args, "--ckptdir", filepath.Join(wantRoot, "checkpoints"))
	assertDMTCPFlagPath(t, spec.Args, "--tmpdir", filepath.Join(wantRoot, "tmp"))
	pythonIndex := indexDMTCPArg(spec.Args, "/usr/local/bin/python3")
	if pythonIndex < 0 || pythonIndex+2 >= len(spec.Args) {
		t.Fatalf("direct Python argv missing: %q", spec.Args)
	}
	if spec.Args[pythonIndex+1] != filepath.Join(spec.Dir, "main.py") || spec.Args[pythonIndex+2] != "safe-argument" {
		t.Fatalf("direct Python argv = %q", spec.Args[pythonIndex:])
	}
	for _, forbidden := range []string{"sh", "bash", "-c"} {
		if containsDMTCPArg(spec.Args, forbidden) {
			t.Fatalf("launch argv unexpectedly contains %q: %q", forbidden, spec.Args)
		}
	}
	for _, directory := range []string{wantRoot, filepath.Join(wantRoot, "coordinator"), filepath.Join(wantRoot, "checkpoints"), filepath.Join(wantRoot, "tmp")} {
		if info, statErr := os.Stat(directory); statErr != nil || !info.IsDir() {
			t.Fatalf("workspace directory %s: info=%v error=%v", directory, info, statErr)
		}
	}

	process.finish(nil)
	if result := <-execution.Result(); result.Err != nil {
		t.Fatalf("Result().Err = %v", result.Err)
	}
}

func TestDMTCPAdapterStartFreshRejectsUnsafeInputsBeforeLaunch(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*DMTCPAdapter, *model.WorkItem)
		want   string
	}{
		{name: "relative shared root", mutate: func(adapter *DMTCPAdapter, _ *model.WorkItem) {
			adapter.Profile.SharedTmpRoot = "relative"
		}, want: "must be absolute"},
		{name: "attempt traversal", mutate: func(_ *DMTCPAdapter, item *model.WorkItem) {
			item.AttemptID = "../escape"
		}, want: "safe path segment"},
		{name: "entrypoint traversal", mutate: func(_ *DMTCPAdapter, item *model.WorkItem) {
			item.Parameters["python_entrypoint"] = model.Parameter{Type: "path", Value: "../escape.py"}
		}, want: "unsafe python_entrypoint"},
		{name: "shell argument", mutate: func(_ *DMTCPAdapter, item *model.WorkItem) {
			item.Parameters["python_args"] = model.Parameter{Type: "list", Value: []string{"ok; touch escaped"}}
		}, want: "shell metacharacter"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter, runner, _, item := newDMTCPLaunchTest(t)
			test.mutate(&adapter, &item)
			if _, err := adapter.StartFresh(context.Background(), item); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("StartFresh() error = %v, want text %q", err, test.want)
			}
			if runner.starts != 0 {
				t.Fatalf("runner starts = %d, want 0", runner.starts)
			}
		})
	}
}

func TestDMTCPExecutionTerminationIsIdempotent(t *testing.T) {
	adapter, _, process, item := newDMTCPLaunchTest(t)
	execution, err := adapter.StartFresh(context.Background(), item)
	if err != nil {
		t.Fatalf("StartFresh() error = %v", err)
	}
	if err := execution.Terminate(context.Background()); err != nil {
		t.Fatalf("Terminate() error = %v", err)
	}
	if err := execution.Terminate(context.Background()); err != nil {
		t.Fatalf("second Terminate() error = %v", err)
	}
	if process.kills != 1 {
		t.Fatalf("process kills = %d, want 1", process.kills)
	}
	process.finish(errors.New("killed"))
	<-execution.Result()
}

func newDMTCPLaunchTest(t *testing.T) (DMTCPAdapter, *recordingDMTCPRunner, *fakeDMTCPProcess, model.WorkItem) {
	t.Helper()
	tmpRoot := t.TempDir()
	process := &fakeDMTCPProcess{wait: make(chan error, 1)}
	runner := &recordingDMTCPRunner{process: process}
	item := model.WorkItem{
		ID:             "work-001",
		AttemptID:      "attempt-001",
		Type:           model.WorkItemTypePythonScript,
		OutputFilename: "result.json",
		Source:         &model.WorkItemSource{RunID: "run-001", ManifestPath: "source.json"},
		Parameters: model.Parameters{
			"python_entrypoint": {Type: "path", Value: "main.py"},
			"python_args":       {Type: "list", Value: []string{"safe-argument"}},
		},
	}
	adapter := DMTCPAdapter{
		Worker: Worker{
			Config: Config{TmpDir: filepath.Join(tmpRoot, "worker"), DataDir: filepath.Join(tmpRoot, "data")},
			SourceBundles: &recordingSourceBundleProvider{body: mustPythonSourceBundle(t, map[string]string{
				"main.py": "print('checkpoint me')\n",
			})},
		},
		Profile: DMTCPLaunchProfile{
			LaunchExecutable: "/opt/dmtcp/bin/dmtcp_launch",
			PythonExecutable: "/usr/local/bin/python3",
			SharedTmpRoot:    filepath.Join(tmpRoot, "shared"),
		},
		Runner: runner,
	}
	return adapter, runner, process, item
}

type recordingDMTCPRunner struct {
	starts  int
	spec    DMTCPCommandSpec
	process DMTCPProcess
}

func (runner *recordingDMTCPRunner) Start(_ context.Context, spec DMTCPCommandSpec) (DMTCPProcess, error) {
	runner.starts++
	runner.spec = spec
	return runner.process, nil
}

type fakeDMTCPProcess struct {
	wait  chan error
	mu    sync.Mutex
	kills int
}

func (process *fakeDMTCPProcess) Wait() error {
	return <-process.wait
}

func (process *fakeDMTCPProcess) Kill() error {
	process.mu.Lock()
	defer process.mu.Unlock()
	process.kills++
	return nil
}

func (process *fakeDMTCPProcess) finish(err error) {
	process.wait <- err
}

func containsDMTCPArg(args []string, want string) bool {
	return indexDMTCPArg(args, want) >= 0
}

func indexDMTCPArg(args []string, want string) int {
	for i, argument := range args {
		if argument == want {
			return i
		}
	}
	return -1
}

func assertDMTCPFlagPath(t *testing.T, args []string, flag string, want string) {
	t.Helper()
	index := indexDMTCPArg(args, flag)
	if index < 0 || index+1 >= len(args) || args[index+1] != want {
		t.Fatalf("%s path in %q, want %q", flag, args, want)
	}
}

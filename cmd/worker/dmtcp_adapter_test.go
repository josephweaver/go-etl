package main

import (
	"context"
	"errors"
	"fmt"
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

func TestDMTCPExecutionPeriodicCapturePublishesManifestLast(t *testing.T) {
	adapter, runner, process, item := newDMTCPCheckpointTest(t, false)
	execution, err := adapter.StartFresh(context.Background(), item)
	if err != nil {
		t.Fatalf("StartFresh() error = %v", err)
	}
	request := dmtcpCaptureRequest(model.CheckpointCaptureKindPeriodic)
	checkpoint, err := execution.CapturePeriodic(context.Background(), request)
	if err != nil {
		t.Fatalf("CapturePeriodic() error = %v", err)
	}
	if !runner.observedManifestAbsent {
		t.Fatal("manifest existed before the checkpoint image was finalized")
	}
	if got := runner.controlCommands(); strings.Join(got, ",") != "--list,--bcheckpoint" {
		t.Fatalf("control commands = %q", got)
	}
	registration := PauseAdapterRegistration{
		WorkItemType:   model.WorkItemTypePythonScript,
		Strategy:       model.PauseStrategyDMTCP,
		AdapterID:      adapter.Profile.AdapterID,
		AdapterVersion: adapter.Profile.AdapterVersion,
		Capabilities:   PauseAdapterCapabilities{Shutdown: true, Periodic: true, Yield: true},
		Adapter:        adapter,
	}
	manifest, err := checkpoint.Validate(request, registration)
	if err != nil {
		t.Fatalf("PreparedCheckpoint.Validate() error = %v", err)
	}
	if manifest.DMTCP == nil || manifest.DMTCP.BuildIdentity != adapter.Profile.BuildIdentity || len(manifest.DMTCP.CheckpointPaths) != 1 {
		t.Fatalf("DMTCP manifest payload = %+v", manifest.DMTCP)
	}
	manifestPath := filepath.Join(adapter.Profile.SharedTmpRoot, filepath.FromSlash(checkpoint.Reference.ManifestRelativePath))
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if string(manifestBytes) != checkpoint.ManifestJSON {
		t.Fatal("manifest file bytes do not match PreparedCheckpoint.ManifestJSON")
	}

	if err := execution.Terminate(context.Background()); err != nil {
		t.Fatalf("Terminate() error = %v", err)
	}
	process.finish(errors.New("killed"))
	<-execution.Result()
}

func TestDMTCPExecutionSuspendCaptureCheckpointsAndKillsWithoutTerminalRace(t *testing.T) {
	adapter, runner, _, item := newDMTCPCheckpointTest(t, true)
	execution, err := adapter.StartFresh(context.Background(), item)
	if err != nil {
		t.Fatalf("StartFresh() error = %v", err)
	}
	request := dmtcpCaptureRequest(model.CheckpointCaptureKindFinal)
	checkpoint, err := execution.CaptureForSuspend(context.Background(), request)
	if err != nil {
		t.Fatalf("CaptureForSuspend() error = %v", err)
	}
	if checkpoint.ManifestJSON == "" {
		t.Fatal("suspending capture returned an empty manifest")
	}
	if got := runner.controlCommands(); strings.Join(got, ",") != "--list,--kcheckpoint" {
		t.Fatalf("control commands = %q", got)
	}
	select {
	case result := <-execution.Result():
		t.Fatalf("expected DMTCP kill raced checkpoint confirmation: %+v", result)
	default:
	}
	if err := execution.Terminate(context.Background()); err != nil {
		t.Fatalf("Terminate() error = %v", err)
	}
}

func TestDMTCPExecutionRejectsIncompleteOrTemporaryImageSets(t *testing.T) {
	for _, temporary := range []bool{false, true} {
		name := "missing"
		if temporary {
			name = "temporary"
		}
		t.Run(name, func(t *testing.T) {
			adapter, runner, process, item := newDMTCPCheckpointTest(t, false)
			runner.omitFinalImage = !temporary
			runner.writeTemporaryImage = temporary
			execution, err := adapter.StartFresh(context.Background(), item)
			if err != nil {
				t.Fatalf("StartFresh() error = %v", err)
			}
			_, err = execution.CapturePeriodic(context.Background(), dmtcpCaptureRequest(model.CheckpointCaptureKindPeriodic))
			want := "incomplete"
			if temporary {
				want = "temporary image"
			}
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("CapturePeriodic() error = %v, want %q", err, want)
			}
			_ = execution.Terminate(context.Background())
			process.finish(errors.New("killed"))
			<-execution.Result()
		})
	}
}

func dmtcpCaptureRequest(kind model.CheckpointCaptureKind) CheckpointCaptureRequest {
	return CheckpointCaptureRequest{
		WorkItemID:                  "work-001",
		WorkItemType:                model.WorkItemTypePythonScript,
		AttemptID:                   "attempt-001",
		ExecutionLineageID:          "lineage-001",
		ResumeGeneration:            1,
		ResumeArtifactID:            "artifact-001",
		ArtifactStorageRelativePath: "goetl/resume/artifact-001",
		CaptureKind:                 kind,
	}
}

func newDMTCPCheckpointTest(t *testing.T, finishPayloadOnCheckpoint bool) (DMTCPAdapter, *checkpointDMTCPRunner, *fakeDMTCPProcess, model.WorkItem) {
	t.Helper()
	adapter, _, process, item := newDMTCPLaunchTest(t)
	runner := &checkpointDMTCPRunner{
		payload:                   process,
		profile:                   adapter.Profile,
		attemptID:                 item.AttemptID,
		finishPayloadOnCheckpoint: finishPayloadOnCheckpoint,
	}
	adapter.Runner = runner
	return adapter, runner, process, item
}

type checkpointDMTCPRunner struct {
	payload                   *fakeDMTCPProcess
	profile                   DMTCPLaunchProfile
	attemptID                 string
	specs                     []DMTCPCommandSpec
	finishPayloadOnCheckpoint bool
	omitFinalImage            bool
	writeTemporaryImage       bool
	observedManifestAbsent    bool
}

func (runner *checkpointDMTCPRunner) Start(_ context.Context, spec DMTCPCommandSpec) (DMTCPProcess, error) {
	runner.specs = append(runner.specs, spec)
	if spec.Executable == runner.profile.LaunchExecutable {
		portPath := filepath.Join(runner.profile.SharedTmpRoot, "dmtcp", runner.attemptID, "coordinator", "coordinator.port")
		if err := os.WriteFile(portPath, []byte("23456\n"), 0600); err != nil {
			return nil, err
		}
		return runner.payload, nil
	}
	return callbackDMTCPProcess{wait: func() error {
		command := spec.Args[len(spec.Args)-1]
		switch command {
		case "--list":
			_, _ = fmt.Fprintln(spec.Stdout, "WorkerState::RUNNING pid=100")
		case "--bcheckpoint", "--kcheckpoint":
			manifestPath := filepath.Join(runner.profile.SharedTmpRoot, "goetl", "resume", "artifact-001", "manifest.json")
			_, err := os.Stat(manifestPath)
			runner.observedManifestAbsent = os.IsNotExist(err)
			checkpointDir := filepath.Join(runner.profile.SharedTmpRoot, "dmtcp", runner.attemptID, "checkpoints")
			if runner.writeTemporaryImage {
				if err := os.WriteFile(filepath.Join(checkpointDir, "ckpt_python.dmtcp.temp"), []byte("temporary"), 0600); err != nil {
					return err
				}
			}
			if !runner.omitFinalImage {
				if err := os.WriteFile(filepath.Join(checkpointDir, "ckpt_python.dmtcp"), []byte("checkpoint-image"), 0600); err != nil {
					return err
				}
			}
			if command == "--kcheckpoint" && runner.finishPayloadOnCheckpoint {
				runner.payload.finish(errors.New("DMTCP computation checkpointed and killed"))
			}
		default:
			return fmt.Errorf("unexpected control command %q", command)
		}
		return nil
	}}, nil
}

func (runner *checkpointDMTCPRunner) controlCommands() []string {
	var commands []string
	for _, spec := range runner.specs {
		if spec.Executable == runner.profile.CommandExecutable {
			commands = append(commands, spec.Args[len(spec.Args)-1])
		}
	}
	return commands
}

type callbackDMTCPProcess struct {
	wait func() error
}

func (process callbackDMTCPProcess) Wait() error { return process.wait() }
func (callbackDMTCPProcess) Kill() error         { return nil }

func newDMTCPLaunchTest(t *testing.T) (DMTCPAdapter, *recordingDMTCPRunner, *fakeDMTCPProcess, model.WorkItem) {
	t.Helper()
	tmpRoot := t.TempDir()
	process := &fakeDMTCPProcess{wait: make(chan error, 1)}
	runner := &recordingDMTCPRunner{process: process}
	item := model.WorkItem{
		ID:               "work-001",
		AttemptID:        "attempt-001",
		Type:             model.WorkItemTypePythonScript,
		OutputFilename:   "result.json",
		InputFingerprint: "input-v1",
		CodeVersion:      "code-v1",
		Source:           &model.WorkItemSource{RunID: "run-001", ManifestPath: "source-v1"},
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
			LaunchExecutable:               "/opt/dmtcp/bin/dmtcp_launch",
			CommandExecutable:              "/opt/dmtcp/bin/dmtcp_command",
			PythonExecutable:               "/usr/local/bin/python3",
			SharedTmpRoot:                  filepath.Join(tmpRoot, "shared"),
			ExpectedClients:                1,
			BuildIdentity:                  "dmtcp-4.2.0-f8009ce7-python-3.11",
			AdapterID:                      "direct-interpreter-dmtcp",
			AdapterVersion:                 "1",
			WorkerExecutionContractVersion: "goet/worker-execution/v1",
			WorkerVersion:                  "test-worker",
			ContainerImageIdentity:         "sha256:test-image",
			OperatingSystem:                "linux",
			Architecture:                   "amd64",
			ContainerRuntime:               "singularity-ce-4.1.2",
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

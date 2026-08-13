package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"goetl/internal/model"
)

func TestDMTCPAdapterSupervisorContainerSmoke(t *testing.T) {
	root := os.Getenv("GOET_DMTCP_ADAPTER_SMOKE_ROOT")
	if root == "" {
		t.Skip("set GOET_DMTCP_ADAPTER_SMOKE_ROOT inside the pinned DMTCP container")
	}
	fixturePath := os.Getenv("GOET_DMTCP_ADAPTER_FIXTURE")
	fixture, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read adapter fixture: %v", err)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	bundle := mustPythonSourceBundle(t, map[string]string{"main.py": string(fixture)})
	sharedRoot := filepath.Join(root, "shared")
	profile := DMTCPLaunchProfile{
		LaunchExecutable:               "/opt/dmtcp/bin/dmtcp_launch",
		CommandExecutable:              "/opt/dmtcp/bin/dmtcp_command",
		RestartExecutable:              "/opt/dmtcp/bin/dmtcp_restart",
		PythonExecutable:               "/usr/local/bin/python",
		SharedTmpRoot:                  sharedRoot,
		CheckpointSignal:               "12",
		ExpectedClients:                1,
		BuildIdentity:                  "dmtcp-4.2.0-f8009ce7-python-3.11.15",
		AdapterID:                      "direct-interpreter-dmtcp",
		AdapterVersion:                 "1",
		WorkerExecutionContractVersion: "goet/worker-execution/v1",
		WorkerVersion:                  "os009-container-smoke",
		ContainerImageIdentity:         "goetl/dmtcp-python:os004",
		OperatingSystem:                "debian-bookworm",
		Architecture:                   "amd64",
		ContainerRuntime:               os.Getenv("GOET_DMTCP_ADAPTER_RUNTIME"),
	}
	if profile.ContainerRuntime == "" {
		profile.ContainerRuntime = "docker"
	}

	baselineRoot := filepath.Join(root, "baseline")
	baselineWorker := dmtcpSmokeWorker(t, baselineRoot, bundle)
	baselineControl := filepath.Join(baselineRoot, "control", "continue")
	baselineItem := dmtcpSmokeItem("attempt-baseline", baselineControl, baselineRoot)
	if err := os.MkdirAll(filepath.Dir(baselineControl), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(baselineControl, []byte("continue\n"), 0600); err != nil {
		t.Fatal(err)
	}
	baselineOutcome := runDMTCPContainerSupervisor(
		t,
		baselineWorker,
		profile,
		baselineItem,
		dmtcpSmokeSupervisorConfig(CheckpointModeShutdown),
		&supervisorTestCheckpointClient{},
		[]string{"lineage-baseline"},
	)
	if baselineOutcome.Kind != ExecutionSupervisorCompleted {
		t.Fatalf("baseline outcome = %+v", baselineOutcome)
	}

	resumedRoot := filepath.Join(root, "resumed")
	resumedWorker := dmtcpSmokeWorker(t, resumedRoot, bundle)
	continuation := filepath.Join(resumedRoot, "control", "continue")
	producingItem := dmtcpSmokeItem("attempt-producing", continuation, resumedRoot)
	confirmation := make(chan model.WorkCheckpointConfirmation, 1)
	checkpointClient := &supervisorTestCheckpointClient{confirm: func(_ context.Context, _ WorkerSession, request model.WorkCheckpointConfirmation) (model.WorkCheckpointAcknowledgement, error) {
		confirmation <- request
		return supervisorAcknowledgement(t, request), nil
	}}
	suspended := runDMTCPContainerSupervisor(
		t,
		resumedWorker,
		profile,
		producingItem,
		dmtcpSmokeSupervisorConfig(CheckpointModeYield),
		checkpointClient,
		[]string{"lineage-resumed", "artifact-resumed-001"},
	)
	if suspended.Kind != ExecutionSupervisorSuspended {
		t.Fatalf("suspending outcome = %+v", suspended)
	}
	checkpoint := <-confirmation
	if checkpoint.CaptureKind != model.CheckpointCaptureKindQuantum || checkpoint.Disposition != model.CheckpointDispositionSuspend {
		t.Fatalf("checkpoint confirmation = %+v", checkpoint)
	}
	if _, err := os.Stat(filepath.Join(resumedRoot, "markers", "post-resume.log")); !os.IsNotExist(err) {
		t.Fatalf("original process reached post-resume marker before restore: %v", err)
	}

	assignment := model.WorkItemResumeAssignment{
		Schema:               model.WorkItemResumeAssignmentSchemaV1,
		ResumedFromAttemptID: producingItem.AttemptID,
		ExecutionLineageID:   suspended.Acknowledgement.ExecutionLineageID,
		ResumeAttemptNumber:  1,
		ManifestJSON:         checkpoint.ManifestJSON,
		Reference:            checkpoint.Reference,
	}
	resumedItem := producingItem
	resumedItem.AttemptID = "attempt-restored"
	resumedItem.Resume = &assignment
	if err := os.WriteFile(continuation, []byte("continue\n"), 0600); err != nil {
		t.Fatal(err)
	}
	completed := runDMTCPContainerSupervisor(
		t,
		resumedWorker,
		profile,
		resumedItem,
		dmtcpSmokeSupervisorConfig(CheckpointModeShutdown),
		&supervisorTestCheckpointClient{},
		nil,
	)
	if completed.Kind != ExecutionSupervisorCompleted {
		t.Fatalf("resumed outcome = %+v", completed)
	}

	baselineOutput, err := os.ReadFile(filepath.Join(baselineWorker.Config.DataDir, baselineItem.OutputFilename))
	if err != nil {
		t.Fatal(err)
	}
	resumedOutput, err := os.ReadFile(filepath.Join(resumedWorker.Config.DataDir, resumedItem.OutputFilename))
	if err != nil {
		t.Fatal(err)
	}
	if string(baselineOutput) != string(resumedOutput) {
		t.Fatalf("resumed output differs from baseline\nbaseline=%s\nresumed=%s", baselineOutput, resumedOutput)
	}
	for _, marker := range []string{
		filepath.Join(resumedRoot, "markers", "pre-checkpoint.log"),
		filepath.Join(resumedRoot, "markers", "post-resume.log"),
	} {
		data, err := os.ReadFile(marker)
		if err != nil {
			t.Fatal(err)
		}
		if len(strings.Fields(string(data))) != 1 {
			t.Fatalf("marker %s was not written exactly once: %q", marker, data)
		}
	}
}

func dmtcpSmokeWorker(t *testing.T, root string, bundle []byte) Worker {
	t.Helper()
	config := Config{
		LogDir:  filepath.Join(root, "worker-logs"),
		TmpDir:  filepath.Join(root, "worker-tmp"),
		DataDir: filepath.Join(root, "worker-data"),
	}
	for _, directory := range []string{config.LogDir, config.TmpDir, config.DataDir, filepath.Join(root, "markers"), filepath.Join(root, "control")} {
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
	}
	return Worker{Config: config, SourceBundles: &recordingSourceBundleProvider{body: bundle}, LocalOnly: true}
}

func dmtcpSmokeItem(attemptID string, continuation string, root string) model.WorkItem {
	return model.WorkItem{
		ID:               "os009-python-checkpoint",
		AttemptID:        attemptID,
		Type:             model.WorkItemTypePythonScript,
		OutputFilename:   "result.json",
		InputFingerprint: "os009-input-v1",
		CodeVersion:      "os009-code-v1",
		Source:           &model.WorkItemSource{RunID: "os009-run", ManifestPath: "os009-source-v1"},
		Parameters: model.Parameters{
			"python_entrypoint": {Type: "path", Value: "main.py"},
			"python_args": {Type: "list", Value: []string{
				continuation,
				filepath.Join(root, "markers", "pre-checkpoint.log"),
				filepath.Join(root, "markers", "post-resume.log"),
			}},
		},
	}
}

func dmtcpSmokeSupervisorConfig(mode CheckpointMode) Config {
	config := Config{
		CheckpointMode:                   mode,
		DrainPauseDelaySeconds:           180,
		CheckpointCaptureTimeoutSeconds:  120,
		CheckpointReportTimeoutSeconds:   30,
		ExecutionTerminationGraceSeconds: 20,
	}
	if mode == CheckpointModeYield {
		config.WorkItemExecutionQuantumSeconds = 1
	}
	return config
}

func runDMTCPContainerSupervisor(
	t *testing.T,
	worker Worker,
	profile DMTCPLaunchProfile,
	item model.WorkItem,
	config Config,
	checkpoints ExecutionSupervisorCheckpointClient,
	ids []string,
) ExecutionSupervisorOutcome {
	t.Helper()
	adapter := &DMTCPAdapter{Worker: worker, Profile: profile}
	registration := PauseAdapterRegistration{
		WorkItemType:   model.WorkItemTypePythonScript,
		Strategy:       model.PauseStrategyDMTCP,
		AdapterID:      profile.AdapterID,
		AdapterVersion: profile.AdapterVersion,
		Capabilities:   PauseAdapterCapabilities{Shutdown: true, Periodic: true, Yield: true},
		Adapter:        adapter,
	}
	var idMu sync.Mutex
	supervisor := ExecutionSupervisor{
		Config:       config,
		Item:         item,
		Registration: registration,
		Session:      WorkerSession{WorkerID: "os009-worker", WorkerSessionID: "os009-session"},
		Checkpoints:  checkpoints,
		NewID: func(kind string) (string, error) {
			idMu.Lock()
			defer idMu.Unlock()
			if len(ids) == 0 {
				return "", fmt.Errorf("unexpected %s id request", kind)
			}
			id := ids[0]
			ids = ids[1:]
			return id, nil
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	outcome, err := supervisor.Run(ctx)
	if err != nil {
		t.Fatalf("ExecutionSupervisor.Run() error = %v", err)
	}
	return outcome
}

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

	writeDMTCPTestOutput(t, adapter.Worker.Config.TmpDir, item.AttemptID)
	process.finish(nil)
	result := <-execution.Result()
	if result.Err != nil {
		t.Fatalf("Result().Err = %v", result.Err)
	}
	if result.Evidence.InputSHA256 == "" || result.Evidence.OutputSHA256 == "" || result.Evidence.PostStateSHA256 == "" {
		t.Fatalf("completion evidence is incomplete: %+v", result.Evidence)
	}
	if _, err := os.Stat(filepath.Join(adapter.Worker.Config.DataDir, item.OutputFilename)); err != nil {
		t.Fatalf("completed output was not published: %v", err)
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
	runner.checkpointOutput = "Computation was checkpointed and killed.\n"
	runner.checkpointWaitErr = dmtcpTestExitError(2)
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

func TestDMTCPExecutionSuspendCaptureRejectsUnexpectedStatusTwoOutput(t *testing.T) {
	adapter, runner, process, item := newDMTCPCheckpointTest(t, false)
	runner.checkpointOutput = "unexpected checkpoint response\n"
	runner.checkpointWaitErr = dmtcpTestExitError(2)
	execution, err := adapter.StartFresh(context.Background(), item)
	if err != nil {
		t.Fatalf("StartFresh() error = %v", err)
	}
	_, err = execution.CaptureForSuspend(context.Background(), dmtcpCaptureRequest(model.CheckpointCaptureKindFinal))
	if err == nil || !strings.Contains(err.Error(), "unexpected checkpoint response") {
		t.Fatalf("CaptureForSuspend() error = %v", err)
	}
	if err := execution.Terminate(context.Background()); err != nil {
		t.Fatalf("Terminate() error = %v", err)
	}
	process.finish(errors.New("killed"))
	<-execution.Result()
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

func TestDMTCPAdapterStartResumeUsesValidatedImageArguments(t *testing.T) {
	adapter, item, assignment := createDMTCPResumeArtifact(t)
	resumeProcess := &fakeDMTCPProcess{wait: make(chan error, 1)}
	runner := &recordingDMTCPRunner{process: resumeProcess}
	adapter.Runner = runner

	execution, err := adapter.StartResume(context.Background(), item, assignment)
	if err != nil {
		t.Fatalf("StartResume() error = %v", err)
	}
	if runner.starts != 1 || runner.spec.Executable != adapter.Profile.RestartExecutable {
		t.Fatalf("restart calls = %d executable = %q", runner.starts, runner.spec.Executable)
	}
	if !containsDMTCPArg(runner.spec.Args, "--new-coordinator") {
		t.Fatalf("restart is not coordinator-isolated: %q", runner.spec.Args)
	}
	var manifest model.ResumeArtifactManifest
	if err := json.Unmarshal([]byte(assignment.ManifestJSON), &manifest); err != nil {
		t.Fatal(err)
	}
	wantImage := filepath.Join(adapter.Profile.SharedTmpRoot, filepath.FromSlash(manifest.StorageRelativePath), filepath.FromSlash(manifest.DMTCP.CheckpointPaths[0]))
	if runner.spec.Args[len(runner.spec.Args)-1] != wantImage {
		t.Fatalf("restart image = %q, want %q", runner.spec.Args[len(runner.spec.Args)-1], wantImage)
	}
	if containsDMTCPArg(runner.spec.Args, "dmtcp_restart_script.sh") || containsDMTCPArg(runner.spec.Args, "-c") {
		t.Fatalf("restart uses an unvalidated shell path: %q", runner.spec.Args)
	}
	writeDMTCPTestOutput(t, adapter.Worker.Config.TmpDir, assignment.ResumedFromAttemptID)
	resumeProcess.finish(nil)
	result := <-execution.Result()
	if result.Err != nil {
		t.Fatalf("Result().Err = %v", result.Err)
	}
	if !strings.Contains(result.Evidence.OutputJSON, `"work_item_id":"work-001"`) {
		t.Fatalf("resumed completion evidence = %s", result.Evidence.OutputJSON)
	}
}

func TestDMTCPExecutionRedactsProtectedValuesBeforePublishingResult(t *testing.T) {
	const secret = "dmtcp-secret-do-not-persist"
	t.Setenv("GOET_TEST_DMTCP_SECRET", secret)
	adapter, runner, process, item := newDMTCPLaunchTest(t)
	item.Parameters["checkpoint_secret"] = secretProtectedParameter("GOET_TEST_DMTCP_SECRET", "env", "CHECKPOINT_SECRET")
	execution, err := adapter.StartFresh(context.Background(), item)
	if err != nil {
		t.Fatalf("StartFresh() error = %v", err)
	}
	_, _ = fmt.Fprintln(runner.spec.Stdout, "stdout "+secret)
	_, _ = fmt.Fprintln(runner.spec.Stderr, "stderr "+secret)
	writeDMTCPTestOutput(t, adapter.Worker.Config.TmpDir, item.AttemptID)
	process.finish(nil)
	result := <-execution.Result()
	if result.Err != nil {
		t.Fatalf("Result().Err = %v", result.Err)
	}
	for _, name := range []string{"stdout.log", "stderr.log"} {
		data, err := os.ReadFile(filepath.Join(adapter.Worker.Config.TmpDir, "attempts", item.AttemptID, "logs", name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), secret) || !strings.Contains(string(data), "${worker_env.GOET_TEST_DMTCP_SECRET}") {
			t.Fatalf("%s was not safely redacted: %q", name, data)
		}
	}
}

func TestDMTCPExecutionRejectsSuccessfulProcessWithoutOutput(t *testing.T) {
	adapter, _, process, item := newDMTCPLaunchTest(t)
	execution, err := adapter.StartFresh(context.Background(), item)
	if err != nil {
		t.Fatalf("StartFresh() error = %v", err)
	}
	process.finish(nil)
	result := <-execution.Result()
	if result.Err == nil || !strings.Contains(result.Err.Error(), "missing GOET_OUTPUT_JSON") {
		t.Fatalf("Result().Err = %v", result.Err)
	}
}

func TestDMTCPAdapterStartResumeRejectsRuntimeMismatchBeforeLaunch(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*DMTCPLaunchProfile)
		want   string
	}{
		{name: "build", mutate: func(profile *DMTCPLaunchProfile) { profile.BuildIdentity = "different-build" }, want: "build identity"},
		{name: "adapter", mutate: func(profile *DMTCPLaunchProfile) { profile.AdapterID = "different-adapter" }, want: "adapter id"},
		{name: "adapter version", mutate: func(profile *DMTCPLaunchProfile) { profile.AdapterVersion = "2" }, want: "adapter version"},
		{name: "image", mutate: func(profile *DMTCPLaunchProfile) { profile.ContainerImageIdentity = "sha256:different" }, want: "container image identity"},
		{name: "runtime", mutate: func(profile *DMTCPLaunchProfile) { profile.ContainerRuntime = "different-runtime" }, want: "container runtime"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter, item, assignment := createDMTCPResumeArtifact(t)
			runner := &recordingDMTCPRunner{process: &fakeDMTCPProcess{wait: make(chan error, 1)}}
			adapter.Runner = runner
			test.mutate(&adapter.Profile)
			if _, err := adapter.StartResume(context.Background(), item, assignment); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("StartResume() error = %v, want %q", err, test.want)
			}
			if runner.starts != 0 {
				t.Fatalf("runner starts = %d, want 0", runner.starts)
			}
		})
	}
}

func TestDMTCPAdapterStartResumeRejectsStoredManifestOrImageTampering(t *testing.T) {
	for _, target := range []string{"manifest", "image"} {
		t.Run(target, func(t *testing.T) {
			adapter, item, assignment := createDMTCPResumeArtifact(t)
			runner := &recordingDMTCPRunner{process: &fakeDMTCPProcess{wait: make(chan error, 1)}}
			adapter.Runner = runner
			var manifest model.ResumeArtifactManifest
			if err := json.Unmarshal([]byte(assignment.ManifestJSON), &manifest); err != nil {
				t.Fatal(err)
			}
			artifactRoot := filepath.Join(adapter.Profile.SharedTmpRoot, filepath.FromSlash(manifest.StorageRelativePath))
			if target == "manifest" {
				if err := os.WriteFile(filepath.Join(artifactRoot, "manifest.json"), []byte("{}"), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				image := filepath.Join(artifactRoot, filepath.FromSlash(manifest.DMTCP.CheckpointPaths[0]))
				if err := os.WriteFile(image, []byte("tampered-image"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := adapter.StartResume(context.Background(), item, assignment); err == nil || !strings.Contains(err.Error(), target) {
				t.Fatalf("StartResume() error = %v, want %q", err, target)
			}
			if runner.starts != 0 {
				t.Fatalf("runner starts = %d, want 0", runner.starts)
			}
		})
	}
}

func TestDMTCPAdapterStartResumeRejectsNonCanonicalManifestPath(t *testing.T) {
	adapter, item, assignment := createDMTCPResumeArtifact(t)
	var manifest model.ResumeArtifactManifest
	if err := json.Unmarshal([]byte(assignment.ManifestJSON), &manifest); err != nil {
		t.Fatal(err)
	}
	assignment.Reference.ManifestRelativePath = manifest.StorageRelativePath + "/metadata/manifest.json"
	assignment.Reference.ManifestSHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte(assignment.ManifestJSON)))
	item.Resume = &assignment
	runner := &recordingDMTCPRunner{process: &fakeDMTCPProcess{wait: make(chan error, 1)}}
	adapter.Runner = runner
	if _, err := adapter.StartResume(context.Background(), item, assignment); err == nil || !strings.Contains(err.Error(), "canonical manifest path") {
		t.Fatalf("StartResume() error = %v", err)
	}
	if runner.starts != 0 {
		t.Fatalf("runner starts = %d, want 0", runner.starts)
	}
}

func TestDMTCPAdapterStartResumeRejectsSymlinkedCheckpointImage(t *testing.T) {
	adapter, item, assignment := createDMTCPResumeArtifact(t)
	var manifest model.ResumeArtifactManifest
	if err := json.Unmarshal([]byte(assignment.ManifestJSON), &manifest); err != nil {
		t.Fatal(err)
	}
	image := filepath.Join(adapter.Profile.SharedTmpRoot, filepath.FromSlash(manifest.StorageRelativePath), filepath.FromSlash(manifest.DMTCP.CheckpointPaths[0]))
	outside := filepath.Join(t.TempDir(), "outside.dmtcp")
	if err := os.WriteFile(outside, []byte("checkpoint-image"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(image); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, image); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}
	runner := &recordingDMTCPRunner{process: &fakeDMTCPProcess{wait: make(chan error, 1)}}
	adapter.Runner = runner
	if _, err := adapter.StartResume(context.Background(), item, assignment); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("StartResume() error = %v", err)
	}
	if runner.starts != 0 {
		t.Fatalf("runner starts = %d, want 0", runner.starts)
	}
}

func createDMTCPResumeArtifact(t *testing.T) (DMTCPAdapter, model.WorkItem, model.WorkItemResumeAssignment) {
	t.Helper()
	adapter, _, process, producingItem := newDMTCPCheckpointTest(t, false)
	execution, err := adapter.StartFresh(context.Background(), producingItem)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := execution.CapturePeriodic(context.Background(), dmtcpCaptureRequest(model.CheckpointCaptureKindPeriodic))
	if err != nil {
		t.Fatal(err)
	}
	if err := execution.Terminate(context.Background()); err != nil {
		t.Fatal(err)
	}
	process.finish(errors.New("terminated test producer"))
	<-execution.Result()
	assignment := model.WorkItemResumeAssignment{
		Schema:               model.WorkItemResumeAssignmentSchemaV1,
		ResumedFromAttemptID: producingItem.AttemptID,
		ExecutionLineageID:   "lineage-001",
		ResumeAttemptNumber:  1,
		ManifestJSON:         checkpoint.ManifestJSON,
		Reference:            checkpoint.Reference,
	}
	item := producingItem
	item.AttemptID = "attempt-002"
	item.Resume = &assignment
	return adapter, item, assignment
}

func writeDMTCPTestOutput(t *testing.T, tmpRoot string, attemptID string) {
	t.Helper()
	path := filepath.Join(tmpRoot, "attempts", attemptID, "work", "output.json")
	if err := os.WriteFile(path, []byte(`{"result":"completed","artifacts":[]}`), 0600); err != nil {
		t.Fatal(err)
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
	checkpointOutput          string
	checkpointWaitErr         error
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
			_, _ = fmt.Fprint(spec.Stdout, runner.checkpointOutput)
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
			return runner.checkpointWaitErr
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

type dmtcpTestExitError int

func (err dmtcpTestExitError) Error() string { return fmt.Sprintf("exit status %d", err) }
func (err dmtcpTestExitError) ExitCode() int { return int(err) }

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
			RestartExecutable:              "/opt/dmtcp/bin/dmtcp_restart",
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
	wait       chan error
	mu         sync.Mutex
	kills      int
	finishOnce sync.Once
}

func (process *fakeDMTCPProcess) Wait() error {
	return <-process.wait
}

func (process *fakeDMTCPProcess) Kill() error {
	process.mu.Lock()
	process.kills++
	process.mu.Unlock()
	process.finish(errors.New("process killed"))
	return nil
}

func (process *fakeDMTCPProcess) finish(err error) {
	process.finishOnce.Do(func() {
		process.wait <- err
	})
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

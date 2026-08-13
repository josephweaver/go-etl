package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"goetl/internal/model"
)

func TestRcloneRestartAdapterStartFreshUsesStructuredCopyTo(t *testing.T) {
	adapter, runner, process, item := newRcloneRestartLaunchTest(t)
	execution, err := adapter.StartFresh(context.Background(), item)
	if err != nil {
		t.Fatalf("StartFresh() error = %v", err)
	}
	t.Cleanup(func() { _ = execution.Terminate(context.Background()) })
	if runner.starts != 1 {
		t.Fatalf("runner starts = %d, want 1", runner.starts)
	}
	if runner.spec.Executable != "/opt/rclone/rclone" {
		t.Fatalf("executable = %q", runner.spec.Executable)
	}
	wantDestination := filepath.Join(adapter.Worker.Config.TmpDir, "rclone-restart", item.AttemptID, "download")
	wantArgs := []string{
		"--config", "/run/secrets/rclone.conf",
		"copyto", "landcore:Data/ETL/Test/fixture.bin", wantDestination,
		"--bwlimit", "3M",
	}
	if strings.Join(runner.spec.Args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("args = %#v, want %#v", runner.spec.Args, wantArgs)
	}
	info, err := os.Stat(filepath.Dir(wantDestination))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0700 {
		t.Fatalf("workspace mode = %o, want 700", info.Mode().Perm())
	}
	if err := execution.Terminate(context.Background()); err != nil {
		t.Fatalf("Terminate() error = %v", err)
	}
	if process.kills != 1 {
		t.Fatalf("process kills = %d, want 1", process.kills)
	}
}

func TestRcloneRestartAdapterStartFreshRejectsUnsupportedShapesBeforeLaunch(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*RcloneRestartAdapter, *model.WorkItem)
		wantErr string
	}{
		{name: "provider disabled", mutate: func(adapter *RcloneRestartAdapter, _ *model.WorkItem) {
			adapter.Worker.Config.EnableGDriveRcloneProvider = false
		}, wantErr: "enabled gdrive_rclone"},
		{name: "missing executable", mutate: func(adapter *RcloneRestartAdapter, _ *model.WorkItem) { adapter.Worker.Config.RcloneExecutable = "" }, wantErr: "rclone_executable"},
		{name: "archive", mutate: func(_ *RcloneRestartAdapter, item *model.WorkItem) {
			mutateRcloneRestartAsset(t, item, func(asset *model.BoundDataAsset) {
				asset.Archive = &model.DataAssetArchive{Type: model.DataAssetArchiveTypeZip, Select: []model.DataAssetArchiveSelect{{Member: "a", As: "a"}}, Expose: model.DataAssetArchiveExposeSelectedPath}
			})
		}, wantErr: "archive extraction"},
		{name: "mutable", mutate: func(_ *RcloneRestartAdapter, item *model.WorkItem) {
			mutateRcloneRestartAsset(t, item, func(asset *model.BoundDataAsset) { value := false; asset.Cache.Immutable = &value })
		}, wantErr: "immutable worker_cache"},
		{name: "missing size", mutate: func(_ *RcloneRestartAdapter, item *model.WorkItem) {
			mutateRcloneRestartAsset(t, item, func(asset *model.BoundDataAsset) { asset.Integrity.SizeBytes = nil })
		}, wantErr: "expected size"},
		{name: "unsafe attempt", mutate: func(_ *RcloneRestartAdapter, item *model.WorkItem) { item.AttemptID = "../escape" }, wantErr: "safe path segment"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter, runner, _, item := newRcloneRestartLaunchTest(t)
			test.mutate(&adapter, &item)
			_, err := adapter.StartFresh(context.Background(), item)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("StartFresh() error = %v, want %q", err, test.wantErr)
			}
			if runner.starts != 0 {
				t.Fatalf("runner starts = %d, want 0", runner.starts)
			}
		})
	}
}

func TestRcloneRestartExecutionTerminationIsIdempotent(t *testing.T) {
	adapter, _, process, item := newRcloneRestartLaunchTest(t)
	execution, err := adapter.StartFresh(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	if err := execution.Terminate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := execution.Terminate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if process.kills != 1 {
		t.Fatalf("kills = %d, want 1", process.kills)
	}
}

func TestRcloneRestartExecutionSuspendWritesSafeManifestLast(t *testing.T) {
	adapter, _, process, item := newRcloneRestartLaunchTest(t)
	execution, err := adapter.StartFresh(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(adapter.Worker.Config.TmpDir, "rclone-restart", item.AttemptID)
	if err := os.WriteFile(filepath.Join(workspace, "download.partial"), []byte("ignored partial bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	request := rcloneRestartCaptureRequest(item)
	manifestPath := filepath.Join(adapter.Profile.SharedTmpRoot, filepath.FromSlash(request.ArtifactStorageRelativePath), "manifest.json")
	process.onKill = func() error {
		if _, err := os.Stat(manifestPath); !os.IsNotExist(err) {
			return errors.New("manifest existed before rclone stopped")
		}
		return nil
	}
	checkpoint, err := execution.CaptureForSuspend(context.Background(), request)
	if err != nil {
		t.Fatalf("CaptureForSuspend() error = %v", err)
	}
	registration := PauseAdapterRegistration{
		WorkItemType: model.WorkItemTypeAssetMaterialize, Strategy: model.PauseStrategyNative,
		AdapterID: adapter.Profile.AdapterID, AdapterVersion: adapter.Profile.AdapterVersion,
		Capabilities: PauseAdapterCapabilities{Shutdown: true}, Adapter: adapter,
	}
	manifest, err := checkpoint.Validate(request, registration)
	if err != nil {
		t.Fatalf("PreparedCheckpoint.Validate() error = %v", err)
	}
	if manifest.Native == nil || manifest.Native.BackendIdentity != adapter.Profile.BackendIdentity {
		t.Fatalf("native manifest = %+v", manifest.Native)
	}
	if len(manifest.Files) != 1 || manifest.Files[0].Path != "native/restart-state.json" {
		t.Fatalf("manifest files = %+v", manifest.Files)
	}
	artifactRoot := filepath.Join(adapter.Profile.SharedTmpRoot, filepath.FromSlash(manifest.StorageRelativePath))
	if _, err := os.Stat(filepath.Join(artifactRoot, "download.partial")); !os.IsNotExist(err) {
		t.Fatalf("partial bytes entered artifact: %v", err)
	}
	stateBytes, err := os.ReadFile(filepath.Join(artifactRoot, "native", "restart-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var state rcloneRestartState
	if err := json.Unmarshal(stateBytes, &state); err != nil {
		t.Fatal(err)
	}
	if state.Schema != rcloneRestartStateSchemaV1 || state.ExpectedSizeBytes != *rcloneRestartAsset(t, item).Integrity.SizeBytes {
		t.Fatalf("restart state = %+v", state)
	}
	for _, secret := range []string{"Data/ETL/Test/fixture.bin", "/run/secrets/rclone.conf", "landcore:"} {
		if strings.Contains(string(stateBytes), secret) || strings.Contains(checkpoint.ManifestJSON, secret) {
			t.Fatalf("resume artifact contains protected operation detail %q", secret)
		}
	}
	select {
	case result := <-execution.Result():
		t.Fatalf("capture exposed killed process before supervisor cleanup: %+v", result)
	default:
	}
	if err := execution.Terminate(context.Background()); err != nil {
		t.Fatal(err)
	}
	<-execution.Result()
}

func TestRcloneRestartExecutionRejectsNonFinalCaptureBeforeKill(t *testing.T) {
	adapter, _, process, item := newRcloneRestartLaunchTest(t)
	execution, err := adapter.StartFresh(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	request := rcloneRestartCaptureRequest(item)
	request.CaptureKind = model.CheckpointCaptureKindQuantum
	if _, err := execution.CaptureForSuspend(context.Background(), request); err == nil || !strings.Contains(err.Error(), "final capture only") {
		t.Fatalf("CaptureForSuspend() error = %v", err)
	}
	if process.kills != 0 {
		t.Fatalf("kills = %d, want 0", process.kills)
	}
	_ = execution.Terminate(context.Background())
}

func TestRcloneRestartAdapterStartResumeValidatesRecipeAndLaunchesFullReplacement(t *testing.T) {
	adapter, item, assignment := newRcloneRestartResumeTest(t)
	process := &fakeRcloneRestartProcess{wait: make(chan error, 1)}
	runner := &recordingRcloneRestartRunner{process: process}
	adapter.Runner = runner

	workspace := filepath.Join(adapter.Worker.Config.TmpDir, "rclone-restart", item.AttemptID)
	if err := os.MkdirAll(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(workspace, "download.transfer.partial")
	if err := os.WriteFile(stale, []byte("stale partial bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	execution, err := adapter.StartResume(context.Background(), item, assignment)
	if err != nil {
		t.Fatalf("StartResume() error = %v", err)
	}
	t.Cleanup(func() { _ = execution.Terminate(context.Background()) })
	if runner.starts != 1 {
		t.Fatalf("runner starts = %d, want 1", runner.starts)
	}
	wantDestination := filepath.Join(workspace, "download")
	wantArgs := []string{
		"--config", "/run/secrets/rclone.conf",
		"copyto", "landcore:Data/ETL/Test/fixture.bin", wantDestination,
		"--bwlimit", "3M",
	}
	if strings.Join(runner.spec.Args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("args = %#v, want %#v", runner.spec.Args, wantArgs)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale partial remains after resume: %v", err)
	}
}

func TestRcloneRestartAdapterStartResumeRejectsInvalidRecipeBeforeLaunch(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*RcloneRestartAdapter, *model.WorkItem, *model.WorkItemResumeAssignment)
		wantErr string
	}{
		{name: "assignment differs from item", mutate: func(_ *RcloneRestartAdapter, _ *model.WorkItem, assignment *model.WorkItemResumeAssignment) {
			assignment.ResumeAttemptNumber++
		}, wantErr: "does not match work item"},
		{name: "backend changed", mutate: func(adapter *RcloneRestartAdapter, _ *model.WorkItem, _ *model.WorkItemResumeAssignment) {
			adapter.Profile.BackendIdentity = "rclone-different"
		}, wantErr: "backend identity"},
		{name: "stored manifest changed", mutate: func(adapter *RcloneRestartAdapter, _ *model.WorkItem, assignment *model.WorkItemResumeAssignment) {
			manifestPath := filepath.Join(adapter.Profile.SharedTmpRoot, filepath.FromSlash(assignment.Reference.ManifestRelativePath))
			if err := os.Chmod(manifestPath, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(manifestPath, append([]byte(assignment.ManifestJSON), ' '), 0600); err != nil {
				t.Fatal(err)
			}
		}, wantErr: "stored rclone restart manifest bytes"},
		{name: "state bytes changed", mutate: func(adapter *RcloneRestartAdapter, _ *model.WorkItem, assignment *model.WorkItemResumeAssignment) {
			var manifest model.ResumeArtifactManifest
			if err := json.Unmarshal([]byte(assignment.ManifestJSON), &manifest); err != nil {
				t.Fatal(err)
			}
			statePath := filepath.Join(adapter.Profile.SharedTmpRoot, filepath.FromSlash(manifest.StorageRelativePath), "native", "restart-state.json")
			if err := os.Chmod(statePath, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(statePath, []byte("{}"), 0600); err != nil {
				t.Fatal(err)
			}
		}, wantErr: "size"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter, item, assignment := newRcloneRestartResumeTest(t)
			process := &fakeRcloneRestartProcess{wait: make(chan error, 1)}
			runner := &recordingRcloneRestartRunner{process: process}
			adapter.Runner = runner
			test.mutate(&adapter, &item, &assignment)
			_, err := adapter.StartResume(context.Background(), item, assignment)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("StartResume() error = %v, want %q", err, test.wantErr)
			}
			if runner.starts != 0 {
				t.Fatalf("runner starts = %d, want 0", runner.starts)
			}
		})
	}
}

func TestRcloneRestartExecutionCompletesWithOrdinaryAssetMaterializeEvidence(t *testing.T) {
	adapter, item, assignment := newRcloneRestartResumeTest(t)
	process := &fakeRcloneRestartProcess{wait: make(chan error, 1)}
	runner := &recordingRcloneRestartRunner{process: process}
	adapter.Runner = runner
	execution, err := adapter.StartResume(context.Background(), item, assignment)
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(adapter.Worker.Config.TmpDir, "rclone-restart", item.AttemptID, "download")
	if err := os.WriteFile(destination, []byte(rcloneRestartFixtureContent), 0600); err != nil {
		t.Fatal(err)
	}
	process.finish(nil)
	result := receiveRcloneRestartResult(t, execution)
	if result.Err != nil {
		t.Fatalf("execution error = %v", result.Err)
	}
	manifest := decodeAssetMaterializeOutput(t, result.Evidence.OutputJSON)
	materialized := manifest.Assets[0]
	wantDestination := filepath.Join(adapter.Worker.Config.effectiveAssetCacheDir(), filepath.FromSlash("materialized/os011/fixture.bin"))
	if materialized.LocalPath != wantDestination || materialized.SourceSHA256 != sha256Text(rcloneRestartFixtureContent) {
		t.Fatalf("materialized asset = %+v", materialized)
	}
	if _, err := os.Stat(wantDestination); err != nil {
		t.Fatalf("promoted destination: %v", err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("attempt download remains after promotion: %v", err)
	}
	if runner.starts != 1 {
		t.Fatalf("runner starts = %d, want 1", runner.starts)
	}
}

func TestRcloneRestartExecutionRejectsIntegrityMismatch(t *testing.T) {
	adapter, _, process, item := newRcloneRestartLaunchTest(t)
	execution, err := adapter.StartFresh(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(adapter.Worker.Config.TmpDir, "rclone-restart", item.AttemptID, "download")
	if err := os.WriteFile(destination, []byte("wrong"), 0600); err != nil {
		t.Fatal(err)
	}
	process.finish(nil)
	result := receiveRcloneRestartResult(t, execution)
	if result.Err == nil || !strings.Contains(result.Err.Error(), "expected size") {
		t.Fatalf("execution error = %v", result.Err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("invalid download remains: %v", err)
	}
}

func TestRcloneRestartProfileRegistersShutdownOnlyNativeAdapter(t *testing.T) {
	config := validRcloneRestartWorkerConfig(t)
	worker := Worker{Config: config}
	registry, err := worker.pauseAdapterRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registration, ok := registry.AdapterFor(model.WorkItemTypeAssetMaterialize)
	if !ok || registry.Len() != 1 {
		t.Fatalf("registry = %+v", registry)
	}
	if registration.Strategy != model.PauseStrategyNative || registration.AdapterID != config.RcloneRestartProfile.AdapterID || registration.AdapterVersion != config.RcloneRestartProfile.AdapterVersion {
		t.Fatalf("registration = %+v", registration)
	}
	if !registration.Capabilities.Supports(CheckpointModeShutdown) || registration.Capabilities.Supports(CheckpointModePeriodic) || registration.Capabilities.Supports(CheckpointModeYield) {
		t.Fatalf("capabilities = %+v", registration.Capabilities)
	}
	adapter, ok := registration.Adapter.(*RcloneRestartAdapter)
	if !ok || adapter.Profile.BackendIdentity != config.RcloneRestartProfile.BackendIdentity {
		t.Fatalf("adapter = %#v", registration.Adapter)
	}
}

func TestRcloneRestartProfileValidationIsExplicitAndShutdownOnly(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{name: "valid", mutate: func(*Config) {}},
		{name: "periodic", mutate: func(config *Config) {
			config.CheckpointMode = CheckpointModePeriodic
			config.CheckpointIntervalSeconds = 30
		}, wantErr: "requires checkpoint_mode \"shutdown\""},
		{name: "yield", mutate: func(config *Config) {
			config.CheckpointMode = CheckpointModeYield
			config.WorkItemExecutionQuantumSeconds = 30
		}, wantErr: "requires checkpoint_mode \"shutdown\""},
		{name: "provider disabled", mutate: func(config *Config) { config.EnableGDriveRcloneProvider = false }, wantErr: "enabled gdrive_rclone"},
		{name: "missing profile", mutate: func(config *Config) { config.RcloneRestartProfile = nil }, wantErr: "requires rclone_restart_profile"},
		{name: "profile without selector", mutate: func(config *Config) { config.PauseAdapterProfile = "" }, wantErr: "requires pause_adapter_profile"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := validRcloneRestartWorkerConfig(t)
			test.mutate(&config)
			err := config.ValidateRuntime()
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateRuntime() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("ValidateRuntime() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestRcloneRestartProfileOmissionLeavesAdapterRegistryEmpty(t *testing.T) {
	registry, err := (Worker{Config: Config{}}).pauseAdapterRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if registry.Len() != 0 {
		t.Fatalf("registry length = %d, want 0", registry.Len())
	}
}

func validRcloneRestartWorkerConfig(t *testing.T) Config {
	t.Helper()
	root := t.TempDir()
	return Config{
		LogDir: filepath.Join(root, "logs"), TmpDir: filepath.Join(root, "tmp"), DataDir: filepath.Join(root, "data"),
		CheckpointMode: CheckpointModeShutdown, DrainPauseDelaySeconds: 10,
		CheckpointCaptureTimeoutSeconds: 1, CheckpointReportTimeoutSeconds: 1,
		ExecutionTerminationGraceSeconds: 1,
		PauseAdapterProfile:              PauseAdapterProfileGDriveRcloneRestart1712,
		EnableGDriveRcloneProvider:       true, RcloneExecutable: "/opt/rclone/rclone",
		RcloneConfigPath: "/run/secrets/rclone.conf",
		RcloneRestartProfile: &RcloneRestartProfileConfig{
			SharedTmpRoot: filepath.Join(root, "shared"), AdapterID: "gdrive-rclone-restart",
			AdapterVersion: "1", WorkerExecutionContractVersion: "goet/worker-execution/v1",
			WorkerVersion: "os011-test", ContainerImageIdentity: "goetl/worker:test",
			OperatingSystem: "linux", Architecture: "amd64", ContainerRuntime: "docker",
			BackendIdentity: "rclone-1.71.2:gdrive:copyto-full-restart-v1",
		},
	}
}

func newRcloneRestartResumeTest(t *testing.T) (RcloneRestartAdapter, model.WorkItem, model.WorkItemResumeAssignment) {
	t.Helper()
	adapter, _, _, producingItem := newRcloneRestartLaunchTest(t)
	execution, err := adapter.StartFresh(context.Background(), producingItem)
	if err != nil {
		t.Fatal(err)
	}
	request := rcloneRestartCaptureRequest(producingItem)
	checkpoint, err := execution.CaptureForSuspend(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := execution.Terminate(context.Background()); err != nil {
		t.Fatal(err)
	}
	assignment := model.WorkItemResumeAssignment{
		Schema: model.WorkItemResumeAssignmentSchemaV1, ResumedFromAttemptID: producingItem.AttemptID,
		ExecutionLineageID: request.ExecutionLineageID, ResumeAttemptNumber: 1,
		ManifestJSON: checkpoint.ManifestJSON, Reference: checkpoint.Reference,
	}
	resumedItem := producingItem
	resumedItem.AttemptID = "attempt-os011-resume"
	resumedItem.Resume = &assignment
	return adapter, resumedItem, assignment
}

func newRcloneRestartLaunchTest(t *testing.T) (RcloneRestartAdapter, *recordingRcloneRestartRunner, *fakeRcloneRestartProcess, model.WorkItem) {
	t.Helper()
	root := t.TempDir()
	size := int64(len(rcloneRestartFixtureContent))
	asset := gdriveRcloneAsset("Data/ETL/Test/fixture.bin", "gdrive/os011/fixture.bin", stringPointer(sha256Text(rcloneRestartFixtureContent)), &size)
	asset.TransferPolicy.ProviderArgs = map[string]string{"rclone_bwlimit": "3M"}
	payload := assetMaterializePayloadForTest(asset)
	payload.MaterializationDomainID = "os011-test-domain"
	payload.DestinationRelativePath = "materialized/os011/fixture.bin"
	payload.MaterializationKey = "sha256:" + sha256Text(payload.DestinationRelativePath)
	item := AssetMaterializeTestItem(payload, asset)
	item.AttemptID = "attempt-os011"
	item.InputFingerprint = "os011-input-v1"
	item.CodeVersion = "os011-code-v1"
	process := &fakeRcloneRestartProcess{wait: make(chan error, 1)}
	runner := &recordingRcloneRestartRunner{process: process}
	adapter := RcloneRestartAdapter{
		Worker: Worker{Config: Config{
			TmpDir:                     filepath.Join(root, "tmp"),
			DataDir:                    filepath.Join(root, "data"),
			EnableGDriveRcloneProvider: true,
			RcloneExecutable:           "/opt/rclone/rclone",
			RcloneConfigPath:           "/run/secrets/rclone.conf",
		}},
		Runner: runner,
		Profile: RcloneRestartProfile{
			SharedTmpRoot: filepath.Join(root, "shared"), AdapterID: "gdrive-rclone-restart",
			AdapterVersion: "1", WorkerExecutionContractVersion: "goet/worker-execution/v1",
			WorkerVersion: "os011-test", ContainerImageIdentity: "goetl/worker:test",
			OperatingSystem: "linux", Architecture: "amd64", ContainerRuntime: "docker",
			BackendIdentity: "rclone-1.71.2:gdrive:copyto-full-restart-v1",
		},
	}
	return adapter, runner, process, item
}

const rcloneRestartFixtureContent = "OS-011 immutable rclone restart fixture\n"

func receiveRcloneRestartResult(t *testing.T, execution SupervisedExecution) ExecutionResult {
	t.Helper()
	select {
	case result := <-execution.Result():
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for rclone restart result")
		return ExecutionResult{}
	}
}

func rcloneRestartCaptureRequest(item model.WorkItem) CheckpointCaptureRequest {
	return CheckpointCaptureRequest{
		WorkItemID: item.ID, WorkItemType: item.Type, AttemptID: item.AttemptID,
		ExecutionLineageID: "lineage-os011", ResumeGeneration: 1,
		ResumeArtifactID: "artifact-os011-001", ArtifactStorageRelativePath: "goetl/resume/artifact-os011-001",
		CaptureKind: model.CheckpointCaptureKindFinal,
	}
}

func rcloneRestartAsset(t *testing.T, item model.WorkItem) model.BoundDataAsset {
	t.Helper()
	assets, err := boundDataAssetsFromWorkItem(item)
	if err != nil || len(assets) != 1 {
		t.Fatalf("bound assets = %+v, error = %v", assets, err)
	}
	return assets[0]
}

func mutateRcloneRestartAsset(t *testing.T, item *model.WorkItem, mutate func(*model.BoundDataAsset)) {
	t.Helper()
	assets, err := boundDataAssetsFromWorkItem(*item)
	if err != nil {
		t.Fatal(err)
	}
	mutate(&assets[0])
	item.Parameters["data_assets"] = model.Parameter{Type: "data_assets", Value: assets}
	payload := assetMaterializePayloadForTest(assets[0])
	item.Parameters["asset_materialize"] = model.Parameter{Type: "asset_materialize", Value: payload}
}

func stringPointer(value string) *string { return &value }

type recordingRcloneRestartRunner struct {
	process *fakeRcloneRestartProcess
	spec    RcloneRestartCommandSpec
	starts  int
}

func (runner *recordingRcloneRestartRunner) Start(_ context.Context, spec RcloneRestartCommandSpec) (RcloneRestartProcess, error) {
	runner.starts++
	runner.spec = spec
	return runner.process, nil
}

type fakeRcloneRestartProcess struct {
	wait       chan error
	finishOnce sync.Once
	mu         sync.Mutex
	kills      int
	onKill     func() error
}

func (process *fakeRcloneRestartProcess) Wait() error { return <-process.wait }

func (process *fakeRcloneRestartProcess) Kill() error {
	process.mu.Lock()
	process.kills++
	process.mu.Unlock()
	if process.onKill != nil {
		if err := process.onKill(); err != nil {
			return err
		}
	}
	process.finish(errors.New("process killed"))
	return nil
}

func (process *fakeRcloneRestartProcess) finish(err error) {
	process.finishOnce.Do(func() { process.wait <- err })
}

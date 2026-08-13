package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"goetl/internal/model"
)

// RcloneRestartAdapter supervises the restart-only transfer shape proven by
// OS-010. Capture, resume validation, and completion promotion are added in
// later OS-011 increments.
type RcloneRestartAdapter struct {
	Worker  Worker
	Runner  RcloneRestartCommandRunner
	Profile RcloneRestartProfile
	Now     func() time.Time
}

const rcloneRestartStateSchemaV1 = "goetl/rclone-restart-state/v1"

type RcloneRestartProfile struct {
	SharedTmpRoot                  string
	AdapterID                      string
	AdapterVersion                 string
	WorkerExecutionContractVersion string
	WorkerVersion                  string
	ContainerImageIdentity         string
	OperatingSystem                string
	Architecture                   string
	ContainerRuntime               string
	BackendIdentity                string
}

func (profile RcloneRestartProfile) validate() error {
	for _, field := range []struct{ name, value string }{
		{"shared temporary root", profile.SharedTmpRoot},
		{"adapter id", profile.AdapterID},
		{"adapter version", profile.AdapterVersion},
		{"worker execution contract version", profile.WorkerExecutionContractVersion},
		{"worker version", profile.WorkerVersion},
		{"container image identity", profile.ContainerImageIdentity},
		{"operating system", profile.OperatingSystem},
		{"architecture", profile.Architecture},
		{"container runtime", profile.ContainerRuntime},
		{"backend identity", profile.BackendIdentity},
	} {
		if err := validateAdapterContractValue(field.name, field.value); err != nil {
			return err
		}
	}
	if !filepath.IsAbs(profile.SharedTmpRoot) {
		return fmt.Errorf("shared temporary root must be absolute")
	}
	return nil
}

type RcloneRestartCommandSpec struct {
	Executable string
	Args       []string
	Stdout     io.Writer
	Stderr     io.Writer
}

type RcloneRestartProcess interface {
	Wait() error
	Kill() error
}

type RcloneRestartCommandRunner interface {
	Start(context.Context, RcloneRestartCommandSpec) (RcloneRestartProcess, error)
}

type execRcloneRestartCommandRunner struct{}

func (execRcloneRestartCommandRunner) Start(ctx context.Context, spec RcloneRestartCommandSpec) (RcloneRestartProcess, error) {
	command := exec.CommandContext(ctx, spec.Executable, spec.Args...)
	command.Stdout = spec.Stdout
	command.Stderr = spec.Stderr
	if err := command.Start(); err != nil {
		return nil, err
	}
	return execRcloneRestartProcess{command: command}, nil
}

type execRcloneRestartProcess struct {
	command *exec.Cmd
}

func (process execRcloneRestartProcess) Wait() error { return process.command.Wait() }
func (process execRcloneRestartProcess) Kill() error { return process.command.Process.Kill() }

type rcloneRestartWorkspace struct {
	Root        string
	Destination string
	StdoutPath  string
	StderrPath  string
}

func (adapter RcloneRestartAdapter) StartFresh(ctx context.Context, item model.WorkItem) (SupervisedExecution, error) {
	if ctx == nil {
		return nil, fmt.Errorf("rclone restart launch context is required")
	}
	if item.Type != model.WorkItemTypeAssetMaterialize {
		return nil, fmt.Errorf("rclone restart adapter does not support work item type %q", item.Type)
	}
	if item.Resume != nil {
		return nil, fmt.Errorf("fresh rclone restart launch cannot consume a resume assignment")
	}
	if err := adapter.Profile.validate(); err != nil {
		return nil, fmt.Errorf("rclone restart profile: %w", err)
	}
	if err := item.Validate(); err != nil {
		return nil, fmt.Errorf("validate rclone restart work item: %w", err)
	}
	if err := validateAdapterArtifactID(item.AttemptID); err != nil {
		return nil, fmt.Errorf("attempt id: %w", err)
	}
	payload, asset, err := AssetMaterializePayloadFromWorkItem(item)
	if err != nil {
		return nil, err
	}
	if err := adapter.validateFreshAsset(payload, asset); err != nil {
		return nil, err
	}
	return adapter.startValidated(ctx, item, payload, asset)
}

func (adapter RcloneRestartAdapter) StartResume(ctx context.Context, item model.WorkItem, assignment model.WorkItemResumeAssignment) (SupervisedExecution, error) {
	if ctx == nil {
		return nil, fmt.Errorf("rclone restart launch context is required")
	}
	if item.Type != model.WorkItemTypeAssetMaterialize {
		return nil, fmt.Errorf("rclone restart adapter does not support work item type %q", item.Type)
	}
	if item.Resume == nil {
		return nil, fmt.Errorf("rclone restart resume launch requires a work-item resume assignment")
	}
	if *item.Resume != assignment {
		return nil, fmt.Errorf("rclone restart resume assignment does not match work item")
	}
	if err := adapter.Profile.validate(); err != nil {
		return nil, fmt.Errorf("rclone restart profile: %w", err)
	}
	if err := item.Validate(); err != nil {
		return nil, fmt.Errorf("validate rclone restart work item: %w", err)
	}
	if err := validateAdapterArtifactID(item.AttemptID); err != nil {
		return nil, fmt.Errorf("attempt id: %w", err)
	}
	payload, asset, err := AssetMaterializePayloadFromWorkItem(item)
	if err != nil {
		return nil, err
	}
	if err := adapter.validateFreshAsset(payload, asset); err != nil {
		return nil, err
	}
	if err := adapter.validateResumeArtifact(item, assignment, payload, asset); err != nil {
		return nil, err
	}
	return adapter.startValidated(ctx, item, payload, asset)
}

func (adapter RcloneRestartAdapter) startValidated(ctx context.Context, item model.WorkItem, payload model.AssetMaterializeWorkItemPayload, asset model.BoundDataAsset) (SupervisedExecution, error) {

	workspace, stdout, stderr, err := createRcloneRestartWorkspace(adapter.Worker.Config.TmpDir, item.AttemptID)
	if err != nil {
		return nil, err
	}
	provider := gdriveRcloneProvider{
		executable: adapter.Worker.Config.RcloneExecutable,
		configPath: adapter.Worker.Config.RcloneConfigPath,
	}
	remotePath := asset.Location.Remote + ":" + asset.Location.DrivePath
	spec := RcloneRestartCommandSpec{
		Executable: provider.executable,
		Args:       provider.copyToArgs(remotePath, workspace.Destination, asset.TransferPolicy),
		Stdout:     stdout,
		Stderr:     stderr,
	}
	runner := adapter.Runner
	if runner == nil {
		runner = execRcloneRestartCommandRunner{}
	}
	process, err := runner.Start(ctx, spec)
	if err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, fmt.Errorf("launch rclone restart transfer: %w", err)
	}
	execution := &rcloneRestartExecution{
		process:   process,
		result:    make(chan ExecutionResult, 1),
		done:      make(chan struct{}),
		worker:    adapter.Worker,
		profile:   adapter.Profile,
		workspace: workspace,
		item:      item,
		payload:   payload,
		asset:     asset,
		now:       adapter.Now,
	}
	if execution.now == nil {
		execution.now = time.Now
	}
	go execution.wait(stdout, stderr)
	return execution, nil
}

func (adapter RcloneRestartAdapter) validateResumeArtifact(item model.WorkItem, assignment model.WorkItemResumeAssignment, payload model.AssetMaterializeWorkItemPayload, asset model.BoundDataAsset) error {
	var manifest model.ResumeArtifactManifest
	if err := decodeCanonicalRcloneRestartJSON([]byte(assignment.ManifestJSON), &manifest); err != nil {
		return fmt.Errorf("decode rclone restart resume manifest: %w", err)
	}
	if manifest.PauseStrategy != model.PauseStrategyNative || manifest.Native == nil {
		return fmt.Errorf("resume artifact is not a native rclone restart generation")
	}
	if manifest.Native.Operation != string(model.WorkItemTypeAssetMaterialize) || manifest.Native.AdapterVersion != adapter.Profile.AdapterVersion || manifest.Native.BackendIdentity != adapter.Profile.BackendIdentity {
		return fmt.Errorf("rclone restart native operation, adapter version, or backend identity does not match launch profile")
	}
	for _, match := range []struct{ name, got, want string }{
		{"adapter id", manifest.Compatibility.AdapterID, adapter.Profile.AdapterID},
		{"adapter version", manifest.Compatibility.AdapterVersion, adapter.Profile.AdapterVersion},
		{"worker execution contract version", manifest.Compatibility.WorkerExecutionContractVersion, adapter.Profile.WorkerExecutionContractVersion},
		{"worker version", manifest.Compatibility.WorkerVersion, adapter.Profile.WorkerVersion},
		{"container image identity", manifest.Compatibility.ContainerImageIdentity, adapter.Profile.ContainerImageIdentity},
		{"operating system", manifest.Compatibility.OperatingSystem, adapter.Profile.OperatingSystem},
		{"architecture", manifest.Compatibility.Architecture, adapter.Profile.Architecture},
		{"container runtime", manifest.Compatibility.ContainerRuntime, adapter.Profile.ContainerRuntime},
	} {
		if match.got != match.want {
			return fmt.Errorf("rclone restart resume %s does not match launch profile", match.name)
		}
	}
	if manifest.InputFingerprint != item.InputFingerprint || manifest.CodeVersion != item.CodeVersion || manifest.SourceVersion != payload.AssetKey {
		return fmt.Errorf("rclone restart resume input, source, or code identity does not match assigned work item")
	}
	if len(manifest.Files) != 1 || len(manifest.Native.StateFilePaths) != 1 || manifest.Files[0].Path != "native/restart-state.json" || manifest.Native.StateFilePaths[0] != manifest.Files[0].Path {
		return fmt.Errorf("rclone restart artifact must contain exactly its canonical state file")
	}
	artifactRoot, err := resolveDMTCPArtifactPath(adapter.Profile.SharedTmpRoot, manifest.StorageRelativePath)
	if err != nil {
		return fmt.Errorf("rclone restart artifact path: %w", err)
	}
	manifestPath, err := resolveDMTCPArtifactPath(adapter.Profile.SharedTmpRoot, assignment.Reference.ManifestRelativePath)
	if err != nil {
		return fmt.Errorf("rclone restart manifest path: %w", err)
	}
	if manifestPath != filepath.Join(artifactRoot, "manifest.json") {
		return fmt.Errorf("rclone restart manifest path is not the artifact's canonical manifest path")
	}
	if err := validateDMTCPPathComponents(adapter.Profile.SharedTmpRoot, manifestPath, false); err != nil {
		return fmt.Errorf("rclone restart manifest: %w", err)
	}
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read rclone restart resume manifest: %w", err)
	}
	if string(manifestBytes) != assignment.ManifestJSON {
		return fmt.Errorf("stored rclone restart manifest bytes do not match resume assignment")
	}
	statePath, err := resolveDMTCPPathInside(artifactRoot, manifest.Files[0].Path)
	if err != nil {
		return fmt.Errorf("rclone restart state path: %w", err)
	}
	if err := validateDMTCPPathComponents(adapter.Profile.SharedTmpRoot, statePath, false); err != nil {
		return fmt.Errorf("rclone restart state: %w", err)
	}
	if err := validateDMTCPResumeFile(statePath, manifest.Files[0]); err != nil {
		return fmt.Errorf("rclone restart state: %w", err)
	}
	stateBytes, err := os.ReadFile(statePath)
	if err != nil {
		return fmt.Errorf("read rclone restart state: %w", err)
	}
	var state rcloneRestartState
	if err := decodeCanonicalRcloneRestartJSON(stateBytes, &state); err != nil {
		return fmt.Errorf("decode rclone restart state: %w", err)
	}
	if state.Schema != rcloneRestartStateSchemaV1 || state.WorkItemID != item.ID || state.WorkItemType != string(item.Type) || state.InputFingerprint != item.InputFingerprint || state.SourceVersion != payload.AssetKey || state.CodeVersion != item.CodeVersion {
		return fmt.Errorf("rclone restart state work identity does not match assigned work item")
	}
	if state.AssetKey != payload.AssetKey || state.MaterializationKey != payload.MaterializationKey || state.ExpectedSizeBytes != *asset.Integrity.SizeBytes || state.ExpectedSHA256 != asset.Integrity.SHA256 {
		return fmt.Errorf("rclone restart state asset identity does not match assigned materialization")
	}
	if state.AdapterID != adapter.Profile.AdapterID || state.AdapterVersion != adapter.Profile.AdapterVersion || state.BackendIdentity != adapter.Profile.BackendIdentity || state.CommandContract != "gdrive-rclone-copyto-full-restart-v1" {
		return fmt.Errorf("rclone restart state execution contract does not match launch profile")
	}
	return nil
}

func decodeCanonicalRcloneRestartJSON(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("trailing JSON content")
	}
	canonical, err := json.Marshal(destination)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, canonical) {
		return fmt.Errorf("JSON is not in canonical form")
	}
	return nil
}

func (adapter RcloneRestartAdapter) validateFreshAsset(payload model.AssetMaterializeWorkItemPayload, asset model.BoundDataAsset) error {
	config := adapter.Worker.Config
	if !config.EnableGDriveRcloneProvider {
		return fmt.Errorf("rclone restart adapter requires enabled gdrive_rclone provider")
	}
	if strings.TrimSpace(config.RcloneExecutable) == "" {
		return fmt.Errorf("rclone restart adapter requires configured rclone_executable")
	}
	if asset.Provider != model.DataProviderGDriveRclone || asset.Location.Type != model.DataProviderGDriveRclone {
		return fmt.Errorf("rclone restart adapter requires gdrive_rclone asset")
	}
	if payload.ProviderType != asset.Provider || payload.ResolvedLocation != asset.Location {
		return fmt.Errorf("rclone restart asset does not match materialization payload")
	}
	if asset.Location.FileID != "" {
		return fmt.Errorf("rclone restart file_id access is not implemented")
	}
	if asset.Archive != nil || payload.Archive != nil {
		return fmt.Errorf("rclone restart adapter does not support archive extraction")
	}
	strategy, err := materializationStrategy(asset)
	if err != nil {
		return err
	}
	if strategy != model.DataAssetCacheStrategyWorkerCache || !asset.Cache.EffectiveImmutable() {
		return fmt.Errorf("rclone restart adapter requires immutable worker_cache materialization")
	}
	if asset.Integrity.SizeBytes == nil || *asset.Integrity.SizeBytes < 1 || asset.Integrity.SHA256 == "" {
		return fmt.Errorf("rclone restart adapter requires expected size and SHA-256")
	}
	if payload.Integrity.SHA256 != asset.Integrity.SHA256 || payload.Integrity.SizeBytes == nil || *payload.Integrity.SizeBytes != *asset.Integrity.SizeBytes {
		return fmt.Errorf("rclone restart integrity does not match materialization payload")
	}
	return nil
}

func createRcloneRestartWorkspace(tmpRoot string, attemptID string) (rcloneRestartWorkspace, *os.File, *os.File, error) {
	if strings.TrimSpace(tmpRoot) == "" {
		return rcloneRestartWorkspace{}, nil, nil, fmt.Errorf("rclone restart temporary root is required")
	}
	workspace := rcloneRestartWorkspace{
		Root:        filepath.Join(tmpRoot, "rclone-restart", attemptID),
		Destination: filepath.Join(tmpRoot, "rclone-restart", attemptID, "download"),
		StdoutPath:  filepath.Join(tmpRoot, "rclone-restart", attemptID, "stdout.log"),
		StderrPath:  filepath.Join(tmpRoot, "rclone-restart", attemptID, "stderr.log"),
	}
	if err := os.MkdirAll(workspace.Root, 0700); err != nil {
		return rcloneRestartWorkspace{}, nil, nil, fmt.Errorf("create rclone restart workspace: %w", err)
	}
	if err := os.Chmod(workspace.Root, 0700); err != nil {
		return rcloneRestartWorkspace{}, nil, nil, fmt.Errorf("protect rclone restart workspace: %w", err)
	}
	if err := clearRcloneRestartTransferFiles(workspace); err != nil {
		return rcloneRestartWorkspace{}, nil, nil, err
	}
	stdout, err := os.OpenFile(workspace.StdoutPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return rcloneRestartWorkspace{}, nil, nil, fmt.Errorf("create rclone stdout log: %w", err)
	}
	stderr, err := os.OpenFile(workspace.StderrPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		_ = stdout.Close()
		return rcloneRestartWorkspace{}, nil, nil, fmt.Errorf("create rclone stderr log: %w", err)
	}
	return workspace, stdout, stderr, nil
}

func clearRcloneRestartTransferFiles(workspace rcloneRestartWorkspace) error {
	entries, err := os.ReadDir(workspace.Root)
	if err != nil {
		return fmt.Errorf("inspect rclone restart workspace: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if name != "download" && !(strings.HasPrefix(name, "download.") && strings.HasSuffix(name, ".partial")) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect stale rclone transfer file: %w", err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("stale rclone transfer path %q is not a regular file", name)
		}
		if err := os.Remove(filepath.Join(workspace.Root, name)); err != nil {
			return fmt.Errorf("remove stale rclone transfer file: %w", err)
		}
	}
	return nil
}

type rcloneRestartExecution struct {
	process   RcloneRestartProcess
	result    chan ExecutionResult
	done      chan struct{}
	worker    Worker
	profile   RcloneRestartProfile
	workspace rcloneRestartWorkspace
	item      model.WorkItem
	payload   model.AssetMaterializeWorkItemPayload
	asset     model.BoundDataAsset
	now       func() time.Time

	mu              sync.Mutex
	processDone     bool
	terminal        ExecutionResult
	suspendCapture  bool
	terminating     bool
	resultPublished bool
	kill            sync.Once
	killErr         error
}

func (execution *rcloneRestartExecution) Result() <-chan ExecutionResult { return execution.result }

func (*rcloneRestartExecution) CapturePeriodic(context.Context, CheckpointCaptureRequest) (PreparedCheckpoint, error) {
	return PreparedCheckpoint{}, fmt.Errorf("rclone restart adapter does not support periodic capture")
}

func (execution *rcloneRestartExecution) CaptureForSuspend(ctx context.Context, request CheckpointCaptureRequest) (PreparedCheckpoint, error) {
	if err := request.Validate(); err != nil {
		return PreparedCheckpoint{}, fmt.Errorf("rclone restart capture request: %w", err)
	}
	if request.CaptureKind != model.CheckpointCaptureKindFinal {
		return PreparedCheckpoint{}, fmt.Errorf("rclone restart adapter supports final capture only")
	}
	if request.WorkItemID != execution.item.ID || request.WorkItemType != execution.item.Type || request.AttemptID != execution.item.AttemptID {
		return PreparedCheckpoint{}, fmt.Errorf("rclone restart capture request does not match running work item")
	}
	execution.mu.Lock()
	if execution.processDone {
		execution.mu.Unlock()
		return PreparedCheckpoint{}, fmt.Errorf("rclone process exited before suspending capture")
	}
	execution.suspendCapture = true
	execution.mu.Unlock()
	if err := execution.killAndWait(ctx, false); err != nil {
		return PreparedCheckpoint{}, fmt.Errorf("stop rclone for restart capture: %w", err)
	}
	return execution.writeRestartArtifact(request)
}

func (*rcloneRestartExecution) Continue(context.Context) error {
	return fmt.Errorf("rclone restart continuation is not implemented")
}

func (execution *rcloneRestartExecution) Terminate(ctx context.Context) error {
	return execution.killAndWait(ctx, true)
}

func (execution *rcloneRestartExecution) killAndWait(ctx context.Context, terminating bool) error {
	if terminating {
		execution.mu.Lock()
		execution.terminating = true
		if execution.processDone {
			execution.publishResultLocked()
		}
		execution.mu.Unlock()
	}
	execution.kill.Do(func() {
		execution.mu.Lock()
		alreadyDone := execution.processDone
		execution.mu.Unlock()
		if !alreadyDone {
			execution.killErr = execution.process.Kill()
			if errors.Is(execution.killErr, os.ErrProcessDone) {
				execution.killErr = nil
			}
		}
	})
	if execution.killErr != nil {
		return execution.killErr
	}
	select {
	case <-execution.done:
		if terminating {
			execution.mu.Lock()
			execution.publishResultLocked()
			execution.mu.Unlock()
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (execution *rcloneRestartExecution) wait(stdout *os.File, stderr *os.File) {
	defer close(execution.done)
	err := execution.process.Wait()
	_ = stdout.Close()
	_ = stderr.Close()
	var evidence WorkEvidence
	if err == nil {
		evidence, err = execution.completeMaterialization()
	}
	if cleanupErr := clearRcloneRestartTransferFiles(execution.workspace); err == nil && cleanupErr != nil {
		err = cleanupErr
	}
	execution.mu.Lock()
	execution.processDone = true
	execution.terminal = ExecutionResult{Evidence: evidence, Err: err}
	if !execution.suspendCapture || execution.terminating {
		execution.publishResultLocked()
	}
	execution.mu.Unlock()
}

func (execution *rcloneRestartExecution) completeMaterialization() (WorkEvidence, error) {
	defer func() { _ = os.Remove(execution.workspace.Destination) }()
	evidence, err := hashFileWithLimit(execution.workspace.Destination, execution.worker.Config.effectiveMaxAssetBytes())
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("verify rclone restart download: %w", err)
	}
	if err := verifyExpectedIntegrity(execution.asset, evidence); err != nil {
		_ = os.Remove(execution.workspace.Destination)
		return WorkEvidence{}, fmt.Errorf("verify rclone restart download: %w", err)
	}

	destination := assetDestinationRequest{
		root: execution.worker.Config.effectiveAssetCacheDir(), payload: execution.payload, asset: execution.asset,
	}
	if materialized, ok, err := existingMaterializedDestination(destination); err != nil || ok {
		_ = os.Remove(execution.workspace.Destination)
		if err != nil {
			return WorkEvidence{}, err
		}
		return execution.materializationEvidence(materialized)
	}

	materialized, err := execution.installWorkerCache(evidence)
	if err != nil {
		return WorkEvidence{}, err
	}
	materialized, err = promoteMaterializedDestination(destination, materialized)
	if err != nil {
		return WorkEvidence{}, err
	}
	return execution.materializationEvidence(materialized)
}

func (execution *rcloneRestartExecution) installWorkerCache(evidence assetEvidence) (model.MaterializedDataAsset, error) {
	cacheKey, err := cacheKeyForAsset(execution.asset)
	if err != nil {
		return model.MaterializedDataAsset{}, err
	}
	cacheDir := filepath.Join(execution.worker.Config.effectiveAssetCacheDir(), filepath.FromSlash(cacheKey))
	sourcePath := filepath.Join(cacheDir, "source")
	manifestPath := filepath.Join(cacheDir, "manifest.json")
	immutable := execution.asset.Cache.EffectiveImmutable()
	if sourceExists(sourcePath) || sourceExists(manifestPath) {
		cached, err := readAndVerifyCache(sourcePath, manifestPath)
		if err != nil {
			return model.MaterializedDataAsset{}, err
		}
		if err := verifyExpectedIntegrity(execution.asset, cached); err != nil {
			return model.MaterializedDataAsset{}, err
		}
		_ = os.Remove(execution.workspace.Destination)
		return materializedAsset(execution.asset, sourcePath, model.DataAssetCacheStrategyWorkerCache, cacheKey, &immutable, cached), nil
	}
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return model.MaterializedDataAsset{}, fmt.Errorf("create asset cache dir %s: %w", cacheDir, err)
	}
	copied, err := copyFileWithLimit(execution.workspace.Destination, sourcePath, execution.worker.Config.effectiveMaxAssetBytes(), 0)
	if err != nil {
		return model.MaterializedDataAsset{}, fmt.Errorf("install rclone restart download: %w", err)
	}
	if copied != evidence {
		_ = os.Remove(sourcePath)
		return model.MaterializedDataAsset{}, fmt.Errorf("installed rclone restart download changed during cache promotion")
	}
	cacheManifest := workerCacheManifest{
		Schema: workerCacheManifestSchemaV1, CacheKey: cacheKey,
		BindingName: execution.asset.BindingName, ProviderName: execution.asset.ProviderName,
		ProviderType: execution.asset.Provider, SourceSizeBytes: evidence.size,
		SourceSHA256: evidence.sha256, Immutable: immutable,
		WrittenAt: execution.now().UTC().Format(time.RFC3339Nano),
	}
	if err := writeCacheManifest(manifestPath, cacheManifest); err != nil {
		_ = os.Remove(sourcePath)
		return model.MaterializedDataAsset{}, err
	}
	_ = os.Remove(execution.workspace.Destination)
	return materializedAsset(execution.asset, sourcePath, model.DataAssetCacheStrategyWorkerCache, cacheKey, &immutable, evidence), nil
}

func (execution *rcloneRestartExecution) materializationEvidence(materialized model.MaterializedDataAsset) (WorkEvidence, error) {
	preState := map[string]any{
		"operator": string(model.WorkItemTypeAssetMaterialize), "asset_key": execution.payload.AssetKey,
		"target_environment_id": execution.payload.TargetEnvironmentID,
	}
	preStateSHA256, err := canonicalObservationSHA256(preState)
	if err != nil {
		return WorkEvidence{}, err
	}
	return execution.worker.assetMaterializeEvidence(execution.payload, materialized, preStateSHA256, preState)
}

func (execution *rcloneRestartExecution) publishResultLocked() {
	if execution.resultPublished {
		return
	}
	execution.resultPublished = true
	execution.result <- execution.terminal
	close(execution.result)
}

type rcloneRestartState struct {
	Schema             string `json:"schema"`
	WorkItemID         string `json:"work_item_id"`
	WorkItemType       string `json:"work_item_type"`
	InputFingerprint   string `json:"input_fingerprint"`
	SourceVersion      string `json:"source_version"`
	CodeVersion        string `json:"code_version"`
	AssetKey           string `json:"asset_key"`
	MaterializationKey string `json:"materialization_key,omitempty"`
	ExpectedSizeBytes  int64  `json:"expected_size_bytes"`
	ExpectedSHA256     string `json:"expected_sha256"`
	AdapterID          string `json:"adapter_id"`
	AdapterVersion     string `json:"adapter_version"`
	BackendIdentity    string `json:"backend_identity"`
	CommandContract    string `json:"command_contract"`
}

func (execution *rcloneRestartExecution) writeRestartArtifact(request CheckpointCaptureRequest) (PreparedCheckpoint, error) {
	if strings.TrimSpace(execution.item.InputFingerprint) == "" || strings.TrimSpace(execution.item.CodeVersion) == "" {
		return PreparedCheckpoint{}, fmt.Errorf("rclone restart manifest requires input fingerprint and code version")
	}
	artifactRoot := filepath.Join(execution.profile.SharedTmpRoot, filepath.FromSlash(request.ArtifactStorageRelativePath))
	rel, err := filepath.Rel(execution.profile.SharedTmpRoot, artifactRoot)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return PreparedCheckpoint{}, fmt.Errorf("rclone restart artifact path escapes shared temporary root")
	}
	if err := os.MkdirAll(filepath.Dir(artifactRoot), 0700); err != nil {
		return PreparedCheckpoint{}, fmt.Errorf("create rclone restart artifact parent: %w", err)
	}
	if err := os.Mkdir(artifactRoot, 0700); err != nil {
		return PreparedCheckpoint{}, fmt.Errorf("create immutable rclone restart artifact: %w", err)
	}
	nativeDir := filepath.Join(artifactRoot, "native")
	if err := os.Mkdir(nativeDir, 0700); err != nil {
		return PreparedCheckpoint{}, fmt.Errorf("create rclone restart state directory: %w", err)
	}
	state := rcloneRestartState{
		Schema:             rcloneRestartStateSchemaV1,
		WorkItemID:         execution.item.ID,
		WorkItemType:       string(execution.item.Type),
		InputFingerprint:   execution.item.InputFingerprint,
		SourceVersion:      execution.payload.AssetKey,
		CodeVersion:        execution.item.CodeVersion,
		AssetKey:           execution.payload.AssetKey,
		MaterializationKey: execution.payload.MaterializationKey,
		ExpectedSizeBytes:  *execution.asset.Integrity.SizeBytes,
		ExpectedSHA256:     execution.asset.Integrity.SHA256,
		AdapterID:          execution.profile.AdapterID,
		AdapterVersion:     execution.profile.AdapterVersion,
		BackendIdentity:    execution.profile.BackendIdentity,
		CommandContract:    "gdrive-rclone-copyto-full-restart-v1",
	}
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return PreparedCheckpoint{}, fmt.Errorf("encode rclone restart state: %w", err)
	}
	stateRelativePath := path.Join("native", "restart-state.json")
	statePath := filepath.Join(artifactRoot, filepath.FromSlash(stateRelativePath))
	if err := writeImmutableSyncedFile(statePath, stateJSON); err != nil {
		return PreparedCheckpoint{}, fmt.Errorf("write rclone restart state: %w", err)
	}
	stateDigest := sha256.Sum256(stateJSON)
	manifest := model.ResumeArtifactManifest{
		Schema: model.ResumeArtifactSchemaV1, ResumeArtifactID: request.ResumeArtifactID,
		ResumeGeneration: request.ResumeGeneration, PauseStrategy: model.PauseStrategyNative,
		WorkItemID: request.WorkItemID, WorkItemType: request.WorkItemType,
		ProducingAttemptID: request.AttemptID, ExecutionLineageID: request.ExecutionLineageID,
		InputFingerprint: execution.item.InputFingerprint, SourceVersion: execution.payload.AssetKey,
		CodeVersion: execution.item.CodeVersion, CreatedAt: execution.now().UTC().Format(time.RFC3339),
		StorageScope: model.ResumeArtifactStorageScopeSharedTmp, StorageRelativePath: request.ArtifactStorageRelativePath,
		RetentionPolicy: model.ResumeArtifactRetentionWhileReferenced,
		Compatibility: model.ResumeArtifactCompatibility{
			AdapterID: execution.profile.AdapterID, AdapterVersion: execution.profile.AdapterVersion,
			WorkerExecutionContractVersion: execution.profile.WorkerExecutionContractVersion,
			WorkerVersion:                  execution.profile.WorkerVersion, ContainerImageIdentity: execution.profile.ContainerImageIdentity,
			OperatingSystem: execution.profile.OperatingSystem, Architecture: execution.profile.Architecture,
			ContainerRuntime: execution.profile.ContainerRuntime,
		},
		Files: []model.ResumeArtifactFile{{Path: stateRelativePath, SizeBytes: int64(len(stateJSON)), SHA256: fmt.Sprintf("%x", stateDigest[:])}},
		Native: &model.NativeResumePayload{Operation: string(model.WorkItemTypeAssetMaterialize), AdapterVersion: execution.profile.AdapterVersion,
			BackendIdentity: execution.profile.BackendIdentity, StateFilePaths: []string{stateRelativePath}},
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return PreparedCheckpoint{}, fmt.Errorf("encode rclone restart manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return PreparedCheckpoint{}, fmt.Errorf("validate rclone restart manifest: %w", err)
	}
	manifestRelativePath := path.Join(request.ArtifactStorageRelativePath, "manifest.json")
	if err := writeImmutableSyncedFile(filepath.Join(execution.profile.SharedTmpRoot, filepath.FromSlash(manifestRelativePath)), manifestJSON); err != nil {
		return PreparedCheckpoint{}, fmt.Errorf("write rclone restart manifest: %w", err)
	}
	manifestDigest := sha256.Sum256(manifestJSON)
	return PreparedCheckpoint{ManifestJSON: string(manifestJSON), Reference: model.ResumeArtifactReference{
		Schema: model.ResumeArtifactSchemaV1, ResumeArtifactID: request.ResumeArtifactID,
		StorageScope: model.ResumeArtifactStorageScopeSharedTmp, ManifestRelativePath: manifestRelativePath,
		ManifestSHA256: fmt.Sprintf("%x", manifestDigest[:]),
	}}, nil
}

func writeImmutableSyncedFile(name string, data []byte) error {
	file, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"goetl/internal/model"
)

// DMTCPLaunchProfile identifies the direct-interpreter runtime selected by
// worker configuration. Registration is deliberately handled by a later
// wiring step so ordinary Python work keeps its existing execution path.
type DMTCPLaunchProfile struct {
	LaunchExecutable               string
	CommandExecutable              string
	PythonExecutable               string
	SharedTmpRoot                  string
	CheckpointSignal               string
	ExpectedClients                int
	BuildIdentity                  string
	AdapterID                      string
	AdapterVersion                 string
	WorkerExecutionContractVersion string
	WorkerVersion                  string
	ContainerImageIdentity         string
	OperatingSystem                string
	Architecture                   string
	ContainerRuntime               string
}

func (profile DMTCPLaunchProfile) validate() error {
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "DMTCP launch executable", value: profile.LaunchExecutable},
		{name: "DMTCP command executable", value: profile.CommandExecutable},
		{name: "Python executable", value: profile.PythonExecutable},
		{name: "shared temporary root", value: profile.SharedTmpRoot},
		{name: "DMTCP build identity", value: profile.BuildIdentity},
		{name: "adapter id", value: profile.AdapterID},
		{name: "adapter version", value: profile.AdapterVersion},
		{name: "worker execution contract version", value: profile.WorkerExecutionContractVersion},
		{name: "worker version", value: profile.WorkerVersion},
		{name: "container image identity", value: profile.ContainerImageIdentity},
		{name: "operating system", value: profile.OperatingSystem},
		{name: "architecture", value: profile.Architecture},
		{name: "container runtime", value: profile.ContainerRuntime},
	} {
		if err := validateAdapterContractValue(field.name, field.value); err != nil {
			return err
		}
		if strings.ContainsAny(field.value, "\r\n\x00") {
			return fmt.Errorf("%s contains an unsafe character", field.name)
		}
	}
	if !filepath.IsAbs(profile.SharedTmpRoot) {
		return fmt.Errorf("shared temporary root must be absolute")
	}
	if profile.CheckpointSignal != "" && profile.CheckpointSignal != "12" {
		return fmt.Errorf("unsupported DMTCP checkpoint signal %q", profile.CheckpointSignal)
	}
	if profile.ExpectedClients < 1 {
		return fmt.Errorf("expected DMTCP clients must be at least 1")
	}
	return nil
}

type DMTCPCommandSpec struct {
	Executable string
	Args       []string
	Dir        string
	Env        []string
	Stdout     io.Writer
	Stderr     io.Writer
}

type DMTCPProcess interface {
	Wait() error
	Kill() error
}

type DMTCPCommandRunner interface {
	Start(context.Context, DMTCPCommandSpec) (DMTCPProcess, error)
}

type execDMTCPCommandRunner struct{}

func (execDMTCPCommandRunner) Start(ctx context.Context, spec DMTCPCommandSpec) (DMTCPProcess, error) {
	command := exec.CommandContext(ctx, spec.Executable, spec.Args...)
	command.Dir = spec.Dir
	command.Env = spec.Env
	command.Stdout = spec.Stdout
	command.Stderr = spec.Stderr
	if err := command.Start(); err != nil {
		return nil, err
	}
	return execDMTCPProcess{command: command}, nil
}

type execDMTCPProcess struct {
	command *exec.Cmd
}

func (process execDMTCPProcess) Wait() error {
	return process.command.Wait()
}

func (process execDMTCPProcess) Kill() error {
	return process.command.Process.Kill()
}

type DMTCPAdapter struct {
	Worker  Worker
	Profile DMTCPLaunchProfile
	Runner  DMTCPCommandRunner
	Now     func() time.Time
}

func (adapter DMTCPAdapter) StartFresh(ctx context.Context, item model.WorkItem) (SupervisedExecution, error) {
	if err := adapter.Profile.validate(); err != nil {
		return nil, fmt.Errorf("DMTCP launch profile: %w", err)
	}
	if item.Type != model.WorkItemTypePythonScript {
		return nil, fmt.Errorf("DMTCP direct-interpreter adapter does not support work item type %q", item.Type)
	}
	if item.Resume != nil {
		return nil, fmt.Errorf("fresh DMTCP launch does not accept a resume assignment")
	}
	if err := item.Validate(); err != nil {
		return nil, fmt.Errorf("validate DMTCP work item: %w", err)
	}
	if err := validateAdapterArtifactID(item.AttemptID); err != nil {
		return nil, fmt.Errorf("attempt id: %w", err)
	}

	staging, err := adapter.Worker.stageWorkItemSourceBundle(item)
	if err != nil {
		return nil, err
	}
	entrypointValue, err := stringParameter(item, "python_entrypoint")
	if err != nil {
		return nil, fmt.Errorf("resolve python_entrypoint: %w", err)
	}
	entrypointPath, err := resolveSourcePathWithinRoot(staging.SourceDir, entrypointValue, "python_entrypoint")
	if err != nil {
		return nil, err
	}
	pythonArgs, err := pythonArgsParameter(item)
	if err != nil {
		return nil, err
	}
	for i, argument := range pythonArgs {
		if err := validateDMTCPArgument(argument); err != nil {
			return nil, fmt.Errorf("python_args[%d]: %w", i, err)
		}
	}

	workspace, err := createDMTCPAttemptWorkspace(adapter.Profile.SharedTmpRoot, item.AttemptID)
	if err != nil {
		return nil, err
	}
	inputPath, outputPath, err := writeDMTCPPythonInput(staging.WorkDir, item)
	if err != nil {
		return nil, err
	}
	stdout, err := os.OpenFile(filepath.Join(staging.LogDir, "stdout.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return nil, fmt.Errorf("open DMTCP stdout log: %w", err)
	}
	stderr, err := os.OpenFile(filepath.Join(staging.LogDir, "stderr.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		_ = stdout.Close()
		return nil, fmt.Errorf("open DMTCP stderr log: %w", err)
	}

	signal := adapter.Profile.CheckpointSignal
	if signal == "" {
		signal = "12"
	}
	launchArgs := []string{
		"--new-coordinator",
		"--port-file", workspace.PortFile,
		"--ckptdir", workspace.CheckpointDir,
		"--tmpdir", workspace.TempDir,
		"--no-gzip",
		"--ckpt-open-files",
		"--allow-file-overwrite",
		"--ckpt-signal", signal,
		adapter.Profile.PythonExecutable,
		entrypointPath,
	}
	launchArgs = append(launchArgs, pythonArgs...)
	spec := DMTCPCommandSpec{
		Executable: adapter.Profile.LaunchExecutable,
		Args:       launchArgs,
		Dir:        staging.SourceDir,
		Env: append(os.Environ(),
			"GOET_WORK_ITEM_ID="+item.ID,
			"GOET_ATTEMPT_ID="+item.AttemptID,
			"GOET_INPUT_JSON="+inputPath,
			"GOET_OUTPUT_JSON="+outputPath,
			"GOET_SOURCE_DIR="+staging.SourceDir,
			"GOET_WORK_DIR="+staging.WorkDir,
			"GOET_ARTIFACT_DIR="+staging.ArtifactDir,
			"GOET_DATA_DIR="+adapter.Worker.Config.DataDir,
			"GOET_TMP_DIR="+adapter.Worker.Config.TmpDir,
			"GOET_LOG_DIR="+staging.LogDir,
			"GOET_PYTHON_ENTRYPOINT="+entrypointPath,
		),
		Stdout: stdout,
		Stderr: stderr,
	}
	runner := adapter.Runner
	if runner == nil {
		runner = execDMTCPCommandRunner{}
	}
	process, err := runner.Start(ctx, spec)
	if err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, fmt.Errorf("launch DMTCP direct interpreter: %w", err)
	}

	execution := &dmtcpExecution{
		process:   process,
		result:    make(chan ExecutionResult, 1),
		runner:    runner,
		profile:   adapter.Profile,
		workspace: workspace,
		item:      item,
		now:       adapter.Now,
	}
	if execution.now == nil {
		execution.now = time.Now
	}
	go execution.wait(stdout, stderr)
	return execution, nil
}

func (DMTCPAdapter) StartResume(context.Context, model.WorkItem, model.WorkItemResumeAssignment) (SupervisedExecution, error) {
	return nil, fmt.Errorf("DMTCP resume launch is not implemented")
}

type dmtcpAttemptWorkspace struct {
	Root           string
	CoordinatorDir string
	CheckpointDir  string
	TempDir        string
	PortFile       string
}

func createDMTCPAttemptWorkspace(sharedRoot string, attemptID string) (dmtcpAttemptWorkspace, error) {
	root := filepath.Join(sharedRoot, "dmtcp", attemptID)
	workspace := dmtcpAttemptWorkspace{
		Root:           root,
		CoordinatorDir: filepath.Join(root, "coordinator"),
		CheckpointDir:  filepath.Join(root, "checkpoints"),
		TempDir:        filepath.Join(root, "tmp"),
	}
	workspace.PortFile = filepath.Join(workspace.CoordinatorDir, "coordinator.port")
	for _, directory := range []string{workspace.Root, workspace.CoordinatorDir, workspace.CheckpointDir, workspace.TempDir} {
		if err := os.MkdirAll(directory, 0700); err != nil {
			return dmtcpAttemptWorkspace{}, fmt.Errorf("create DMTCP workspace %s: %w", directory, err)
		}
		if err := os.Chmod(directory, 0700); err != nil {
			return dmtcpAttemptWorkspace{}, fmt.Errorf("protect DMTCP workspace %s: %w", directory, err)
		}
	}
	return workspace, nil
}

func writeDMTCPPythonInput(workDir string, item model.WorkItem) (string, string, error) {
	input, err := json.MarshalIndent(pythonInputDocument{WorkItem: item}, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("encode DMTCP Python input: %w", err)
	}
	inputPath := filepath.Join(workDir, "input.json")
	if err := os.WriteFile(inputPath, input, 0600); err != nil {
		return "", "", fmt.Errorf("write DMTCP Python input: %w", err)
	}
	return inputPath, filepath.Join(workDir, "output.json"), nil
}

func validateDMTCPArgument(argument string) error {
	if strings.ContainsAny(argument, "\x00\r\n;&|`$><") {
		return fmt.Errorf("contains a shell metacharacter or control character")
	}
	return nil
}

type dmtcpExecution struct {
	process   DMTCPProcess
	result    chan ExecutionResult
	runner    DMTCPCommandRunner
	profile   DMTCPLaunchProfile
	workspace dmtcpAttemptWorkspace
	item      model.WorkItem
	now       func() time.Time

	mu              sync.Mutex
	processDone     bool
	processErr      error
	suspendCapture  bool
	terminating     bool
	resultPublished bool
	kill            sync.Once
	killErr         error
}

func (execution *dmtcpExecution) wait(stdout io.Closer, stderr io.Closer) {
	err := execution.process.Wait()
	_ = stdout.Close()
	_ = stderr.Close()
	execution.mu.Lock()
	execution.processDone = true
	execution.processErr = err
	if !execution.suspendCapture || execution.terminating {
		execution.publishResultLocked()
	}
	execution.mu.Unlock()
}

func (execution *dmtcpExecution) Result() <-chan ExecutionResult {
	return execution.result
}

func (execution *dmtcpExecution) CapturePeriodic(ctx context.Context, request CheckpointCaptureRequest) (PreparedCheckpoint, error) {
	return execution.capture(ctx, request, false)
}

func (execution *dmtcpExecution) CaptureForSuspend(ctx context.Context, request CheckpointCaptureRequest) (PreparedCheckpoint, error) {
	execution.mu.Lock()
	if execution.processDone {
		execution.mu.Unlock()
		return PreparedCheckpoint{}, fmt.Errorf("DMTCP payload exited before suspending capture")
	}
	execution.suspendCapture = true
	execution.mu.Unlock()
	return execution.capture(ctx, request, true)
}

func (*dmtcpExecution) Continue(context.Context) error {
	return fmt.Errorf("DMTCP continuation is not implemented")
}

func (execution *dmtcpExecution) Terminate(context.Context) error {
	execution.kill.Do(func() {
		execution.mu.Lock()
		execution.terminating = true
		alreadyDone := execution.processDone
		execution.mu.Unlock()
		if !alreadyDone {
			execution.killErr = execution.process.Kill()
			if errors.Is(execution.killErr, os.ErrProcessDone) {
				execution.killErr = nil
			}
		}
		execution.mu.Lock()
		if execution.processDone {
			execution.publishResultLocked()
		}
		execution.mu.Unlock()
	})
	return execution.killErr
}

func (execution *dmtcpExecution) capture(ctx context.Context, request CheckpointCaptureRequest, suspend bool) (PreparedCheckpoint, error) {
	if err := request.Validate(); err != nil {
		return PreparedCheckpoint{}, fmt.Errorf("DMTCP capture request: %w", err)
	}
	if request.WorkItemID != execution.item.ID || request.WorkItemType != execution.item.Type || request.AttemptID != execution.item.AttemptID {
		return PreparedCheckpoint{}, fmt.Errorf("DMTCP capture request does not match running work item")
	}
	port, err := readDMTCPPort(execution.workspace.PortFile)
	if err != nil {
		return PreparedCheckpoint{}, err
	}
	clientList, err := execution.runControlCommand(ctx, port, "--list")
	if err != nil {
		return PreparedCheckpoint{}, fmt.Errorf("list DMTCP clients: %w", err)
	}
	if count := strings.Count(clientList, "WorkerState::RUNNING"); count != execution.profile.ExpectedClients {
		return PreparedCheckpoint{}, fmt.Errorf("DMTCP client set is incomplete: found %d running clients, want %d", count, execution.profile.ExpectedClients)
	}

	command := "--bcheckpoint"
	if suspend {
		command = "--kcheckpoint"
	}
	if err := clearMutableDMTCPImages(execution.workspace.CheckpointDir); err != nil {
		return PreparedCheckpoint{}, err
	}
	if _, err := execution.runControlCommand(ctx, port, command); err != nil {
		return PreparedCheckpoint{}, fmt.Errorf("request DMTCP checkpoint: %w", err)
	}
	files, checkpointPaths, err := execution.copyCheckpointImages(request)
	if err != nil {
		return PreparedCheckpoint{}, err
	}
	return execution.writeManifestLast(request, files, checkpointPaths)
}

func (execution *dmtcpExecution) runControlCommand(ctx context.Context, port string, command string) (string, error) {
	var output bytes.Buffer
	process, err := execution.runner.Start(ctx, DMTCPCommandSpec{
		Executable: execution.profile.CommandExecutable,
		Args:       []string{"--coord-host", "127.0.0.1", "--coord-port", port, command},
		Stdout:     &output,
		Stderr:     &output,
	})
	if err != nil {
		return "", err
	}
	if err := process.Wait(); err != nil {
		return "", fmt.Errorf("%s: %w", strings.TrimSpace(output.String()), err)
	}
	return output.String(), nil
}

func readDMTCPPort(portFile string) (string, error) {
	data, err := os.ReadFile(portFile)
	if err != nil {
		return "", fmt.Errorf("read DMTCP coordinator port: %w", err)
	}
	port := strings.TrimSpace(string(data))
	value, err := strconv.Atoi(port)
	if err != nil || value < 1 || value > 65535 {
		return "", fmt.Errorf("DMTCP coordinator port is invalid")
	}
	return port, nil
}

func clearMutableDMTCPImages(directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("read mutable DMTCP checkpoint directory: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "ckpt_") || (!strings.HasSuffix(name, ".dmtcp") && !strings.HasSuffix(name, ".dmtcp.temp")) {
			continue
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("mutable DMTCP checkpoint path %q is not an owned regular file", name)
		}
		if err := os.Remove(filepath.Join(directory, name)); err != nil {
			return fmt.Errorf("clear mutable DMTCP checkpoint image %q: %w", name, err)
		}
	}
	return nil
}

func (execution *dmtcpExecution) copyCheckpointImages(request CheckpointCaptureRequest) ([]model.ResumeArtifactFile, []string, error) {
	entries, err := os.ReadDir(execution.workspace.CheckpointDir)
	if err != nil {
		return nil, nil, fmt.Errorf("read DMTCP checkpoint directory: %w", err)
	}
	var images []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, ".dmtcp.temp") {
			return nil, nil, fmt.Errorf("DMTCP checkpoint contains temporary image %q", name)
		}
		if strings.HasPrefix(name, "ckpt_") && strings.HasSuffix(name, ".dmtcp") {
			if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
				return nil, nil, fmt.Errorf("DMTCP checkpoint image %q is not a regular file", name)
			}
			images = append(images, name)
		}
	}
	sort.Strings(images)
	if len(images) != execution.profile.ExpectedClients {
		return nil, nil, fmt.Errorf("DMTCP checkpoint image set is incomplete: found %d images, want %d", len(images), execution.profile.ExpectedClients)
	}

	artifactRoot := filepath.Join(execution.profile.SharedTmpRoot, filepath.FromSlash(request.ArtifactStorageRelativePath))
	rel, err := filepath.Rel(execution.profile.SharedTmpRoot, artifactRoot)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, nil, fmt.Errorf("DMTCP artifact path escapes shared temporary root")
	}
	if err := os.MkdirAll(filepath.Dir(artifactRoot), 0700); err != nil {
		return nil, nil, fmt.Errorf("create DMTCP artifact parent: %w", err)
	}
	if err := os.Mkdir(artifactRoot, 0700); err != nil {
		return nil, nil, fmt.Errorf("create immutable DMTCP artifact directory: %w", err)
	}
	destinationDir := filepath.Join(artifactRoot, "dmtcp")
	if err := os.Mkdir(destinationDir, 0700); err != nil {
		return nil, nil, fmt.Errorf("create DMTCP image artifact directory: %w", err)
	}

	files := make([]model.ResumeArtifactFile, 0, len(images))
	checkpointPaths := make([]string, 0, len(images))
	for _, name := range images {
		relativePath := path.Join("dmtcp", name)
		file, err := copyDMTCPImage(filepath.Join(execution.workspace.CheckpointDir, name), filepath.Join(destinationDir, name), relativePath)
		if err != nil {
			return nil, nil, err
		}
		files = append(files, file)
		checkpointPaths = append(checkpointPaths, relativePath)
	}
	return files, checkpointPaths, nil
}

func copyDMTCPImage(source string, destination string, relativePath string) (model.ResumeArtifactFile, error) {
	src, err := os.Open(source)
	if err != nil {
		return model.ResumeArtifactFile{}, fmt.Errorf("open DMTCP checkpoint image: %w", err)
	}
	defer src.Close()
	dst, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return model.ResumeArtifactFile{}, fmt.Errorf("create immutable DMTCP checkpoint image: %w", err)
	}
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(dst, hash), src)
	closeErr := dst.Close()
	if copyErr != nil {
		return model.ResumeArtifactFile{}, fmt.Errorf("copy DMTCP checkpoint image: %w", copyErr)
	}
	if closeErr != nil {
		return model.ResumeArtifactFile{}, fmt.Errorf("close DMTCP checkpoint image: %w", closeErr)
	}
	if size == 0 {
		return model.ResumeArtifactFile{}, fmt.Errorf("DMTCP checkpoint image %q is empty", relativePath)
	}
	return model.ResumeArtifactFile{Path: relativePath, SizeBytes: size, SHA256: fmt.Sprintf("%x", hash.Sum(nil))}, nil
}

func (execution *dmtcpExecution) writeManifestLast(request CheckpointCaptureRequest, files []model.ResumeArtifactFile, checkpointPaths []string) (PreparedCheckpoint, error) {
	if strings.TrimSpace(execution.item.InputFingerprint) == "" || strings.TrimSpace(execution.item.CodeVersion) == "" || execution.item.Source == nil || strings.TrimSpace(execution.item.Source.ManifestPath) == "" {
		return PreparedCheckpoint{}, fmt.Errorf("DMTCP manifest requires input fingerprint, source manifest path, and code version")
	}
	manifest := model.ResumeArtifactManifest{
		Schema:              model.ResumeArtifactSchemaV1,
		ResumeArtifactID:    request.ResumeArtifactID,
		ResumeGeneration:    request.ResumeGeneration,
		PauseStrategy:       model.PauseStrategyDMTCP,
		WorkItemID:          request.WorkItemID,
		WorkItemType:        request.WorkItemType,
		ProducingAttemptID:  request.AttemptID,
		ExecutionLineageID:  request.ExecutionLineageID,
		InputFingerprint:    execution.item.InputFingerprint,
		SourceVersion:       execution.item.Source.ManifestPath,
		CodeVersion:         execution.item.CodeVersion,
		CreatedAt:           execution.now().UTC().Format(time.RFC3339),
		StorageScope:        model.ResumeArtifactStorageScopeSharedTmp,
		StorageRelativePath: request.ArtifactStorageRelativePath,
		RetentionPolicy:     model.ResumeArtifactRetentionWhileReferenced,
		Compatibility: model.ResumeArtifactCompatibility{
			AdapterID:                      execution.profile.AdapterID,
			AdapterVersion:                 execution.profile.AdapterVersion,
			WorkerExecutionContractVersion: execution.profile.WorkerExecutionContractVersion,
			WorkerVersion:                  execution.profile.WorkerVersion,
			ContainerImageIdentity:         execution.profile.ContainerImageIdentity,
			OperatingSystem:                execution.profile.OperatingSystem,
			Architecture:                   execution.profile.Architecture,
			ContainerRuntime:               execution.profile.ContainerRuntime,
		},
		Files: files,
		DMTCP: &model.DMTCPResumePayload{
			BuildIdentity:   execution.profile.BuildIdentity,
			CheckpointPaths: checkpointPaths,
		},
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return PreparedCheckpoint{}, fmt.Errorf("encode DMTCP manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return PreparedCheckpoint{}, fmt.Errorf("validate DMTCP manifest: %w", err)
	}
	manifestRelativePath := path.Join(request.ArtifactStorageRelativePath, "manifest.json")
	manifestPath := filepath.Join(execution.profile.SharedTmpRoot, filepath.FromSlash(manifestRelativePath))
	manifestFile, err := os.OpenFile(manifestPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return PreparedCheckpoint{}, fmt.Errorf("create immutable DMTCP manifest: %w", err)
	}
	if _, err := manifestFile.Write(manifestJSON); err != nil {
		_ = manifestFile.Close()
		return PreparedCheckpoint{}, fmt.Errorf("write DMTCP manifest: %w", err)
	}
	if err := manifestFile.Sync(); err != nil {
		_ = manifestFile.Close()
		return PreparedCheckpoint{}, fmt.Errorf("sync DMTCP manifest: %w", err)
	}
	if err := manifestFile.Close(); err != nil {
		return PreparedCheckpoint{}, fmt.Errorf("close DMTCP manifest: %w", err)
	}
	digest := sha256.Sum256(manifestJSON)
	return PreparedCheckpoint{
		ManifestJSON: string(manifestJSON),
		Reference: model.ResumeArtifactReference{
			Schema:               model.ResumeArtifactSchemaV1,
			ResumeArtifactID:     request.ResumeArtifactID,
			StorageScope:         model.ResumeArtifactStorageScopeSharedTmp,
			ManifestRelativePath: manifestRelativePath,
			ManifestSHA256:       fmt.Sprintf("%x", digest[:]),
		},
	}, nil
}

func (execution *dmtcpExecution) publishResultLocked() {
	if execution.resultPublished {
		return
	}
	execution.resultPublished = true
	execution.result <- ExecutionResult{Err: execution.processErr}
	close(execution.result)
}

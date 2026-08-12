package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"goetl/internal/model"
)

// DMTCPLaunchProfile identifies the direct-interpreter runtime selected by
// worker configuration. Registration is deliberately handled by a later
// wiring step so ordinary Python work keeps its existing execution path.
type DMTCPLaunchProfile struct {
	LaunchExecutable string
	PythonExecutable string
	SharedTmpRoot    string
	CheckpointSignal string
}

func (profile DMTCPLaunchProfile) validate() error {
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "DMTCP launch executable", value: profile.LaunchExecutable},
		{name: "Python executable", value: profile.PythonExecutable},
		{name: "shared temporary root", value: profile.SharedTmpRoot},
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
		process: process,
		result:  make(chan ExecutionResult, 1),
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
	process DMTCPProcess
	result  chan ExecutionResult
	kill    sync.Once
	killErr error
}

func (execution *dmtcpExecution) wait(stdout io.Closer, stderr io.Closer) {
	err := execution.process.Wait()
	_ = stdout.Close()
	_ = stderr.Close()
	execution.result <- ExecutionResult{Err: err}
	close(execution.result)
}

func (execution *dmtcpExecution) Result() <-chan ExecutionResult {
	return execution.result
}

func (*dmtcpExecution) CapturePeriodic(context.Context, CheckpointCaptureRequest) (PreparedCheckpoint, error) {
	return PreparedCheckpoint{}, fmt.Errorf("DMTCP checkpoint capture is not implemented")
}

func (*dmtcpExecution) CaptureForSuspend(context.Context, CheckpointCaptureRequest) (PreparedCheckpoint, error) {
	return PreparedCheckpoint{}, fmt.Errorf("DMTCP checkpoint capture is not implemented")
}

func (*dmtcpExecution) Continue(context.Context) error {
	return fmt.Errorf("DMTCP continuation is not implemented")
}

func (execution *dmtcpExecution) Terminate(context.Context) error {
	execution.kill.Do(func() {
		execution.killErr = execution.process.Kill()
		if errors.Is(execution.killErr, os.ErrProcessDone) {
			execution.killErr = nil
		}
	})
	return execution.killErr
}

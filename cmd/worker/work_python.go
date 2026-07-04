package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	fp "goetl/internal/fingerprint"
	"goetl/internal/model"
)

type pythonInputDocument struct {
	WorkItem model.WorkItem `json:"work_item"`
}

func (w Worker) runPythonScript(item model.WorkItem) (WorkEvidence, error) {
	staging, err := w.stageWorkItemSourceBundle(item)
	if err != nil {
		return WorkEvidence{}, err
	}

	entrypointValue, err := stringParameter(item, "python_entrypoint")
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("resolve python_entrypoint: %w", err)
	}

	entrypointPath, err := resolveSourcePathWithinRoot(staging.SourceDir, entrypointValue, "python_entrypoint")
	if err != nil {
		return WorkEvidence{}, err
	}

	environmentPath, hasEnvironment, err := optionalSourcePathParameter(item, staging.SourceDir, "python_environment")
	if err != nil {
		return WorkEvidence{}, err
	}

	args, err := pythonArgsParameter(item)
	if err != nil {
		return WorkEvidence{}, err
	}

	inputDocument := pythonInputDocument{WorkItem: item}
	inputJSON, err := json.MarshalIndent(inputDocument, "", "  ")
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("encode python input json: %w", err)
	}

	inputPath := filepath.Join(staging.WorkDir, "input.json")
	if err := os.WriteFile(inputPath, inputJSON, 0644); err != nil {
		return WorkEvidence{}, fmt.Errorf("write python input json %s: %w", inputPath, err)
	}

	outputPath := filepath.Join(staging.WorkDir, "output.json")
	stdoutPath := filepath.Join(staging.LogDir, "stdout.log")
	stderrPath := filepath.Join(staging.LogDir, "stderr.log")

	stdoutFile, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("open stdout log %s: %w", stdoutPath, err)
	}
	defer stdoutFile.Close()

	stderrFile, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("open stderr log %s: %w", stderrPath, err)
	}
	defer stderrFile.Close()

	command := exec.Command(w.pythonExecutable(), append([]string{entrypointPath}, args...)...)
	command.Dir = staging.SourceDir
	command.Stdout = stdoutFile
	command.Stderr = stderrFile
	command.Env = append(os.Environ(),
		"GOET_WORK_ITEM_ID="+item.ID,
		"GOET_ATTEMPT_ID="+item.AttemptID,
		"GOET_INPUT_JSON="+inputPath,
		"GOET_OUTPUT_JSON="+outputPath,
		"GOET_SOURCE_DIR="+staging.SourceDir,
		"GOET_WORK_DIR="+staging.WorkDir,
		"GOET_DATA_DIR="+w.Config.DataDir,
		"GOET_TMP_DIR="+w.Config.TmpDir,
		"GOET_LOG_DIR="+staging.LogDir,
		"GOET_PYTHON_ENTRYPOINT="+entrypointPath,
	)
	if hasEnvironment {
		command.Env = append(command.Env, "GOET_PYTHON_ENVIRONMENT_JSON="+environmentPath)
	}

	if err := command.Start(); err != nil {
		return WorkEvidence{}, fmt.Errorf("launch python process: %w", err)
	}

	if err := command.Wait(); err != nil {
		return WorkEvidence{}, fmt.Errorf("python process exited with error: %w", err)
	}

	outputJSON, err := os.ReadFile(outputPath)
	if os.IsNotExist(err) {
		return WorkEvidence{}, fmt.Errorf("missing GOET_OUTPUT_JSON after successful python process exit: %s", outputPath)
	}
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("read python output json %s: %w", outputPath, err)
	}

	dataPath := filepath.Join(w.Config.DataDir, item.OutputFilename)
	preState, err := outputFileState(dataPath)
	if err != nil {
		return WorkEvidence{}, err
	}

	if err := os.WriteFile(dataPath, outputJSON, 0644); err != nil {
		return WorkEvidence{}, fmt.Errorf("write completed python output %s: %w", dataPath, err)
	}

	postState, err := outputFileState(dataPath)
	if err != nil {
		return WorkEvidence{}, err
	}

	inputSHA256, err := inputObservationSHA256(item, inputDocument)
	if err != nil {
		return WorkEvidence{}, err
	}

	outputSHA256 := fp.SHA256Hex(outputJSON)

	return outputEvidence(item, dataPath, int64(len(outputJSON)), preState, postState, inputSHA256, outputSHA256, model.WorkReuseCandidate{})
}

func (w Worker) pythonExecutable() string {
	executable := strings.TrimSpace(w.Config.PythonExecutable)
	if executable == "" {
		return "python3"
	}
	return executable
}

func optionalSourcePathParameter(item model.WorkItem, root string, name string) (string, bool, error) {
	parameter, ok := item.Parameters[name]
	if !ok {
		return "", false, nil
	}

	if parameter.Type != "string" && parameter.Type != "path" {
		return "", false, fmt.Errorf("parameter %s has type %s, want string or path", name, parameter.Type)
	}

	value, ok := parameter.Value.(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", false, fmt.Errorf("parameter %s value must be a non-empty string", name)
	}

	path, err := resolveSourcePathWithinRoot(root, value, name)
	if err != nil {
		return "", false, err
	}

	return path, true, nil
}

func resolveSourcePathWithinRoot(root string, value string, name string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve %s root %s: %w", name, root, err)
	}

	candidate := value
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(rootAbs, candidate)
	}
	candidate = filepath.Clean(candidate)

	rel, err := filepath.Rel(rootAbs, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve %s path %s: %w", name, value, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe %s path: %s", name, value)
	}

	info, err := os.Stat(candidate)
	if err != nil {
		return "", fmt.Errorf("check %s path %s: %w", name, candidate, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s path is a directory: %s", name, candidate)
	}

	return candidate, nil
}

func pythonArgsParameter(item model.WorkItem) ([]string, error) {
	parameter, ok := item.Parameters["python_args"]
	if !ok {
		return nil, nil
	}
	if parameter.Type != "list" {
		return nil, fmt.Errorf("parameter python_args has type %s, want list", parameter.Type)
	}

	switch value := parameter.Value.(type) {
	case []string:
		return append([]string(nil), value...), nil
	case []any:
		args := make([]string, len(value))
		for i, raw := range value {
			text, ok := raw.(string)
			if !ok {
				return nil, fmt.Errorf("parameter python_args must be a list of strings")
			}
			args[i] = text
		}
		return args, nil
	default:
		return nil, fmt.Errorf("parameter python_args must be a list of strings")
	}
}

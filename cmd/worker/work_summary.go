package main

import (
	"fmt"
	"os"
	"path/filepath"

	"goetl/internal/model"
)

func (w Worker) summarizeInputFile(item model.WorkItem) (WorkEvidence, error) {
	inputPath, err := stringParameter(item, "input_path")
	if err != nil {
		return WorkEvidence{}, err
	}

	tmpPath := filepath.Join(w.Config.TmpDir, item.OutputFilename)
	dataPath := filepath.Join(w.Config.DataDir, item.OutputFilename)

	if err := w.log("starting work item: " + item.ID); err != nil {
		return WorkEvidence{}, err
	}

	preState, err := outputFileState(dataPath)
	if err != nil {
		return WorkEvidence{}, err
	}

	info, err := os.Stat(inputPath)
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("check input file %s: %w", inputPath, err)
	}
	if info.IsDir() {
		return WorkEvidence{}, fmt.Errorf("input path is a directory: %s", inputPath)
	}

	summary := fmt.Sprintf("input_path=%s\nsize_bytes=%d\n", inputPath, info.Size())
	if err := os.WriteFile(tmpPath, []byte(summary), 0644); err != nil {
		return WorkEvidence{}, fmt.Errorf("write temporary output %s: %w", tmpPath, err)
	}

	if err := os.Remove(dataPath); err != nil && !os.IsNotExist(err) {
		return WorkEvidence{}, fmt.Errorf("remove existing output %s: %w", dataPath, err)
	}

	if err := os.Rename(tmpPath, dataPath); err != nil {
		return WorkEvidence{}, fmt.Errorf("move output from %s to %s: %w", tmpPath, dataPath, err)
	}

	if err := w.log("completed work item: " + item.ID); err != nil {
		return WorkEvidence{}, err
	}

	return outputEvidence(item, dataPath, int64(len(summary)), preState)
}

func stringParameter(item model.WorkItem, name string) (string, error) {
	parameter, ok := item.Parameters[name]
	if !ok {
		return "", fmt.Errorf("parameter %s is required", name)
	}
	if parameter.Type != "string" && parameter.Type != "path" {
		return "", fmt.Errorf("parameter %s has type %s, want string or path", name, parameter.Type)
	}

	value, ok := parameter.Value.(string)
	if !ok || value == "" {
		return "", fmt.Errorf("parameter %s value must be a non-empty string", name)
	}

	return value, nil
}

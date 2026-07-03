package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"goetl/internal/model"
)

func (w Worker) writeDemoOutput(item model.WorkItem) (WorkEvidence, error) {
	tmpPath := filepath.Join(w.Config.TmpDir, item.OutputFilename)
	dataPath := filepath.Join(w.Config.DataDir, item.OutputFilename)

	if err := w.log("starting work item: " + item.ID); err != nil {
		return WorkEvidence{}, err
	}

	preState, err := outputFileState(dataPath)
	if err != nil {
		return WorkEvidence{}, err
	}

	output := []byte("completed " + item.ID + "\n")
	if err := os.WriteFile(tmpPath, output, 0644); err != nil {
		return WorkEvidence{}, fmt.Errorf("write temporary output %s: %w", tmpPath, err)
	}

	if err := w.log("wrote temporary output: " + tmpPath); err != nil {
		return WorkEvidence{}, err
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

	return outputEvidence(item, dataPath, int64(len(output)), preState)
}

type outputEvidenceJSON struct {
	WorkItemID     string `json:"work_item_id"`
	OutputFilename string `json:"output_filename"`
	OutputPath     string `json:"output_path"`
	BytesWritten   int64  `json:"bytes_written"`
}

type outputStateJSON struct {
	OutputExists bool   `json:"output_exists"`
	OutputPath   string `json:"output_path,omitempty"`
	BytesWritten int64  `json:"bytes_written,omitempty"`
}

func outputFileState(path string) (outputStateJSON, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return outputStateJSON{OutputExists: false}, nil
	}
	if err != nil {
		return outputStateJSON{}, fmt.Errorf("check output state %s: %w", path, err)
	}
	if info.IsDir() {
		return outputStateJSON{}, fmt.Errorf("output path is a directory: %s", path)
	}

	return outputStateJSON{
		OutputExists: true,
		OutputPath:   path,
		BytesWritten: info.Size(),
	}, nil
}

func outputEvidence(item model.WorkItem, dataPath string, bytesWritten int64, preState outputStateJSON) (WorkEvidence, error) {
	outputJSON, err := json.Marshal(outputEvidenceJSON{
		WorkItemID:     item.ID,
		OutputFilename: item.OutputFilename,
		OutputPath:     dataPath,
		BytesWritten:   bytesWritten,
	})
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("encode output evidence: %w", err)
	}

	preStateJSON, err := json.Marshal(preState)
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("encode pre-state evidence: %w", err)
	}

	postStateJSON, err := json.Marshal(outputStateJSON{
		OutputExists: true,
		OutputPath:   dataPath,
		BytesWritten: bytesWritten,
	})
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("encode post-state evidence: %w", err)
	}

	return WorkEvidence{
		OutputJSON:    string(outputJSON),
		PreStateJSON:  string(preStateJSON),
		PostStateJSON: string(postStateJSON),
	}, nil
}

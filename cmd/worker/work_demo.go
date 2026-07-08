package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	fp "goetl/internal/fingerprint"
	"goetl/internal/model"
)

func (w Worker) writeDemoOutput(ctx OperationContext) (WorkEvidence, error) {
	item := ctx.WorkItem
	tmpPath := filepath.Join(w.Config.TmpDir, item.OutputFilename)
	dataPath := filepath.Join(w.Config.DataDir, item.OutputFilename)

	if err := ctx.Logger.Log("starting work item: " + item.ID); err != nil {
		return WorkEvidence{}, err
	}
	if secret, ok := ctx.Sensitive["demo_secret"]; ok {
		if err := ctx.Logger.Log("trusted demo secret received: " + secret.Plaintext()); err != nil {
			return WorkEvidence{}, err
		}
	}

	preState, err := outputFileState(dataPath)
	if err != nil {
		return WorkEvidence{}, err
	}

	output := []byte("completed " + item.ID + "\n")
	inputSHA256, err := inputObservationSHA256(item, nil)
	if err != nil {
		return WorkEvidence{}, err
	}
	expectedOutputSHA256 := fp.SHA256Hex(output)
	preStateSHA256, err := canonicalObservationSHA256(preState)
	if err != nil {
		return WorkEvidence{}, err
	}
	if candidate, ok := matchingReuseCandidate(item, inputSHA256, expectedOutputSHA256, preStateSHA256); ok {
		if err := ctx.Logger.Log("skipped work item: " + item.ID); err != nil {
			return WorkEvidence{}, err
		}
		return outputEvidence(item, dataPath, int64(len(output)), preState, preState, inputSHA256, expectedOutputSHA256, candidate)
	}

	if err := os.WriteFile(tmpPath, output, 0644); err != nil {
		return WorkEvidence{}, fmt.Errorf("write temporary output %s: %w", tmpPath, err)
	}

	if err := ctx.Logger.Log("wrote temporary output: " + tmpPath); err != nil {
		return WorkEvidence{}, err
	}

	if err := os.Remove(dataPath); err != nil && !os.IsNotExist(err) {
		return WorkEvidence{}, fmt.Errorf("remove existing output %s: %w", dataPath, err)
	}

	if err := os.Rename(tmpPath, dataPath); err != nil {
		return WorkEvidence{}, fmt.Errorf("move output from %s to %s: %w", tmpPath, dataPath, err)
	}

	if err := ctx.Logger.Log("completed work item: " + item.ID); err != nil {
		return WorkEvidence{}, err
	}

	postState, err := outputFileState(dataPath)
	if err != nil {
		return WorkEvidence{}, err
	}

	return outputEvidence(item, dataPath, int64(len(output)), preState, postState, inputSHA256, expectedOutputSHA256, model.WorkReuseCandidate{})
}

type outputEvidenceJSON struct {
	WorkItemID      string `json:"work_item_id"`
	OutputFilename  string `json:"output_filename"`
	OutputPath      string `json:"output_path"`
	BytesWritten    int64  `json:"bytes_written"`
	Skipped         bool   `json:"skipped,omitempty"`
	SkipReason      string `json:"skip_reason,omitempty"`
	InputSHA256     string `json:"input_sha256"`
	OutputSHA256    string `json:"output_sha256"`
	PreStateSHA256  string `json:"pre_state_sha256"`
	PostStateSHA256 string `json:"post_state_sha256"`
}

type outputStateJSON struct {
	OutputExists bool   `json:"output_exists"`
	OutputPath   string `json:"output_path,omitempty"`
	BytesWritten int64  `json:"bytes_written,omitempty"`
	OutputSHA256 string `json:"output_sha256,omitempty"`
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
	outputSHA256, err := fileSHA256(path)
	if err != nil {
		return outputStateJSON{}, err
	}

	return outputStateJSON{
		OutputExists: true,
		OutputPath:   path,
		BytesWritten: info.Size(),
		OutputSHA256: outputSHA256,
	}, nil
}

func outputEvidence(item model.WorkItem, dataPath string, bytesWritten int64, preState outputStateJSON, postState outputStateJSON, inputSHA256 string, outputSHA256 string, candidate model.WorkReuseCandidate) (WorkEvidence, error) {
	preStateSHA256, err := canonicalObservationSHA256(preState)
	if err != nil {
		return WorkEvidence{}, err
	}
	postStateSHA256, err := canonicalObservationSHA256(postState)
	if err != nil {
		return WorkEvidence{}, err
	}
	skipped := candidate.AttemptID != ""
	skipReason := ""
	if skipped {
		skipReason = "matched_worker_observed_state"
	}

	outputJSON, err := json.Marshal(outputEvidenceJSON{
		WorkItemID:      item.ID,
		OutputFilename:  item.OutputFilename,
		OutputPath:      dataPath,
		BytesWritten:    bytesWritten,
		Skipped:         skipped,
		SkipReason:      skipReason,
		InputSHA256:     inputSHA256,
		OutputSHA256:    outputSHA256,
		PreStateSHA256:  preStateSHA256,
		PostStateSHA256: postStateSHA256,
	})
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("encode output evidence: %w", err)
	}

	preStateJSON, err := json.Marshal(preState)
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("encode pre-state evidence: %w", err)
	}

	postStateJSON, err := json.Marshal(postState)
	if err != nil {
		return WorkEvidence{}, fmt.Errorf("encode post-state evidence: %w", err)
	}

	return WorkEvidence{
		Skipped:         skipped,
		SkippedParentID: candidate.AttemptID,
		SkipReason:      skipReason,
		InputSHA256:     inputSHA256,
		OutputSHA256:    outputSHA256,
		PreStateSHA256:  preStateSHA256,
		PostStateSHA256: postStateSHA256,
		OutputJSON:      string(outputJSON),
		PreStateJSON:    string(preStateJSON),
		PostStateJSON:   string(postStateJSON),
	}, nil
}

func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read output for sha256 %s: %w", path, err)
	}
	return fp.SHA256Hex(data), nil
}

func canonicalObservationSHA256(value any) (string, error) {
	normalized, err := normalizedJSONValue(value)
	if err != nil {
		return "", err
	}
	_, hash, err := fp.CanonicalJSONSHA256(normalized)
	if err != nil {
		return "", fmt.Errorf("hash observed state: %w", err)
	}
	return hash, nil
}

func normalizedJSONValue(value any) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal observed state: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var normalized any
	if err := decoder.Decode(&normalized); err != nil {
		return nil, fmt.Errorf("decode observed state: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("observed state must contain one JSON document")
	}
	return normalized, nil
}

func inputObservationSHA256(item model.WorkItem, input any) (string, error) {
	observation := map[string]any{
		"type":            string(item.Type),
		"output_filename": item.OutputFilename,
		"parameters":      item.Parameters,
	}
	if input != nil {
		observation["input"] = input
	}
	return canonicalObservationSHA256(observation)
}

func matchingReuseCandidate(item model.WorkItem, inputSHA256 string, outputSHA256 string, preStateSHA256 string) (model.WorkReuseCandidate, bool) {
	for _, candidate := range item.ReuseCandidates {
		if candidate.AttemptID == "" {
			continue
		}
		if candidate.InputSHA256 != "" && candidate.InputSHA256 != inputSHA256 {
			continue
		}
		if candidate.OutputSHA256 != "" && candidate.OutputSHA256 != outputSHA256 {
			continue
		}
		if candidate.PostStateSHA256 != "" && candidate.PostStateSHA256 != preStateSHA256 {
			continue
		}
		return candidate, true
	}
	return model.WorkReuseCandidate{}, false
}

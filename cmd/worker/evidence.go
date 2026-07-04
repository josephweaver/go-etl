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

const (
	pythonInputEvidenceSchema  = "goet/python-workitem-input/v1"
	pythonOutputEvidenceSchema = "goet/python-workitem-output/v1"
	pythonScriptOperation      = "python_script"
)

type pythonInputEvidenceJSON struct {
	Schema         string              `json:"schema"`
	RunnerContract string              `json:"runner_contract"`
	WorkItemID     string              `json:"work_item_id"`
	AttemptID      string              `json:"attempt_id,omitempty"`
	Operation      string              `json:"operation"`
	Entrypoint     string              `json:"entrypoint"`
	Environment    string              `json:"environment,omitempty"`
	PythonArgs     []string            `json:"python_args,omitempty"`
	InputDocument  pythonInputDocument `json:"input_document"`
}

type pythonOutputEvidenceJSON struct {
	Schema          string          `json:"schema"`
	WorkItemID      string          `json:"work_item_id"`
	Operation       string          `json:"operation"`
	Entrypoint      string          `json:"entrypoint"`
	Environment     string          `json:"environment,omitempty"`
	ExitCode        int             `json:"exit_code"`
	LogicalOutput   json.RawMessage `json:"logical_output"`
	InputSHA256     string          `json:"input_sha256"`
	OutputSHA256    string          `json:"output_sha256"`
	PreStateSHA256  string          `json:"pre_state_sha256"`
	PostStateSHA256 string          `json:"post_state_sha256"`
	StdoutSHA256    string          `json:"stdout_sha256,omitempty"`
	StderrSHA256    string          `json:"stderr_sha256,omitempty"`
}

func canonicalJSONDocument(raw []byte, name string) ([]byte, string, any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, "", nil, fmt.Errorf("decode %s: %w", name, err)
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, "", nil, fmt.Errorf("%s must contain one JSON document", name)
	}

	canonical, hash, err := fp.CanonicalJSONSHA256(decoded)
	if err != nil {
		return nil, "", nil, fmt.Errorf("canonicalize %s: %w", name, err)
	}

	return canonical, hash, decoded, nil
}

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create parent directory for %s: %w", path, err)
	}

	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", path, err)
	}
	tempPath := temp.Name()
	defer func() {
		_ = os.Remove(tempPath)
	}()

	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write temp file %s: %w", tempPath, err)
	}
	if err := temp.Chmod(perm); err != nil {
		_ = temp.Close()
		return fmt.Errorf("chmod temp file %s: %w", tempPath, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temp file %s: %w", tempPath, err)
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove existing file %s: %w", path, err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("promote temp file %s to %s: %w", tempPath, path, err)
	}

	return nil
}

func pythonInputObservationSHA256(item model.WorkItem, entrypoint string, environment string, pythonArgs []string, inputDocument pythonInputDocument) (string, error) {
	observation := pythonInputEvidenceJSON{
		Schema:         pythonInputEvidenceSchema,
		RunnerContract: pythonOutputEvidenceSchema,
		WorkItemID:     item.ID,
		AttemptID:      item.AttemptID,
		Operation:      pythonScriptOperation,
		Entrypoint:     entrypoint,
		PythonArgs:     append([]string(nil), pythonArgs...),
		InputDocument:  inputDocument,
	}
	if environment != "" {
		observation.Environment = environment
	}

	return canonicalObservationSHA256(observation)
}

func pythonOutputEvidenceJSONText(
	item model.WorkItem,
	entrypoint string,
	environment string,
	exitCode int,
	logicalOutput []byte,
	inputSHA256 string,
	outputSHA256 string,
	preStateSHA256 string,
	postStateSHA256 string,
	stdoutSHA256 string,
	stderrSHA256 string,
) (string, error) {
	wrapper := pythonOutputEvidenceJSON{
		Schema:          pythonOutputEvidenceSchema,
		WorkItemID:      item.ID,
		Operation:       pythonScriptOperation,
		Entrypoint:      entrypoint,
		ExitCode:        exitCode,
		LogicalOutput:   json.RawMessage(logicalOutput),
		InputSHA256:     inputSHA256,
		OutputSHA256:    outputSHA256,
		PreStateSHA256:  preStateSHA256,
		PostStateSHA256: postStateSHA256,
		StdoutSHA256:    stdoutSHA256,
		StderrSHA256:    stderrSHA256,
	}
	if environment != "" {
		wrapper.Environment = environment
	}

	outputJSON, err := json.Marshal(wrapper)
	if err != nil {
		return "", fmt.Errorf("encode python output evidence: %w", err)
	}

	return string(outputJSON), nil
}

func logFileSHA256(path string) (string, bool, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", false, nil
	} else if err != nil {
		return "", false, fmt.Errorf("check log file %s: %w", path, err)
	}

	hash, err := fileSHA256(path)
	if err != nil {
		return "", false, err
	}
	return hash, true, nil
}

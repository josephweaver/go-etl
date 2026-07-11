package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"goetl/internal/model"
)

const (
	directExitSuccess = 0
	directExitFailure = 1

	directResultSchema    = "gorc/worker-direct-result/v1"
	directStatusCompleted = "completed"
	directStatusFailed    = "failed"
)

var directAttemptIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type directOptions struct {
	ConfigPath   string
	WorkItemPath string
	ResultPath   string
}

type DirectExecutionResult struct {
	Schema         string                   `json:"schema"`
	Status         string                   `json:"status"`
	WorkItemID     string                   `json:"work_item_id"`
	AttemptID      string                   `json:"attempt_id"`
	WorkItemType   string                   `json:"work_item_type"`
	OutputFilename string                   `json:"output_filename"`
	StartedAt      string                   `json:"started_at"`
	FinishedAt     string                   `json:"finished_at"`
	DataOutputPath string                   `json:"data_output_path,omitempty"`
	AttemptDir     string                   `json:"attempt_dir,omitempty"`
	Evidence       *DirectExecutionEvidence `json:"evidence,omitempty"`
	Error          string                   `json:"error,omitempty"`
}

type DirectExecutionEvidence struct {
	Skipped         bool   `json:"skipped,omitempty"`
	SkippedParentID string `json:"skipped_parent_id,omitempty"`
	SkipReason      string `json:"skip_reason,omitempty"`
	InputSHA256     string `json:"input_sha256,omitempty"`
	OutputSHA256    string `json:"output_sha256,omitempty"`
	PreStateSHA256  string `json:"pre_state_sha256,omitempty"`
	PostStateSHA256 string `json:"post_state_sha256,omitempty"`
	OutputJSON      string `json:"output_json,omitempty"`
	PreStateJSON    string `json:"pre_state_json,omitempty"`
	PostStateJSON   string `json:"post_state_json,omitempty"`
}

func parseDirectOptions(args []string, stderr io.Writer) (directOptions, error) {
	options := directOptions{ResultPath: "worker-result.json"}
	flags := flag.NewFlagSet("worker execute", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&options.ConfigPath, "config", "", "worker config JSON path")
	flags.StringVar(&options.WorkItemPath, "work-item", "", "resolved work-item JSON path")
	flags.StringVar(&options.ResultPath, "result", options.ResultPath, "direct result JSON path")

	if err := flags.Parse(args); err != nil {
		return directOptions{}, err
	}
	if flags.NArg() != 0 {
		return directOptions{}, fmt.Errorf("unexpected positional arguments: %v", flags.Args())
	}
	if options.ConfigPath == "" {
		return directOptions{}, fmt.Errorf("--config is required")
	}
	if options.WorkItemPath == "" {
		return directOptions{}, fmt.Errorf("--work-item is required")
	}
	if options.ResultPath == "" {
		return directOptions{}, fmt.Errorf("--result must not be empty")
	}
	return options, nil
}

func loadDirectWorkItem(path string) (model.WorkItem, error) {
	info, err := os.Stat(path)
	if err != nil {
		return model.WorkItem{}, fmt.Errorf("stat work-item file %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return model.WorkItem{}, fmt.Errorf("work-item path is not a regular file: %s", path)
	}

	file, err := os.Open(path)
	if err != nil {
		return model.WorkItem{}, fmt.Errorf("open work-item file %s: %w", path, err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	var item model.WorkItem
	if err := decoder.Decode(&item); err != nil {
		return model.WorkItem{}, fmt.Errorf("decode work-item file %s: %w", path, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return model.WorkItem{}, fmt.Errorf("work-item file %s must contain exactly one JSON document", path)
	}

	item, err = normalizeDirectWorkItem(item)
	if err != nil {
		return model.WorkItem{}, fmt.Errorf("normalize work-item file %s: %w", path, err)
	}
	if err := item.Validate(); err != nil {
		return model.WorkItem{}, fmt.Errorf("validate work-item file %s: %w", path, err)
	}
	return item, nil
}

func normalizeDirectWorkItem(item model.WorkItem) (model.WorkItem, error) {
	if item.AttemptID == "" {
		item.AttemptID = "direct-attempt-" + randomHex(8)
	}
	if !validDirectAttemptID(item.AttemptID) {
		return model.WorkItem{}, fmt.Errorf("attempt id %q must match [A-Za-z0-9._-]+ and must not be . or ..", item.AttemptID)
	}
	return item, nil
}

func validDirectAttemptID(value string) bool {
	return value != "." && value != ".." && directAttemptIDPattern.MatchString(value)
}

func runDirectCommand(args []string, stdout io.Writer, stderr io.Writer) int {
	options, err := parseDirectOptions(args, stderr)
	if err != nil {
		fmt.Fprintln(stderr, "invalid direct command:", err)
		return directExitFailure
	}
	if err := validateDirectResultPath(options.ResultPath, options.ConfigPath, options.WorkItemPath); err != nil {
		fmt.Fprintln(stderr, "invalid direct result path:", err)
		return directExitFailure
	}

	if err := removeDirectResult(options.ResultPath); err != nil {
		fmt.Fprintln(stderr, "prepare direct result:", err)
		return directExitFailure
	}

	cfg, err := loadDirectConfig(options.ConfigPath)
	if err != nil {
		fmt.Fprintln(stderr, "invalid direct config:", err)
		return directExitFailure
	}
	item, err := loadDirectWorkItem(options.WorkItemPath)
	if err != nil {
		fmt.Fprintln(stderr, "invalid direct work item:", err)
		return directExitFailure
	}

	worker := Worker{Config: cfg}
	if err := worker.Validate(); err != nil {
		fmt.Fprintln(stderr, "invalid direct worker:", err)
		return directExitFailure
	}

	startedAt := time.Now().UTC()
	evidence, runErr := worker.Run(item)
	finishedAt := time.Now().UTC()
	result := buildDirectExecutionResult(cfg, item, startedAt, finishedAt, evidence, runErr)
	if err := writeDirectExecutionResult(options.ResultPath, result); err != nil {
		fmt.Fprintln(stderr, "write direct result:", err)
		return directExitFailure
	}

	if runErr != nil {
		fmt.Fprintln(stderr, "direct work failed:", runErr)
		return directExitFailure
	}
	fmt.Fprintln(stdout, options.ResultPath)
	return directExitSuccess
}

func validateDirectResultPath(resultPath string, inputPaths ...string) error {
	resultAbsolute, err := filepath.Abs(resultPath)
	if err != nil {
		return fmt.Errorf("resolve result path %s: %w", resultPath, err)
	}
	resultInfo, resultStatErr := os.Stat(resultPath)

	for _, inputPath := range inputPaths {
		inputAbsolute, err := filepath.Abs(inputPath)
		if err != nil {
			return fmt.Errorf("resolve input path %s: %w", inputPath, err)
		}
		if filepath.Clean(resultAbsolute) == filepath.Clean(inputAbsolute) {
			return fmt.Errorf("result path must differ from input path %s", inputPath)
		}
		if resultStatErr == nil {
			if inputInfo, err := os.Stat(inputPath); err == nil && os.SameFile(resultInfo, inputInfo) {
				return fmt.Errorf("result path must differ from input path %s", inputPath)
			}
		}
	}
	return nil
}

func removeDirectResult(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove existing result %s: %w", path, err)
	}
	return nil
}

func buildDirectExecutionResult(cfg Config, item model.WorkItem, startedAt time.Time, finishedAt time.Time, evidence WorkEvidence, runErr error) DirectExecutionResult {
	result := DirectExecutionResult{
		Schema:         directResultSchema,
		Status:         directStatusCompleted,
		WorkItemID:     item.ID,
		AttemptID:      item.AttemptID,
		WorkItemType:   string(item.Type),
		OutputFilename: item.OutputFilename,
		StartedAt:      startedAt.Format(time.RFC3339Nano),
		FinishedAt:     finishedAt.Format(time.RFC3339Nano),
	}
	if runErr != nil {
		result.Status = directStatusFailed
		result.Error = runErr.Error()
		return result
	}

	result.Evidence = directExecutionEvidence(evidence)
	dataOutputPath := filepath.Join(cfg.DataDir, item.OutputFilename)
	if info, err := os.Stat(dataOutputPath); err == nil && !info.IsDir() {
		result.DataOutputPath = dataOutputPath
	}
	attemptDir := filepath.Join(cfg.TmpDir, "attempts", item.AttemptID)
	if info, err := os.Stat(attemptDir); err == nil && info.IsDir() {
		result.AttemptDir = attemptDir
	}
	return result
}

func directExecutionEvidence(evidence WorkEvidence) *DirectExecutionEvidence {
	return &DirectExecutionEvidence{
		Skipped:         evidence.Skipped,
		SkippedParentID: evidence.SkippedParentID,
		SkipReason:      evidence.SkipReason,
		InputSHA256:     evidence.InputSHA256,
		OutputSHA256:    evidence.OutputSHA256,
		PreStateSHA256:  evidence.PreStateSHA256,
		PostStateSHA256: evidence.PostStateSHA256,
		OutputJSON:      evidence.OutputJSON,
		PreStateJSON:    evidence.PreStateJSON,
		PostStateJSON:   evidence.PostStateJSON,
	}
}

func writeDirectExecutionResult(path string, result DirectExecutionResult) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("encode result: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create result parent for %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write result %s: %w", path, err)
	}
	return nil
}

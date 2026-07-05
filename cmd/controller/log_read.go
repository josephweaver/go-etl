package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"goetl/internal/model"
)

type submissionLogResponse struct {
	SubmissionID string                 `json:"submission_id"`
	Entries      []model.LogObservation `json:"entries"`
	Tail         int                    `json:"tail"`
	Truncated    bool                   `json:"truncated"`
}

type submittedLogRecord struct {
	Observation model.LogObservation
	Timestamp   time.Time
	Order       int
}

func (c *Controller) submissionLogsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !c.requireNormalAdmission(w) {
		return
	}
	if c.workflowStore == nil {
		http.Error(w, "workflow store required", http.StatusServiceUnavailable)
		return
	}

	submissionID, ok := submissionIDFromLogsPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	_, found, err := c.workflowStore.GetWorkflowRun(r.Context(), submissionID)
	if err != nil {
		http.Error(w, "query submission status", http.StatusInternalServerError)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}

	tail, err := parseSubmissionLogTail(r.URL.Query().Get("tail"), c.logReadDefaultTail, c.logReadMaxTail)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	level, err := parseSubmissionLogLevel(r.URL.Query().Get("level"))
	if err != nil {
		http.Error(w, "invalid level: "+err.Error(), http.StatusBadRequest)
		return
	}
	stream, err := parseSubmissionLogStream(r.URL.Query().Get("stream"))
	if err != nil {
		http.Error(w, "invalid stream: "+err.Error(), http.StatusBadRequest)
		return
	}
	attemptID := r.URL.Query().Get("attempt_id")

	entries, truncated, err := c.readSubmissionLogEntries(
		submissionID,
		tail,
		level,
		stream,
		attemptID,
	)
	if err != nil {
		http.Error(w, "read submission logs", http.StatusInternalServerError)
		return
	}

	response := submissionLogResponse{
		SubmissionID: submissionID,
		Entries:      entries,
		Tail:         tail,
		Truncated:    truncated,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "encode submission logs", http.StatusInternalServerError)
	}
}

func (c *Controller) readSubmissionLogEntries(
	submissionID string,
	tail int,
	levelFilter string,
	streamFilter string,
	attemptIDFilter string,
) ([]model.LogObservation, bool, error) {
	records := make([]submittedLogRecord, 0)

	submissionFile, err := c.submissionLogPath(submissionID)
	if err != nil {
		return nil, false, err
	}
	if err := readSubmissionLogLines(
		submissionFile,
		levelFilter,
		streamFilter,
		attemptIDFilter,
		&records,
	); err != nil {
		return nil, false, err
	}

	attemptDir := filepath.Join(c.logRootPath, "submissions", submissionID, "attempts")
	attemptEntries, err := os.ReadDir(attemptDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, false, err
	}
	if err == nil {
		for _, attemptEntry := range attemptEntries {
			if attemptEntry.IsDir() {
				continue
			}
			if filepath.Ext(attemptEntry.Name()) != ".jsonl" {
				continue
			}
			path := filepath.Join(attemptDir, attemptEntry.Name())
			if err := readSubmissionLogLines(
				path,
				levelFilter,
				streamFilter,
				attemptIDFilter,
				&records,
			); err != nil {
				return nil, false, err
			}
		}
	}

	sort.SliceStable(records, func(i, j int) bool {
		if records[i].Timestamp.Equal(records[j].Timestamp) {
			return records[i].Order < records[j].Order
		}
		return records[i].Timestamp.Before(records[j].Timestamp)
	})

	total := len(records)
	truncated := total > tail
	start := 0
	if truncated {
		start = total - tail
	}

	filtered := make([]model.LogObservation, 0, total-start)
	for _, record := range records[start:] {
		filtered = append(filtered, record.Observation)
	}

	return filtered, truncated, nil
}

func (c *Controller) submissionLogPath(submissionID string) (string, error) {
	if c.logRootPath == "" {
		return "", fmt.Errorf("log root path is required")
	}
	return filepath.Join(c.logRootPath, "submissions", submissionID, "submission.jsonl"), nil
}

func readSubmissionLogLines(
	path string,
	levelFilter string,
	streamFilter string,
	attemptIDFilter string,
	records *[]submittedLogRecord,
) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var observation model.LogObservation
		if err := json.Unmarshal(line, &observation); err != nil {
			return err
		}
		if !matchesSubmissionLogFilter(observation, levelFilter, streamFilter, attemptIDFilter) {
			continue
		}
		ts, err := parseLogTimestamp(observation.Timestamp)
		if err != nil {
			return err
		}
		*records = append(*records, submittedLogRecord{
			Observation: observation,
			Timestamp:   ts,
			Order:       len(*records),
		})
	}
	return scanner.Err()
}

func matchesSubmissionLogFilter(
	observation model.LogObservation,
	levelFilter string,
	streamFilter string,
	attemptIDFilter string,
) bool {
	if levelFilter != "" && !model.IsAtLeastLogLevel(string(observation.Level), levelFilter) {
		return false
	}
	if streamFilter != "" && observation.Stream != streamFilter {
		return false
	}
	if attemptIDFilter != "" && observation.AttemptID != attemptIDFilter {
		return false
	}
	return true
}

func parseSubmissionLogTail(raw string, defaultTail int, maxTail int) (int, error) {
	if raw == "" {
		return defaultTail, nil
	}

	tail, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("tail must be a positive integer")
	}
	if tail <= 0 {
		return 0, fmt.Errorf("tail must be positive")
	}
	if tail > maxTail {
		return 0, fmt.Errorf("tail must be <= %d", maxTail)
	}
	return tail, nil
}

func parseSubmissionLogLevel(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	if _, err := model.CompareLogLevel(raw, string(model.LogLevelDebug)); err != nil {
		return "", err
	}
	return raw, nil
}

func parseSubmissionLogStream(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	switch model.LogStream(raw) {
	case model.LogStreamStdout, model.LogStreamStderr, model.LogStreamSystem:
		return raw, nil
	default:
		return "", fmt.Errorf("unsupported stream")
	}
}

func parseLogTimestamp(raw string) (time.Time, error) {
	if ts, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return ts, nil
	}
	return time.Parse(time.RFC3339, raw)
}

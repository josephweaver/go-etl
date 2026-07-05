package model

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"

	LogStreamStdout = "stdout"
	LogStreamStderr = "stderr"
	LogStreamSystem = "system"
)

type LogLevel string
type LogStream string

var logLevelOrder = map[LogLevel]int{
	LogLevelDebug: 0,
	LogLevelInfo:  1,
	LogLevelWarn:  2,
	LogLevelError: 3,
}

var validLogLevels = map[LogLevel]struct{}{
	LogLevelDebug: {},
	LogLevelInfo:  {},
	LogLevelWarn:  {},
	LogLevelError: {},
}

var validLogStreams = map[LogStream]struct{}{
	LogStreamStdout: {},
	LogStreamStderr: {},
	LogStreamSystem: {},
}

type LogObservation struct {
	ObservationID string `json:"observation_id,omitempty"`
	SubmissionID  string `json:"submission_id,omitempty"`
	WorkflowID    string `json:"workflow_id,omitempty"`
	WorkflowName  string `json:"workflow_name,omitempty"`
	RunID         string `json:"run_id,omitempty"`
	StepID        string `json:"step_id,omitempty"`
	StepName      string `json:"step_name,omitempty"`
	WorkItemID    string `json:"work_item_id,omitempty"`
	AttemptID     string `json:"attempt_id,omitempty"`
	WorkerID      string `json:"worker_id,omitempty"`

	Component string   `json:"component"`
	Stream    string   `json:"stream,omitempty"`
	Level     LogLevel `json:"level"`
	Timestamp string   `json:"timestamp"`
	Sequence  uint64   `json:"sequence,omitempty"`
	Message   string   `json:"message"`
}

func (observation LogObservation) Validate() error {
	if strings.TrimSpace(observation.Component) == "" {
		return fmt.Errorf("component is required")
	}

	if strings.TrimSpace(observation.Timestamp) == "" {
		return fmt.Errorf("timestamp is required")
	}

	timestamp, err := parseTimestamp(observation.Timestamp)
	if err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}
	if timestamp.IsZero() {
		return fmt.Errorf("timestamp must be non-zero")
	}

	if strings.TrimSpace(observation.Message) == "" {
		return fmt.Errorf("message is required")
	}

	if _, ok := validLogLevels[observation.Level]; !ok {
		return fmt.Errorf("invalid level: %s", observation.Level)
	}

	if observation.Stream != "" {
		if _, ok := validLogStreams[LogStream(observation.Stream)]; !ok {
			return fmt.Errorf("invalid stream: %s", observation.Stream)
		}
	}

	if err := validateLogObservationID("observation_id", observation.ObservationID); err != nil {
		return err
	}
	if err := validateLogObservationID("submission_id", observation.SubmissionID); err != nil {
		return err
	}
	if err := validateLogObservationID("workflow_id", observation.WorkflowID); err != nil {
		return err
	}
	if err := validateLogObservationID("run_id", observation.RunID); err != nil {
		return err
	}
	if err := validateLogObservationID("step_id", observation.StepID); err != nil {
		return err
	}
	if err := validateLogObservationID("work_item_id", observation.WorkItemID); err != nil {
		return err
	}
	if err := validateLogObservationID("attempt_id", observation.AttemptID); err != nil {
		return err
	}
	if err := validateLogObservationID("worker_id", observation.WorkerID); err != nil {
		return err
	}

	return nil
}

func IsAtLeastLogLevel(level, minimum string) bool {
	a, ok := logLevelOrder[LogLevel(level)]
	if !ok {
		return false
	}

	min, ok := logLevelOrder[LogLevel(minimum)]
	if !ok {
		return false
	}

	return a >= min
}

func CompareLogLevel(a, b string) (int, error) {
	aRank, ok := logLevelOrder[LogLevel(a)]
	if !ok {
		return 0, fmt.Errorf("invalid level: %s", a)
	}

	bRank, ok := logLevelOrder[LogLevel(b)]
	if !ok {
		return 0, fmt.Errorf("invalid level: %s", b)
	}

	switch {
	case aRank == bRank:
		return 0, nil
	case aRank < bRank:
		return -1, nil
	default:
		return 1, nil
	}
}

func parseTimestamp(value string) (time.Time, error) {
	if timestamp, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return timestamp, nil
	}
	if timestamp, err := time.Parse(time.RFC3339, value); err == nil {
		return timestamp, nil
	}

	return time.Time{}, fmt.Errorf("must be RFC3339 or RFC3339Nano timestamp")
}

func validateLogObservationID(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	cleaned := filepath.ToSlash(value)
	if strings.TrimSpace(cleaned) != cleaned {
		return fmt.Errorf("%s must not contain leading/trailing whitespace", field)
	}

	if strings.Contains(cleaned, "..") {
		return fmt.Errorf("%s must not contain path traversal", field)
	}

	if strings.ContainsAny(cleaned, "/\\") {
		return fmt.Errorf("%s must be a path-safe identifier", field)
	}

	if filepath.Base(cleaned) != cleaned {
		return fmt.Errorf("%s must be a path-safe identifier", field)
	}

	return nil
}

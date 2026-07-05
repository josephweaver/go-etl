package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"goetl/internal/model"
)

type logObservationSink interface {
	Write(model.LogObservation) error
}

type filesystemLogSink struct {
	rootPath string
	level    string
	mu       sync.Mutex
}

func newFilesystemLogSink(logRootPath, level string) (logObservationSink, error) {
	if strings.TrimSpace(logRootPath) == "" {
		return nil, fmt.Errorf("log root path is required")
	}
	if _, err := model.CompareLogLevel(level, string(model.LogLevelDebug)); err != nil {
		return nil, fmt.Errorf("invalid log level: %w", err)
	}

	return &filesystemLogSink{rootPath: filepath.Clean(logRootPath), level: level}, nil
}

func (s *filesystemLogSink) Write(observation model.LogObservation) error {
	if !model.IsAtLeastLogLevel(string(observation.Level), s.level) {
		return nil
	}

	path, err := sinkLogPath(s.rootPath, observation)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create log directory %s: %w", filepath.Dir(path), err)
	}

	line, err := json.Marshal(observation)
	if err != nil {
		return fmt.Errorf("marshal observation: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("append %s: %w", path, err)
	}

	return nil
}

func sinkLogPath(logRootPath string, observation model.LogObservation) (string, error) {
	if observation.SubmissionID == "" {
		return filepath.Join(logRootPath, "controller", "controller.jsonl"), nil
	}

	if err := validatePathID(observation.SubmissionID, "submission_id"); err != nil {
		return "", err
	}

	base := filepath.Join(logRootPath, "submissions", observation.SubmissionID)
	if observation.AttemptID == "" {
		return filepath.Join(base, "submission.jsonl"), nil
	}

	if err := validatePathID(observation.AttemptID, "attempt_id"); err != nil {
		return "", err
	}

	return filepath.Join(base, "attempts", fmt.Sprintf("%s.jsonl", observation.AttemptID)), nil
}

func validatePathID(value string, field string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}

	cleaned := filepath.ToSlash(value)
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

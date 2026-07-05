package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"goetl/internal/model"
)

const fallbackObservationsFileName = "fallback-observations.jsonl"

func appendFallbackLogObservation(logDir string, observation model.LogObservation) error {
	data, err := json.Marshal(observation)
	if err != nil {
		return fmt.Errorf("marshal fallback observation: %w", err)
	}

	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("create log directory %s: %w", logDir, err)
	}

	fallbackPath := filepath.Join(logDir, fallbackObservationsFileName)
	file, err := os.OpenFile(fallbackPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open fallback log file %s: %w", fallbackPath, err)
	}
	defer file.Close()

	if _, err := file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write fallback log file %s: %w", fallbackPath, err)
	}

	return nil
}

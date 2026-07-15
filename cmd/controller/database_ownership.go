package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func acquireControllerDatabaseOwnership(db *sql.DB) (func() error, error) {
	if db == nil {
		return nil, fmt.Errorf("controller startup database ownership: database handle is required")
	}

	var path string
	if err := db.QueryRow(`PRAGMA database_list`).Scan(new(int), new(string), &path); err != nil {
		return nil, fmt.Errorf("controller startup database ownership: inspect database path: %w", err)
	}
	if path == "" || path == ":memory:" {
		return func() error { return nil }, nil
	}

	return acquireControllerDatabaseOwnershipForPath(path)
}

func acquireControllerDatabaseOwnershipForPath(path string) (func() error, error) {
	if path == "" || path == ":memory:" {
		return func() error { return nil }, nil
	}
	if path != filepath.Clean(path) {
		path = filepath.Clean(path)
	}

	lockPath := path + ".controller.lock"
	if err := createControllerDatabaseLockFile(lockPath, path); err != nil {
		return nil, err
	}

	return func() error {
		if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("controller startup database ownership: remove lock file: %w", err)
		}
		return nil
	}, nil
}

type controllerDatabaseLockMetadata struct {
	PID          int    `json:"pid"`
	DatabasePath string `json:"database_path"`
	CreatedAt    string `json:"created_at"`
}

var controllerDatabaseLockOwnerActive = controllerDatabaseLockOwnerProcessActive

func createControllerDatabaseLockFile(lockPath string, databasePath string) error {
	if err := writeControllerDatabaseLockFile(lockPath, databasePath); err != nil {
		if !os.IsExist(err) {
			return fmt.Errorf("controller startup database ownership: create lock file: %w", err)
		}

		removed, removeErr := removeStaleControllerDatabaseLockFile(lockPath)
		if removeErr != nil {
			return removeErr
		}
		if !removed {
			return fmt.Errorf("controller startup database ownership: database is already owned")
		}
		if err := writeControllerDatabaseLockFile(lockPath, databasePath); err != nil {
			if os.IsExist(err) {
				return fmt.Errorf("controller startup database ownership: database is already owned")
			}
			return fmt.Errorf("controller startup database ownership: create lock file: %w", err)
		}
	}

	return nil
}

func writeControllerDatabaseLockFile(lockPath string, databasePath string) error {
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}

	closed := false
	defer func() {
		if !closed {
			_ = lockFile.Close()
		}
	}()
	removeAfterClose := func() {
		if !closed {
			_ = lockFile.Close()
			closed = true
		}
		_ = os.Remove(lockPath)
	}

	metadata := controllerDatabaseLockMetadata{
		PID:          os.Getpid(),
		DatabasePath: databasePath,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := json.NewEncoder(lockFile).Encode(metadata); err != nil {
		removeAfterClose()
		return err
	}
	if err := lockFile.Close(); err != nil {
		closed = true
		_ = os.Remove(lockPath)
		return err
	}
	closed = true
	return nil
}

func removeStaleControllerDatabaseLockFile(lockPath string) (bool, error) {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, fmt.Errorf("controller startup database ownership: read existing lock file: %w", err)
	}

	var metadata controllerDatabaseLockMetadata
	if err := json.Unmarshal(bytes.TrimSpace(data), &metadata); err != nil || metadata.PID <= 0 {
		return false, nil
	}

	active, err := controllerDatabaseLockOwnerActive(metadata.PID)
	if err != nil {
		return false, fmt.Errorf("controller startup database ownership: inspect lock owner pid %d: %w", metadata.PID, err)
	}
	if active {
		return false, nil
	}
	if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("controller startup database ownership: remove stale lock file: %w", err)
	}
	return true, nil
}

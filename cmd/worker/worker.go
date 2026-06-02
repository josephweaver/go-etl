package main

import (
	"fmt"
	"os"
	"path/filepath"
)

type Worker struct {
	Config Config
}

func (w Worker) Run(item WorkItem) error {
	fmt.Println("worker starting")
	fmt.Println("log dir:", w.Config.LogDir)

	if err := w.log("worker starting"); err != nil {
		return err
	}

	return w.runWorkItem(item)
}

func (w Worker) Validate() error {
	if err := requireDir(w.Config.LogDir); err != nil {
		return err
	}

	if err := requireDir(w.Config.TmpDir); err != nil {
		return err
	}

	if err := requireDir(w.Config.DataDir); err != nil {
		return err
	}

	return nil
}

func (w Worker) log(message string) error {
	path := filepath.Join(w.Config.LogDir, "worker.log")

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open log file %s: %w", path, err)
	}
	defer file.Close()

	if _, err := fmt.Fprintln(file, message); err != nil {
		return fmt.Errorf("write log file %s: %w", path, err)
	}
	return nil
}

func requireDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("check directory %s: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	return nil
}

func (w Worker) runWorkItem(item WorkItem) error {
	if err := item.Validate(); err != nil {
		return fmt.Errorf("invalid work item: %w", err)
	}

	tmpPath := filepath.Join(w.Config.TmpDir, item.OutputFilename)
	dataPath := filepath.Join(w.Config.DataDir, item.OutputFilename)

	if err := w.log("starting work item: " + item.ID); err != nil {
		return err
	}

	if err := os.WriteFile(tmpPath, []byte("completed "+item.ID+"\n"), 0644); err != nil {
		return fmt.Errorf("write temporary output %s: %w", tmpPath, err)
	}

	if err := w.log("wrote temporary output: " + tmpPath); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, dataPath); err != nil {
		return fmt.Errorf("move output from %s to %s: %w", tmpPath, dataPath, err)
	}

	return w.log("completed work item: " + item.ID)
}

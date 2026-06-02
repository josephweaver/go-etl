package main

import (
	"fmt"
	"os"
	"path/filepath"

	"goetl/internal/model"
)

func (w Worker) writeDemoOutput(item model.WorkItem) error {
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

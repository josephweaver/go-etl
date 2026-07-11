package main

import (
	"fmt"
	"os"
	"strings"

	"goetl/internal/model"
)

type SourceBundleProvider interface {
	SourceBundle(item model.WorkItem) ([]byte, error)
}

type ControllerSourceBundleProvider struct {
	Controller WorkerControllerClient
}

func (p ControllerSourceBundleProvider) SourceBundle(item model.WorkItem) ([]byte, error) {
	if item.Source == nil {
		return nil, fmt.Errorf("work item source is required")
	}
	runID := strings.TrimSpace(item.Source.RunID)
	if runID == "" {
		return nil, fmt.Errorf("work item source run id is required")
	}
	if !p.Controller.Initialized() {
		return nil, fmt.Errorf("controller source-bundle provider requires an initialized controller client")
	}
	return p.Controller.SourceBundle(runID)
}

type FileSourceBundleProvider struct {
	Path string
}

func (p FileSourceBundleProvider) SourceBundle(model.WorkItem) ([]byte, error) {
	if strings.TrimSpace(p.Path) == "" {
		return nil, fmt.Errorf("source-bundle path is required")
	}
	info, err := os.Stat(p.Path)
	if err != nil {
		return nil, fmt.Errorf("stat source-bundle file %s: %w", p.Path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("source-bundle path is not a regular file: %s", p.Path)
	}
	body, err := os.ReadFile(p.Path)
	if err != nil {
		return nil, fmt.Errorf("read source-bundle file %s: %w", p.Path, err)
	}
	return body, nil
}

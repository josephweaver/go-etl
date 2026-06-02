package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type WorkItemType string

const (
	WorkItemTypeWriteDemoOutput WorkItemType = "write_demo_output"
)

type WorkItem struct {
	ID             string       `json:"id"`
	Type           WorkItemType `json:"type"`
	OutputFilename string       `json:"output_filename"`
}

func loadWorkItem(path string) (WorkItem, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkItem{}, fmt.Errorf("read work item file %s: %w", path, err)
	}

	var item WorkItem
	if err := json.Unmarshal(data, &item); err != nil {
		return WorkItem{}, fmt.Errorf("decode work item file %s: %w", path, err)
	}

	if err := item.Validate(); err != nil {
		return WorkItem{}, fmt.Errorf("validate work item file %s: %w", path, err)
	}

	return item, nil
}

func (item WorkItem) Validate() error {
	if item.ID == "" {
		return fmt.Errorf("work item id is required")
	}

	if item.Type == "" {
		return fmt.Errorf("work item type is required")
	}

	if item.Type != WorkItemTypeWriteDemoOutput {
		return fmt.Errorf("unsupported work item type: %s", item.Type)
	}

	if item.OutputFilename == "" {
		return fmt.Errorf("output filename is required")
	}

	if filepath.Base(item.OutputFilename) != item.OutputFilename {
		return fmt.Errorf("output filename must not contain a directory: %s", item.OutputFilename)
	}

	return nil
}

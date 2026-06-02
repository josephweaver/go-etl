package model

import (
	"fmt"
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

type WorkCompletion struct {
	ID string `json:"id"`
}

func (item WorkItem) Validate() error {
	if item.ID == "" {
		return fmt.Errorf("work item id is required")
	}

	if item.Type == "" {
		return fmt.Errorf("work item type is required")
	}

	if item.OutputFilename == "" {
		return fmt.Errorf("output filename is required")
	}

	if filepath.Base(item.OutputFilename) != item.OutputFilename {
		return fmt.Errorf("output filename must not contain a directory: %s", item.OutputFilename)
	}

	return nil
}

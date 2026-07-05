package workflow

import (
	"fmt"

	"goetl/internal/model"
	"goetl/internal/variable"
)

type Step struct {
	ID           string
	FanOut       *FanOutStep
	ParallelWith string `json:"parallel_with,omitempty"`
}

func CompileStep(resolver variable.Resolver, step Step) ([]model.WorkItem, error) {
	if step.ID == "" {
		return nil, fmt.Errorf("workflow step id is required")
	}

	if step.FanOut == nil {
		return nil, fmt.Errorf("workflow step %s has no compiler", step.ID)
	}

	fanOut := *step.FanOut
	if fanOut.ID == "" {
		fanOut.ID = step.ID
	}
	if fanOut.WorkItem.IDPrefix == "" {
		fanOut.WorkItem.IDPrefix = step.ID
	}

	return CompileFanOutStep(resolver, fanOut)
}

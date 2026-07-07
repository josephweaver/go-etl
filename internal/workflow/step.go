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
	compiled, err := CompileStepItems(resolver, step)
	if err != nil {
		return nil, err
	}

	items := make([]model.WorkItem, 0, len(compiled))
	for _, item := range compiled {
		items = append(items, item.WorkItem)
	}
	return items, nil
}

func CompileStepItems(resolver variable.Resolver, step Step) ([]CompiledFanOutWorkItem, error) {
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

	return CompileFanOutStepItems(resolver, fanOut)
}

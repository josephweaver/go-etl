package workflow

import (
	"encoding/json"
	"fmt"
	"strings"

	"goetl/internal/document"
	"goetl/internal/model"
)

func WorkflowFromCanonicalDocument(doc document.CanonicalWorkflowDocument) (Workflow, error) {
	steps := make([]Step, 0, len(doc.Steps))
	for index, step := range doc.Steps {
		adapted, err := stepFromCanonical(step)
		if err != nil {
			return Workflow{}, fmt.Errorf("step %d (%s): %w", index, step.ID, err)
		}
		steps = append(steps, adapted)
	}
	return Workflow{
		ID:        doc.ID,
		Variables: doc.Variables,
		Steps:     steps,
	}, nil
}

func stepFromCanonical(step document.CanonicalWorkflowStep) (Step, error) {
	template, err := workItemTemplateFromCanonical(step)
	if err != nil {
		return Step{}, err
	}
	return Step{
		ID:           step.ID,
		ParallelWith: step.ParallelWith,
		FanOut: &FanOutStep{
			ID:       step.ID,
			WorkItem: template,
		},
	}, nil
}

func workItemTemplateFromCanonical(step document.CanonicalWorkflowStep) (FanOutWorkItemTemplate, error) {
	idAccessor, err := fanoutAccessorFromExpression(step.FanOut.ID)
	if err != nil {
		return FanOutWorkItemTemplate{}, fmt.Errorf("fan_out.id: %w", err)
	}
	parameters, err := parametersFromCanonical(step.Work.Parameters)
	if err != nil {
		return FanOutWorkItemTemplate{}, err
	}
	constraints, err := resourceConstraintsFromCanonical(step.Work.ResourceConstraints)
	if err != nil {
		return FanOutWorkItemTemplate{}, err
	}

	return FanOutWorkItemTemplate{
		FanOutExpression:    step.FanOut.Over,
		IDTokenAccessor:     idAccessor,
		OutputAccessor:      idAccessor,
		Type:                model.WorkItemType(step.Work.Type),
		OutputPrefix:        defaultString(step.Work.OutputPrefix, step.ID),
		OutputExtension:     defaultString(step.Work.OutputExtension, ".json"),
		Parameters:          parameters,
		ParameterAccessors:  step.Work.ParameterAccessors,
		ResourceConstraints: constraints,
	}, nil
}

func fanoutAccessorFromExpression(expression string) (string, error) {
	inner, ok := strings.CutPrefix(expression, "${")
	if !ok {
		return "", fmt.Errorf("must be a ${fanout...} expression")
	}
	inner, ok = strings.CutSuffix(inner, "}")
	if !ok {
		return "", fmt.Errorf("must be a ${fanout...} expression")
	}
	inner = strings.TrimSpace(inner)
	if inner == "fanout" {
		return "", nil
	}
	if strings.HasPrefix(inner, "fanout.") {
		field := strings.TrimPrefix(inner, "fanout")
		if field == "." {
			return "", fmt.Errorf("must identify a fanout field")
		}
		return field, nil
	}
	return "", fmt.Errorf("must reference fanout or fanout.<field>")
}

func parametersFromCanonical(values map[string]any) (model.Parameters, error) {
	if len(values) == 0 {
		return nil, nil
	}
	parameters := make(model.Parameters, len(values))
	for name, value := range values {
		typed, err := document.TypedExpressionFromValue(value)
		if err != nil {
			return nil, fmt.Errorf("parameter %s: %w", name, err)
		}
		parameters[name] = model.Parameter{
			Type:  typed.Type.String(),
			Value: typed.Expression,
		}
	}
	return parameters, nil
}

func resourceConstraintsFromCanonical(values []any) ([]ResourceConstraintDeclaration, error) {
	if len(values) == 0 {
		return nil, nil
	}
	data, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("encode resource_constraints: %w", err)
	}
	var constraints []ResourceConstraintDeclaration
	if err := json.Unmarshal(data, &constraints); err != nil {
		return nil, fmt.Errorf("decode resource_constraints: %w", err)
	}
	return constraints, nil
}

func defaultString(value string, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

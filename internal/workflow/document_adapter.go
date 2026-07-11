package workflow

import (
	"encoding/json"
	"fmt"
	"strings"

	"goetl/internal/document"
	"goetl/internal/model"
	"goetl/internal/variable"
)

func WorkflowFromCanonicalDocument(doc document.CanonicalWorkflowDocument) (Workflow, error) {
	definitions, err := document.DataDefinitionsFromValue(doc.Data)
	if err != nil {
		return Workflow{}, fmt.Errorf("data: %w", err)
	}
	steps := make([]Step, 0, len(doc.Steps))
	for index, step := range doc.Steps {
		adapted, err := stepFromCanonical(step, definitions)
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

func stepFromCanonical(step document.CanonicalWorkflowStep, definitions model.DataDefinitions) (Step, error) {
	template, err := workItemTemplateFromCanonical(step, definitions)
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

func workItemTemplateFromCanonical(step document.CanonicalWorkflowStep, definitions model.DataDefinitions) (FanOutWorkItemTemplate, error) {
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
	explicitCache, err := explicitCacheDataFromCanonical(step, definitions)
	if err != nil {
		return FanOutWorkItemTemplate{}, err
	}
	explicitCommit, err := explicitCommitDataFromCanonical(step, definitions)
	if err != nil {
		return FanOutWorkItemTemplate{}, err
	}
	if err := rejectCanonicalHiddenPlannerParameters(step); err != nil {
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
		ExplicitCacheData:   explicitCache,
		ExplicitCommitData:  explicitCommit,
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

func explicitCacheDataFromCanonical(step document.CanonicalWorkflowStep, definitions model.DataDefinitions) (*ExplicitCacheDataTemplate, error) {
	raw, hasMaterialize := step.Data["materialize"]
	if !hasMaterialize {
		if model.WorkItemType(step.Work.Type) == model.WorkItemTypeCacheData {
			return nil, fmt.Errorf("cache_data step requires data.materialize")
		}
		return nil, nil
	}
	if model.WorkItemType(step.Work.Type) != model.WorkItemTypeCacheData {
		return nil, fmt.Errorf("data.materialize requires work.type %q", model.WorkItemTypeCacheData)
	}
	items, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("data.materialize must be an object")
	}
	if len(items) != 1 {
		return nil, fmt.Errorf("data.materialize must contain exactly one alias")
	}

	var alias string
	var rawBinding any
	for key, value := range items {
		alias = key
		rawBinding = value
		break
	}
	fields, ok := rawBinding.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("data.materialize.%s must be an object", alias)
	}
	assetName, err := canonicalStringField(fields, "asset", "data.materialize."+alias)
	if err != nil {
		return nil, err
	}
	with, err := canonicalTypedExpressionMap(fields, "with", "data.materialize."+alias)
	if err != nil {
		return nil, err
	}
	selection, err := canonicalStringListField(fields, "select", "data.materialize."+alias)
	if err != nil {
		return nil, err
	}
	return &ExplicitCacheDataTemplate{
		Definitions: definitions,
		Alias:       alias,
		Asset:       assetName,
		Selection:   selection,
		With:        with,
	}, nil
}

func explicitCommitDataFromCanonical(step document.CanonicalWorkflowStep, definitions model.DataDefinitions) (*ExplicitCommitDataTemplate, error) {
	raw, hasOutputs := step.Data["outputs"]
	if !hasOutputs {
		if model.WorkItemType(step.Work.Type) == model.WorkItemTypeCommitData {
			return nil, fmt.Errorf("commit_data step requires data.outputs")
		}
		return nil, nil
	}
	if model.WorkItemType(step.Work.Type) != model.WorkItemTypeCommitData {
		return nil, fmt.Errorf("data.outputs requires work.type %q", model.WorkItemTypeCommitData)
	}
	items, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("data.outputs must be an object")
	}
	if len(items) != 1 {
		return nil, fmt.Errorf("data.outputs must contain exactly one output")
	}

	var alias string
	var rawBinding any
	for key, value := range items {
		alias = key
		rawBinding = value
		break
	}
	fields, ok := rawBinding.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("data.outputs.%s must be an object", alias)
	}
	fromFields, err := requiredCanonicalObjectField(fields, "from", "data.outputs."+alias)
	if err != nil {
		return nil, err
	}
	fromStep, err := canonicalStringField(fromFields, "step", "data.outputs."+alias+".from")
	if err != nil {
		return nil, err
	}
	fromArtifact, err := canonicalStringField(fromFields, "artifact", "data.outputs."+alias+".from")
	if err != nil {
		return nil, err
	}
	target, err := canonicalStringField(fields, "target", "data.outputs."+alias)
	if err != nil {
		return nil, err
	}
	with, err := canonicalTypedExpressionMap(fields, "with", "data.outputs."+alias)
	if err != nil {
		return nil, err
	}
	return &ExplicitCommitDataTemplate{
		Definitions:  definitions,
		Alias:        alias,
		Target:       target,
		FromStep:     fromStep,
		FromArtifact: fromArtifact,
		With:         with,
	}, nil
}

func rejectCanonicalHiddenPlannerParameters(step document.CanonicalWorkflowStep) error {
	if model.WorkItemType(step.Work.Type) == model.WorkItemTypeCommitData {
		return nil
	}
	for name := range step.Work.Parameters {
		if name == "data_assets" {
			return fmt.Errorf("canonical work parameter %q is not allowed; use an explicit cache_data step with data.materialize", name)
		}
		if name == "publish" || name == "publish_targets" {
			return fmt.Errorf("canonical work parameter %q is not allowed; use an explicit commit_data step with data.outputs", name)
		}
	}
	return nil
}

func requiredCanonicalObjectField(fields map[string]any, name string, context string) (map[string]any, error) {
	value, ok := fields[name]
	if !ok {
		return nil, fmt.Errorf("%s %s is required", context, name)
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s %s must be an object", context, name)
	}
	return object, nil
}

func canonicalStringField(fields map[string]any, name string, context string) (string, error) {
	value, ok := fields[name]
	if !ok {
		return "", fmt.Errorf("%s %s is required", context, name)
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("%s %s must be a non-empty string", context, name)
	}
	return text, nil
}

func canonicalTypedExpressionMap(fields map[string]any, name string, context string) (map[string]variable.TypedExpression, error) {
	value, ok := fields[name]
	if !ok {
		return nil, nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s %s must be an object", context, name)
	}
	result := make(map[string]variable.TypedExpression, len(object))
	for key, item := range object {
		expression, err := document.TypedExpressionFromValue(item)
		if err != nil {
			return nil, fmt.Errorf("%s %s.%s: %w", context, name, key, err)
		}
		result[key] = expression
	}
	return result, nil
}

func canonicalStringListField(fields map[string]any, name string, context string) ([]string, error) {
	value, ok := fields[name]
	if !ok {
		return nil, nil
	}
	list, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s %s must be a list", context, name)
	}
	values := make([]string, 0, len(list))
	for index, item := range list {
		text, ok := item.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("%s %s[%d] must be a non-empty string", context, name, index)
		}
		values = append(values, text)
	}
	return values, nil
}

func defaultString(value string, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

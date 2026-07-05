package workflow

import (
	"fmt"
	"strings"

	"goetl/internal/model"
	"goetl/internal/variable"
)

type FanOutWorkItemTemplate struct {
	FanOutExpression   string
	TokenAccessor      string
	IDTokenAccessor    string
	OutputAccessor     string
	Type               model.WorkItemType
	IDPrefix           string
	OutputPrefix       string
	OutputExtension    string
	Parameters         model.Parameters
	ParameterAccessors map[string]string
}

type FanOutStep struct {
	ID       string
	WorkItem FanOutWorkItemTemplate
}

func CompileFanOutStep(resolver variable.Resolver, step FanOutStep) ([]model.WorkItem, error) {
	if step.ID == "" {
		return nil, fmt.Errorf("workflow step id is required")
	}

	return CompileFanOutWorkItems(resolver, step.WorkItem)
}

func CompileFanOutWorkItems(resolver variable.Resolver, template FanOutWorkItemTemplate) ([]model.WorkItem, error) {
	values, err := resolver.ResolveFanOutExpression(template.FanOutExpression)
	if err != nil {
		return nil, err
	}

	items := make([]model.WorkItem, 0, len(values))
	for index, value := range values {
		idToken, err := fanOutTemplateToken(value, template.TokenAccessor, template.IDTokenAccessor)
		if err != nil {
			return nil, fmt.Errorf("compile fan-out item %d id token: %w", index, err)
		}

		outputToken, err := fanOutTemplateToken(value, template.TokenAccessor, template.OutputAccessor)
		if err != nil {
			return nil, fmt.Errorf("compile fan-out item %d output token: %w", index, err)
		}

		item := model.WorkItem{
			ID:             fmt.Sprintf("%s-%s", template.IDPrefix, idToken),
			Type:           template.Type,
			OutputFilename: fmt.Sprintf("%s-%s%s", template.OutputPrefix, outputToken, template.OutputExtension),
			Parameters:     copyParameters(template.Parameters),
		}
		if err := bindParameterAccessors(item.Parameters, value, template.ParameterAccessors); err != nil {
			return nil, fmt.Errorf("compile fan-out item %d parameters: %w", index, err)
		}
		if err := item.ValidateForWorkflowCompile(); err != nil {
			return nil, fmt.Errorf("compile fan-out item %d: %w", index, err)
		}

		items = append(items, item)
	}

	return items, nil
}

func copyParameters(parameters model.Parameters) model.Parameters {
	if len(parameters) == 0 {
		return nil
	}

	copied := make(model.Parameters, len(parameters))
	for name, parameter := range parameters {
		copied[name] = parameter
	}
	return copied
}

func bindParameterAccessors(parameters model.Parameters, value variable.ResolvedValue, accessors map[string]string) error {
	if len(accessors) == 0 {
		return nil
	}

	for name, accessor := range accessors {
		if parameters == nil {
			return fmt.Errorf("parameter %s has accessor but no parameter template", name)
		}

		parameter, ok := parameters[name]
		if !ok {
			return fmt.Errorf("parameter %s accessor has no parameter template", name)
		}

		resolved, err := variable.ApplyAccessor(value, accessor)
		if err != nil {
			return fmt.Errorf("parameter %s: %w", name, err)
		}

		parameter.Value = resolved.Value
		parameters[name] = parameter
	}

	return nil
}

func fanOutTemplateToken(value variable.ResolvedValue, fallbackAccessor string, accessor string) (string, error) {
	if accessor == "" {
		accessor = fallbackAccessor
	}

	if accessor != "" {
		resolved, err := variable.ApplyAccessor(value, accessor)
		if err != nil {
			return "", err
		}
		value = resolved
	}

	return fanOutToken(value)
}

func fanOutToken(value variable.ResolvedValue) (string, error) {
	var token string
	switch value.Type.Kind {
	case variable.KindString, variable.KindPath:
		token = fmt.Sprint(value.Value)
	case variable.KindInt:
		token = fmt.Sprintf("%d", value.Value)
	default:
		return "", fmt.Errorf("unsupported fan-out value type: %s", value.Type)
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("fan-out value token is required")
	}

	return token, nil
}

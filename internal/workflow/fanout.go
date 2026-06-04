package workflow

import (
	"fmt"
	"strings"

	"goetl/internal/model"
	"goetl/internal/variable"
)

type FanOutWorkItemTemplate struct {
	FanOutExpression string
	TokenAccessor    string
	IDTokenAccessor  string
	OutputAccessor   string
	Type             model.WorkItemType
	IDPrefix         string
	OutputPrefix     string
	OutputExtension  string
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
		}
		if err := item.Validate(); err != nil {
			return nil, fmt.Errorf("compile fan-out item %d: %w", index, err)
		}

		items = append(items, item)
	}

	return items, nil
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

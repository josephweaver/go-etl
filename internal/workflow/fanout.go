package workflow

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"goetl/internal/model"
	"goetl/internal/variable"
)

type FanOutWorkItemTemplate struct {
	FanOutExpression    string
	TokenAccessor       string
	IDTokenAccessor     string
	OutputAccessor      string
	Type                model.WorkItemType
	IDPrefix            string
	OutputPrefix        string
	OutputExtension     string
	Parameters          model.Parameters
	ParameterAccessors  map[string]string
	ResourceConstraints []ResourceConstraintDeclaration `json:"resource_constraints,omitempty"`
	ExplicitCacheData   *ExplicitCacheDataTemplate      `json:"explicit_cache_data,omitempty"`
}

type ResourceConstraintDeclaration struct {
	ResourceKey            variable.TypedExpression
	RequestedUnits         variable.TypedExpression
	Operator               variable.TypedExpression
	TargetUnits            variable.TypedExpression
	ResourceKeyAccessor    string `json:"resource_key_accessor,omitempty"`
	RequestedUnitsAccessor string `json:"requested_units_accessor,omitempty"`
	OperatorAccessor       string `json:"operator_accessor,omitempty"`
	TargetUnitsAccessor    string `json:"target_units_accessor,omitempty"`
}

type CompiledFanOutWorkItem struct {
	WorkItem            model.WorkItem
	ResourceConstraints []model.WorkItemResourceConstraint
}

func ResolveResourceConstraints(
	resolver variable.Resolver,
	workItemID string,
	declarations []ResourceConstraintDeclaration,
	createdAt string,
) ([]model.WorkItemResourceConstraint, error) {
	constraints, err := resolveResourceConstraintDeclarations(resolver, variable.ResolvedValue{}, workItemID, declarations)
	if err != nil {
		return nil, err
	}
	for index := range constraints {
		constraints[index].CreatedAt = createdAt
		if err := constraints[index].Validate(); err != nil {
			return nil, err
		}
	}
	return constraints, nil
}

type FanOutStep struct {
	ID       string
	WorkItem FanOutWorkItemTemplate
}

func CompileFanOutStep(resolver variable.Resolver, step FanOutStep) ([]model.WorkItem, error) {
	compiled, err := CompileFanOutStepItems(resolver, step)
	if err != nil {
		return nil, err
	}

	items := make([]model.WorkItem, 0, len(compiled))
	for _, item := range compiled {
		items = append(items, item.WorkItem)
	}
	return items, nil
}

func CompileFanOutStepItems(resolver variable.Resolver, step FanOutStep) ([]CompiledFanOutWorkItem, error) {
	if step.ID == "" {
		return nil, fmt.Errorf("workflow step id is required")
	}

	return CompileFanOutWorkItemResults(resolver, step.WorkItem)
}

func CompileFanOutWorkItems(resolver variable.Resolver, template FanOutWorkItemTemplate) ([]model.WorkItem, error) {
	compiled, err := CompileFanOutWorkItemResults(resolver, template)
	if err != nil {
		return nil, err
	}

	items := make([]model.WorkItem, 0, len(compiled))
	for _, item := range compiled {
		items = append(items, item.WorkItem)
	}
	return items, nil
}

func CompileFanOutWorkItemResults(resolver variable.Resolver, template FanOutWorkItemTemplate) ([]CompiledFanOutWorkItem, error) {
	values, err := resolver.ResolveFanOutExpression(template.FanOutExpression)
	if err != nil {
		return nil, err
	}

	items := make([]CompiledFanOutWorkItem, 0, len(values))
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
		explicitConstraints, err := compileExplicitCacheDataWorkItem(resolver, value, &item, template.ExplicitCacheData)
		if err != nil {
			return nil, fmt.Errorf("compile fan-out item %d explicit cache_data: %w", index, err)
		}
		if err := item.ValidateForWorkflowCompile(); err != nil {
			return nil, fmt.Errorf("compile fan-out item %d: %w", index, err)
		}

		constraints, err := resolveResourceConstraintDeclarations(resolver, value, item.ID, template.ResourceConstraints)
		if err != nil {
			return nil, fmt.Errorf("compile fan-out item %d resource constraints: %w", index, err)
		}
		if len(explicitConstraints) > 0 {
			constraints = appendExplicitResourceConstraints(explicitConstraints, constraints)
		}

		items = append(items, CompiledFanOutWorkItem{
			WorkItem:            item,
			ResourceConstraints: constraints,
		})
	}

	return items, nil
}

func (d *ResourceConstraintDeclaration) UnmarshalJSON(data []byte) error {
	var encoded struct {
		ResourceKey            json.RawMessage `json:"resource_key"`
		RequestedUnits         json.RawMessage `json:"requested_units"`
		Operator               json.RawMessage `json:"operator"`
		TargetUnits            json.RawMessage `json:"target_units"`
		ResourceKeyAccessor    string          `json:"resource_key_accessor,omitempty"`
		RequestedUnitsAccessor string          `json:"requested_units_accessor,omitempty"`
		OperatorAccessor       string          `json:"operator_accessor,omitempty"`
		TargetUnitsAccessor    string          `json:"target_units_accessor,omitempty"`
	}
	if err := json.Unmarshal(data, &encoded); err != nil {
		return err
	}

	resourceKey, err := decodeConstraintExpression("resource_key", encoded.ResourceKey, variable.TypeString)
	if err != nil {
		return err
	}
	requestedUnits, err := decodeConstraintExpression("requested_units", encoded.RequestedUnits, variable.TypeInt)
	if err != nil {
		return err
	}
	operator, err := decodeConstraintExpression("operator", encoded.Operator, variable.TypeString)
	if err != nil {
		return err
	}
	targetUnits, err := decodeConstraintExpression("target_units", encoded.TargetUnits, variable.TypeInt)
	if err != nil {
		return err
	}

	d.ResourceKey = resourceKey
	d.RequestedUnits = requestedUnits
	d.Operator = operator
	d.TargetUnits = targetUnits
	d.ResourceKeyAccessor = encoded.ResourceKeyAccessor
	d.RequestedUnitsAccessor = encoded.RequestedUnitsAccessor
	d.OperatorAccessor = encoded.OperatorAccessor
	d.TargetUnitsAccessor = encoded.TargetUnitsAccessor
	return nil
}

func decodeConstraintExpression(name string, data json.RawMessage, defaultType variable.Type) (variable.TypedExpression, error) {
	if len(data) == 0 {
		return variable.TypedExpression{}, fmt.Errorf("%s is required", name)
	}

	var typed variable.TypedExpression
	if err := json.Unmarshal(data, &typed); err == nil {
		if err := typed.ValidateDefinition(); err != nil {
			return variable.TypedExpression{}, fmt.Errorf("%s: %w", name, err)
		}
		return typed, nil
	}

	var value any
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return variable.TypedExpression{}, fmt.Errorf("decode %s: %w", name, err)
	}

	expression := variable.TypedExpression{Type: defaultType, Expression: value}
	if err := expression.ValidateDefinition(); err != nil {
		return variable.TypedExpression{}, fmt.Errorf("%s: %w", name, err)
	}
	return expression, nil
}

func resolveResourceConstraintDeclarations(
	resolver variable.Resolver,
	fanOutValue variable.ResolvedValue,
	workItemID string,
	declarations []ResourceConstraintDeclaration,
) ([]model.WorkItemResourceConstraint, error) {
	if len(declarations) == 0 {
		return nil, nil
	}

	constraints := make([]model.WorkItemResourceConstraint, 0, len(declarations))
	seenResourceKeys := make(map[string]bool, len(declarations))
	for index, declaration := range declarations {
		resourceKey, err := resolveConstraintString(resolver, fanOutValue, declaration.ResourceKey, declaration.ResourceKeyAccessor, "resource_key")
		if err != nil {
			return nil, fmt.Errorf("constraint %d resource_key: %w", index, err)
		}
		requestedUnits, err := resolveConstraintInt(resolver, fanOutValue, declaration.RequestedUnits, declaration.RequestedUnitsAccessor, "requested_units")
		if err != nil {
			return nil, fmt.Errorf("constraint %d requested_units: %w", index, err)
		}
		operator, err := resolveConstraintString(resolver, fanOutValue, declaration.Operator, declaration.OperatorAccessor, "operator")
		if err != nil {
			return nil, fmt.Errorf("constraint %d operator: %w", index, err)
		}
		targetUnits, err := resolveConstraintInt(resolver, fanOutValue, declaration.TargetUnits, declaration.TargetUnitsAccessor, "target_units")
		if err != nil {
			return nil, fmt.Errorf("constraint %d target_units: %w", index, err)
		}

		if seenResourceKeys[resourceKey] {
			return nil, fmt.Errorf("duplicate resource_key %q", resourceKey)
		}
		seenResourceKeys[resourceKey] = true

		constraint := model.WorkItemResourceConstraint{
			WorkItemID:      workItemID,
			ConstraintIndex: index,
			ResourceKey:     resourceKey,
			RequestedUnits:  requestedUnits,
			Operator:        model.WorkItemResourceConstraintOperator(operator),
			TargetUnits:     targetUnits,
		}
		if err := validateCompiledResourceConstraint(constraint); err != nil {
			return nil, fmt.Errorf("constraint %d: %w", index, err)
		}
		constraints = append(constraints, constraint)
	}
	return constraints, nil
}

func resolveConstraintString(resolver variable.Resolver, fanOutValue variable.ResolvedValue, expression variable.TypedExpression, accessor string, name string) (string, error) {
	value, err := resolveConstraintValue(resolver, fanOutValue, expression, accessor, name)
	if err != nil {
		return "", err
	}
	if value.Type != variable.TypeString && value.Type != variable.TypePath {
		return "", fmt.Errorf("has type %s, want string or path", value.Type)
	}
	text, ok := value.Value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("is required")
	}
	return text, nil
}

func resolveConstraintInt(resolver variable.Resolver, fanOutValue variable.ResolvedValue, expression variable.TypedExpression, accessor string, name string) (int, error) {
	value, err := resolveConstraintValue(resolver, fanOutValue, expression, accessor, name)
	if err != nil {
		return 0, err
	}
	if value.Type != variable.TypeInt {
		return 0, fmt.Errorf("has type %s, want int", value.Type)
	}
	integer, ok := value.Value.(int)
	if !ok {
		return 0, fmt.Errorf("must be an int")
	}
	return integer, nil
}

func resolveConstraintValue(resolver variable.Resolver, fanOutValue variable.ResolvedValue, expression variable.TypedExpression, accessor string, name string) (variable.ResolvedValue, error) {
	if accessor != "" {
		value, err := variable.ApplyAccessor(fanOutValue, accessor)
		if err != nil {
			return variable.ResolvedValue{}, err
		}
		return value, nil
	}

	if text, ok := expression.Expression.(string); ok && strings.HasPrefix(text, "${") && strings.HasSuffix(text, "}") {
		referenceText := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(text, "${"), "}"))
		if strings.HasPrefix(referenceText, string(variable.NamespaceStep)+".") {
			field := strings.TrimPrefix(referenceText, string(variable.NamespaceStep))
			return variable.ApplyAccessor(fanOutValue, field)
		}

		reference, err := variable.ParseReference(referenceText)
		if err != nil {
			return variable.ResolvedValue{}, err
		}
		return resolver.Resolve(reference)
	}

	return literalConstraintValue(expression, name)
}

func literalConstraintValue(expression variable.TypedExpression, name string) (variable.ResolvedValue, error) {
	switch expression.Type {
	case variable.TypeString, variable.TypePath:
		text, ok := expression.Expression.(string)
		if !ok {
			return variable.ResolvedValue{}, fmt.Errorf("%s expression must be a string", name)
		}
		return variable.ResolvedValue{Type: expression.Type, Value: text}, nil
	case variable.TypeInt:
		switch value := expression.Expression.(type) {
		case int:
			return variable.ResolvedValue{Type: variable.TypeInt, Value: value}, nil
		case json.Number:
			integer, err := strconv.Atoi(value.String())
			if err != nil {
				return variable.ResolvedValue{}, fmt.Errorf("parse %s: %w", name, err)
			}
			return variable.ResolvedValue{Type: variable.TypeInt, Value: integer}, nil
		default:
			return variable.ResolvedValue{}, fmt.Errorf("%s expression must be an integer", name)
		}
	default:
		return variable.ResolvedValue{}, fmt.Errorf("%s has type %s, want scalar resource constraint value", name, expression.Type)
	}
}

func validateCompiledResourceConstraint(constraint model.WorkItemResourceConstraint) error {
	if strings.TrimSpace(constraint.WorkItemID) == "" {
		return fmt.Errorf("work item id is required")
	}
	if constraint.ConstraintIndex < 0 {
		return fmt.Errorf("constraint index must be non-negative")
	}
	if strings.TrimSpace(constraint.ResourceKey) == "" {
		return fmt.Errorf("resource key is required")
	}
	if constraint.RequestedUnits <= 0 {
		return fmt.Errorf("requested units must be greater than 0")
	}
	if !supportedResourceConstraintOperator(constraint.Operator) {
		return fmt.Errorf("unsupported resource constraint operator %q", constraint.Operator)
	}
	if constraint.TargetUnits < 0 {
		return fmt.Errorf("target units must be non-negative")
	}
	return nil
}

func supportedResourceConstraintOperator(operator model.WorkItemResourceConstraintOperator) bool {
	switch operator {
	case model.WorkItemResourceConstraintOperatorEqual,
		model.WorkItemResourceConstraintOperatorNotEqual,
		model.WorkItemResourceConstraintOperatorLessThan,
		model.WorkItemResourceConstraintOperatorGreater,
		model.WorkItemResourceConstraintOperatorLessEq,
		model.WorkItemResourceConstraintOperatorGreaterEq:
		return true
	default:
		return false
	}
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

		parameter.Value = parameterValueFromResolved(resolved)
		parameter.Sensitive = resolved.Sensitive
		parameter.RedactionLabel = resolved.RedactionLabel
		parameter.ProtectedRef = resolved.ProtectedRef
		parameters[name] = parameter
	}

	return nil
}

func parameterValueFromResolved(value variable.ResolvedValue) any {
	switch value.Type.Kind {
	case variable.KindObject:
		fields := make(map[string]any, len(value.Object))
		for name, field := range value.Object {
			fields[name] = parameterValueFromResolved(field)
		}
		return fields
	case variable.KindList:
		items := make([]any, 0, len(value.List))
		for _, item := range value.List {
			items = append(items, parameterValueFromResolved(item))
		}
		return items
	default:
		return value.Value
	}
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

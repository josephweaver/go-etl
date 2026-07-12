package workflow

import (
	"fmt"
	"sort"
	"strings"

	fp "goetl/internal/fingerprint"
	"goetl/internal/model"
	"goetl/internal/variable"
)

type DataAssetCollectionPlan struct {
	Asset                 string
	MaterializationDomain string
	DimensionOrder        []string
	Dimensions            map[string][]variable.ResolvedValue
	FixedParameters       map[string]variable.ResolvedValue
	Selection             []string
	PathTemplate          string
	CollectionFingerprint string
	Members               []DataAssetCollectionMember
}

type DataAssetCollectionMember struct {
	Index                   int
	Bindings                map[string]variable.ResolvedValue
	Instance                DataAssetInstance
	DestinationRelativePath string
	MaterializationKey      string
}

func PlanDataAssetCollection(
	resolver variable.Resolver,
	context FanOutItemContext,
	definitionName string,
	definition model.DataInputDefinition,
	selection []string,
	bindings map[string]variable.TypedExpression,
	materializationDomainID string,
) (DataAssetCollectionPlan, error) {
	if strings.TrimSpace(materializationDomainID) == "" {
		return DataAssetCollectionPlan{}, fmt.Errorf("materialization domain id is required")
	}
	effectiveSelection, err := definition.EffectiveSelection(selection)
	if err != nil {
		return DataAssetCollectionPlan{}, err
	}
	dimensionNames := collectionDimensionNames(definition.Collection)
	dimensionSet := stringSet(dimensionNames)
	fixed, err := resolveFixedAssetParameters(resolver, context, definition, bindings, dimensionSet)
	if err != nil {
		return DataAssetCollectionPlan{}, err
	}
	dimensions, err := collectionDimensionValues(definition)
	if err != nil {
		return DataAssetCollectionPlan{}, err
	}
	pathTemplate, _, err := collectionPlanPathTemplate(definition, fixed)
	if err != nil {
		return DataAssetCollectionPlan{}, err
	}

	plan := DataAssetCollectionPlan{
		Asset:                 definitionName,
		MaterializationDomain: materializationDomainID,
		DimensionOrder:        dimensionNames,
		Dimensions:            dimensions,
		FixedParameters:       fixed,
		Selection:             effectiveSelection,
		PathTemplate:          pathTemplate,
	}
	plan.CollectionFingerprint, err = CollectionFingerprint(plan)
	if err != nil {
		return DataAssetCollectionPlan{}, err
	}

	memberBindings := expandedMemberBindings(dimensionNames, dimensions, fixed)
	seenTuples := map[string]struct{}{}
	seenDestinations := map[string]DataAssetCollectionMember{}
	plan.Members = make([]DataAssetCollectionMember, 0, len(memberBindings))
	for index, binding := range memberBindings {
		tupleKey, err := resolvedBindingFingerprint(binding)
		if err != nil {
			return DataAssetCollectionPlan{}, err
		}
		if _, duplicate := seenTuples[tupleKey]; duplicate {
			return DataAssetCollectionPlan{}, fmt.Errorf("duplicate collection member tuple at index %d", index)
		}
		seenTuples[tupleKey] = struct{}{}

		instance, err := instantiateDataAssetWithContext(resolver, context, definitionName, definition, effectiveSelection, typedExpressionsFromResolved(binding))
		if err != nil {
			return DataAssetCollectionPlan{}, fmt.Errorf("collection member %d: %w", index, err)
		}
		destination, err := concreteDestinationRelativePath(definition, binding)
		if err != nil {
			return DataAssetCollectionPlan{}, fmt.Errorf("collection member %d: %w", index, err)
		}
		materializationKey, err := MaterializationIdentityKey(instance.AssetKey, materializationDomainID, destination)
		if err != nil {
			return DataAssetCollectionPlan{}, fmt.Errorf("collection member %d: %w", index, err)
		}
		member := DataAssetCollectionMember{
			Index:                   index,
			Bindings:                binding,
			Instance:                instance,
			DestinationRelativePath: destination,
			MaterializationKey:      materializationKey,
		}
		if previous, exists := seenDestinations[destination]; exists && destination != "" {
			if previous.Instance.AssetKey != instance.AssetKey {
				return DataAssetCollectionPlan{}, fmt.Errorf(
					"destination %q collision: source asset keys %s and %s",
					destination,
					previous.Instance.AssetKey,
					instance.AssetKey,
				)
			}
			return DataAssetCollectionPlan{}, fmt.Errorf("destination %q is produced by multiple collection members", destination)
		}
		seenDestinations[destination] = member
		plan.Members = append(plan.Members, member)
	}
	return plan, nil
}

func MaterializationIdentityKey(sourceAssetKey string, materializationDomainID string, destinationRelativePath string) (string, error) {
	if strings.TrimSpace(sourceAssetKey) == "" {
		return "", fmt.Errorf("source asset key is required")
	}
	if strings.TrimSpace(materializationDomainID) == "" {
		return "", fmt.Errorf("materialization domain id is required")
	}
	identity := map[string]any{
		"source_asset_key":          sourceAssetKey,
		"materialization_domain_id": materializationDomainID,
		"destination_relative_path": destinationRelativePath,
	}
	_, hash, err := fp.CanonicalJSONSHA256(normalizedCanonicalValue(identity))
	if err != nil {
		return "", err
	}
	return "sha256:" + hash, nil
}

func CollectionFingerprint(plan DataAssetCollectionPlan) (string, error) {
	dimensions := make(map[string][]any, len(plan.Dimensions))
	for _, name := range plan.DimensionOrder {
		values := plan.Dimensions[name]
		dimensions[name] = resolvedValuesAny(values)
	}
	identity := map[string]any{
		"asset_definition":          plan.Asset,
		"dimension_order":           append([]string{}, plan.DimensionOrder...),
		"dimensions":                dimensions,
		"fixed_parameters":          resolvedValueMapAny(plan.FixedParameters),
		"selection":                 append([]string{}, plan.Selection...),
		"path_template":             plan.PathTemplate,
		"materialization_domain_id": plan.MaterializationDomain,
	}
	_, hash, err := fp.CanonicalJSONSHA256(normalizedCanonicalValue(identity))
	if err != nil {
		return "", err
	}
	return "sha256:" + hash, nil
}

func resolveFixedAssetParameters(
	resolver variable.Resolver,
	context FanOutItemContext,
	definition model.DataInputDefinition,
	bindings map[string]variable.TypedExpression,
	dimensionSet map[string]struct{},
) (map[string]variable.ResolvedValue, error) {
	fixed := map[string]variable.ResolvedValue{}
	for name := range bindings {
		if _, ok := definition.Parameters[name]; !ok {
			return nil, fmt.Errorf("unknown asset parameter %q", name)
		}
		if _, isDimension := dimensionSet[name]; isDimension {
			return nil, fmt.Errorf("collection dimension parameter %q cannot be overridden in phase one", name)
		}
	}
	names := sortedParameterNames(definition.Parameters)
	for _, name := range names {
		if _, isDimension := dimensionSet[name]; isDimension {
			continue
		}
		expression, ok := bindings[name]
		if !ok {
			return nil, fmt.Errorf("missing asset parameter %q", name)
		}
		resolved, err := resolveAssetParameterValue(resolver, context, expression)
		if err != nil {
			return nil, fmt.Errorf("asset parameter %s: %w", name, err)
		}
		if err := validateAssetParameterType(name, definition.Parameters[name], resolved); err != nil {
			return nil, err
		}
		fixed[name] = resolved
	}
	return fixed, nil
}

func collectionDimensionValues(definition model.DataInputDefinition) (map[string][]variable.ResolvedValue, error) {
	dimensions := map[string][]variable.ResolvedValue{}
	if definition.Collection == nil {
		return dimensions, nil
	}
	for _, dimension := range definition.Collection.Dimensions {
		values, err := dimension.DomainValues(definition.Parameters[dimension.Parameter])
		if err != nil {
			return nil, fmt.Errorf("collection dimension %s: %w", dimension.Parameter, err)
		}
		for _, value := range values {
			resolved, err := resolvedValueFromCollectionValue(definition.Parameters[dimension.Parameter], value)
			if err != nil {
				return nil, fmt.Errorf("collection dimension %s: %w", dimension.Parameter, err)
			}
			dimensions[dimension.Parameter] = append(dimensions[dimension.Parameter], resolved)
		}
	}
	return dimensions, nil
}

func expandedMemberBindings(
	dimensionOrder []string,
	dimensions map[string][]variable.ResolvedValue,
	fixed map[string]variable.ResolvedValue,
) []map[string]variable.ResolvedValue {
	if len(dimensionOrder) == 0 {
		return []map[string]variable.ResolvedValue{copyResolvedMap(fixed)}
	}
	members := []map[string]variable.ResolvedValue{}
	var expand func(int, map[string]variable.ResolvedValue)
	expand = func(position int, current map[string]variable.ResolvedValue) {
		if position == len(dimensionOrder) {
			for name, value := range fixed {
				current[name] = value
			}
			members = append(members, copyResolvedMap(current))
			return
		}
		name := dimensionOrder[position]
		for _, value := range dimensions[name] {
			next := copyResolvedMap(current)
			next[name] = value
			expand(position+1, next)
		}
	}
	expand(0, map[string]variable.ResolvedValue{})
	return members
}

func concreteDestinationRelativePath(definition model.DataInputDefinition, bindings map[string]variable.ResolvedValue) (string, error) {
	return concreteDestinationRelativePathFromParameters(definition, resolvedValueMapAny(bindings))
}

func concreteDestinationRelativePathFromParameters(definition model.DataInputDefinition, parameters map[string]any) (string, error) {
	template := definition.Binding.Materialization.PathTemplate
	if template == "" {
		return "", nil
	}
	path, required, err := model.NormalizeMaterializationOutputPathTemplate(template, parameters)
	if err != nil {
		return "", err
	}
	if len(required) > 0 {
		return "", fmt.Errorf("destination path has unresolved placeholders %v", required)
	}
	if _, err := model.ValidateArtifactRelativePath(path); err != nil {
		return "", fmt.Errorf("destination path: %w", err)
	}
	return path, nil
}

func collectionPlanPathTemplate(definition model.DataInputDefinition, fixed map[string]variable.ResolvedValue) (string, []string, error) {
	if definition.Collection == nil {
		return definition.Binding.Materialization.PathTemplate, nil, nil
	}
	return definition.CollectionOutputPathTemplate(resolvedValueMapAny(fixed))
}

func resolvedValueFromCollectionValue(parameter model.DataParameterDefinition, value any) (variable.ResolvedValue, error) {
	switch parameter.Type {
	case "string":
		text, ok := value.(string)
		if !ok {
			return variable.ResolvedValue{}, fmt.Errorf("invalid string value")
		}
		return variable.ResolvedValue{Type: variable.TypeString, Value: text}, nil
	case "int":
		integer, ok := value.(int)
		if !ok {
			return variable.ResolvedValue{}, fmt.Errorf("invalid int value")
		}
		return variable.ResolvedValue{Type: variable.TypeInt, Value: integer}, nil
	case "bool":
		boolean, ok := value.(bool)
		if !ok {
			return variable.ResolvedValue{}, fmt.Errorf("invalid bool value")
		}
		return variable.ResolvedValue{Type: variable.TypeBool, Value: boolean}, nil
	default:
		return variable.ResolvedValue{}, fmt.Errorf("unsupported parameter type %q", parameter.Type)
	}
}

func typedExpressionsFromResolved(values map[string]variable.ResolvedValue) map[string]variable.TypedExpression {
	expressions := make(map[string]variable.TypedExpression, len(values))
	for name, value := range values {
		expressions[name] = variable.TypedExpression{Type: value.Type, Expression: value.Value}
	}
	return expressions
}

func collectionDimensionNames(collection *model.DataAssetCollectionDefinition) []string {
	if collection == nil {
		return nil
	}
	names := make([]string, 0, len(collection.Dimensions))
	for _, dimension := range collection.Dimensions {
		names = append(names, dimension.Parameter)
	}
	return names
}

func sortedParameterNames(parameters map[string]model.DataParameterDefinition) []string {
	names := make([]string, 0, len(parameters))
	for name := range parameters {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func copyResolvedMap(values map[string]variable.ResolvedValue) map[string]variable.ResolvedValue {
	copied := make(map[string]variable.ResolvedValue, len(values))
	for name, value := range values {
		copied[name] = value
	}
	return copied
}

func resolvedValueMapAny(values map[string]variable.ResolvedValue) map[string]any {
	converted := make(map[string]any, len(values))
	for name, value := range values {
		converted[name] = value.Value
	}
	return converted
}

func resolvedValuesAny(values []variable.ResolvedValue) []any {
	converted := make([]any, 0, len(values))
	for _, value := range values {
		converted = append(converted, value.Value)
	}
	return converted
}

func resolvedBindingFingerprint(bindings map[string]variable.ResolvedValue) (string, error) {
	_, hash, err := fp.CanonicalJSONSHA256(normalizedCanonicalValue(resolvedValueMapAny(bindings)))
	if err != nil {
		return "", err
	}
	return "sha256:" + hash, nil
}

package workflow

import (
	"fmt"
	"strings"

	"goetl/internal/model"
	"goetl/internal/variable"
)

type ExplicitAssetMaterializeTemplate struct {
	Definitions model.DataDefinitions
	Alias       string
	Asset       string
	Selection   []string
	With        map[string]variable.TypedExpression
}

func compileStandaloneExplicitAssetMaterializeWorkItems(
	resolver variable.Resolver,
	template FanOutWorkItemTemplate,
) ([]CompiledFanOutWorkItem, error) {
	if len(template.ParameterAccessors) > 0 {
		return nil, fmt.Errorf("parameter_accessors require fan_out")
	}
	item := model.WorkItem{
		ID:             template.IDPrefix,
		Type:           template.Type,
		OutputFilename: template.OutputPrefix + template.OutputExtension,
		Parameters:     copyParameters(template.Parameters),
	}
	if len(template.ParameterExpressions) > 0 && item.Parameters == nil {
		item.Parameters = model.Parameters{}
	}
	context := FanOutItemContext{}
	if err := bindParameterExpressions(resolver, context, item.Parameters, template.ParameterExpressions); err != nil {
		return nil, fmt.Errorf("parameters: %w", err)
	}
	items, err := compileExplicitAssetMaterializeMemberWorkItems(resolver, context, item, template.OutputExtension, template.ExplicitAssetMaterialize, template.ResourceConstraints)
	if err != nil {
		return nil, err
	}
	return items, nil
}

func rejectCollectionAssetMaterializeFanOut(template FanOutWorkItemTemplate) error {
	if template.Type != model.WorkItemTypeAssetMaterialize || template.ExplicitAssetMaterialize == nil || template.FanOutExpression == "" {
		return nil
	}
	definition, ok := template.ExplicitAssetMaterialize.Definitions.Inputs[template.ExplicitAssetMaterialize.Asset]
	if !ok {
		return nil
	}
	if definition.Collection != nil {
		return fmt.Errorf("asset.materialize collection step must not declare fan_out")
	}
	return nil
}

func compileExplicitAssetMaterializeWorkItem(
	resolver variable.Resolver,
	context FanOutItemContext,
	item *model.WorkItem,
	template *ExplicitAssetMaterializeTemplate,
) ([]model.WorkItemResourceConstraint, error) {
	if template == nil {
		return nil, nil
	}
	if item.Type != model.WorkItemTypeAssetMaterialize {
		return nil, fmt.Errorf("explicit asset materialize requires work item type %q", model.WorkItemTypeAssetMaterialize)
	}
	if err := rejectAssetMaterializeComputeParameters(item.Parameters); err != nil {
		return nil, err
	}

	definition, ok := template.Definitions.Inputs[template.Asset]
	if !ok {
		return nil, fmt.Errorf("data input %q is not defined", template.Asset)
	}
	instance, err := instantiateDataAssetWithContext(resolver, context, template.Asset, definition, template.Selection, template.With)
	if err != nil {
		return nil, err
	}
	instance.BoundAsset.BindingName = template.Alias

	targetEnvironmentID, err := targetEnvironmentIDFromParameters(item.Parameters)
	if err != nil {
		return nil, err
	}
	payload, constraints, err := AssetMaterializePayload(instance.BoundAsset, targetEnvironmentID, instance.AssetKey)
	if err != nil {
		return nil, err
	}
	for index := range constraints {
		constraints[index].WorkItemID = item.ID
		constraints[index].ConstraintIndex = index
	}

	if item.Parameters == nil {
		item.Parameters = model.Parameters{}
	}
	item.Parameters["asset_materialize"] = model.Parameter{Type: "asset_materialize", Value: payload}
	item.Parameters["data_assets"] = model.Parameter{Type: "data_assets", Value: []model.BoundDataAsset{instance.BoundAsset}}
	item.Parameters["target_environment_id"] = model.Parameter{Type: "string", Value: targetEnvironmentID}
	return constraints, nil
}

func compileExplicitAssetMaterializeMemberWorkItems(
	resolver variable.Resolver,
	context FanOutItemContext,
	baseItem model.WorkItem,
	outputExtension string,
	template *ExplicitAssetMaterializeTemplate,
	declaredConstraints []ResourceConstraintDeclaration,
) ([]CompiledFanOutWorkItem, error) {
	if template == nil {
		return []CompiledFanOutWorkItem{{WorkItem: baseItem}}, nil
	}
	if baseItem.Type != model.WorkItemTypeAssetMaterialize {
		return nil, fmt.Errorf("explicit asset materialize requires work item type %q", model.WorkItemTypeAssetMaterialize)
	}
	if err := rejectAssetMaterializeComputeParameters(baseItem.Parameters); err != nil {
		return nil, err
	}
	definition, ok := template.Definitions.Inputs[template.Asset]
	if !ok {
		return nil, fmt.Errorf("data input %q is not defined", template.Asset)
	}
	targetEnvironmentID, err := targetEnvironmentIDFromParameters(baseItem.Parameters)
	if err != nil {
		return nil, err
	}
	materialization := model.DataAssetMaterialization{
		Scope:        definition.Binding.Materialization.Scope,
		Strategy:     definition.Binding.Materialization.Strategy,
		PathTemplate: definition.Binding.Materialization.PathTemplate,
	}
	domain, err := model.ResolveMaterializationDomain(materialization, targetEnvironmentID)
	if err != nil {
		return nil, err
	}
	plan, err := PlanDataAssetCollection(resolver, context, template.Asset, definition, template.Selection, template.With, domain.ID)
	if err != nil {
		return nil, err
	}

	compiled := make([]CompiledFanOutWorkItem, 0, len(plan.Members))
	seenIDs := map[string]int{}
	seenOutputs := map[string]int{}
	for index, member := range plan.Members {
		item := baseItem
		item.Parameters = copyParameters(baseItem.Parameters)
		if definition.Collection != nil {
			token, err := collectionMemberIDToken(plan.DimensionOrder, member.Bindings)
			if err != nil {
				return nil, fmt.Errorf("collection member %d id token: %w", index, err)
			}
			item.ID = baseItem.ID + "--" + token
			item.OutputFilename = strings.TrimSuffix(baseItem.OutputFilename, outputExtension) + "--" + token + outputExtension
		}

		asset := member.Instance.BoundAsset
		asset.BindingName = template.Alias
		payload, constraints, err := AssetMaterializePayload(asset, targetEnvironmentID, member.Instance.AssetKey)
		if err != nil {
			return nil, fmt.Errorf("collection member %d: %w", index, err)
		}
		payload.MaterializationDomainID = domain.ID
		payload.DestinationRelativePath = member.DestinationRelativePath
		payload.MaterializationKey = member.MaterializationKey
		if member.MaterializationKey != "" {
			payload.DedupeKey = fmt.Sprintf("asset_materialize:%s:%s", domain.ID, member.MaterializationKey)
		}
		if definition.Collection != nil {
			payload.CollectionMember = &model.MaterializedDataAssetCollectionMember{
				CollectionFingerprint:   plan.CollectionFingerprint,
				MemberIndex:             member.Index,
				MemberCount:             len(plan.Members),
				DimensionOrder:          append([]string{}, plan.DimensionOrder...),
				MemberBindings:          resolvedValueMapAny(member.Bindings),
				DestinationRelativePath: member.DestinationRelativePath,
			}
		}
		if err := payload.Validate(); err != nil {
			return nil, fmt.Errorf("collection member %d: %w", index, err)
		}

		for constraintIndex := range constraints {
			constraints[constraintIndex].WorkItemID = item.ID
			constraints[constraintIndex].ConstraintIndex = constraintIndex
		}
		if item.Parameters == nil {
			item.Parameters = model.Parameters{}
		}
		item.Parameters["asset_materialize"] = model.Parameter{Type: "asset_materialize", Value: payload}
		item.Parameters["data_assets"] = model.Parameter{Type: "data_assets", Value: []model.BoundDataAsset{asset}}
		item.Parameters["target_environment_id"] = model.Parameter{Type: "string", Value: targetEnvironmentID}

		if err := item.ValidateForWorkflowCompile(); err != nil {
			return nil, fmt.Errorf("collection member %d: %w", index, err)
		}
		if previous, ok := seenIDs[item.ID]; ok {
			return nil, fmt.Errorf("collection member %d: duplicate generated work-item id %q also produced by member %d", index, item.ID, previous)
		}
		seenIDs[item.ID] = index
		if previous, ok := seenOutputs[item.OutputFilename]; ok {
			return nil, fmt.Errorf("collection member %d: duplicate output filename %q also produced by member %d", index, item.OutputFilename, previous)
		}
		seenOutputs[item.OutputFilename] = index

		resolvedDeclared, err := resolveResourceConstraintDeclarations(resolver, context, item.ID, declaredConstraints)
		if err != nil {
			return nil, fmt.Errorf("collection member %d resource constraints: %w", index, err)
		}
		if len(resolvedDeclared) > 0 {
			constraints = appendExplicitResourceConstraints(constraints, resolvedDeclared)
		}
		compiled = append(compiled, CompiledFanOutWorkItem{WorkItem: item, ResourceConstraints: constraints})
	}
	return compiled, nil
}

func collectionMemberIDToken(order []string, bindings map[string]variable.ResolvedValue) (string, error) {
	parts := make([]string, 0, len(order))
	for _, name := range order {
		value, ok := bindings[name]
		if !ok {
			return "", fmt.Errorf("missing binding %q", name)
		}
		token, err := fanOutToken(value)
		if err != nil {
			return "", fmt.Errorf("%s: %w", name, err)
		}
		part := name + "-" + token
		if err := validateFanOutRenderedToken(part, "collection member id"); err != nil {
			return "", err
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return "scalar", nil
	}
	token := strings.Join(parts, "--")
	if err := validateFanOutRenderedToken(token, "collection member id"); err != nil {
		return "", err
	}
	return token, nil
}

func rejectAssetMaterializeComputeParameters(parameters model.Parameters) error {
	for name := range parameters {
		switch name {
		case "target_environment_id":
			continue
		default:
			return fmt.Errorf("%s step does not accept work parameter %q", model.WorkItemTypeAssetMaterialize, name)
		}
	}
	return nil
}

func appendExplicitResourceConstraints(explicit []model.WorkItemResourceConstraint, declared []model.WorkItemResourceConstraint) []model.WorkItemResourceConstraint {
	if len(declared) == 0 {
		return explicit
	}
	combined := make([]model.WorkItemResourceConstraint, 0, len(explicit)+len(declared))
	combined = append(combined, explicit...)
	for _, constraint := range declared {
		constraint.ConstraintIndex += len(explicit)
		combined = append(combined, constraint)
	}
	return combined
}

func PlanStageAssetMaterializeWorkItems(result CompileStageResult) (CompileStageResult, error) {
	if err := ValidateExplicitAssetMaterializeWorkItems(result); err != nil {
		return CompileStageResult{}, err
	}
	return result, nil
}

func ValidateExplicitAssetMaterializeWorkItems(result CompileStageResult) error {
	seenExplicit := map[string]string{}
	for _, item := range result.WorkItems {
		if item.WorkItem.Type != model.WorkItemTypeAssetMaterialize {
			continue
		}
		dedupeKey, err := explicitAssetMaterializeDedupeKey(item)
		if err != nil {
			return err
		}
		if previous, ok := seenExplicit[dedupeKey]; ok {
			return fmt.Errorf("duplicate explicit asset_materialize materializer for %s: %s and %s", dedupeKey, previous, item.WorkItem.ID)
		}
		seenExplicit[dedupeKey] = item.WorkItem.ID
	}
	return nil
}

func explicitAssetMaterializeDedupeKey(item CompileStageWorkItem) (string, error) {
	parameter, ok := item.WorkItem.Parameters["asset_materialize"]
	if !ok {
		return "", fmt.Errorf("explicit asset_materialize work item %s missing asset_materialize parameter", item.WorkItem.ID)
	}
	payload, ok := parameter.Value.(model.AssetMaterializeWorkItemPayload)
	if ok {
		if payload.DedupeKey == "" {
			return "", fmt.Errorf("explicit asset_materialize work item %s missing dedupe_key", item.WorkItem.ID)
		}
		return payload.DedupeKey, nil
	}
	return "", fmt.Errorf("explicit asset_materialize work item %s has unsupported asset_materialize payload %T", item.WorkItem.ID, parameter.Value)
}

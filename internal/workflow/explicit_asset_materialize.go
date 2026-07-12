package workflow

import (
	"fmt"

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
	if len(result.WorkItems) == 0 {
		return result, nil
	}

	legacyInput := CompileStageResult{
		WorkflowID: result.WorkflowID,
		StageIndex: result.StageIndex,
		Steps:      result.Steps,
		WorkItems:  make([]CompileStageWorkItem, 0, len(result.WorkItems)),
	}
	explicit := make([]CompileStageWorkItem, 0)
	seenExplicit := map[string]string{}
	for _, item := range result.WorkItems {
		if item.WorkItem.Type == model.WorkItemTypeAssetMaterialize {
			dedupeKey, err := explicitAssetMaterializeDedupeKey(item)
			if err != nil {
				return CompileStageResult{}, err
			}
			if previous, ok := seenExplicit[dedupeKey]; ok {
				return CompileStageResult{}, fmt.Errorf("duplicate explicit asset_materialize materializer for %s: %s and %s", dedupeKey, previous, item.WorkItem.ID)
			}
			seenExplicit[dedupeKey] = item.WorkItem.ID
			explicit = append(explicit, item)
			continue
		}
		legacyInput.WorkItems = append(legacyInput.WorkItems, item)
	}

	planned, err := PlanAssetMaterializeWorkItems(legacyInput)
	if err != nil {
		return CompileStageResult{}, err
	}
	planned.WorkItems = append(explicit, planned.WorkItems...)
	return planned, nil
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

package workflow

import (
	"fmt"

	"goetl/internal/model"
	"goetl/internal/variable"
)

type ExplicitCacheDataTemplate struct {
	Definitions model.DataDefinitions
	Alias       string
	Asset       string
	Selection   []string
	With        map[string]variable.TypedExpression
}

func compileExplicitCacheDataWorkItem(
	resolver variable.Resolver,
	fanOutValue variable.ResolvedValue,
	item *model.WorkItem,
	template *ExplicitCacheDataTemplate,
) ([]model.WorkItemResourceConstraint, error) {
	if template == nil {
		return nil, nil
	}
	if item.Type != model.WorkItemTypeCacheData {
		return nil, fmt.Errorf("explicit cache data requires work item type %q", model.WorkItemTypeCacheData)
	}
	if err := rejectCacheDataComputeParameters(item.Parameters); err != nil {
		return nil, err
	}

	definition, ok := template.Definitions.Inputs[template.Asset]
	if !ok {
		return nil, fmt.Errorf("data input %q is not defined", template.Asset)
	}
	instance, err := InstantiateDataAsset(resolver, fanOutValue, template.Asset, definition, template.Selection, template.With)
	if err != nil {
		return nil, err
	}
	instance.BoundAsset.BindingName = template.Alias

	targetEnvironmentID, err := targetEnvironmentIDFromParameters(item.Parameters)
	if err != nil {
		return nil, err
	}
	payload, constraints, err := CacheDataPayload(instance.BoundAsset, targetEnvironmentID, instance.AssetKey)
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
	item.Parameters["cache_data"] = model.Parameter{Type: "cache_data", Value: payload}
	item.Parameters["data_assets"] = model.Parameter{Type: "data_assets", Value: []model.BoundDataAsset{instance.BoundAsset}}
	item.Parameters["target_environment_id"] = model.Parameter{Type: "string", Value: targetEnvironmentID}
	return constraints, nil
}

func rejectCacheDataComputeParameters(parameters model.Parameters) error {
	for name := range parameters {
		switch name {
		case "target_environment_id":
			continue
		default:
			return fmt.Errorf("cache_data step does not accept work parameter %q", name)
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

func PlanStageCacheDataWorkItems(result CompileStageResult) (CompileStageResult, error) {
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
		if item.WorkItem.Type == model.WorkItemTypeCacheData {
			dedupeKey, err := explicitCacheDataDedupeKey(item)
			if err != nil {
				return CompileStageResult{}, err
			}
			if previous, ok := seenExplicit[dedupeKey]; ok {
				return CompileStageResult{}, fmt.Errorf("duplicate explicit cache_data materializer for %s: %s and %s", dedupeKey, previous, item.WorkItem.ID)
			}
			seenExplicit[dedupeKey] = item.WorkItem.ID
			explicit = append(explicit, item)
			continue
		}
		legacyInput.WorkItems = append(legacyInput.WorkItems, item)
	}

	planned, err := PlanCacheDataWorkItems(legacyInput)
	if err != nil {
		return CompileStageResult{}, err
	}
	planned.WorkItems = append(explicit, planned.WorkItems...)
	return planned, nil
}

func explicitCacheDataDedupeKey(item CompileStageWorkItem) (string, error) {
	parameter, ok := item.WorkItem.Parameters["cache_data"]
	if !ok {
		return "", fmt.Errorf("explicit cache_data work item %s missing cache_data parameter", item.WorkItem.ID)
	}
	payload, ok := parameter.Value.(model.CacheDataWorkItemPayload)
	if ok {
		if payload.DedupeKey == "" {
			return "", fmt.Errorf("explicit cache_data work item %s missing dedupe_key", item.WorkItem.ID)
		}
		return payload.DedupeKey, nil
	}
	return "", fmt.Errorf("explicit cache_data work item %s has unsupported cache_data payload %T", item.WorkItem.ID, parameter.Value)
}

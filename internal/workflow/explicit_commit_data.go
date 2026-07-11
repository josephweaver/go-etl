package workflow

import (
	"fmt"

	"goetl/internal/model"
	"goetl/internal/variable"
)

type ExplicitCommitDataTemplate struct {
	Definitions  model.DataDefinitions
	Alias        string
	Target       string
	FromStep     string
	FromArtifact string
	With         map[string]variable.TypedExpression
}

func compileExplicitCommitDataWorkItem(
	resolver variable.Resolver,
	context FanOutItemContext,
	idToken string,
	item *model.WorkItem,
	template *ExplicitCommitDataTemplate,
) ([]model.WorkItemResourceConstraint, error) {
	if template == nil {
		return nil, nil
	}
	if item.Type != model.WorkItemTypeCommitData {
		return nil, fmt.Errorf("explicit commit data requires work item type %q", model.WorkItemTypeCommitData)
	}
	if err := rejectCommitDataComputeParameters(item.Parameters); err != nil {
		return nil, err
	}

	targetName := template.Target
	definition, ok := template.Definitions.Outputs[targetName]
	if !ok {
		return nil, fmt.Errorf("data output target %q is not defined", targetName)
	}
	parameters, err := resolveOutputParameters(resolver, context, definition, template.With)
	if err != nil {
		return nil, err
	}
	target, err := definition.BoundOutputTarget(targetName, template.FromArtifact, parameters)
	if err != nil {
		return nil, err
	}
	targetEnvironmentID, err := targetEnvironmentIDFromParameters(item.Parameters)
	if err != nil {
		return nil, err
	}
	constraints, err := CommitDataResourceConstraints(target, targetEnvironmentID)
	if err != nil {
		return nil, err
	}
	for index := range constraints {
		constraints[index].WorkItemID = item.ID
		constraints[index].ConstraintIndex = index
	}

	sourceWorkItemID := template.FromStep + "-" + idToken
	payload := model.CommitDataWorkItemPayload{
		Operator:            string(model.WorkItemTypeCommitData),
		TargetEnvironmentID: targetEnvironmentID,
		Source: model.CommitDataSource{
			FromWorkItemID: sourceWorkItemID,
			FromArtifact:   template.FromArtifact,
		},
		PublishTarget:       target,
		ResourceConstraints: constraints,
	}
	if err := payload.Validate(); err != nil {
		return nil, err
	}

	if item.Parameters == nil {
		item.Parameters = model.Parameters{}
	}
	item.DependsOn = appendUniqueString(item.DependsOn, sourceWorkItemID)
	item.Parameters["commit_data"] = model.Parameter{Type: "commit_data", Value: payload}
	item.Parameters["target_environment_id"] = model.Parameter{Type: "string", Value: targetEnvironmentID}
	return constraints, nil
}

func rejectCommitDataComputeParameters(parameters model.Parameters) error {
	for name := range parameters {
		switch name {
		case "target_environment_id":
			continue
		default:
			return fmt.Errorf("commit_data step does not accept work parameter %q", name)
		}
	}
	return nil
}

func resolveOutputParameters(
	resolver variable.Resolver,
	context FanOutItemContext,
	definition model.DataOutputDefinition,
	bindings map[string]variable.TypedExpression,
) (map[string]any, error) {
	for name := range definition.Parameters {
		if _, ok := bindings[name]; !ok {
			return nil, fmt.Errorf("missing output parameter %q", name)
		}
	}
	for name := range bindings {
		if _, ok := definition.Parameters[name]; !ok {
			return nil, fmt.Errorf("unknown output parameter %q", name)
		}
	}
	parameters := make(map[string]any, len(bindings))
	for name, binding := range bindings {
		resolved, err := resolveAssetParameterValue(resolver, context, binding)
		if err != nil {
			return nil, fmt.Errorf("output parameter %s: %w", name, err)
		}
		value, err := assetParameterAny(resolved)
		if err != nil {
			return nil, fmt.Errorf("output parameter %s: %w", name, err)
		}
		if err := validateAssetParameterType(name, definition.Parameters[name], resolved); err != nil {
			return nil, err
		}
		parameters[name] = value
	}
	return parameters, nil
}

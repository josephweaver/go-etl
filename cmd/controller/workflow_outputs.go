package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	fp "goetl/internal/fingerprint"
	"goetl/internal/model"
	"goetl/internal/variable"
)

const (
	maxCompletedWorkOutputJSONBytes = 16 * 1024
	maxLogicalStepOutputJSONBytes   = 256 * 1024
)

func validateCompletedWorkOutputJSONSize(outputJSON string) error {
	return validateOutputJSONSize("output_json", outputJSON, maxCompletedWorkOutputJSONBytes)
}

func validateLogicalStepOutputJSONSize(outputJSON string) error {
	return validateOutputJSONSize("logical step output_json", outputJSON, maxLogicalStepOutputJSONBytes)
}

func validateOutputJSONSize(name string, outputJSON string, limit int) error {
	size := len([]byte(outputJSON))
	if size == 0 {
		return fmt.Errorf("%s is required", name)
	}
	if size > limit {
		return fmt.Errorf("%s is %d bytes, limit is %d bytes; store bulk data externally and return a small artifact reference", name, size, limit)
	}
	return nil
}

func validateArtifactManifestOutputJSON(outputJSON string) error {
	manifest, found, err := artifactManifestFromOutputJSON(outputJSON)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if err := manifest.Validate(); err != nil {
		return fmt.Errorf("artifact manifest: %w", err)
	}
	return nil
}

func artifactManifestFromOutputJSON(outputJSON string) (model.ArtifactManifest, bool, error) {
	decoder := json.NewDecoder(strings.NewReader(outputJSON))
	decoder.UseNumber()

	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return model.ArtifactManifest{}, false, fmt.Errorf("decode output JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return model.ArtifactManifest{}, false, fmt.Errorf("output JSON must contain one JSON document")
	}

	object, ok := decoded.(map[string]any)
	if !ok {
		return model.ArtifactManifest{}, false, nil
	}
	schema, ok := object["schema"].(string)
	if !ok || schema != model.ArtifactManifestSchemaV1 {
		return model.ArtifactManifest{}, false, nil
	}

	data, err := json.Marshal(object)
	if err != nil {
		return model.ArtifactManifest{}, false, fmt.Errorf("encode artifact manifest candidate: %w", err)
	}
	var manifest model.ArtifactManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return model.ArtifactManifest{}, false, fmt.Errorf("decode artifact manifest: %w", err)
	}
	return manifest, true, nil
}

func resolvedOutputFromJSON(raw string) (variable.ResolvedValue, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()

	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return variable.ResolvedValue{}, fmt.Errorf("decode output JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return variable.ResolvedValue{}, fmt.Errorf("output JSON must contain one JSON document")
	}

	value, err := resolvedOutputValue(decoded, "/")
	if err != nil {
		return variable.ResolvedValue{}, err
	}
	return value, nil
}

func resolvedOutputValue(value any, path string) (variable.ResolvedValue, error) {
	switch typed := value.(type) {
	case nil:
		return variable.ResolvedValue{}, fmt.Errorf("output %s is null, null outputs are not supported", path)
	case map[string]any:
		fields := make(map[string]variable.ResolvedValue, len(typed))
		for name, field := range typed {
			resolved, err := resolvedOutputValue(field, appendOutputJSONPointer(path, name))
			if err != nil {
				return variable.ResolvedValue{}, err
			}
			fields[name] = resolved
		}
		return variable.ResolvedObject(fields), nil
	case []any:
		items := make([]variable.ResolvedValue, 0, len(typed))
		for index, item := range typed {
			resolved, err := resolvedOutputValue(item, appendOutputJSONPointer(path, strconv.Itoa(index)))
			if err != nil {
				return variable.ResolvedValue{}, err
			}
			items = append(items, resolved)
		}
		return variable.ResolvedList(items), nil
	case string:
		return variable.ResolvedValue{Type: variable.TypeString, Value: typed}, nil
	case bool:
		return variable.ResolvedValue{Type: variable.TypeBool, Value: typed}, nil
	case json.Number:
		if strings.ContainsAny(typed.String(), ".eE") {
			return variable.ResolvedValue{}, fmt.Errorf("output %s has non-integer number %s", path, typed.String())
		}
		integer64, err := typed.Int64()
		if err != nil {
			return variable.ResolvedValue{}, fmt.Errorf("output %s has integer number outside supported int range %s", path, typed.String())
		}
		integer := int(integer64)
		if int64(integer) != integer64 {
			return variable.ResolvedValue{}, fmt.Errorf("output %s has integer number outside supported int range %s", path, typed.String())
		}
		return variable.ResolvedValue{Type: variable.TypeInt, Value: integer}, nil
	default:
		return variable.ResolvedValue{}, fmt.Errorf("output %s has unsupported JSON value %T", path, value)
	}
}

func canonicalOutputJSONFromResolved(value variable.ResolvedValue) (string, string, error) {
	canonicalValue, err := outputCanonicalValue(value)
	if err != nil {
		return "", "", err
	}
	canonical, hash, err := fp.CanonicalJSONSHA256(canonicalValue)
	if err != nil {
		return "", "", err
	}
	return string(canonical), hash, nil
}

func outputCanonicalValue(value variable.ResolvedValue) (any, error) {
	switch value.Type {
	case variable.TypeString, variable.TypePath, variable.TypeDatetime:
		text, ok := value.Value.(string)
		if !ok {
			return nil, fmt.Errorf("invalid %s value", value.Type)
		}
		return text, nil
	case variable.TypeBool:
		boolean, ok := value.Value.(bool)
		if !ok {
			return nil, fmt.Errorf("invalid bool value")
		}
		return boolean, nil
	case variable.TypeInt:
		integer, ok := value.Value.(int)
		if !ok {
			return nil, fmt.Errorf("invalid int value")
		}
		return integer, nil
	case variable.TypeObject:
		fields := make(map[string]any, len(value.Object))
		for name, field := range value.Object {
			canonicalField, err := outputCanonicalValue(field)
			if err != nil {
				return nil, fmt.Errorf("convert object field %s: %w", name, err)
			}
			fields[name] = canonicalField
		}
		return fields, nil
	case variable.TypeList:
		items := make([]any, 0, len(value.List))
		for index, item := range value.List {
			canonicalItem, err := outputCanonicalValue(item)
			if err != nil {
				return nil, fmt.Errorf("convert list item %d: %w", index, err)
			}
			items = append(items, canonicalItem)
		}
		return items, nil
	default:
		return nil, fmt.Errorf("unsupported resolved value type: %s", value.Type)
	}
}

func aggregateStepOutputJSON(step model.WorkflowDependencyStep) (string, string, error) {
	if len(step.WorkItems) == 0 {
		outputJSON, outputJSONSHA256, err := canonicalOutputJSONFromResolved(variable.ResolvedValue{
			Type: variable.TypeList,
			List: []variable.ResolvedValue{},
		})
		if err != nil {
			return "", "", err
		}
		if err := validateLogicalStepOutputJSONSize(outputJSON); err != nil {
			return "", "", err
		}
		return outputJSON, outputJSONSHA256, nil
	}

	ordered := append([]model.WorkflowDependencyWorkItemMembership(nil), step.WorkItems...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].WorkItemIndex == ordered[j].WorkItemIndex {
			return ordered[i].WorkItemID < ordered[j].WorkItemID
		}
		return ordered[i].WorkItemIndex < ordered[j].WorkItemIndex
	})

	seenIndexes := make(map[int]bool, len(ordered))
	outputs := make([]variable.ResolvedValue, 0, len(ordered))
	for _, item := range ordered {
		if seenIndexes[item.WorkItemIndex] {
			return "", "", fmt.Errorf("step %d has duplicate work item index %d", step.StepIndex, item.WorkItemIndex)
		}
		seenIndexes[item.WorkItemIndex] = true

		switch item.State {
		case model.WorkItemMembershipStateCompleted, model.WorkItemMembershipStateSkipped:
		default:
			return "", "", fmt.Errorf("step %d work item %s is %s, output aggregation is not ready", step.StepIndex, item.WorkItemID, item.State)
		}
		if item.OutputJSON == "" {
			return "", "", fmt.Errorf("step %d work item %s is missing output JSON", step.StepIndex, item.WorkItemID)
		}
		if err := validateCompletedWorkOutputJSONSize(item.OutputJSON); err != nil {
			return "", "", fmt.Errorf("step %d work item %s: %w", step.StepIndex, item.WorkItemID, err)
		}

		output, err := resolvedOutputFromJSON(item.OutputJSON)
		if err != nil {
			return "", "", fmt.Errorf("step %d work item %s: %w", step.StepIndex, item.WorkItemID, err)
		}
		if output.Type != variable.TypeObject {
			return "", "", fmt.Errorf("step %d work item %s output has type %s, want object", step.StepIndex, item.WorkItemID, output.Type)
		}
		outputs = append(outputs, output)
	}

	if len(outputs) == 1 {
		outputJSON, outputJSONSHA256, err := canonicalOutputJSONFromResolved(outputs[0])
		if err != nil {
			return "", "", err
		}
		if err := validateLogicalStepOutputJSONSize(outputJSON); err != nil {
			return "", "", err
		}
		return outputJSON, outputJSONSHA256, nil
	}
	outputJSON, outputJSONSHA256, err := canonicalOutputJSONFromResolved(variable.ResolvedList(outputs))
	if err != nil {
		return "", "", err
	}
	if err := validateLogicalStepOutputJSONSize(outputJSON); err != nil {
		return "", "", err
	}
	return outputJSON, outputJSONSHA256, nil
}

func workflowStepScope(plan model.WorkflowDependencyPlan, beforeStepIndex int) (variable.Scope, error) {
	if beforeStepIndex < 0 {
		return nil, fmt.Errorf("before step index must be non-negative")
	}

	steps := flattenDependencySteps(plan)
	sortStepsByIndex(steps)

	stepOutputs := make([]variable.TypedExpression, 0)
	for _, step := range steps {
		if step.StepIndex >= beforeStepIndex {
			continue
		}
		if step.State != model.WorkflowStepStateCompleted {
			return nil, fmt.Errorf("workflow step %d is %s, want completed", step.StepIndex, step.State)
		}
		if step.OutputJSON == "" {
			if step.OutputJSONPruned {
				return nil, fmt.Errorf("workflow.step[%d] output was pruned before downstream work was materialized", step.StepIndex)
			}
			return nil, fmt.Errorf("workflow step %d is missing output JSON", step.StepIndex)
		}
		if err := validateLogicalStepOutputJSONSize(step.OutputJSON); err != nil {
			return nil, fmt.Errorf("workflow step %d: %w", step.StepIndex, err)
		}
		output, err := resolvedOutputFromJSON(step.OutputJSON)
		if err != nil {
			return nil, fmt.Errorf("workflow step %d: %w", step.StepIndex, err)
		}
		expression, err := variable.TypedExpressionFromResolved(output)
		if err != nil {
			return nil, fmt.Errorf("workflow step %d: %w", step.StepIndex, err)
		}
		stepOutputs = append(stepOutputs, expression)
	}

	return variable.NewScope(variable.Variable{
		Name: variable.Name{
			Namespace: variable.NamespaceWorkflow,
			Key:       "step",
		},
		TypedExpression: variable.TypedExpression{
			Type:       variable.TypeList,
			Expression: stepOutputs,
		},
	})
}

func flattenDependencySteps(plan model.WorkflowDependencyPlan) []model.WorkflowDependencyStep {
	steps := make([]model.WorkflowDependencyStep, 0)
	for _, stage := range plan.Stages {
		for _, step := range stage.Steps {
			steps = append(steps, cloneDependencyStep(step))
		}
	}
	return steps
}

func appendOutputJSONPointer(path, segment string) string {
	segment = strings.ReplaceAll(segment, "~", "~0")
	segment = strings.ReplaceAll(segment, "/", "~1")
	if path == "/" {
		return "/" + segment
	}
	return path + "/" + segment
}

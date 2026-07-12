package main

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
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

	return artifactManifestFromDecodedOutput(decoded)
}

func artifactManifestFromDecodedOutput(decoded any) (model.ArtifactManifest, bool, error) {
	object, ok := decoded.(map[string]any)
	if !ok {
		return model.ArtifactManifest{}, false, nil
	}
	schema, ok := object["schema"].(string)
	if ok && schema == model.ArtifactManifestSchemaV1 {
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

	if logicalOutput, ok := object["logical_output"]; ok {
		return artifactManifestFromDecodedOutput(logicalOutput)
	}

	return model.ArtifactManifest{}, false, nil
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
	collectionOutputs := make([]materializedCollectionMemberOutput, 0, len(ordered))
	collectionCandidateCount := 0
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
		if collectionOutput, found, err := materializedCollectionMemberOutputFromJSON(item); err != nil {
			return "", "", fmt.Errorf("step %d work item %s: %w", step.StepIndex, item.WorkItemID, err)
		} else if found {
			collectionCandidateCount++
			if collectionOutput.asset.CollectionMember != nil {
				collectionOutputs = append(collectionOutputs, collectionOutput)
			}
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

	if len(collectionOutputs) > 0 {
		if len(collectionOutputs) != len(ordered) || collectionCandidateCount != len(ordered) {
			return "", "", fmt.Errorf("step %d mixes collection materialization outputs with ordinary outputs", step.StepIndex)
		}
		manifest, err := aggregateMaterializedCollectionOutput(collectionOutputs)
		if err != nil {
			return "", "", fmt.Errorf("step %d: %w", step.StepIndex, err)
		}
		outputJSON, outputJSONSHA256, err := canonicalOutputJSONFromAny(manifest)
		if err != nil {
			return "", "", err
		}
		if err := validateLogicalStepOutputJSONSize(outputJSON); err != nil {
			return "", "", err
		}
		return outputJSON, outputJSONSHA256, nil
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

type materializedCollectionMemberOutput struct {
	workItemID    string
	workItemIndex int
	manifest      model.MaterializedDataAssetManifest
	asset         model.MaterializedDataAsset
}

func materializedCollectionMemberOutputFromJSON(item model.WorkflowDependencyWorkItemMembership) (materializedCollectionMemberOutput, bool, error) {
	decoder := json.NewDecoder(strings.NewReader(item.OutputJSON))
	decoder.UseNumber()
	var decoded map[string]any
	if err := decoder.Decode(&decoded); err != nil {
		return materializedCollectionMemberOutput{}, false, fmt.Errorf("decode materialized asset output candidate: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return materializedCollectionMemberOutput{}, false, fmt.Errorf("output JSON must contain one JSON document")
	}
	if decoded["schema"] != model.MaterializedDataAssetManifestSchemaV1 {
		return materializedCollectionMemberOutput{}, false, nil
	}
	data, err := json.Marshal(decoded)
	if err != nil {
		return materializedCollectionMemberOutput{}, false, fmt.Errorf("encode materialized asset output candidate: %w", err)
	}
	var manifest model.MaterializedDataAssetManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return materializedCollectionMemberOutput{}, false, fmt.Errorf("decode materialized data asset manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return materializedCollectionMemberOutput{}, false, fmt.Errorf("materialized data asset manifest: %w", err)
	}
	if len(manifest.Assets) != 1 {
		return materializedCollectionMemberOutput{}, false, fmt.Errorf("collection materialization output must contain exactly one asset, got %d", len(manifest.Assets))
	}
	return materializedCollectionMemberOutput{
		workItemID:    item.WorkItemID,
		workItemIndex: item.WorkItemIndex,
		manifest:      manifest,
		asset:         manifest.Assets[0],
	}, true, nil
}

func aggregateMaterializedCollectionOutput(outputs []materializedCollectionMemberOutput) (model.MaterializedAssetCollectionManifest, error) {
	if len(outputs) == 0 {
		return model.MaterializedAssetCollectionManifest{}, fmt.Errorf("collection outputs are required")
	}
	sort.Slice(outputs, func(i, j int) bool {
		left := outputs[i].asset.CollectionMember.MemberIndex
		right := outputs[j].asset.CollectionMember.MemberIndex
		if left == right {
			return outputs[i].workItemID < outputs[j].workItemID
		}
		return left < right
	})

	firstAsset := outputs[0].asset
	firstMember := firstAsset.CollectionMember
	if firstMember.MemberCount <= 0 {
		return model.MaterializedAssetCollectionManifest{}, fmt.Errorf("collection member_count must be positive")
	}
	if len(outputs) != firstMember.MemberCount {
		return model.MaterializedAssetCollectionManifest{}, fmt.Errorf("collection member count = %d, want %d", len(outputs), firstMember.MemberCount)
	}

	root, relativeTemplate, err := collectionPathTemplate(firstAsset)
	if err != nil {
		return model.MaterializedAssetCollectionManifest{}, err
	}
	dimensions := map[string]model.MaterializedAssetCollectionDimension{}
	dimensionValueKeys := map[string]map[string]struct{}{}
	memberEvidence := make([]any, 0, len(outputs))
	seenIndexes := map[int]struct{}{}
	seenMaterializationKeys := map[string]string{}
	seenDestinations := map[string]string{}
	var pathTemplateIdentity string

	for _, output := range outputs {
		asset := output.asset
		member := asset.CollectionMember
		if member == nil {
			return model.MaterializedAssetCollectionManifest{}, fmt.Errorf("work item %s is missing collection member metadata", output.workItemID)
		}
		if _, exists := seenIndexes[member.MemberIndex]; exists {
			return model.MaterializedAssetCollectionManifest{}, fmt.Errorf("duplicate collection member index %d", member.MemberIndex)
		}
		seenIndexes[member.MemberIndex] = struct{}{}
		if member.MemberIndex < 0 || member.MemberIndex >= firstMember.MemberCount {
			return model.MaterializedAssetCollectionManifest{}, fmt.Errorf("collection member index %d out of range", member.MemberIndex)
		}
		if asset.BindingName != firstAsset.BindingName {
			return model.MaterializedAssetCollectionManifest{}, fmt.Errorf("collection asset mismatch: %s != %s", asset.BindingName, firstAsset.BindingName)
		}
		if asset.MaterializationDomainID != firstAsset.MaterializationDomainID {
			return model.MaterializedAssetCollectionManifest{}, fmt.Errorf("collection materialization domain mismatch: %s != %s", asset.MaterializationDomainID, firstAsset.MaterializationDomainID)
		}
		if member.CollectionFingerprint != firstMember.CollectionFingerprint {
			return model.MaterializedAssetCollectionManifest{}, fmt.Errorf("collection fingerprint mismatch: %s != %s", member.CollectionFingerprint, firstMember.CollectionFingerprint)
		}
		if !sameStrings(member.DimensionOrder, firstMember.DimensionOrder) {
			return model.MaterializedAssetCollectionManifest{}, fmt.Errorf("collection dimension order mismatch: %v != %v", member.DimensionOrder, firstMember.DimensionOrder)
		}
		if member.MemberCount != firstMember.MemberCount {
			return model.MaterializedAssetCollectionManifest{}, fmt.Errorf("collection member_count mismatch: %d != %d", member.MemberCount, firstMember.MemberCount)
		}
		if asset.DestinationRelativePath == "" || asset.DestinationRelativePath != member.DestinationRelativePath {
			return model.MaterializedAssetCollectionManifest{}, fmt.Errorf("collection destination mismatch for member %d", member.MemberIndex)
		}
		if asset.MaterializationKey == "" {
			return model.MaterializedAssetCollectionManifest{}, fmt.Errorf("collection member %d missing materialization key", member.MemberIndex)
		}
		if previous, exists := seenMaterializationKeys[asset.MaterializationKey]; exists {
			return model.MaterializedAssetCollectionManifest{}, fmt.Errorf("duplicate collection materialization key %s for %s and %s", asset.MaterializationKey, previous, output.workItemID)
		}
		seenMaterializationKeys[asset.MaterializationKey] = output.workItemID
		if previous, exists := seenDestinations[asset.DestinationRelativePath]; exists {
			return model.MaterializedAssetCollectionManifest{}, fmt.Errorf("duplicate collection destination %s for %s and %s", asset.DestinationRelativePath, previous, output.workItemID)
		}
		seenDestinations[asset.DestinationRelativePath] = output.workItemID
		if pathTemplateIdentity == "" {
			pathTemplateIdentity = member.PathTemplateIdentity
		} else if member.PathTemplateIdentity != "" && member.PathTemplateIdentity != pathTemplateIdentity {
			return model.MaterializedAssetCollectionManifest{}, fmt.Errorf("collection path template identity mismatch")
		}

		memberRoot, memberTemplate, err := collectionPathTemplate(asset)
		if err != nil {
			return model.MaterializedAssetCollectionManifest{}, fmt.Errorf("member %d: %w", member.MemberIndex, err)
		}
		if memberRoot != root {
			return model.MaterializedAssetCollectionManifest{}, fmt.Errorf("collection materialization root mismatch: %s != %s", memberRoot, root)
		}
		if memberTemplate != relativeTemplate {
			return model.MaterializedAssetCollectionManifest{}, fmt.Errorf("collection destination template mismatch: %s != %s", memberTemplate, relativeTemplate)
		}
		rendered, err := renderCollectionRelativeTemplate(relativeTemplate, member.DimensionOrder, member.MemberBindings)
		if err != nil {
			return model.MaterializedAssetCollectionManifest{}, fmt.Errorf("member %d: %w", member.MemberIndex, err)
		}
		if rendered != asset.DestinationRelativePath {
			return model.MaterializedAssetCollectionManifest{}, fmt.Errorf("member %d destination %s does not conform to template %s", member.MemberIndex, asset.DestinationRelativePath, relativeTemplate)
		}

		for _, name := range member.DimensionOrder {
			value := member.MemberBindings[name]
			valueType, valueKey, err := collectionDimensionValue(value)
			if err != nil {
				return model.MaterializedAssetCollectionManifest{}, fmt.Errorf("member %d binding %s: %w", member.MemberIndex, name, err)
			}
			dimension := dimensions[name]
			if dimension.Type == "" {
				dimension.Type = valueType
				dimensionValueKeys[name] = map[string]struct{}{}
			} else if dimension.Type != valueType {
				return model.MaterializedAssetCollectionManifest{}, fmt.Errorf("dimension %s type mismatch: %s != %s", name, valueType, dimension.Type)
			}
			if _, exists := dimensionValueKeys[name][valueKey]; !exists {
				dimension.Values = append(dimension.Values, value)
				dimensionValueKeys[name][valueKey] = struct{}{}
			}
			dimensions[name] = dimension
		}

		evidence := map[string]any{
			"member_index":              member.MemberIndex,
			"materialization_key":       asset.MaterializationKey,
			"destination_relative_path": asset.DestinationRelativePath,
			"destination_sha256":        asset.DestinationSHA256,
			"member_bindings":           member.MemberBindings,
		}
		if asset.DestinationSizeBytes != nil {
			evidence["destination_size_bytes"] = *asset.DestinationSizeBytes
		}
		memberEvidence = append(memberEvidence, evidence)
	}
	for index := 0; index < firstMember.MemberCount; index++ {
		if _, exists := seenIndexes[index]; !exists {
			return model.MaterializedAssetCollectionManifest{}, fmt.Errorf("missing collection member index %d", index)
		}
	}

	_, membersSHA256, err := fp.CanonicalJSONSHA256(memberEvidence)
	if err != nil {
		return model.MaterializedAssetCollectionManifest{}, fmt.Errorf("hash collection member evidence: %w", err)
	}
	manifest := model.MaterializedAssetCollectionManifest{
		Schema:                  model.MaterializedAssetCollectionManifestSchemaV1,
		Asset:                   firstAsset.BindingName,
		MaterializationDomainID: firstAsset.MaterializationDomainID,
		DimensionOrder:          append([]string{}, firstMember.DimensionOrder...),
		Dimensions:              dimensions,
		Path:                    root + relativeTemplate,
		RequiredBindings:        append([]string{}, firstMember.DimensionOrder...),
		MemberCount:             firstMember.MemberCount,
		MembersSHA256:           "sha256:" + membersSHA256,
		CollectionFingerprint:   firstMember.CollectionFingerprint,
	}
	if err := manifest.Validate(); err != nil {
		return model.MaterializedAssetCollectionManifest{}, fmt.Errorf("materialized asset collection manifest: %w", err)
	}
	return manifest, nil
}

func collectionPathTemplate(asset model.MaterializedDataAsset) (string, string, error) {
	member := asset.CollectionMember
	if member == nil {
		return "", "", fmt.Errorf("collection member metadata is required")
	}
	if asset.DestinationRelativePath == "" {
		return "", "", fmt.Errorf("destination_relative_path is required")
	}
	localPath := filepath.ToSlash(asset.LocalPath)
	destination := filepath.ToSlash(asset.DestinationRelativePath)
	if !strings.HasSuffix(localPath, destination) {
		return "", "", fmt.Errorf("local_path %s does not end with destination_relative_path %s", asset.LocalPath, asset.DestinationRelativePath)
	}
	root := strings.TrimSuffix(localPath, destination)
	if root == "" {
		return "", "", fmt.Errorf("materialization root is required")
	}
	relativeTemplate := destination
	for _, name := range member.DimensionOrder {
		value, ok := member.MemberBindings[name]
		if !ok {
			return "", "", fmt.Errorf("member binding %q is required", name)
		}
		_, valueKey, err := collectionDimensionValue(value)
		if err != nil {
			return "", "", fmt.Errorf("member binding %s: %w", name, err)
		}
		if valueKey == "" || !strings.Contains(relativeTemplate, valueKey) {
			return "", "", fmt.Errorf("destination_relative_path %s does not contain binding %s value %s", asset.DestinationRelativePath, name, valueKey)
		}
		relativeTemplate = strings.ReplaceAll(relativeTemplate, valueKey, "${"+name+"}")
	}
	for _, name := range member.DimensionOrder {
		if !strings.Contains(relativeTemplate, "${"+name+"}") {
			return "", "", fmt.Errorf("destination template %s is missing binding %s", relativeTemplate, name)
		}
	}
	return root, relativeTemplate, nil
}

func renderCollectionRelativeTemplate(template string, order []string, bindings map[string]any) (string, error) {
	rendered := template
	for _, name := range order {
		value, ok := bindings[name]
		if !ok {
			return "", fmt.Errorf("member binding %q is required", name)
		}
		_, valueKey, err := collectionDimensionValue(value)
		if err != nil {
			return "", err
		}
		rendered = strings.ReplaceAll(rendered, "${"+name+"}", valueKey)
	}
	if strings.Contains(rendered, "${") {
		return "", fmt.Errorf("destination template has unresolved bindings")
	}
	return rendered, nil
}

func collectionDimensionValue(value any) (string, string, error) {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return "", "", fmt.Errorf("string binding must not be empty")
		}
		return "string", typed, nil
	case int:
		return "int", strconv.Itoa(typed), nil
	case bool:
		return "bool", strconv.FormatBool(typed), nil
	case json.Number:
		integer, err := strconv.Atoi(typed.String())
		if err != nil {
			return "", "", fmt.Errorf("number binding must be an int")
		}
		return "int", strconv.Itoa(integer), nil
	default:
		return "", "", fmt.Errorf("unsupported binding type %T", value)
	}
}

func sameStrings(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func canonicalOutputJSONFromAny(value any) (string, string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", "", fmt.Errorf("encode output: %w", err)
	}
	resolved, err := resolvedOutputFromJSON(string(data))
	if err != nil {
		return "", "", err
	}
	return canonicalOutputJSONFromResolved(resolved)
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

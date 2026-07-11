package document

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"goetl/internal/variable"
)

var ErrNotCanonicalWorkflow = errors.New("not a canonical workflow document")

type CanonicalWorkflowDocument struct {
	APIVersion     string
	Kind           string
	ID             string
	Variables      []variable.Variable
	Data           map[string]any
	Steps          []CanonicalWorkflowStep
	SourceManifest map[string]any
}

type CanonicalWorkflowStep struct {
	ID           string
	ParallelWith string
	FanOut       CanonicalFanOut
	Data         map[string]any
	Work         CanonicalWork
}

type CanonicalFanOut struct {
	Over string
	As   string
	ID   string
}

type CanonicalWork struct {
	Type                string
	OutputPrefix        string
	OutputExtension     string
	Parameters          map[string]any
	ParameterAccessors  map[string]string
	ResourceConstraints []any
}

func DecodeCanonicalWorkflowSource(data []byte, options DecodeOptions) (CanonicalWorkflowDocument, error) {
	if notCanonical, err := jsonSourceIsNotCanonicalWorkflow(data, options); err != nil {
		return CanonicalWorkflowDocument{}, err
	} else if notCanonical {
		return CanonicalWorkflowDocument{}, ErrNotCanonicalWorkflow
	}

	value, err := DecodeSource(data, options)
	if err != nil {
		return CanonicalWorkflowDocument{}, err
	}
	root, ok := value.(map[string]any)
	if !ok {
		return CanonicalWorkflowDocument{}, sourceError(options.Path, 1, 1, string(options.Format), fmt.Errorf("workflow document must be an object"))
	}
	return CanonicalWorkflowFromValue(root)
}

func jsonSourceIsNotCanonicalWorkflow(data []byte, options DecodeOptions) (bool, error) {
	format, err := SourceFormatFor(options)
	if err != nil {
		return false, err
	}
	if format != SourceFormatJSON {
		return false, nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return false, nil
	}
	if _, ok := fields["workflow"]; ok {
		return true, nil
	}
	if _, hasAPIVersion := fields["api_version"]; !hasAPIVersion {
		if _, hasKind := fields["kind"]; !hasKind {
			return true, nil
		}
	}
	return false, nil
}

func CanonicalWorkflowFromValue(root map[string]any) (CanonicalWorkflowDocument, error) {
	if _, ok := root["workflow"]; ok {
		return CanonicalWorkflowDocument{}, ErrNotCanonicalWorkflow
	}
	if _, ok := root["api_version"]; !ok {
		if _, hasKind := root["kind"]; !hasKind {
			return CanonicalWorkflowDocument{}, ErrNotCanonicalWorkflow
		}
	}
	if err := rejectGoCasedFields(root, "workflow document"); err != nil {
		return CanonicalWorkflowDocument{}, err
	}

	apiVersion, err := requiredString(root, "api_version", "workflow document")
	if err != nil {
		return CanonicalWorkflowDocument{}, err
	}
	kind, err := requiredString(root, "kind", "workflow document")
	if err != nil {
		return CanonicalWorkflowDocument{}, err
	}
	id, err := requiredString(root, "id", "workflow document")
	if err != nil {
		return CanonicalWorkflowDocument{}, err
	}
	if apiVersion != APIVersionV1Alpha1 {
		return CanonicalWorkflowDocument{}, fmt.Errorf("api_version must be %q, got %q", APIVersionV1Alpha1, apiVersion)
	}
	if kind != KindWorkflow {
		return CanonicalWorkflowDocument{}, fmt.Errorf("kind must be %q, got %q", KindWorkflow, kind)
	}

	variables, err := optionalVariables(root, variable.NamespaceWorkflow)
	if err != nil {
		return CanonicalWorkflowDocument{}, err
	}
	steps, err := requiredWorkflowSteps(root)
	if err != nil {
		return CanonicalWorkflowDocument{}, err
	}

	dataSection, err := optionalObject(root, "data", "workflow document")
	if err != nil {
		return CanonicalWorkflowDocument{}, err
	}
	sourceManifest, err := optionalObject(root, "source_manifest", "workflow document")
	if err != nil {
		return CanonicalWorkflowDocument{}, err
	}

	return CanonicalWorkflowDocument{
		APIVersion:     apiVersion,
		Kind:           kind,
		ID:             id,
		Variables:      variables,
		Data:           dataSection,
		Steps:          steps,
		SourceManifest: sourceManifest,
	}, nil
}

func requiredWorkflowSteps(root map[string]any) ([]CanonicalWorkflowStep, error) {
	raw, ok := root["steps"]
	if !ok {
		return nil, fmt.Errorf("workflow document steps is required")
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("workflow document steps must be a list")
	}
	steps := make([]CanonicalWorkflowStep, 0, len(items))
	for index, item := range items {
		fields, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("workflow document steps[%d] must be an object", index)
		}
		step, err := canonicalWorkflowStep(fields, index)
		if err != nil {
			return nil, err
		}
		steps = append(steps, step)
	}
	return steps, nil
}

func canonicalWorkflowStep(fields map[string]any, index int) (CanonicalWorkflowStep, error) {
	context := fmt.Sprintf("workflow document steps[%d]", index)
	if err := rejectGoCasedFields(fields, context); err != nil {
		return CanonicalWorkflowStep{}, err
	}
	id, err := requiredString(fields, "id", context)
	if err != nil {
		return CanonicalWorkflowStep{}, err
	}
	parallelWith, err := optionalString(fields, "parallel_with", context)
	if err != nil {
		return CanonicalWorkflowStep{}, err
	}
	fanOutFields, err := requiredObject(fields, "fan_out", context)
	if err != nil {
		return CanonicalWorkflowStep{}, err
	}
	fanOut, err := canonicalFanOut(fanOutFields, context+".fan_out")
	if err != nil {
		return CanonicalWorkflowStep{}, err
	}
	workFields, err := requiredObject(fields, "work", context)
	if err != nil {
		return CanonicalWorkflowStep{}, err
	}
	work, err := canonicalWork(workFields, context+".work")
	if err != nil {
		return CanonicalWorkflowStep{}, err
	}
	dataSection, err := optionalObject(fields, "data", context)
	if err != nil {
		return CanonicalWorkflowStep{}, err
	}
	return CanonicalWorkflowStep{
		ID:           id,
		ParallelWith: parallelWith,
		FanOut:       fanOut,
		Data:         dataSection,
		Work:         work,
	}, nil
}

func canonicalFanOut(fields map[string]any, context string) (CanonicalFanOut, error) {
	if err := rejectGoCasedFields(fields, context); err != nil {
		return CanonicalFanOut{}, err
	}
	over, err := requiredString(fields, "over", context)
	if err != nil {
		return CanonicalFanOut{}, err
	}
	as, err := requiredString(fields, "as", context)
	if err != nil {
		return CanonicalFanOut{}, err
	}
	id, err := requiredString(fields, "id", context)
	if err != nil {
		return CanonicalFanOut{}, err
	}
	return CanonicalFanOut{Over: over, As: as, ID: id}, nil
}

func canonicalWork(fields map[string]any, context string) (CanonicalWork, error) {
	if err := rejectGoCasedFields(fields, context); err != nil {
		return CanonicalWork{}, err
	}
	workType, err := requiredString(fields, "type", context)
	if err != nil {
		return CanonicalWork{}, err
	}
	outputPrefix, err := optionalString(fields, "output_prefix", context)
	if err != nil {
		return CanonicalWork{}, err
	}
	outputExtension, err := optionalString(fields, "output_extension", context)
	if err != nil {
		return CanonicalWork{}, err
	}
	parameters, err := optionalObject(fields, "parameters", context)
	if err != nil {
		return CanonicalWork{}, err
	}
	parameterAccessors, err := optionalStringMap(fields, "parameter_accessors", context)
	if err != nil {
		return CanonicalWork{}, err
	}
	resourceConstraints, err := optionalList(fields, "resource_constraints", context)
	if err != nil {
		return CanonicalWork{}, err
	}
	return CanonicalWork{
		Type:                workType,
		OutputPrefix:        outputPrefix,
		OutputExtension:     outputExtension,
		Parameters:          parameters,
		ParameterAccessors:  parameterAccessors,
		ResourceConstraints: resourceConstraints,
	}, nil
}

func optionalVariables(root map[string]any, namespace variable.Namespace) ([]variable.Variable, error) {
	raw, ok := root["variables"]
	if !ok {
		return nil, nil
	}
	fields, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("workflow document variables must be an object")
	}
	return LoadVariables(fields, namespace)
}

func requiredString(fields map[string]any, name string, context string) (string, error) {
	value, ok := fields[name]
	if !ok {
		return "", fmt.Errorf("%s %s is required", context, name)
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("%s %s must be a non-empty string", context, name)
	}
	return text, nil
}

func optionalString(fields map[string]any, name string, context string) (string, error) {
	value, ok := fields[name]
	if !ok {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s %s must be a string", context, name)
	}
	return text, nil
}

func requiredObject(fields map[string]any, name string, context string) (map[string]any, error) {
	value, ok := fields[name]
	if !ok {
		return nil, fmt.Errorf("%s %s is required", context, name)
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s %s must be an object", context, name)
	}
	return object, nil
}

func optionalObject(fields map[string]any, name string, context string) (map[string]any, error) {
	value, ok := fields[name]
	if !ok {
		return nil, nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s %s must be an object", context, name)
	}
	return object, nil
}

func optionalStringMap(fields map[string]any, name string, context string) (map[string]string, error) {
	object, err := optionalObject(fields, name, context)
	if err != nil || object == nil {
		return nil, err
	}
	values := make(map[string]string, len(object))
	for key, value := range object {
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("%s %s.%s must be a string", context, name, key)
		}
		values[key] = text
	}
	return values, nil
}

func optionalList(fields map[string]any, name string, context string) ([]any, error) {
	value, ok := fields[name]
	if !ok {
		return nil, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s %s must be a list", context, name)
	}
	return items, nil
}

func rejectGoCasedFields(fields map[string]any, context string) error {
	goCased := []string{
		"ID",
		"Variables",
		"Steps",
		"FanOut",
		"WorkItem",
		"FanOutExpression",
		"TokenAccessor",
		"IDTokenAccessor",
		"OutputAccessor",
		"Type",
		"Parameters",
		"ParameterAccessors",
	}
	for _, name := range goCased {
		if _, ok := fields[name]; ok {
			return fmt.Errorf("%s mixes canonical snake_case with Go field %q", context, name)
		}
	}
	return nil
}

package workflow

import (
	"fmt"
	"sort"
	"strings"

	fp "goetl/internal/fingerprint"
	"goetl/internal/model"
	"goetl/internal/variable"
)

type DataAssetInstance struct {
	DefinitionName string
	Selection      []string
	Parameters     map[string]any
	BoundAsset     model.BoundDataAsset
	AssetKey       string
	Diagnostic     string
}

func InstantiateDataAsset(
	resolver variable.Resolver,
	fanOutValue variable.ResolvedValue,
	definitionName string,
	definition model.DataInputDefinition,
	selection []string,
	bindings map[string]variable.TypedExpression,
) (DataAssetInstance, error) {
	parameters, err := resolveAssetParameters(resolver, fanOutValue, definition, bindings)
	if err != nil {
		return DataAssetInstance{}, err
	}
	effectiveSelection, err := definition.EffectiveSelection(selection)
	if err != nil {
		return DataAssetInstance{}, err
	}
	asset, err := definition.BoundInputAssetWithSelection(definitionName, effectiveSelection, parameters)
	if err != nil {
		return DataAssetInstance{}, err
	}
	key, err := CanonicalDataAssetInstanceKey(definitionName, effectiveSelection, asset)
	if err != nil {
		return DataAssetInstance{}, err
	}
	return DataAssetInstance{
		DefinitionName: definitionName,
		Selection:      effectiveSelection,
		Parameters:     parameters,
		BoundAsset:     asset,
		AssetKey:       key,
		Diagnostic:     dataAssetDiagnostic(definitionName, effectiveSelection, parameters),
	}, nil
}

func CanonicalDataAssetInstanceKey(definitionName string, selection []string, asset model.BoundDataAsset) (string, error) {
	if strings.TrimSpace(definitionName) == "" {
		return "", fmt.Errorf("asset definition name is required")
	}
	if err := asset.Validate(); err != nil {
		return "", err
	}
	bindingFingerprint, err := dataAssetBindingFingerprint(asset)
	if err != nil {
		return "", err
	}
	identity := map[string]any{
		"asset_definition":      definitionName,
		"resolved_parameters":   asset.Parameters,
		"selection":             append([]string{}, selection...),
		"provider_name":         asset.ProviderName,
		"provider_type":         asset.Provider,
		"kind":                  asset.Kind,
		"format":                asset.Format,
		"resolved_location":     asset.Location,
		"integrity":             asset.Integrity,
		"cache":                 asset.Cache,
		"archive_selection":     asset.Archive,
		"expose_mode":           archiveExposeMode(asset.Archive),
		"transfer_policy":       asset.TransferPolicy,
		"binding_fingerprint":   bindingFingerprint,
		"materialization_scope": asset.Materialization.Scope,
	}
	_, hash, err := fp.CanonicalJSONSHA256(normalizedCanonicalValue(identity))
	if err != nil {
		return "", err
	}
	return "sha256:" + hash, nil
}

func resolveAssetParameters(
	resolver variable.Resolver,
	fanOutValue variable.ResolvedValue,
	definition model.DataInputDefinition,
	bindings map[string]variable.TypedExpression,
) (map[string]any, error) {
	for name := range definition.Parameters {
		if _, ok := bindings[name]; !ok {
			return nil, fmt.Errorf("missing asset parameter %q", name)
		}
	}
	for name := range bindings {
		if _, ok := definition.Parameters[name]; !ok {
			return nil, fmt.Errorf("unknown asset parameter %q", name)
		}
	}

	names := make([]string, 0, len(bindings))
	for name := range bindings {
		names = append(names, name)
	}
	sort.Strings(names)

	parameters := make(map[string]any, len(bindings))
	for _, name := range names {
		resolved, err := resolveAssetParameterValue(resolver, fanOutValue, bindings[name])
		if err != nil {
			return nil, fmt.Errorf("asset parameter %s: %w", name, err)
		}
		value, err := assetParameterAny(resolved)
		if err != nil {
			return nil, fmt.Errorf("asset parameter %s: %w", name, err)
		}
		if err := validateAssetParameterType(name, definition.Parameters[name], resolved); err != nil {
			return nil, err
		}
		parameters[name] = value
	}
	return parameters, nil
}

func resolveAssetParameterValue(resolver variable.Resolver, fanOutValue variable.ResolvedValue, expression variable.TypedExpression) (variable.ResolvedValue, error) {
	if text, ok := expression.Expression.(string); ok {
		if referenceText, isReference := wholeReferenceExpressionText(text); isReference {
			if referenceText == string(variable.NamespaceFanOut) {
				return fanOutValue, nil
			}
			if strings.HasPrefix(referenceText, string(variable.NamespaceFanOut)+".") {
				accessor := strings.TrimPrefix(referenceText, string(variable.NamespaceFanOut))
				resolved, err := variable.ApplyAccessor(fanOutValue, accessor)
				if err != nil {
					return variable.ResolvedValue{}, err
				}
				if resolved.Type != expression.Type {
					return variable.ResolvedValue{}, fmt.Errorf("reference has type %s, want %s", resolved.Type, expression.Type)
				}
				return resolved, nil
			}
			reference, err := variable.ParseReference(referenceText)
			if err != nil {
				return variable.ResolvedValue{}, err
			}
			resolved, err := resolver.Resolve(reference)
			if err != nil {
				return variable.ResolvedValue{}, err
			}
			if resolved.Type != expression.Type {
				return variable.ResolvedValue{}, fmt.Errorf("reference has type %s, want %s", resolved.Type, expression.Type)
			}
			return resolved, nil
		}
	}

	switch expression.Type {
	case variable.TypeString, variable.TypePath:
		text, ok := expression.Expression.(string)
		if !ok {
			return variable.ResolvedValue{}, fmt.Errorf("%s expression must be a string", expression.Type)
		}
		return variable.ResolvedValue{Type: expression.Type, Value: text}, nil
	case variable.TypeInt:
		integer, ok := expression.Expression.(int)
		if !ok {
			return variable.ResolvedValue{}, fmt.Errorf("int expression must be an int or whole-value reference")
		}
		return variable.ResolvedValue{Type: variable.TypeInt, Value: integer}, nil
	case variable.TypeBool:
		boolean, ok := expression.Expression.(bool)
		if !ok {
			return variable.ResolvedValue{}, fmt.Errorf("bool expression must be a bool or whole-value reference")
		}
		return variable.ResolvedValue{Type: variable.TypeBool, Value: boolean}, nil
	default:
		return variable.ResolvedValue{}, fmt.Errorf("asset parameter has type %s, want scalar", expression.Type)
	}
}

func assetParameterAny(value variable.ResolvedValue) (any, error) {
	switch value.Type {
	case variable.TypeString, variable.TypePath:
		text, ok := value.Value.(string)
		if !ok {
			return nil, fmt.Errorf("invalid string value")
		}
		return text, nil
	case variable.TypeInt:
		integer, ok := value.Value.(int)
		if !ok {
			return nil, fmt.Errorf("invalid int value")
		}
		return integer, nil
	case variable.TypeBool:
		boolean, ok := value.Value.(bool)
		if !ok {
			return nil, fmt.Errorf("invalid bool value")
		}
		return boolean, nil
	default:
		return nil, fmt.Errorf("has type %s, want scalar", value.Type)
	}
}

func validateAssetParameterType(name string, definition model.DataParameterDefinition, value variable.ResolvedValue) error {
	if definition.Type == "" {
		return nil
	}
	switch definition.Type {
	case "string":
		if value.Type != variable.TypeString && value.Type != variable.TypePath {
			return fmt.Errorf("asset parameter %s has type %s, want string", name, value.Type)
		}
	case "int":
		if value.Type != variable.TypeInt {
			return fmt.Errorf("asset parameter %s has type %s, want int", name, value.Type)
		}
	case "bool":
		if value.Type != variable.TypeBool {
			return fmt.Errorf("asset parameter %s has type %s, want bool", name, value.Type)
		}
	default:
		return fmt.Errorf("asset parameter %s has unsupported declared type %q", name, definition.Type)
	}
	return nil
}

func dataAssetBindingFingerprint(asset model.BoundDataAsset) (string, error) {
	identity := map[string]any{
		"provider_name":   asset.ProviderName,
		"provider_type":   asset.Provider,
		"kind":            asset.Kind,
		"format":          asset.Format,
		"location":        asset.Location,
		"integrity":       asset.Integrity,
		"cache":           asset.Cache,
		"archive":         asset.Archive,
		"materialization": asset.Materialization,
		"transfer_policy": asset.TransferPolicy,
	}
	_, hash, err := fp.CanonicalJSONSHA256(normalizedCanonicalValue(identity))
	if err != nil {
		return "", err
	}
	return "sha256:" + hash, nil
}

func dataAssetDiagnostic(definitionName string, selection []string, parameters map[string]any) string {
	names := make([]string, 0, len(parameters))
	for name := range parameters {
		names = append(names, name)
	}
	sort.Strings(names)

	parts := make([]string, 0, len(names)+1)
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s=%v", name, parameters[name]))
	}
	if len(selection) > 0 {
		parts = append(parts, "select="+strings.Join(selection, ","))
	}
	return fmt.Sprintf("%s[%s]", definitionName, strings.Join(parts, ";"))
}

func wholeReferenceExpressionText(expression string) (string, bool) {
	if strings.HasPrefix(expression, `\${`) {
		return "", false
	}
	if !strings.HasPrefix(expression, "${") || !strings.HasSuffix(expression, "}") {
		return "", false
	}
	referenceText := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(expression, "${"), "}"))
	if referenceText == "" {
		return "", false
	}
	if strings.Contains(referenceText, "${") || strings.ContainsAny(referenceText, "{}") {
		return "", false
	}
	return referenceText, true
}

package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const MaterializedAssetCollectionManifestSchemaV1 = "goet/materialized-asset-collection/v1"

type MaterializedAssetCollectionManifest struct {
	Schema                  string                                          `json:"schema"`
	Asset                   string                                          `json:"asset"`
	MaterializationDomainID string                                          `json:"materialization_domain_id"`
	DimensionOrder          []string                                        `json:"dimension_order"`
	Dimensions              map[string]MaterializedAssetCollectionDimension `json:"dimensions"`
	Path                    string                                          `json:"path"`
	RequiredBindings        []string                                        `json:"required_bindings"`
	MemberCount             int                                             `json:"member_count"`
	MembersSHA256           string                                          `json:"members_sha256"`
	CollectionFingerprint   string                                          `json:"collection_fingerprint"`
}

type MaterializedAssetCollectionDimension struct {
	Type   string `json:"type"`
	Values []any  `json:"values"`
}

type MaterializedDataAssetCollectionMember struct {
	CollectionFingerprint   string         `json:"collection_fingerprint"`
	MemberIndex             int            `json:"member_index"`
	MemberCount             int            `json:"member_count"`
	DimensionOrder          []string       `json:"dimension_order,omitempty"`
	MemberBindings          map[string]any `json:"member_bindings,omitempty"`
	DestinationRelativePath string         `json:"destination_relative_path,omitempty"`
	PathTemplateIdentity    string         `json:"path_template_identity,omitempty"`
}

func (manifest MaterializedAssetCollectionManifest) EffectiveSchema() string {
	if strings.TrimSpace(manifest.Schema) == "" {
		return MaterializedAssetCollectionManifestSchemaV1
	}
	return manifest.Schema
}

func (manifest MaterializedAssetCollectionManifest) Validate() error {
	if manifest.EffectiveSchema() != MaterializedAssetCollectionManifestSchemaV1 {
		return fmt.Errorf("unsupported materialized asset collection manifest schema %q", manifest.Schema)
	}
	if err := validateDataName(manifest.Asset, "materialized asset collection asset"); err != nil {
		return err
	}
	if strings.TrimSpace(manifest.MaterializationDomainID) == "" {
		return fmt.Errorf("materialization_domain_id is required")
	}
	if strings.TrimSpace(manifest.MaterializationDomainID) != manifest.MaterializationDomainID {
		return fmt.Errorf("materialization_domain_id must not contain leading or trailing whitespace")
	}
	if err := validatePrefixedSHA256("members_sha256", manifest.MembersSHA256); err != nil {
		return err
	}
	if err := validatePrefixedSHA256("collection_fingerprint", manifest.CollectionFingerprint); err != nil {
		return err
	}
	if manifest.MemberCount <= 0 {
		return fmt.Errorf("member_count must be positive")
	}
	if err := validateDimensionOrderAndBindings(manifest.DimensionOrder, manifest.Dimensions, manifest.RequiredBindings); err != nil {
		return err
	}
	cardinality := uint64(1)
	for _, name := range manifest.DimensionOrder {
		dimension := manifest.Dimensions[name]
		count, err := dimension.cardinality()
		if err != nil {
			return fmt.Errorf("dimension %s: %w", name, err)
		}
		if cardinality > math.MaxUint64/count {
			return fmt.Errorf("member_count overflow")
		}
		cardinality *= count
	}
	maxInt := uint64(int(^uint(0) >> 1))
	if cardinality > maxInt || manifest.MemberCount != int(cardinality) {
		return fmt.Errorf("member_count = %d, want %d", manifest.MemberCount, cardinality)
	}
	pathBindings, err := collectionManifestPathBindings(manifest.Path)
	if err != nil {
		return err
	}
	if !sameStringSlice(pathBindings, manifest.RequiredBindings) {
		return fmt.Errorf("path bindings = %v, want required_bindings %v", pathBindings, manifest.RequiredBindings)
	}
	return nil
}

func (dimension MaterializedAssetCollectionDimension) cardinality() (uint64, error) {
	if len(dimension.Values) == 0 {
		return 0, fmt.Errorf("values must not be empty")
	}
	parameter := DataParameterDefinition{Type: dimension.Type}
	wrapper := DataAssetCollectionDimension{Parameter: "value", Values: dimension.Values}
	if err := wrapper.validateExplicitValues(parameter); err != nil {
		return 0, err
	}
	return uint64(len(dimension.Values)), nil
}

func (dimension *MaterializedAssetCollectionDimension) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type   string            `json:"type"`
		Values []json.RawMessage `json:"values"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	dimension.Type = raw.Type
	dimension.Values = make([]any, 0, len(raw.Values))
	for index, encodedValue := range raw.Values {
		value, err := unmarshalCollectionScalar(encodedValue)
		if err != nil {
			return fmt.Errorf("values[%d]: %w", index, err)
		}
		dimension.Values = append(dimension.Values, value)
	}
	return nil
}

func (member MaterializedDataAssetCollectionMember) Validate() error {
	if err := validatePrefixedSHA256("collection_fingerprint", member.CollectionFingerprint); err != nil {
		return err
	}
	if member.MemberCount <= 0 {
		return fmt.Errorf("member_count must be positive")
	}
	if member.MemberIndex < 0 || member.MemberIndex >= member.MemberCount {
		return fmt.Errorf("member_index must be in [0, member_count)")
	}
	if member.DestinationRelativePath != "" {
		if _, err := ValidateArtifactRelativePath(member.DestinationRelativePath); err != nil {
			return fmt.Errorf("destination_relative_path: %w", err)
		}
	}
	if member.PathTemplateIdentity != "" {
		if err := validatePrefixedSHA256("path_template_identity", member.PathTemplateIdentity); err != nil {
			return err
		}
	}
	for _, name := range member.DimensionOrder {
		if err := validateDataName(name, "collection member dimension"); err != nil {
			return err
		}
		value, ok := member.MemberBindings[name]
		if !ok {
			return fmt.Errorf("member binding %q is required", name)
		}
		if _, err := collectionValueKey(value); err != nil {
			return fmt.Errorf("member binding %s: %w", name, err)
		}
	}
	return nil
}

func validateDimensionOrderAndBindings(
	order []string,
	dimensions map[string]MaterializedAssetCollectionDimension,
	requiredBindings []string,
) error {
	if len(order) == 0 {
		return fmt.Errorf("dimension_order is required")
	}
	if len(order) != len(dimensions) {
		return fmt.Errorf("dimension_order and dimensions must contain the same names")
	}
	seen := make(map[string]struct{}, len(order))
	for _, name := range order {
		if err := validateDataName(name, "dimension_order entry"); err != nil {
			return err
		}
		if _, duplicate := seen[name]; duplicate {
			return fmt.Errorf("duplicate dimension %q", name)
		}
		seen[name] = struct{}{}
		if _, ok := dimensions[name]; !ok {
			return fmt.Errorf("dimension %q is missing", name)
		}
	}
	if !sameStringSlice(order, requiredBindings) {
		return fmt.Errorf("required_bindings must match dimension_order")
	}
	return nil
}

func collectionManifestPathBindings(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, fmt.Errorf("path is required")
	}
	required := []string{}
	seen := map[string]struct{}{}
	for i := 0; i < len(value); {
		if strings.HasPrefix(value[i:], `\${`) {
			end := strings.IndexByte(value[i+3:], '}')
			if end < 0 {
				i += len(`\${`)
				continue
			}
			i += 3 + end + 1
			continue
		}
		if strings.HasPrefix(value[i:], "${") {
			end := strings.IndexByte(value[i+2:], '}')
			if end < 0 {
				return nil, fmt.Errorf("path has unterminated interpolation")
			}
			name := value[i+2 : i+2+end]
			if strings.Contains(name, "${") || strings.ContainsAny(name, "{}") {
				return nil, fmt.Errorf("path has nested or malformed interpolation")
			}
			if strings.Contains(name, ".") {
				return nil, fmt.Errorf("path binding %q must not be namespace-qualified", name)
			}
			if err := validateDataName(name, "path binding"); err != nil {
				return nil, err
			}
			if _, exists := seen[name]; !exists {
				required = append(required, name)
				seen[name] = struct{}{}
			}
			i += 2 + end + 1
			continue
		}
		if value[i] == '}' {
			return nil, fmt.Errorf("path has unsupported interpolation syntax")
		}
		i++
	}
	return required, nil
}

func validatePrefixedSHA256(field string, value string) error {
	if !strings.HasPrefix(value, "sha256:") {
		return fmt.Errorf("%s must use sha256: prefix", field)
	}
	return validateOptionalSHA256(field, strings.TrimPrefix(value, "sha256:"))
}

func sameStringSlice(left []string, right []string) bool {
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

func (dimension MaterializedAssetCollectionDimension) MarshalJSON() ([]byte, error) {
	type alias MaterializedAssetCollectionDimension
	return json.Marshal(alias(dimension))
}

func (member *MaterializedDataAssetCollectionMember) UnmarshalJSON(data []byte) error {
	type rawMember MaterializedDataAssetCollectionMember
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var raw rawMember
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	bindings := make(map[string]any, len(raw.MemberBindings))
	for key, value := range raw.MemberBindings {
		normalized, err := normalizeJSONScalar(value)
		if err != nil {
			return fmt.Errorf("member_bindings.%s: %w", key, err)
		}
		bindings[key] = normalized
	}
	*member = MaterializedDataAssetCollectionMember(raw)
	member.MemberBindings = bindings
	return nil
}

func normalizeJSONScalar(value any) (any, error) {
	switch typed := value.(type) {
	case string, bool, int:
		return typed, nil
	case json.Number:
		integer, err := strconv.Atoi(typed.String())
		if err != nil {
			return nil, fmt.Errorf("number must be an int")
		}
		return integer, nil
	default:
		return nil, fmt.Errorf("must be a scalar string, int, or bool")
	}
}

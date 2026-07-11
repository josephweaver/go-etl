package model

import (
	"fmt"
	"sort"
	"strings"
)

type DataDefinitions struct {
	Inputs  map[string]DataInputDefinition  `json:"inputs,omitempty"`
	Outputs map[string]DataOutputDefinition `json:"outputs,omitempty"`
}

type DataInputDefinition struct {
	Kind       string                             `json:"kind"`
	Format     string                             `json:"format,omitempty"`
	Parameters map[string]DataParameterDefinition `json:"parameters,omitempty"`
	Files      map[string]DataFileRoleDefinition  `json:"files,omitempty"`
	Select     []string                           `json:"select,omitempty"`
	Binding    DataInputBindingDefinition         `json:"binding,omitempty"`
	Metadata   map[string]any                     `json:"metadata,omitempty"`
}

type DataParameterDefinition struct {
	Type string `json:"type,omitempty"`
}

type DataFileRoleDefinition struct {
	Member   string `json:"member"`
	As       string `json:"as,omitempty"`
	Required *bool  `json:"required,omitempty"`
}

type DataInputBindingDefinition struct {
	ProviderName    string                        `json:"provider_name,omitempty"`
	Provider        string                        `json:"provider"`
	Location        DataDefinitionLocation        `json:"location,omitempty"`
	Archive         DataArchiveBindingDefinition  `json:"archive,omitempty"`
	Integrity       DataAssetIntegrityTemplate    `json:"integrity,omitempty"`
	Cache           DataDefinitionCache           `json:"cache,omitempty"`
	Materialization DataDefinitionMaterialization `json:"materialization,omitempty"`
	TransferPolicy  DataAssetTransferPolicy       `json:"transfer_policy,omitempty"`
	Metadata        map[string]any                `json:"metadata,omitempty"`
}

type DataDefinitionLocation struct {
	URI          string `json:"uri,omitempty"`
	URLTemplate  string `json:"url_template,omitempty"`
	URITemplate  string `json:"uri_template,omitempty"`
	Name         string `json:"name,omitempty"`
	LocationName string `json:"location_name,omitempty"`
	Path         string `json:"path,omitempty"`
	PathTemplate string `json:"path_template,omitempty"`
	Remote       string `json:"remote,omitempty"`
	DrivePath    string `json:"drive_path,omitempty"`
	FileID       string `json:"file_id,omitempty"`
}

type DataArchiveBindingDefinition struct {
	Type   string `json:"type,omitempty"`
	Expose string `json:"expose,omitempty"`
}

type DataDefinitionCache struct {
	Strategy         string `json:"strategy,omitempty"`
	CacheKey         string `json:"cache_key,omitempty"`
	CacheKeyTemplate string `json:"cache_key_template,omitempty"`
	Immutable        *bool  `json:"immutable,omitempty"`
}

type DataDefinitionMaterialization struct {
	Scope    string `json:"scope,omitempty"`
	Strategy string `json:"strategy,omitempty"`
}

type DataOutputDefinition struct {
	Kind       string                             `json:"kind"`
	Format     string                             `json:"format,omitempty"`
	Parameters map[string]DataParameterDefinition `json:"parameters,omitempty"`
	Binding    DataOutputBindingDefinition        `json:"binding,omitempty"`
	Metadata   map[string]any                     `json:"metadata,omitempty"`
}

type DataOutputBindingDefinition struct {
	Provider        string                 `json:"provider"`
	Location        DataDefinitionLocation `json:"location,omitempty"`
	OverwritePolicy string                 `json:"overwrite_policy,omitempty"`
	Metadata        map[string]any         `json:"metadata,omitempty"`
}

func (definitions DataDefinitions) Validate() error {
	for _, name := range sortedInputDefinitionNames(definitions.Inputs) {
		if err := validateDataName(name, "data input definition name"); err != nil {
			return err
		}
		if err := definitions.Inputs[name].Validate(name); err != nil {
			return fmt.Errorf("data input %s: %w", name, err)
		}
	}
	for _, name := range sortedOutputDefinitionNames(definitions.Outputs) {
		if err := validateDataName(name, "data output definition name"); err != nil {
			return err
		}
		if err := definitions.Outputs[name].Validate(name); err != nil {
			return fmt.Errorf("data output %s: %w", name, err)
		}
	}
	return nil
}

func (definition DataInputDefinition) Validate(name string) error {
	_, err := definition.ProviderTemplate(name, nil)
	return err
}

func (definition DataInputDefinition) BoundInputAsset(name string, parameters map[string]any) (BoundDataAsset, error) {
	return definition.BoundInputAssetWithSelection(name, nil, parameters)
}

func (definition DataInputDefinition) BoundInputAssetWithSelection(name string, selection []string, parameters map[string]any) (BoundDataAsset, error) {
	provider, err := definition.ProviderTemplate(name, selection)
	if err != nil {
		return BoundDataAsset{}, err
	}
	return provider.Bind(name, parameters)
}

func (definition DataInputDefinition) ProviderTemplate(name string, selection []string) (DataProviderTemplate, error) {
	if err := validateDataName(name, "data input definition name"); err != nil {
		return DataProviderTemplate{}, err
	}
	if strings.TrimSpace(definition.Kind) == "" {
		return DataProviderTemplate{}, fmt.Errorf("kind is required")
	}
	parameterNames, err := definition.parameterNames()
	if err != nil {
		return DataProviderTemplate{}, err
	}
	if err := definition.validateFileRoles(); err != nil {
		return DataProviderTemplate{}, err
	}
	effectiveSelection, err := definition.effectiveSelection(selection)
	if err != nil {
		return DataProviderTemplate{}, err
	}
	archive, err := definition.archiveTemplate(effectiveSelection)
	if err != nil {
		return DataProviderTemplate{}, err
	}

	provider := DataProviderTemplate{
		Name:            definition.effectiveProviderName(name),
		Kind:            definition.Kind,
		Format:          definition.Format,
		Provider:        definition.Binding.Provider,
		Parameters:      parameterNames,
		Archive:         archive,
		Integrity:       definition.Binding.Integrity,
		Cache:           definition.Binding.cacheTemplate(),
		Materialization: definition.Binding.materializationTemplate(),
		TransferPolicy:  definition.Binding.TransferPolicy,
		Metadata:        mergeMetadata(definition.Metadata, definition.Binding.Metadata),
	}
	if err := provider.applyCanonicalLocation(definition.Binding.Location); err != nil {
		return DataProviderTemplate{}, err
	}
	if err := provider.Validate(); err != nil {
		return DataProviderTemplate{}, err
	}
	return provider, nil
}

func (definition DataOutputDefinition) Validate(name string) error {
	_, err := definition.BoundOutputTarget(name, "artifact", nil)
	return err
}

func (definition DataOutputDefinition) BoundOutputTarget(name string, fromArtifact string, parameters map[string]any) (BoundPublishTarget, error) {
	if err := validateDataName(name, "data output definition name"); err != nil {
		return BoundPublishTarget{}, err
	}
	if strings.TrimSpace(definition.Kind) == "" {
		return BoundPublishTarget{}, fmt.Errorf("kind is required")
	}
	if _, err := definition.parameterNames(); err != nil {
		return BoundPublishTarget{}, err
	}
	if err := validateOverwritePolicy(definition.Binding.OverwritePolicy); err != nil {
		return BoundPublishTarget{}, err
	}
	location, err := definition.Binding.boundLocation(parameters)
	if err != nil {
		return BoundPublishTarget{}, err
	}
	target := BoundPublishTarget{
		Name:            name,
		FromArtifact:    fromArtifact,
		TargetName:      name,
		Location:        location,
		OverwritePolicy: definition.Binding.OverwritePolicy,
		Parameters:      parameters,
		Metadata:        mergeMetadata(definition.Metadata, definition.Binding.Metadata),
	}
	if err := target.Validate(); err != nil {
		return BoundPublishTarget{}, err
	}
	return target, nil
}

func (definition DataInputDefinition) parameterNames() ([]string, error) {
	names := make([]string, 0, len(definition.Parameters))
	for name, parameter := range definition.Parameters {
		if err := validateDataName(name, "data asset parameter"); err != nil {
			return nil, err
		}
		if parameter.Type != "" {
			switch parameter.Type {
			case "string", "int", "bool":
			default:
				return nil, fmt.Errorf("unsupported data asset parameter %s type %q", name, parameter.Type)
			}
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func (definition DataOutputDefinition) parameterNames() ([]string, error) {
	names := make([]string, 0, len(definition.Parameters))
	for name, parameter := range definition.Parameters {
		if err := validateDataName(name, "data output parameter"); err != nil {
			return nil, err
		}
		if parameter.Type != "" {
			switch parameter.Type {
			case "string", "int", "bool":
			default:
				return nil, fmt.Errorf("unsupported data output parameter %s type %q", name, parameter.Type)
			}
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func (definition DataInputDefinition) validateFileRoles() error {
	for _, name := range sortedFileRoleNames(definition.Files) {
		if err := validateDataName(name, "data file role name"); err != nil {
			return err
		}
		role := definition.Files[name]
		if strings.TrimSpace(role.Member) == "" {
			return fmt.Errorf("file role %s member is required", name)
		}
		if _, err := validateDataRelativePath(canonicalAssetTemplate(role.Member), "file role member"); err != nil {
			return fmt.Errorf("file role %s: %w", name, err)
		}
		if role.As != "" {
			if _, err := validateDataRelativePath(canonicalAssetTemplate(role.As), "file role as"); err != nil {
				return fmt.Errorf("file role %s: %w", name, err)
			}
		}
	}
	return nil
}

func (definition DataInputDefinition) effectiveSelection(selection []string) ([]string, error) {
	if len(selection) == 0 {
		selection = definition.Select
	}
	if len(selection) == 0 {
		return sortedFileRoleNames(definition.Files), nil
	}
	seen := make(map[string]struct{}, len(selection))
	effective := make([]string, 0, len(selection))
	for _, role := range selection {
		if err := validateDataName(role, "data select role"); err != nil {
			return nil, err
		}
		if _, duplicate := seen[role]; duplicate {
			return nil, fmt.Errorf("duplicate selected role %q", role)
		}
		if _, ok := definition.Files[role]; !ok {
			return nil, fmt.Errorf("selected role %q is not defined", role)
		}
		seen[role] = struct{}{}
		effective = append(effective, role)
	}
	return effective, nil
}

func (definition DataInputDefinition) archiveTemplate(selection []string) (*DataAssetArchiveTemplate, error) {
	if definition.Binding.Archive.Type == "" {
		return nil, nil
	}
	archive := DataAssetArchiveTemplate{
		Type:   definition.Binding.Archive.Type,
		Expose: definition.Binding.Archive.Expose,
	}
	for _, roleName := range selection {
		role := definition.Files[roleName]
		archive.Select = append(archive.Select, DataAssetArchiveSelectTemplate{
			MemberTemplate: canonicalAssetTemplate(role.Member),
			As:             canonicalAssetTemplate(role.As),
			Required:       role.Required,
		})
	}
	if err := archive.Validate(); err != nil {
		return nil, err
	}
	return &archive, nil
}

func (definition DataInputDefinition) effectiveProviderName(name string) string {
	if definition.Binding.ProviderName != "" {
		return definition.Binding.ProviderName
	}
	return name
}

func (binding DataInputBindingDefinition) cacheTemplate() DataAssetCacheTemplate {
	cacheKey := binding.Cache.CacheKeyTemplate
	if cacheKey == "" {
		cacheKey = binding.Cache.CacheKey
	}
	return DataAssetCacheTemplate{
		Strategy:         binding.Cache.Strategy,
		CacheKeyTemplate: canonicalAssetTemplate(cacheKey),
		Immutable:        binding.Cache.Immutable,
	}
}

func (binding DataInputBindingDefinition) materializationTemplate() DataAssetMaterializationTemplate {
	return DataAssetMaterializationTemplate{Strategy: binding.Materialization.Strategy}
}

func (provider *DataProviderTemplate) applyCanonicalLocation(location DataDefinitionLocation) error {
	switch provider.Provider {
	case DataProviderHTTP:
		provider.URLTemplate = firstNonEmpty(location.URLTemplate, location.URITemplate, location.URI)
	case DataProviderLocalFile, DataProviderRegisteredLocation:
		provider.Location = &DataLocationPathTemplate{
			Name:         firstNonEmpty(location.LocationName, location.Name),
			PathTemplate: canonicalAssetTemplate(firstNonEmpty(location.PathTemplate, location.Path)),
		}
	case DataProviderGDriveRclone:
		provider.GDrive = &GDriveRcloneTemplate{
			Remote:         location.Remote,
			PathTemplate:   canonicalAssetTemplate(firstNonEmpty(location.DrivePath, location.PathTemplate, location.Path)),
			FileIDTemplate: canonicalAssetTemplate(location.FileID),
		}
	default:
		return fmt.Errorf("unsupported data provider %q", provider.Provider)
	}
	return nil
}

func (binding DataOutputBindingDefinition) boundLocation(parameters map[string]any) (DataAssetLocation, error) {
	switch binding.Provider {
	case DataProviderRegisteredLocation, DataProviderLocalFile:
		location := DataAssetLocation{
			Type:         binding.Provider,
			LocationName: firstNonEmpty(binding.Location.LocationName, binding.Location.Name),
			Path:         renderCanonicalTemplate(firstNonEmpty(binding.Location.PathTemplate, binding.Location.Path), parameters),
		}
		return location, location.Validate()
	case DataProviderGDriveRclone:
		location := DataAssetLocation{
			Type:      DataProviderGDriveRclone,
			Remote:    binding.Location.Remote,
			DrivePath: renderCanonicalTemplate(firstNonEmpty(binding.Location.DrivePath, binding.Location.PathTemplate, binding.Location.Path), parameters),
			FileID:    renderCanonicalTemplate(binding.Location.FileID, parameters),
		}
		return location, location.Validate()
	default:
		return DataAssetLocation{}, fmt.Errorf("unsupported data output provider %q", binding.Provider)
	}
}

func canonicalAssetTemplate(value string) string {
	value = strings.ReplaceAll(value, "${asset.", "${")
	return strings.ReplaceAll(value, "${fanout.", "${")
}

func renderCanonicalTemplate(value string, parameters map[string]any) string {
	return renderTemplate(canonicalAssetTemplate(value), parameters)
}

func mergeMetadata(first map[string]any, second map[string]any) map[string]any {
	if len(first) == 0 && len(second) == 0 {
		return nil
	}
	merged := make(map[string]any, len(first)+len(second))
	for key, value := range first {
		merged[key] = value
	}
	for key, value := range second {
		merged[key] = value
	}
	return merged
}

func sortedInputDefinitionNames(values map[string]DataInputDefinition) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedOutputDefinitionNames(values map[string]DataOutputDefinition) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedFileRoleNames(values map[string]DataFileRoleDefinition) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

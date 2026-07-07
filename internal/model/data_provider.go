package model

import (
	"fmt"
	"strings"
)

const (
	DataProviderHTTP               = "http"
	DataProviderLocalFile          = "local_file"
	DataProviderRegisteredLocation = "registered_location"
	DataProviderGDriveRclone       = "gdrive_rclone"
)

type DataProviderTemplate struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Format   string `json:"format,omitempty"`
	Provider string `json:"provider"`

	URLTemplate string `json:"url_template,omitempty"`
	URITemplate string `json:"uri_template,omitempty"`

	Location *DataLocationPathTemplate `json:"location,omitempty"`

	GDrive *GDriveRcloneTemplate `json:"gdrive,omitempty"`

	Parameters      []string                         `json:"parameters,omitempty"`
	Integrity       DataAssetIntegrityTemplate       `json:"integrity,omitempty"`
	Cache           DataAssetCacheTemplate           `json:"cache,omitempty"`
	Archive         *DataAssetArchiveTemplate        `json:"archive,omitempty"`
	Materialization DataAssetMaterializationTemplate `json:"materialization,omitempty"`
	TransferPolicy  DataAssetTransferPolicy          `json:"transfer_policy,omitempty"`
	Metadata        map[string]any                   `json:"metadata,omitempty"`
}

type GDriveRcloneTemplate struct {
	Remote         string `json:"remote"`
	PathTemplate   string `json:"path_template"`
	FileIDTemplate string `json:"file_id_template,omitempty"`
}

func (provider DataProviderTemplate) Validate() error {
	if err := validateDataName(provider.Name, "data provider name"); err != nil {
		return err
	}
	if strings.TrimSpace(provider.Kind) == "" {
		return fmt.Errorf("data provider kind is required")
	}
	if !isSupportedDataProvider(provider.Provider) {
		return fmt.Errorf("unsupported data provider %q", provider.Provider)
	}
	declared, err := validateParameterNames(provider.Parameters)
	if err != nil {
		return err
	}
	if err := provider.validateProviderFields(); err != nil {
		return err
	}
	if err := provider.validateTemplateReferences(declared); err != nil {
		return err
	}
	if err := provider.Integrity.Validate(); err != nil {
		return err
	}
	if err := provider.Cache.Validate(); err != nil {
		return err
	}
	if provider.Archive != nil {
		if err := provider.Archive.Validate(); err != nil {
			return err
		}
	}
	if err := provider.Materialization.Validate(); err != nil {
		return err
	}
	return provider.TransferPolicy.Validate()
}

func (provider DataProviderTemplate) Bind(bindingName string, parameters map[string]any) (BoundDataAsset, error) {
	if err := provider.Validate(); err != nil {
		return BoundDataAsset{}, err
	}
	if err := validateDataName(bindingName, "bound data asset binding_name"); err != nil {
		return BoundDataAsset{}, err
	}
	for _, name := range provider.Parameters {
		value, ok := parameters[name]
		if !ok || value == nil {
			return BoundDataAsset{}, fmt.Errorf("required parameter %q is missing", name)
		}
	}

	renderedLocation, err := provider.boundLocation(parameters)
	if err != nil {
		return BoundDataAsset{}, err
	}
	archive, err := provider.boundArchive(parameters)
	if err != nil {
		return BoundDataAsset{}, err
	}
	asset := BoundDataAsset{
		BindingName:  bindingName,
		ProviderName: provider.Name,
		Kind:         provider.Kind,
		Format:       provider.Format,
		Provider:     provider.Provider,
		Location:     renderedLocation,
		Integrity: DataAssetIntegrity{
			SHA256:    renderTemplate(provider.Integrity.SHA256Template, parameters),
			SizeBytes: provider.Integrity.SizeBytes,
			Required:  provider.Integrity.Required,
		},
		Cache: DataAssetCache{
			Strategy:  provider.Cache.Strategy,
			CacheKey:  renderTemplate(provider.Cache.CacheKeyTemplate, parameters),
			Immutable: provider.Cache.Immutable,
		},
		Archive: archive,
		Materialization: DataAssetMaterialization{
			Strategy: provider.Materialization.Strategy,
		},
		TransferPolicy: provider.TransferPolicy,
		Parameters:     parameters,
		Metadata:       provider.Metadata,
	}
	if err := asset.Validate(); err != nil {
		return BoundDataAsset{}, err
	}
	return asset, nil
}

func (provider DataProviderTemplate) validateProviderFields() error {
	switch provider.Provider {
	case DataProviderHTTP:
		if provider.URLTemplate == "" && provider.URITemplate == "" {
			return fmt.Errorf("http data provider url_template or uri_template is required")
		}
		if provider.URLTemplate != "" && !isHTTPURI(provider.URLTemplate) {
			return fmt.Errorf("http data provider url_template must use http or https")
		}
		if provider.URITemplate != "" && !isHTTPURI(provider.URITemplate) {
			return fmt.Errorf("http data provider uri_template must use http or https")
		}
	case DataProviderLocalFile, DataProviderRegisteredLocation:
		if provider.Location == nil {
			return fmt.Errorf("%s data provider location is required", provider.Provider)
		}
		if err := provider.Location.Validate(); err != nil {
			return err
		}
	case DataProviderGDriveRclone:
		if provider.GDrive == nil {
			return fmt.Errorf("gdrive_rclone data provider gdrive is required")
		}
		if err := provider.GDrive.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (provider DataProviderTemplate) validateTemplateReferences(declared map[string]struct{}) error {
	templates := []struct {
		field string
		value string
	}{
		{field: "url_template", value: provider.URLTemplate},
		{field: "uri_template", value: provider.URITemplate},
		{field: "integrity.sha256", value: provider.Integrity.SHA256Template},
		{field: "cache.cache_key_template", value: provider.Cache.CacheKeyTemplate},
	}
	if provider.Location != nil {
		templates = append(templates, struct {
			field string
			value string
		}{field: "location.path_template", value: provider.Location.PathTemplate})
	}
	if provider.GDrive != nil {
		templates = append(templates,
			struct {
				field string
				value string
			}{field: "gdrive.path_template", value: provider.GDrive.PathTemplate},
			struct {
				field string
				value string
			}{field: "gdrive.file_id_template", value: provider.GDrive.FileIDTemplate},
		)
	}
	if provider.Archive != nil {
		for i, selector := range provider.Archive.Select {
			templates = append(templates, struct {
				field string
				value string
			}{field: fmt.Sprintf("archive.select[%d].member_template", i), value: selector.MemberTemplate})
			templates = append(templates, struct {
				field string
				value string
			}{field: fmt.Sprintf("archive.select[%d].as", i), value: selector.As})
		}
	}
	for _, template := range templates {
		for _, name := range templateParameterNames(template.value) {
			if _, ok := declared[name]; !ok {
				return fmt.Errorf("%s references undeclared parameter %q", template.field, name)
			}
		}
	}
	return nil
}

func (provider DataProviderTemplate) boundLocation(parameters map[string]any) (DataAssetLocation, error) {
	switch provider.Provider {
	case DataProviderHTTP:
		uri := provider.URLTemplate
		if uri == "" {
			uri = provider.URITemplate
		}
		return DataAssetLocation{Type: DataProviderHTTP, URI: renderTemplate(uri, parameters)}, nil
	case DataProviderLocalFile, DataProviderRegisteredLocation:
		return DataAssetLocation{
			Type:         provider.Provider,
			LocationName: provider.Location.Name,
			Path:         renderTemplate(provider.Location.PathTemplate, parameters),
		}, nil
	case DataProviderGDriveRclone:
		return DataAssetLocation{
			Type:      DataProviderGDriveRclone,
			Remote:    provider.GDrive.Remote,
			DrivePath: renderTemplate(provider.GDrive.PathTemplate, parameters),
			FileID:    renderTemplate(provider.GDrive.FileIDTemplate, parameters),
		}, nil
	default:
		return DataAssetLocation{}, fmt.Errorf("unsupported data provider %q", provider.Provider)
	}
}

func (provider DataProviderTemplate) boundArchive(parameters map[string]any) (*DataAssetArchive, error) {
	if provider.Archive == nil {
		return nil, nil
	}
	archive := DataAssetArchive{
		Type:   provider.Archive.Type,
		Expose: provider.Archive.Expose,
	}
	for _, selector := range provider.Archive.Select {
		archive.Select = append(archive.Select, DataAssetArchiveSelect{
			Member:   renderTemplate(selector.MemberTemplate, parameters),
			As:       renderTemplate(selector.As, parameters),
			Required: selector.Required,
		})
	}
	if err := archive.Validate(); err != nil {
		return nil, err
	}
	return &archive, nil
}

func (template GDriveRcloneTemplate) Validate() error {
	if err := validateRcloneRemote(template.Remote); err != nil {
		return err
	}
	if _, err := validateDataRelativePath(template.PathTemplate, "gdrive path_template"); err != nil {
		return err
	}
	if strings.TrimSpace(template.FileIDTemplate) != template.FileIDTemplate {
		return fmt.Errorf("gdrive file_id_template must not contain leading or trailing whitespace")
	}
	return nil
}

func isSupportedDataProvider(provider string) bool {
	switch provider {
	case DataProviderHTTP, DataProviderLocalFile, DataProviderRegisteredLocation, DataProviderGDriveRclone:
		return true
	default:
		return false
	}
}

func isHTTPURI(value string) bool {
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
}

func validateRcloneRemote(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("gdrive remote is required")
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("gdrive remote must not contain leading or trailing whitespace")
	}
	if strings.ContainsAny(value, `\/:`) {
		return fmt.Errorf("gdrive remote must be a remote name, not a path")
	}
	return nil
}

func validateParameterNames(parameters []string) (map[string]struct{}, error) {
	seen := make(map[string]struct{}, len(parameters))
	for _, name := range parameters {
		if err := validateDataName(name, "data provider parameter"); err != nil {
			return nil, err
		}
		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("duplicate data provider parameter %q", name)
		}
		seen[name] = struct{}{}
	}
	return seen, nil
}

func templateParameterNames(value string) []string {
	var names []string
	remaining := value
	for {
		start := strings.Index(remaining, "${")
		if start < 0 {
			return names
		}
		remaining = remaining[start+2:]
		end := strings.Index(remaining, "}")
		if end < 0 {
			names = append(names, "")
			return names
		}
		names = append(names, remaining[:end])
		remaining = remaining[end+1:]
	}
}

func renderTemplate(value string, parameters map[string]any) string {
	rendered := value
	for name, parameterValue := range parameters {
		rendered = strings.ReplaceAll(rendered, "${"+name+"}", fmt.Sprint(parameterValue))
	}
	return rendered
}

package workflow

import (
	"fmt"
	"strings"

	"goetl/internal/document"
	"goetl/internal/model"
	"goetl/internal/variable"
)

type ExplicitArchiveOperationTemplate struct {
	Extract *ExplicitArchiveExtractTemplate
	Create  *ExplicitArchiveCreateTemplate
}

type ExplicitArchiveExtractTemplate struct {
	ArchiveType string
	Source      ArchiveExtractSourceTemplate
	Members     []ArchiveExtractMemberTemplate
	OutputPath  string
}

type ArchiveExtractSourceTemplate struct {
	MaterializedAsset *ArchiveMaterializedAssetSourceTemplate
	LocalPath         string
}

type ArchiveMaterializedAssetSourceTemplate struct {
	Step    string
	Binding string
}

type ArchiveExtractMemberTemplate struct {
	Member   string
	As       string
	Required bool
}

type ExplicitArchiveCreateTemplate struct {
	ArchiveType string
	Entries     []ArchiveCreateEntryTemplate
	OutputPath  string
}

type ArchiveCreateEntryTemplate struct {
	From ArchiveCreateEntrySourceTemplate
	As   string
}

type ArchiveCreateEntrySourceTemplate struct {
	Artifact  *ArchiveArtifactSourceTemplate
	LocalPath string
}

type ArchiveArtifactSourceTemplate struct {
	Step string
	Name string
}

func explicitArchiveOperationFromCanonical(step document.CanonicalWorkflowStep) (*ExplicitArchiveOperationTemplate, error) {
	rawArchive, hasArchive := step.Data["archive"]
	workType := model.WorkItemType(step.Work.Type)
	if !hasArchive {
		switch workType {
		case model.WorkItemTypeArchiveExtract:
			return nil, fmt.Errorf("%s step requires data.archive.extract", model.WorkItemTypeArchiveExtract)
		case model.WorkItemTypeArchiveCreate:
			return nil, fmt.Errorf("%s step requires data.archive.create", model.WorkItemTypeArchiveCreate)
		default:
			return nil, nil
		}
	}
	archive, ok := rawArchive.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("data.archive must be an object")
	}
	rawExtract, hasExtract := archive["extract"]
	rawCreate, hasCreate := archive["create"]
	if hasExtract && hasCreate {
		return nil, fmt.Errorf("data.archive must contain exactly one of extract or create")
	}
	if hasExtract {
		if workType != model.WorkItemTypeArchiveExtract {
			return nil, fmt.Errorf("data.archive.extract requires work.type %q", model.WorkItemTypeArchiveExtract)
		}
		extract, err := explicitArchiveExtractFromCanonical(rawExtract)
		if err != nil {
			return nil, err
		}
		return &ExplicitArchiveOperationTemplate{Extract: extract}, nil
	}
	if hasCreate {
		if workType != model.WorkItemTypeArchiveCreate {
			return nil, fmt.Errorf("data.archive.create requires work.type %q", model.WorkItemTypeArchiveCreate)
		}
		create, err := explicitArchiveCreateFromCanonical(rawCreate)
		if err != nil {
			return nil, err
		}
		return &ExplicitArchiveOperationTemplate{Create: create}, nil
	}
	return nil, fmt.Errorf("data.archive must contain extract or create")
}

func explicitArchiveExtractFromCanonical(raw any) (*ExplicitArchiveExtractTemplate, error) {
	fields, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("data.archive.extract must be an object")
	}
	archiveType, err := canonicalStringField(fields, "type", "data.archive.extract")
	if err != nil {
		return nil, err
	}
	sourceFields, err := requiredCanonicalObjectField(fields, "source", "data.archive.extract")
	if err != nil {
		return nil, err
	}
	source, err := archiveExtractSourceFromCanonical(sourceFields)
	if err != nil {
		return nil, err
	}
	members, err := archiveExtractMembersFromCanonical(fields)
	if err != nil {
		return nil, err
	}
	outputFields, err := requiredCanonicalObjectField(fields, "output", "data.archive.extract")
	if err != nil {
		return nil, err
	}
	outputPath, err := canonicalStringField(outputFields, "path", "data.archive.extract.output")
	if err != nil {
		return nil, err
	}
	return &ExplicitArchiveExtractTemplate{
		ArchiveType: archiveType,
		Source:      source,
		Members:     members,
		OutputPath:  outputPath,
	}, nil
}

func archiveExtractSourceFromCanonical(fields map[string]any) (ArchiveExtractSourceTemplate, error) {
	var source ArchiveExtractSourceTemplate
	if rawMaterialized, ok := fields["materialized_asset"]; ok {
		materializedFields, ok := rawMaterialized.(map[string]any)
		if !ok {
			return source, fmt.Errorf("data.archive.extract.source.materialized_asset must be an object")
		}
		step, err := canonicalStringField(materializedFields, "step", "data.archive.extract.source.materialized_asset")
		if err != nil {
			return source, err
		}
		binding, err := canonicalStringField(materializedFields, "binding", "data.archive.extract.source.materialized_asset")
		if err != nil {
			return source, err
		}
		source.MaterializedAsset = &ArchiveMaterializedAssetSourceTemplate{Step: step, Binding: binding}
	}
	if rawLocalPath, ok := fields["local_path"]; ok {
		text, ok := rawLocalPath.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return source, fmt.Errorf("data.archive.extract.source local_path must be a non-empty string")
		}
		source.LocalPath = text
	}
	return source, nil
}

func archiveExtractMembersFromCanonical(fields map[string]any) ([]ArchiveExtractMemberTemplate, error) {
	raw, ok := fields["members"]
	if !ok {
		return nil, fmt.Errorf("data.archive.extract members is required")
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("data.archive.extract members must be a list")
	}
	members := make([]ArchiveExtractMemberTemplate, 0, len(items))
	for index, item := range items {
		fields, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("data.archive.extract members[%d] must be an object", index)
		}
		context := fmt.Sprintf("data.archive.extract.members[%d]", index)
		member, err := canonicalStringField(fields, "member", context)
		if err != nil {
			return nil, err
		}
		as, err := canonicalStringField(fields, "as", context)
		if err != nil {
			return nil, err
		}
		required, err := optionalCanonicalBoolField(fields, "required", context)
		if err != nil {
			return nil, err
		}
		members = append(members, ArchiveExtractMemberTemplate{Member: member, As: as, Required: required})
	}
	return members, nil
}

func explicitArchiveCreateFromCanonical(raw any) (*ExplicitArchiveCreateTemplate, error) {
	fields, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("data.archive.create must be an object")
	}
	archiveType, err := canonicalStringField(fields, "type", "data.archive.create")
	if err != nil {
		return nil, err
	}
	entries, err := archiveCreateEntriesFromCanonical(fields)
	if err != nil {
		return nil, err
	}
	outputFields, err := requiredCanonicalObjectField(fields, "output", "data.archive.create")
	if err != nil {
		return nil, err
	}
	outputPath, err := canonicalStringField(outputFields, "path", "data.archive.create.output")
	if err != nil {
		return nil, err
	}
	return &ExplicitArchiveCreateTemplate{ArchiveType: archiveType, Entries: entries, OutputPath: outputPath}, nil
}

func archiveCreateEntriesFromCanonical(fields map[string]any) ([]ArchiveCreateEntryTemplate, error) {
	raw, ok := fields["entries"]
	if !ok {
		return nil, fmt.Errorf("data.archive.create entries is required")
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("data.archive.create entries must be a list")
	}
	entries := make([]ArchiveCreateEntryTemplate, 0, len(items))
	for index, item := range items {
		fields, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("data.archive.create entries[%d] must be an object", index)
		}
		context := fmt.Sprintf("data.archive.create.entries[%d]", index)
		fromFields, err := requiredCanonicalObjectField(fields, "from", context)
		if err != nil {
			return nil, err
		}
		from, err := archiveCreateEntrySourceFromCanonical(fromFields)
		if err != nil {
			return nil, err
		}
		as, err := canonicalStringField(fields, "as", context)
		if err != nil {
			return nil, err
		}
		entries = append(entries, ArchiveCreateEntryTemplate{From: from, As: as})
	}
	return entries, nil
}

func archiveCreateEntrySourceFromCanonical(fields map[string]any) (ArchiveCreateEntrySourceTemplate, error) {
	var source ArchiveCreateEntrySourceTemplate
	if rawArtifact, ok := fields["artifact"]; ok {
		artifactFields, ok := rawArtifact.(map[string]any)
		if !ok {
			return source, fmt.Errorf("data.archive.create.entries.from.artifact must be an object")
		}
		step, err := canonicalStringField(artifactFields, "step", "data.archive.create.entries.from.artifact")
		if err != nil {
			return source, err
		}
		name, err := canonicalStringField(artifactFields, "name", "data.archive.create.entries.from.artifact")
		if err != nil {
			return source, err
		}
		source.Artifact = &ArchiveArtifactSourceTemplate{Step: step, Name: name}
	}
	if rawLocalPath, ok := fields["local_path"]; ok {
		text, ok := rawLocalPath.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return source, fmt.Errorf("data.archive.create.entries.from local_path must be a non-empty string")
		}
		source.LocalPath = text
	}
	return source, nil
}

func compileStandaloneExplicitArchiveOperationWorkItems(
	resolver variable.Resolver,
	template FanOutWorkItemTemplate,
) ([]CompiledFanOutWorkItem, error) {
	if len(template.ParameterAccessors) > 0 {
		return nil, fmt.Errorf("parameter_accessors require fan_out")
	}
	item := model.WorkItem{
		ID:             template.IDPrefix,
		Type:           template.Type,
		OutputFilename: template.OutputPrefix + template.OutputExtension,
		Parameters:     copyParameters(template.Parameters),
	}
	if len(template.ParameterExpressions) > 0 && item.Parameters == nil {
		item.Parameters = model.Parameters{}
	}
	context := FanOutItemContext{}
	if err := bindParameterExpressions(resolver, context, item.Parameters, template.ParameterExpressions); err != nil {
		return nil, fmt.Errorf("parameters: %w", err)
	}
	if err := rejectLegacyHiddenPlannerParameters(item.Parameters); err != nil {
		return nil, fmt.Errorf("parameters: %w", err)
	}
	constraints, err := resolveResourceConstraintDeclarations(resolver, context, item.ID, template.ResourceConstraints)
	if err != nil {
		return nil, fmt.Errorf("resource constraints: %w", err)
	}
	if err := compileExplicitArchiveOperationWorkItem(resolver, context, "", &item, template.ExplicitArchiveOperation, constraints); err != nil {
		return nil, fmt.Errorf("explicit archive operation: %w", err)
	}
	if err := item.ValidateForWorkflowCompile(); err != nil {
		return nil, err
	}
	return []CompiledFanOutWorkItem{{WorkItem: item, ResourceConstraints: constraints}}, nil
}

func compileExplicitArchiveOperationWorkItem(
	resolver variable.Resolver,
	context FanOutItemContext,
	idToken string,
	item *model.WorkItem,
	template *ExplicitArchiveOperationTemplate,
	constraints []model.WorkItemResourceConstraint,
) error {
	if template == nil {
		return nil
	}
	if template.Extract != nil {
		return compileExplicitArchiveExtractWorkItem(resolver, context, idToken, item, template.Extract, constraints)
	}
	if template.Create != nil {
		return compileExplicitArchiveCreateWorkItem(resolver, context, idToken, item, template.Create, constraints)
	}
	return fmt.Errorf("archive operation template is empty")
}

func compileExplicitArchiveExtractWorkItem(
	resolver variable.Resolver,
	context FanOutItemContext,
	idToken string,
	item *model.WorkItem,
	template *ExplicitArchiveExtractTemplate,
	constraints []model.WorkItemResourceConstraint,
) error {
	if item.Type != model.WorkItemTypeArchiveExtract {
		return fmt.Errorf("explicit archive extract requires work item type %q", model.WorkItemTypeArchiveExtract)
	}
	if len(item.Parameters) > 0 {
		return fmt.Errorf("%s step does not accept work parameters", model.WorkItemTypeArchiveExtract)
	}
	payload := model.ArchiveExtractWorkItemPayload{
		Operator:            string(model.WorkItemTypeArchiveExtract),
		ArchiveType:         template.ArchiveType,
		ResourceConstraints: append([]model.WorkItemResourceConstraint(nil), constraints...),
	}
	source, dependency, err := compileArchiveExtractSource(resolver, context, idToken, template.Source)
	if err != nil {
		return err
	}
	payload.Source = source
	if dependency != "" {
		item.DependsOn = appendUniqueString(item.DependsOn, dependency)
	}
	for index, member := range template.Members {
		compiled, err := compileArchiveExtractMember(resolver, context, member)
		if err != nil {
			return fmt.Errorf("member %d: %w", index, err)
		}
		payload.Members = append(payload.Members, compiled)
	}
	outputPath, err := renderArchiveOperationTemplate(resolver, context, template.OutputPath, "output.path")
	if err != nil {
		return err
	}
	payload.OutputPath = outputPath
	if err := payload.Validate(); err != nil {
		return err
	}
	item.Parameters = model.Parameters{"archive_extract": {Type: "archive_extract", Value: payload}}
	return nil
}

func compileArchiveExtractSource(
	resolver variable.Resolver,
	context FanOutItemContext,
	idToken string,
	template ArchiveExtractSourceTemplate,
) (model.ArchiveExtractSource, string, error) {
	if template.MaterializedAsset != nil {
		binding, err := renderArchiveOperationTemplate(resolver, context, template.MaterializedAsset.Binding, "source.materialized_asset.binding")
		if err != nil {
			return model.ArchiveExtractSource{}, "", err
		}
		workItemID := archiveSourceWorkItemID(template.MaterializedAsset.Step, idToken)
		return model.ArchiveExtractSource{
			MaterializedAsset: &model.ArchiveMaterializedAssetSource{
				FromWorkItemID: workItemID,
				BindingName:    binding,
			},
		}, workItemID, nil
	}
	if template.LocalPath != "" {
		path, err := renderArchiveOperationTemplate(resolver, context, template.LocalPath, "source.local_path")
		if err != nil {
			return model.ArchiveExtractSource{}, "", err
		}
		return model.ArchiveExtractSource{LocalPath: path}, "", nil
	}
	return model.ArchiveExtractSource{}, "", nil
}

func compileArchiveExtractMember(
	resolver variable.Resolver,
	context FanOutItemContext,
	template ArchiveExtractMemberTemplate,
) (model.ArchiveExtractMember, error) {
	member, err := renderArchiveOperationTemplate(resolver, context, template.Member, "member")
	if err != nil {
		return model.ArchiveExtractMember{}, err
	}
	as, err := renderArchiveOperationTemplate(resolver, context, template.As, "as")
	if err != nil {
		return model.ArchiveExtractMember{}, err
	}
	return model.ArchiveExtractMember{Member: member, As: as, Required: template.Required}, nil
}

func compileExplicitArchiveCreateWorkItem(
	resolver variable.Resolver,
	context FanOutItemContext,
	idToken string,
	item *model.WorkItem,
	template *ExplicitArchiveCreateTemplate,
	constraints []model.WorkItemResourceConstraint,
) error {
	if item.Type != model.WorkItemTypeArchiveCreate {
		return fmt.Errorf("explicit archive create requires work item type %q", model.WorkItemTypeArchiveCreate)
	}
	if len(item.Parameters) > 0 {
		return fmt.Errorf("%s step does not accept work parameters", model.WorkItemTypeArchiveCreate)
	}
	payload := model.ArchiveCreateWorkItemPayload{
		Operator:            string(model.WorkItemTypeArchiveCreate),
		ArchiveType:         template.ArchiveType,
		ResourceConstraints: append([]model.WorkItemResourceConstraint(nil), constraints...),
	}
	for index, entry := range template.Entries {
		compiled, dependency, err := compileArchiveCreateEntry(resolver, context, idToken, entry)
		if err != nil {
			return fmt.Errorf("entry %d: %w", index, err)
		}
		if dependency != "" {
			item.DependsOn = appendUniqueString(item.DependsOn, dependency)
		}
		payload.Entries = append(payload.Entries, compiled)
	}
	outputPath, err := renderArchiveOperationTemplate(resolver, context, template.OutputPath, "output.path")
	if err != nil {
		return err
	}
	payload.OutputPath = outputPath
	if err := payload.Validate(); err != nil {
		return err
	}
	item.Parameters = model.Parameters{"archive_create": {Type: "archive_create", Value: payload}}
	return nil
}

func compileArchiveCreateEntry(
	resolver variable.Resolver,
	context FanOutItemContext,
	idToken string,
	template ArchiveCreateEntryTemplate,
) (model.ArchiveCreateEntry, string, error) {
	source, dependency, err := compileArchiveCreateEntrySource(resolver, context, idToken, template.From)
	if err != nil {
		return model.ArchiveCreateEntry{}, "", err
	}
	as, err := renderArchiveOperationTemplate(resolver, context, template.As, "as")
	if err != nil {
		return model.ArchiveCreateEntry{}, "", err
	}
	return model.ArchiveCreateEntry{From: source, As: as}, dependency, nil
}

func compileArchiveCreateEntrySource(
	resolver variable.Resolver,
	context FanOutItemContext,
	idToken string,
	template ArchiveCreateEntrySourceTemplate,
) (model.ArchiveCreateEntrySource, string, error) {
	if template.Artifact != nil {
		name, err := renderArchiveOperationTemplate(resolver, context, template.Artifact.Name, "from.artifact.name")
		if err != nil {
			return model.ArchiveCreateEntrySource{}, "", err
		}
		workItemID := archiveSourceWorkItemID(template.Artifact.Step, idToken)
		return model.ArchiveCreateEntrySource{
			Artifact: &model.ArchiveArtifactSource{
				FromWorkItemID: workItemID,
				Name:           name,
			},
		}, workItemID, nil
	}
	if template.LocalPath != "" {
		path, err := renderArchiveOperationTemplate(resolver, context, template.LocalPath, "from.local_path")
		if err != nil {
			return model.ArchiveCreateEntrySource{}, "", err
		}
		return model.ArchiveCreateEntrySource{LocalPath: path}, "", nil
	}
	return model.ArchiveCreateEntrySource{}, "", nil
}

func renderArchiveOperationTemplate(resolver variable.Resolver, context FanOutItemContext, value string, field string) (string, error) {
	if !strings.Contains(value, "${") {
		return value, nil
	}
	rendered, err := renderFanOutTemplate(resolver, context, value, false)
	if err != nil {
		return "", fmt.Errorf("%s: %w", field, err)
	}
	return rendered, nil
}

func archiveSourceWorkItemID(step string, idToken string) string {
	if idToken == "" {
		return step
	}
	return step + "-" + idToken
}

func optionalCanonicalBoolField(fields map[string]any, name string, context string) (bool, error) {
	value, ok := fields[name]
	if !ok {
		return false, nil
	}
	boolean, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("%s %s must be a boolean", context, name)
	}
	return boolean, nil
}

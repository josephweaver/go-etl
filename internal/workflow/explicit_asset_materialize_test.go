package workflow

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"goetl/internal/document"
	"goetl/internal/model"
	"goetl/internal/variable"
)

func TestCanonicalExplicitAssetMaterializeStepCompilesVisibleCacheWork(t *testing.T) {
	doc, err := document.DecodeCanonicalWorkflowSource([]byte(`{
		"api_version": "goet/v1alpha1",
		"kind": "Workflow",
		"id": "yan-roy-header-cache",
		"variables": {
			"tiles": ["h18v07"]
		},
		"data": {
			"inputs": {
				"yan_roy_field_segments": {
					"kind": "envi_field_segments",
					"parameters": {
						"tile": {"type": "string"}
					},
					"files": {
						"raster": {
							"member": "${asset.tile}/WELD_${asset.tile}_2010_field_segments",
							"as": "WELD_${asset.tile}_2010_field_segments",
							"required": true
						},
						"header": {
							"member": "${asset.tile}/WELD_${asset.tile}_2010_field_segments.hdr",
							"as": "WELD_${asset.tile}_2010_field_segments.hdr",
							"required": true
						}
					},
					"select": ["raster", "header"],
					"binding": {
						"provider_name": "gdrive_release_data",
						"provider": "gdrive_rclone",
						"location": {
							"remote": "gdrive",
							"drive_path": "Data/Field_Boundaries/ReleaseData.7z"
						},
						"archive": {
							"type": "seven_zip",
							"expose": "selected_directory"
						},
						"cache": {
							"strategy": "worker_cache",
							"cache_key": "gdrive/field_boundaries/release-data/source.7z",
							"immutable": true
						},
						"materialization": {
							"scope": "shared",
							"strategy": "worker_cache"
						}
					}
				}
			}
		},
		"steps": [
			{
				"id": "cache-field-segment-headers",
				"fan_out": {
					"over": "${workflow.tiles[*]}",
					"as": "tile",
					"id": "${fanout}"
				},
				"data": {
					"materialize": {
						"field_segments": {
							"asset": "yan_roy_field_segments",
							"select": ["header"],
							"with": {
								"tile": "${fanout}"
							}
						}
					}
				},
				"work": {
					"type": "asset.materialize",
					"parameters": {
						"target_environment_id": "msu-hpcc"
					}
				}
			}
		]
	}`), document.DecodeOptions{Path: "workflow.json"})
	if err != nil {
		t.Fatalf("DecodeCanonicalWorkflowSource() error = %v", err)
	}
	workflow, err := WorkflowFromCanonicalDocument(doc)
	if err != nil {
		t.Fatalf("WorkflowFromCanonicalDocument() error = %v", err)
	}
	scope, err := variable.NewScope(workflow.Variables...)
	if err != nil {
		t.Fatalf("NewScope() error = %v", err)
	}
	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})
	plan, err := NormalizeStages(workflow)
	if err != nil {
		t.Fatalf("NormalizeStages() error = %v", err)
	}

	stage, err := CompileWorkflowStage(resolver, workflow, plan, 0)
	if err != nil {
		t.Fatalf("CompileWorkflowStage() error = %v", err)
	}

	if len(stage.WorkItems) != 1 {
		t.Fatalf("work item count = %d, want explicit asset_materialize only", len(stage.WorkItems))
	}
	item := stage.WorkItems[0]
	if item.WorkItem.Type != model.WorkItemTypeAssetMaterialize {
		t.Fatalf("work item type = %q, want asset_materialize", item.WorkItem.Type)
	}
	if item.WorkItem.ID != "cache-field-segment-headers-h18v07" {
		t.Fatalf("work item id = %q", item.WorkItem.ID)
	}
	if len(item.WorkItem.DependsOn) != 0 {
		t.Fatalf("depends_on = %+v, want no hidden dependency", item.WorkItem.DependsOn)
	}
	if len(item.ResourceConstraints) != 1 || item.ResourceConstraints[0].ResourceKey != "provider:gdrive-rclone:gdrive/download" {
		t.Fatalf("resource constraints = %+v", item.ResourceConstraints)
	}

	payload := decodeAssetMaterializePayload(t, item)
	if payload.TargetEnvironmentID != "msu-hpcc" {
		t.Fatalf("target_environment_id = %q", payload.TargetEnvironmentID)
	}
	if payload.BindingName != "field_segments" {
		t.Fatalf("binding name = %q", payload.BindingName)
	}
	if payload.ProviderName != "gdrive_release_data" || payload.ProviderType != model.DataProviderGDriveRclone {
		t.Fatalf("payload provider = %s/%s", payload.ProviderName, payload.ProviderType)
	}
	if payload.AssetKey == "" || !strings.HasPrefix(payload.AssetKey, "sha256:") {
		t.Fatalf("asset key = %q", payload.AssetKey)
	}

	assets, err := boundDataAssetsFromParameters(item.WorkItem.Parameters)
	if err != nil {
		t.Fatalf("boundDataAssetsFromParameters() error = %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("bound asset count = %d", len(assets))
	}
	if len(assets[0].Archive.Select) != 1 {
		t.Fatalf("archive select count = %d", len(assets[0].Archive.Select))
	}
	if got := assets[0].Archive.Select[0].Member; got != "h18v07/WELD_h18v07_2010_field_segments.hdr" {
		t.Fatalf("archive member = %q", got)
	}
}

func TestCanonicalExplicitCollectionAssetMaterializeCompilesMembers(t *testing.T) {
	items := compileCanonicalCollectionMaterializeItems(t, canonicalCollectionMaterializeJSON(), document.SourceFormatJSON)
	if len(items) != 16 {
		t.Fatalf("work item count = %d, want 16", len(items))
	}

	fingerprint := ""
	for index, item := range items {
		year := 2008 + index
		wantID := "materialize-cdl--year-" + stringInt(year)
		if item.WorkItem.ID != wantID {
			t.Fatalf("item %d id = %q, want %q", index, item.WorkItem.ID, wantID)
		}
		if item.WorkItemIndex != index {
			t.Fatalf("item %d work item index = %d", index, item.WorkItemIndex)
		}
		payload := decodeAssetMaterializePayload(t, item)
		if payload.MaterializationDomainID != "msu-hpcc" {
			t.Fatalf("item %d materialization domain = %q", index, payload.MaterializationDomainID)
		}
		if payload.DestinationRelativePath != "cdl/midwest/"+stringInt(year)+".tif" {
			t.Fatalf("item %d destination = %q", index, payload.DestinationRelativePath)
		}
		if !strings.HasPrefix(payload.AssetKey, "sha256:") || !strings.HasPrefix(payload.MaterializationKey, "sha256:") {
			t.Fatalf("item %d keys asset=%q materialization=%q", index, payload.AssetKey, payload.MaterializationKey)
		}
		if payload.ResolvedLocation.URI != "https://example.invalid/cdl/midwest/"+stringInt(year)+".zip" {
			t.Fatalf("item %d uri = %q", index, payload.ResolvedLocation.URI)
		}
		if payload.Cache.CacheKey != "cdl/midwest/"+stringInt(year)+".zip" {
			t.Fatalf("item %d cache key = %q", index, payload.Cache.CacheKey)
		}
		if payload.TransferLimits.MaxBytesPerSecond != 1024 {
			t.Fatalf("item %d transfer limit = %d", index, payload.TransferLimits.MaxBytesPerSecond)
		}
		if len(item.ResourceConstraints) != 1 || item.ResourceConstraints[0].TargetUnits != 2 {
			t.Fatalf("item %d resource constraints = %+v", index, item.ResourceConstraints)
		}
		if payload.CollectionMember == nil {
			t.Fatalf("item %d missing collection member", index)
		}
		member := payload.CollectionMember
		if member.MemberIndex != index || member.MemberCount != 16 {
			t.Fatalf("item %d member metadata = %+v", index, member)
		}
		if !reflect.DeepEqual(member.DimensionOrder, []string{"year"}) || member.MemberBindings["year"] != year {
			t.Fatalf("item %d member bindings = %+v", index, member)
		}
		if fingerprint == "" {
			fingerprint = member.CollectionFingerprint
		}
		if member.CollectionFingerprint != fingerprint {
			t.Fatalf("item %d fingerprint = %q, want %q", index, member.CollectionFingerprint, fingerprint)
		}
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		if strings.Contains(string(data), "${asset.year}") {
			t.Fatalf("item %d payload contains unresolved asset placeholder: %s", index, data)
		}
	}
}

func TestExplicitScalarAssetMaterializeWithoutFanOutCompilesOneItem(t *testing.T) {
	workflow := Workflow{
		ID: "scalar-materialize",
		Steps: []Step{
			{
				ID: "materialize-one",
				FanOut: &FanOutStep{
					WorkItem: FanOutWorkItemTemplate{
						Type:            model.WorkItemTypeAssetMaterialize,
						IDPrefix:        "materialize-one",
						OutputPrefix:    "materialize-one",
						OutputExtension: ".json",
						Parameters: model.Parameters{
							"target_environment_id": {Type: "string", Value: "target-local"},
						},
						ExplicitAssetMaterialize: &ExplicitAssetMaterializeTemplate{
							Definitions: model.DataDefinitions{
								Inputs: map[string]model.DataInputDefinition{
									"asset": explicitCacheDefinitionForTest(),
								},
							},
							Alias: "input",
							Asset: "asset",
							With: map[string]variable.TypedExpression{
								"tile": {Type: variable.TypeString, Expression: "h18v07"},
							},
						},
					},
				},
			},
		},
	}
	plan, err := NormalizeStages(workflow)
	if err != nil {
		t.Fatalf("NormalizeStages() error = %v", err)
	}
	stage, err := CompileWorkflowStage(variable.NewResolver(variable.NewSet(), variable.ResolverConfig{}), workflow, plan, 0)
	if err != nil {
		t.Fatalf("CompileWorkflowStage() error = %v", err)
	}
	if len(stage.WorkItems) != 1 {
		t.Fatalf("work item count = %d, want 1", len(stage.WorkItems))
	}
	item := stage.WorkItems[0]
	if item.WorkItem.ID != "materialize-one" || item.WorkItem.OutputFilename != "materialize-one.json" {
		t.Fatalf("item identity = %s/%s", item.WorkItem.ID, item.WorkItem.OutputFilename)
	}
	payload := decodeAssetMaterializePayload(t, item)
	if payload.CollectionMember != nil {
		t.Fatalf("scalar payload collection_member = %+v", payload.CollectionMember)
	}
}

func TestCanonicalExplicitCollectionAssetMaterializeJSONAndYAMLEquivalent(t *testing.T) {
	jsonItems := compileCanonicalCollectionMaterializeItems(t, canonicalCollectionMaterializeJSON(), document.SourceFormatJSON)
	yamlItems := compileCanonicalCollectionMaterializeItems(t, canonicalCollectionMaterializeYAML(), document.SourceFormatYAML)

	if got, want := collectionMaterializeSummaries(t, jsonItems), collectionMaterializeSummaries(t, yamlItems); !reflect.DeepEqual(got, want) {
		t.Fatalf("compiled collection summaries differ:\njson=%#v\nyaml=%#v", got, want)
	}
}

func TestExplicitCollectionAssetMaterializeRejectsFanOutAndDimensionOverride(t *testing.T) {
	withFanOut := strings.Replace(canonicalCollectionMaterializeYAML(), "    data:\n", "    fan_out:\n      over: ${workflow.region[*]}\n      as: region\n      id: ${fanout}\n    data:\n", 1)
	doc, err := document.DecodeCanonicalWorkflowSource([]byte(withFanOut), document.DecodeOptions{Format: document.SourceFormatYAML})
	if err != nil {
		t.Fatalf("DecodeCanonicalWorkflowSource() error = %v", err)
	}
	workflow, err := WorkflowFromCanonicalDocument(doc)
	if err != nil {
		t.Fatalf("WorkflowFromCanonicalDocument() error = %v", err)
	}
	plan, err := NormalizeStages(workflow)
	if err != nil {
		t.Fatalf("NormalizeStages() error = %v", err)
	}
	_, err = CompileWorkflowStage(collectionMaterializeResolver(t, workflow), workflow, plan, 0)
	if err == nil || !strings.Contains(err.Error(), "collection step must not declare fan_out") {
		t.Fatalf("fan_out error = %v", err)
	}

	override := strings.Replace(canonicalCollectionMaterializeYAML(), "            region: ${workflow.region}\n", "            region: ${workflow.region}\n            year: 2010\n", 1)
	doc, err = document.DecodeCanonicalWorkflowSource([]byte(override), document.DecodeOptions{Format: document.SourceFormatYAML})
	if err != nil {
		t.Fatalf("DecodeCanonicalWorkflowSource() override error = %v", err)
	}
	workflow, err = WorkflowFromCanonicalDocument(doc)
	if err != nil {
		t.Fatalf("WorkflowFromCanonicalDocument() override error = %v", err)
	}
	plan, err = NormalizeStages(workflow)
	if err != nil {
		t.Fatalf("NormalizeStages() override error = %v", err)
	}
	_, err = CompileWorkflowStage(collectionMaterializeResolver(t, workflow), workflow, plan, 0)
	if err == nil || !strings.Contains(err.Error(), `dimension parameter "year" cannot be overridden`) {
		t.Fatalf("dimension override error = %v", err)
	}
}

func TestExplicitCollectionAssetMaterializeRejectsDuplicateMaterializationIdentity(t *testing.T) {
	source := strings.Replace(canonicalCollectionMaterializeYAML(), "  - id: materialize-cdl\n", "  - id: materialize-cdl\n    parallel_with: materialize\n", 1)
	source += strings.Replace(strings.Split(canonicalCollectionMaterializeYAML(), "  - id: materialize-cdl\n")[1], "    data:\n", "  - id: materialize-cdl-again\n    parallel_with: materialize\n    data:\n", 1)
	doc, err := document.DecodeCanonicalWorkflowSource([]byte(source), document.DecodeOptions{Format: document.SourceFormatYAML})
	if err != nil {
		t.Fatalf("DecodeCanonicalWorkflowSource() error = %v", err)
	}
	workflow, err := WorkflowFromCanonicalDocument(doc)
	if err != nil {
		t.Fatalf("WorkflowFromCanonicalDocument() error = %v", err)
	}
	plan, err := NormalizeStages(workflow)
	if err != nil {
		t.Fatalf("NormalizeStages() error = %v", err)
	}
	_, err = CompileWorkflowStage(collectionMaterializeResolver(t, workflow), workflow, plan, 0)
	if err == nil || !strings.Contains(err.Error(), "duplicate explicit asset_materialize materializer") {
		t.Fatalf("duplicate materialization error = %v", err)
	}
}

func TestExplicitAssetMaterializeRejectsComputeParameters(t *testing.T) {
	workflow := explicitCacheWorkflowForTest()
	workflow.Steps[0].FanOut.WorkItem.Parameters["python_entrypoint"] = model.Parameter{Type: "path", Value: "scripts/run.py"}
	plan, err := NormalizeStages(workflow)
	if err != nil {
		t.Fatalf("NormalizeStages() error = %v", err)
	}

	_, err = CompileWorkflowStage(explicitCacheResolverForTest(t, workflow), workflow, plan, 0)
	if err == nil || !strings.Contains(err.Error(), `asset.materialize step does not accept work parameter "python_entrypoint"`) {
		t.Fatalf("CompileWorkflowStage() error = %v", err)
	}
}

type collectionMaterializeSummary struct {
	ID                    string
	Index                 int
	AssetKey              string
	MaterializationKey    string
	Destination           string
	CollectionFingerprint string
	MemberBindings        map[string]any
}

func compileCanonicalCollectionMaterializeItems(t *testing.T, source string, format document.SourceFormat) []CompileStageWorkItem {
	t.Helper()
	doc, err := document.DecodeCanonicalWorkflowSource([]byte(source), document.DecodeOptions{Format: format})
	if err != nil {
		t.Fatalf("DecodeCanonicalWorkflowSource(%s) error = %v", format, err)
	}
	workflow, err := WorkflowFromCanonicalDocument(doc)
	if err != nil {
		t.Fatalf("WorkflowFromCanonicalDocument(%s) error = %v", format, err)
	}
	plan, err := NormalizeStages(workflow)
	if err != nil {
		t.Fatalf("NormalizeStages(%s) error = %v", format, err)
	}
	stage, err := CompileWorkflowStage(collectionMaterializeResolver(t, workflow), workflow, plan, 0)
	if err != nil {
		t.Fatalf("CompileWorkflowStage(%s) error = %v", format, err)
	}
	return stage.WorkItems
}

func collectionMaterializeResolver(t *testing.T, workflow Workflow) variable.Resolver {
	t.Helper()
	scope, err := variable.NewScope(workflow.Variables...)
	if err != nil {
		t.Fatalf("NewScope() error = %v", err)
	}
	return variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})
}

func collectionMaterializeSummaries(t *testing.T, items []CompileStageWorkItem) []collectionMaterializeSummary {
	t.Helper()
	summaries := make([]collectionMaterializeSummary, 0, len(items))
	for _, item := range items {
		payload := decodeAssetMaterializePayload(t, item)
		summary := collectionMaterializeSummary{
			ID:                 item.WorkItem.ID,
			Index:              item.WorkItemIndex,
			AssetKey:           payload.AssetKey,
			MaterializationKey: payload.MaterializationKey,
			Destination:        payload.DestinationRelativePath,
		}
		if payload.CollectionMember != nil {
			summary.CollectionFingerprint = payload.CollectionMember.CollectionFingerprint
			summary.MemberBindings = payload.CollectionMember.MemberBindings
		}
		summaries = append(summaries, summary)
	}
	return summaries
}

func canonicalCollectionMaterializeJSON() string {
	return `{
		"api_version": "goet/v1alpha1",
		"kind": "Workflow",
		"id": "collection-materialize",
		"variables": {
			"region": "midwest"
		},
		"data": {
			"inputs": {
				"cdl": {
					"kind": "raster",
					"format": "geotiff",
					"parameters": {
						"year": {"type": "int"},
						"region": {"type": "string"}
					},
					"collection": {
						"dimensions": [
							{"parameter": "year", "range": {"from": 2008, "through": 2023}}
						]
					},
					"files": {
						"raster": {
							"member": "${asset.year}_30m_cdls.tif",
							"required": true
						}
					},
					"select": ["raster"],
					"binding": {
						"provider": "http",
						"location": {
							"url_template": "https://example.invalid/cdl/${region}/${year}.zip"
						},
						"archive": {
							"type": "zip",
							"expose": "selected_path"
						},
						"cache": {
							"strategy": "worker_cache",
							"cache_key_template": "cdl/${region}/${year}.zip",
							"immutable": true
						},
						"materialization": {
							"scope": "shared",
							"strategy": "worker_cache",
							"path_template": "cdl/${asset.region}/${asset.year}.tif"
						},
						"transfer_policy": {
							"max_concurrent_source_transfers": 2,
							"max_bytes_per_second": 1024
						}
					}
				}
			}
		},
		"steps": [
			{
				"id": "materialize-cdl",
				"data": {
					"materialize": {
						"cdl": {
							"asset": "cdl",
							"with": {
								"region": "${workflow.region}"
							}
						}
					}
				},
				"work": {
					"type": "asset.materialize",
					"parameters": {
						"target_environment_id": "msu-hpcc"
					}
				}
			}
		]
	}`
}

func canonicalCollectionMaterializeYAML() string {
	return `
api_version: goet/v1alpha1
kind: Workflow
id: collection-materialize
variables:
  region: midwest
data:
  inputs:
    cdl:
      kind: raster
      format: geotiff
      parameters:
        year:
          type: int
        region:
          type: string
      collection:
        dimensions:
          - parameter: year
            range:
              from: 2008
              through: 2023
      files:
        raster:
          member: ${asset.year}_30m_cdls.tif
          required: true
      select:
        - raster
      binding:
        provider: http
        location:
          url_template: https://example.invalid/cdl/${region}/${year}.zip
        archive:
          type: zip
          expose: selected_path
        cache:
          strategy: worker_cache
          cache_key_template: cdl/${region}/${year}.zip
          immutable: true
        materialization:
          scope: shared
          strategy: worker_cache
          path_template: cdl/${asset.region}/${asset.year}.tif
        transfer_policy:
          max_concurrent_source_transfers: 2
          max_bytes_per_second: 1024
steps:
  - id: materialize-cdl
    data:
      materialize:
        cdl:
          asset: cdl
          with:
            region: ${workflow.region}
    work:
      type: asset.materialize
      parameters:
        target_environment_id: msu-hpcc
`
}

func TestExplicitAssetMaterializeRejectsDuplicateMaterializerInstanceInStage(t *testing.T) {
	workflow := explicitCacheWorkflowForTest()
	second := workflow.Steps[0]
	fanOut := *workflow.Steps[0].FanOut
	second.FanOut = &fanOut
	second.ID = "cache-again"
	second.FanOut.ID = "cache-again"
	second.FanOut.WorkItem.IDPrefix = "cache-again"
	workflow.Steps = append(workflow.Steps, second)
	workflow.Steps[0].ParallelWith = "cache"
	workflow.Steps[1].ParallelWith = "cache"
	plan, err := NormalizeStages(workflow)
	if err != nil {
		t.Fatalf("NormalizeStages() error = %v", err)
	}

	_, err = CompileWorkflowStage(explicitCacheResolverForTest(t, workflow), workflow, plan, 0)
	if err == nil || !strings.Contains(err.Error(), "duplicate explicit asset_materialize materializer") {
		t.Fatalf("CompileWorkflowStage() error = %v", err)
	}
}

func explicitCacheWorkflowForTest() Workflow {
	return Workflow{
		ID: "explicit-cache",
		Variables: []variable.Variable{
			{
				Name: variable.Name{Namespace: variable.NamespaceWorkflow, Key: "tiles"},
				TypedExpression: variable.TypedExpression{
					Type: variable.TypeList,
					Expression: []variable.TypedExpression{
						{Type: variable.TypeString, Expression: "h18v07"},
					},
				},
			},
		},
		Steps: []Step{
			{
				ID: "cache",
				FanOut: &FanOutStep{
					ID: "cache",
					WorkItem: FanOutWorkItemTemplate{
						FanOutExpression: "${workflow.tiles[*]}",
						Type:             model.WorkItemTypeAssetMaterialize,
						IDPrefix:         "cache",
						OutputPrefix:     "cache",
						OutputExtension:  ".json",
						Parameters: model.Parameters{
							"target_environment_id": {Type: "string", Value: "target-local"},
						},
						ExplicitAssetMaterialize: &ExplicitAssetMaterializeTemplate{
							Definitions: model.DataDefinitions{
								Inputs: map[string]model.DataInputDefinition{
									"asset": explicitCacheDefinitionForTest(),
								},
							},
							Alias: "input",
							Asset: "asset",
							With: map[string]variable.TypedExpression{
								"tile": {Type: variable.TypeString, Expression: "${fanout}"},
							},
						},
					},
				},
			},
		},
	}
}

func explicitCacheDefinitionForTest() model.DataInputDefinition {
	immutable := true
	return model.DataInputDefinition{
		Kind: "fixture",
		Parameters: map[string]model.DataParameterDefinition{
			"tile": {Type: "string"},
		},
		Files: map[string]model.DataFileRoleDefinition{
			"header": {
				Member: "${asset.tile}/header.txt",
			},
		},
		Select: []string{"header"},
		Binding: model.DataInputBindingDefinition{
			ProviderName: "fixture_provider",
			Provider:     model.DataProviderLocalFile,
			Location: model.DataDefinitionLocation{
				Name: "fixture_data",
				Path: "source.zip",
			},
			Archive: model.DataArchiveBindingDefinition{
				Type:   model.DataAssetArchiveTypeZip,
				Expose: model.DataAssetArchiveExposeSelectedPath,
			},
			Cache: model.DataDefinitionCache{
				Strategy:  model.DataAssetCacheStrategyWorkerCache,
				CacheKey:  "fixtures/source.zip",
				Immutable: &immutable,
			},
			Materialization: model.DataDefinitionMaterialization{
				Scope:    model.DataMaterializationScopeShared,
				Strategy: model.DataAssetCacheStrategyWorkerCache,
			},
		},
	}
}

func explicitCacheResolverForTest(t *testing.T, workflow Workflow) variable.Resolver {
	t.Helper()
	scope, err := variable.NewScope(workflow.Variables...)
	if err != nil {
		t.Fatalf("NewScope() error = %v", err)
	}
	return variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})
}

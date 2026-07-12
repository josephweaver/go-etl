package workflow

import (
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

package workflow

import (
	"reflect"
	"testing"

	"goetl/internal/document"
	"goetl/internal/model"
	"goetl/internal/variable"
)

type functionFanOutIdentity struct {
	ID       string
	AssetKey string
	Member   string
	Year     string
	Region   string
}

func TestFunctionProducedFanOutCompilesEquivalentJSONAndYAMLWorkItems(t *testing.T) {
	jsonSource := []byte(`{
		"api_version": "goet/v1alpha1",
		"kind": "Workflow",
		"id": "function-fanout-proof",
		"variables": {
			"pair_groups": [
				[
					{"id": "2024-north", "year": "2024", "region": "north"},
					{"id": "2024-south", "year": "2024", "region": "south"}
				],
				[
					{"id": "2025-north", "year": "2025", "region": "north"}
				]
			],
			"pairs": {
				"$type": "list",
				"$call": "list.flatten",
				"$args": [
					{"$ref": "pair_groups"}
				]
			}
		},
		"data": {
			"inputs": {
				"field_segments": {
					"kind": "fixture_segments",
					"parameters": {
						"year": {"type": "string"},
						"region": {"type": "string"}
					},
					"files": {
						"segment": {
							"member": "${asset.year}/${asset.region}/field_segments.csv",
							"as": "field_segments_${asset.year}_${asset.region}.csv",
							"required": true
						}
					},
					"select": ["segment"],
					"binding": {
						"provider_name": "fixture_data",
						"provider": "local_file",
						"location": {
							"name": "fixture_data",
							"path": "release.zip"
						},
						"archive": {
							"type": "zip",
							"expose": "selected_path"
						},
						"cache": {
							"strategy": "worker_cache",
							"cache_key": "fixtures/release.zip",
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
				"id": "cache-segments",
				"fan_out": {
					"over": "${workflow.pairs[*]}",
					"as": "pair",
					"id": "${fanout.id}"
				},
				"data": {
					"materialize": {
						"field_segments": {
							"asset": "field_segments",
							"with": {
								"year": "${fanout.year}",
								"region": "${fanout.region}"
							}
						}
					}
				},
				"work": {
					"type": "cache_data",
					"parameters": {
						"target_environment_id": "target-local"
					}
				}
			}
		]
	}`)
	yamlSource := []byte(`
api_version: goet/v1alpha1
kind: Workflow
id: function-fanout-proof
variables:
  pair_groups:
    -
      - id: 2024-north
        year: "2024"
        region: north
      - id: 2024-south
        year: "2024"
        region: south
    -
      - id: 2025-north
        year: "2025"
        region: north
  pairs:
    $type: list
    $call: list.flatten
    $args:
      - $ref: pair_groups
data:
  inputs:
    field_segments:
      kind: fixture_segments
      parameters:
        year:
          type: string
        region:
          type: string
      files:
        segment:
          member: ${asset.year}/${asset.region}/field_segments.csv
          as: field_segments_${asset.year}_${asset.region}.csv
          required: true
      select:
        - segment
      binding:
        provider_name: fixture_data
        provider: local_file
        location:
          name: fixture_data
          path: release.zip
        archive:
          type: zip
          expose: selected_path
        cache:
          strategy: worker_cache
          cache_key: fixtures/release.zip
          immutable: true
        materialization:
          scope: shared
          strategy: worker_cache
steps:
  - id: cache-segments
    fan_out:
      over: ${workflow.pairs[*]}
      as: pair
      id: ${fanout.id}
    data:
      materialize:
        field_segments:
          asset: field_segments
          with:
            year: ${fanout.year}
            region: ${fanout.region}
    work:
      type: cache_data
      parameters:
        target_environment_id: target-local
`)

	jsonIdentities := compileFunctionFanOutIdentities(t, jsonSource, document.SourceFormatJSON)
	yamlIdentities := compileFunctionFanOutIdentities(t, yamlSource, document.SourceFormatYAML)

	if !reflect.DeepEqual(jsonIdentities, yamlIdentities) {
		t.Fatalf("compiled identities differ:\njson=%#v\nyaml=%#v", jsonIdentities, yamlIdentities)
	}

	want := []functionFanOutIdentity{
		{ID: "cache-segments-2024-north", Member: "2024/north/field_segments.csv", Year: "2024", Region: "north"},
		{ID: "cache-segments-2024-south", Member: "2024/south/field_segments.csv", Year: "2024", Region: "south"},
		{ID: "cache-segments-2025-north", Member: "2025/north/field_segments.csv", Year: "2025", Region: "north"},
	}
	if len(jsonIdentities) != len(want) {
		t.Fatalf("identity count = %d, want %d: %#v", len(jsonIdentities), len(want), jsonIdentities)
	}
	seenAssetKeys := map[string]bool{}
	for index, identity := range jsonIdentities {
		if identity.ID != want[index].ID || identity.Member != want[index].Member || identity.Year != want[index].Year || identity.Region != want[index].Region {
			t.Fatalf("identity %d = %#v, want %#v", index, identity, want[index])
		}
		if identity.AssetKey == "" || seenAssetKeys[identity.AssetKey] {
			t.Fatalf("asset key %d = %q, seen=%v", index, identity.AssetKey, seenAssetKeys)
		}
		seenAssetKeys[identity.AssetKey] = true
	}
}

func TestFunctionFanOutProofKeepsOrdinaryListFanOut(t *testing.T) {
	scope, err := variable.NewScope(testIntListVariable("years", 2024, 2025))
	if err != nil {
		t.Fatal(err)
	}
	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})

	items, err := CompileFanOutWorkItems(resolver, FanOutWorkItemTemplate{
		FanOutExpression: "${workflow.years[*]}",
		Type:             model.WorkItemTypeWriteDemoOutput,
		IDPrefix:         "ordinary",
		OutputPrefix:     "ordinary",
		OutputExtension:  ".txt",
	})
	if err != nil {
		t.Fatalf("CompileFanOutWorkItems() error = %v", err)
	}
	if len(items) != 2 || items[0].ID != "ordinary-2024" || items[1].ID != "ordinary-2025" {
		t.Fatalf("ordinary fan-out items = %+v", items)
	}
}

func compileFunctionFanOutIdentities(t *testing.T, source []byte, format document.SourceFormat) []functionFanOutIdentity {
	t.Helper()
	doc, err := document.DecodeCanonicalWorkflowSource(source, document.DecodeOptions{Format: format})
	if err != nil {
		t.Fatalf("DecodeCanonicalWorkflowSource(%s) error = %v", format, err)
	}
	workflow, err := WorkflowFromCanonicalDocument(doc)
	if err != nil {
		t.Fatalf("WorkflowFromCanonicalDocument(%s) error = %v", format, err)
	}
	scope, err := variable.NewScope(workflow.Variables...)
	if err != nil {
		t.Fatalf("NewScope(%s) error = %v", format, err)
	}
	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{
		FunctionRegistry: variable.DefaultFunctionRegistry(),
	})
	plan, err := NormalizeStages(workflow)
	if err != nil {
		t.Fatalf("NormalizeStages(%s) error = %v", format, err)
	}
	stage, err := CompileWorkflowStage(resolver, workflow, plan, 0)
	if err != nil {
		t.Fatalf("CompileWorkflowStage(%s) error = %v", format, err)
	}
	if len(stage.WorkItems) != 3 {
		t.Fatalf("work item count for %s = %d, want 3", format, len(stage.WorkItems))
	}

	identities := make([]functionFanOutIdentity, 0, len(stage.WorkItems))
	for _, item := range stage.WorkItems {
		if item.WorkItem.Type != model.WorkItemTypeCacheData {
			t.Fatalf("work item %s type = %q, want cache_data", item.WorkItem.ID, item.WorkItem.Type)
		}
		payload := decodeCacheDataPayload(t, item)
		assets, err := boundDataAssetsFromParameters(item.WorkItem.Parameters)
		if err != nil {
			t.Fatalf("boundDataAssetsFromParameters(%s) error = %v", item.WorkItem.ID, err)
		}
		if len(assets) != 1 {
			t.Fatalf("work item %s bound assets = %d, want 1", item.WorkItem.ID, len(assets))
		}
		asset := assets[0]
		if payload.AssetKey == "" || payload.DedupeKey != "cache_data:target-local:"+payload.AssetKey {
			t.Fatalf("work item %s payload asset_key=%q dedupe_key=%q", item.WorkItem.ID, payload.AssetKey, payload.DedupeKey)
		}
		if asset.Archive == nil || len(asset.Archive.Select) != 1 {
			t.Fatalf("work item %s archive select = %+v", item.WorkItem.ID, asset.Archive)
		}
		year, _ := asset.Parameters["year"].(string)
		region, _ := asset.Parameters["region"].(string)
		identities = append(identities, functionFanOutIdentity{
			ID:       item.WorkItem.ID,
			AssetKey: payload.AssetKey,
			Member:   asset.Archive.Select[0].Member,
			Year:     year,
			Region:   region,
		})
	}
	return identities
}

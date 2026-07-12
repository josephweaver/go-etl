package workflow

import (
	"encoding/json"
	"strings"
	"testing"

	"goetl/internal/model"
)

func TestPlanStageAssetMaterializeWorkItemsDoesNotCreateHiddenMaterializationFromDataAssets(t *testing.T) {
	asset := testAssetMaterializeAsset("cropland_year", "2023_30m_cdls.tif")
	stage := testAssetMaterializeStage(testComputeStageItem("compute-a", asset, "target-local"))

	planned, err := PlanStageAssetMaterializeWorkItems(stage)
	if err != nil {
		t.Fatalf("PlanStageAssetMaterializeWorkItems() error = %v", err)
	}

	if len(planned.WorkItems) != 1 {
		t.Fatalf("work item count = %d, want authored compute only", len(planned.WorkItems))
	}
	compute := planned.WorkItems[0].WorkItem
	if compute.Type != model.WorkItemTypePythonScript {
		t.Fatalf("work item type = %q, want python_script", compute.Type)
	}
	if len(compute.DependsOn) != 0 {
		t.Fatalf("compute depends_on = %+v, want no hidden dependency", compute.DependsOn)
	}
	if _, ok := compute.Parameters["data_assets"]; !ok {
		t.Fatal("compute item lost data_assets parameter")
	}
	if len(AssetMaterializeItems(planned)) != 0 {
		t.Fatalf("asset_materialize item count = %d, want 0", len(AssetMaterializeItems(planned)))
	}
}

func TestAssetMaterializePayloadUsesConfiguredSourceCapacityAndTransferLimits(t *testing.T) {
	asset := testAssetMaterializeAsset("cropland_year", "2023_30m_cdls.tif")
	asset.TransferPolicy = model.DataAssetTransferPolicy{
		MaxConcurrentSourceTransfers:   2,
		RequestedBandwidthMiBPerSecond: 25,
		MaxBytesPerSecond:              26214400,
		ProviderArgs: map[string]string{
			"rclone_bwlimit": "25M",
		},
	}
	assetKey, err := AssetMaterializeAssetKey(asset, "target-local")
	if err != nil {
		t.Fatalf("AssetMaterializeAssetKey() error = %v", err)
	}
	payload, constraints, err := AssetMaterializePayload(asset, "target-local", assetKey)
	if err != nil {
		t.Fatalf("AssetMaterializePayload() error = %v", err)
	}
	if len(constraints) != 1 || constraints[0].TargetUnits != 2 {
		t.Fatalf("constraints = %+v, want source capacity 2", constraints)
	}
	if payload.TransferLimits.MaxBytesPerSecond != 26214400 {
		t.Fatalf("transfer max bytes/sec = %d", payload.TransferLimits.MaxBytesPerSecond)
	}
	if payload.TransferPolicy.ProviderArgs["rclone_bwlimit"] != "25M" {
		t.Fatalf("provider args = %+v", payload.TransferPolicy.ProviderArgs)
	}
	if len(payload.ResourceConstraints) != 1 || payload.ResourceConstraints[0].TargetUnits != 2 {
		t.Fatalf("payload resource constraints = %+v", payload.ResourceConstraints)
	}
}

func TestAssetMaterializeResourceKeysAreSanitizedByProviderSource(t *testing.T) {
	tests := []struct {
		name  string
		asset model.BoundDataAsset
		want  string
	}{
		{
			name:  "http host",
			asset: testAssetMaterializeAsset("cropland_year", "2023_30m_cdls.tif"),
			want:  "provider:http:example.invalid/download",
		},
		{
			name:  "gdrive remote",
			asset: testGDriveAssetMaterializeAsset("landcore shared", "Risk Model/data.txt"),
			want:  "provider:gdrive-rclone:landcore-shared/download",
		},
		{
			name:  "local root",
			asset: testLocalAssetMaterializeAsset("fixture_data", "input.txt"),
			want:  "provider:local-file:fixture_data/read",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			constraints, err := AssetMaterializeResourceConstraints(tt.asset, "target-local")
			if err != nil {
				t.Fatalf("AssetMaterializeResourceConstraints() error = %v", err)
			}
			if len(constraints) != 1 || constraints[0].ResourceKey != tt.want {
				t.Fatalf("constraints = %+v, want resource key %q", constraints, tt.want)
			}
		})
	}
}

func TestCompileWorkflowStageRejectsLegacyDataOperatorParameters(t *testing.T) {
	tests := []struct {
		name      string
		parameter model.Parameter
		want      string
	}{
		{
			name:      "data_assets",
			parameter: model.Parameter{Type: "data_assets", Value: []model.BoundDataAsset{testAssetMaterializeAsset("cropland_year", "2023_30m_cdls.tif")}},
			want:      `legacy work parameter "data_assets" is not allowed`,
		},
		{
			name:      "publish",
			parameter: model.Parameter{Type: "publish_targets", Value: []model.BoundPublishTarget{testPublishTarget("published_data", "reports/summary.csv")}},
			want:      `legacy work parameter "publish" is not allowed`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolver := testWorkflowResolver(t, 2024)
			workflow := Workflow{
				ID: "cdl",
				Steps: []Step{
					{
						ID: "compute",
						FanOut: &FanOutStep{
							WorkItem: FanOutWorkItemTemplate{
								FanOutExpression: "${years[*]}",
								Type:             model.WorkItemTypePythonScript,
								IDPrefix:         "compute",
								OutputPrefix:     "compute",
								OutputExtension:  ".json",
								Parameters: model.Parameters{
									"python_entrypoint":     {Type: "path", Value: "scripts/run.py"},
									"target_environment_id": {Type: "string", Value: "target-local"},
									test.name:               test.parameter,
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

			_, err = CompileWorkflowStage(resolver, workflow, plan, 0)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("CompileWorkflowStage() error = %v, want %q", err, test.want)
			}
		})
	}
}

func testAssetMaterializeStage(items ...CompileStageWorkItem) CompileStageResult {
	return CompileStageResult{
		WorkflowID: "cdl",
		StageIndex: 0,
		Steps: []WorkflowStageStep{
			{StageIndex: 0, StepIndex: 0, StepID: "compute"},
		},
		WorkItems: items,
	}
}

func testPublishTarget(locationName string, path string) model.BoundPublishTarget {
	return model.BoundPublishTarget{
		Name:            "publish_summary",
		FromArtifact:    "summary",
		TargetName:      "publish_summary",
		Location:        model.DataAssetLocation{Type: model.DataProviderRegisteredLocation, LocationName: locationName, Path: path},
		OverwritePolicy: model.PublishedDataAssetOverwriteFailIfExists,
	}
}

func testComputeStageItem(id string, asset model.BoundDataAsset, targetEnvironmentID string) CompileStageWorkItem {
	return CompileStageWorkItem{
		WorkflowID:    "cdl",
		StageIndex:    0,
		StepIndex:     0,
		StepID:        "compute",
		WorkItemIndex: 0,
		WorkItem: model.WorkItem{
			ID:             id,
			Type:           model.WorkItemTypePythonScript,
			OutputFilename: id + ".json",
			Parameters: model.Parameters{
				"data_assets": {
					Type:  "data_assets",
					Value: []model.BoundDataAsset{asset},
				},
				"target_environment_id": {
					Type:  "string",
					Value: targetEnvironmentID,
				},
				"python_entrypoint": {
					Type:  "path",
					Value: "scripts/run.py",
				},
			},
		},
	}
}

func testAssetMaterializeAsset(bindingName string, archiveMember string) model.BoundDataAsset {
	required := true
	return model.BoundDataAsset{
		BindingName:  bindingName,
		ProviderName: "cdl_zip",
		Kind:         "raster_archive",
		Format:       "geotiff_zip",
		Provider:     model.DataProviderHTTP,
		Location: model.DataAssetLocation{
			Type: model.DataProviderHTTP,
			URI:  "https://example.invalid/2023_30m_cdls.zip",
		},
		Integrity: model.DataAssetIntegrity{
			SHA256:   validWorkflowDataSHA256,
			Required: true,
		},
		Cache: model.DataAssetCache{
			Strategy: model.DataAssetCacheStrategyWorkerCache,
			CacheKey: "cdl/2023/30m/source.zip",
		},
		Archive: &model.DataAssetArchive{
			Type: model.DataAssetArchiveTypeZip,
			Select: []model.DataAssetArchiveSelect{
				{Member: archiveMember, As: "cdl.tif", Required: &required},
			},
			Expose: model.DataAssetArchiveExposeSelectedPath,
		},
		Parameters: map[string]any{"year": 2023},
	}
}

func testGDriveAssetMaterializeAsset(remote string, drivePath string) model.BoundDataAsset {
	asset := testAssetMaterializeAsset("drive_data", "2023_30m_cdls.tif")
	asset.ProviderName = "drive_provider"
	asset.Provider = model.DataProviderGDriveRclone
	asset.Location = model.DataAssetLocation{
		Type:      model.DataProviderGDriveRclone,
		Remote:    remote,
		DrivePath: drivePath,
	}
	asset.Archive = nil
	return asset
}

func testLocalAssetMaterializeAsset(rootName string, relPath string) model.BoundDataAsset {
	asset := testAssetMaterializeAsset("local_data", "2023_30m_cdls.tif")
	asset.ProviderName = "local_provider"
	asset.Provider = model.DataProviderLocalFile
	asset.Location = model.DataAssetLocation{
		Type:         model.DataProviderLocalFile,
		LocationName: rootName,
		Path:         relPath,
	}
	asset.Archive = nil
	asset.Cache = model.DataAssetCache{Strategy: model.DataAssetCacheStrategyReference}
	return asset
}

func decodeAssetMaterializePayload(t *testing.T, item CompileStageWorkItem) model.AssetMaterializeWorkItemPayload {
	t.Helper()
	parameter := item.WorkItem.Parameters["asset_materialize"]
	data, err := json.Marshal(parameter.Value)
	if err != nil {
		t.Fatalf("marshal asset_materialize payload: %v", err)
	}
	var payload model.AssetMaterializeWorkItemPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode asset_materialize payload: %v", err)
	}
	return payload
}

func decodeCommitDataPayload(t *testing.T, item CompileStageWorkItem) model.CommitDataWorkItemPayload {
	t.Helper()
	parameter := item.WorkItem.Parameters["commit_data"]
	data, err := json.Marshal(parameter.Value)
	if err != nil {
		t.Fatalf("marshal commit_data payload: %v", err)
	}
	var payload model.CommitDataWorkItemPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode commit_data payload: %v", err)
	}
	return payload
}

func AssetMaterializeItems(result CompileStageResult) []CompileStageWorkItem {
	var items []CompileStageWorkItem
	for _, item := range result.WorkItems {
		if item.WorkItem.Type == model.WorkItemTypeAssetMaterialize {
			items = append(items, item)
		}
	}
	return items
}

func computeDataItems(result CompileStageResult) []CompileStageWorkItem {
	var items []CompileStageWorkItem
	for _, item := range result.WorkItems {
		if item.WorkItem.Type != model.WorkItemTypeAssetMaterialize && item.WorkItem.Type != model.WorkItemTypeCommitData {
			items = append(items, item)
		}
	}
	return items
}

func commitDataItems(result CompileStageResult) []CompileStageWorkItem {
	var items []CompileStageWorkItem
	for _, item := range result.WorkItems {
		if item.WorkItem.Type == model.WorkItemTypeCommitData {
			items = append(items, item)
		}
	}
	return items
}

const validWorkflowDataSHA256 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

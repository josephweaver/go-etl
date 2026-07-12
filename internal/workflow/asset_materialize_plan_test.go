package workflow

import (
	"encoding/json"
	"testing"

	"goetl/internal/model"
)

func TestPlanAssetMaterializeWorkItemsDeduplicatesSameProviderParametersForTwoComputeJobs(t *testing.T) {
	asset := testAssetMaterializeAsset("cropland_year", "2023_30m_cdls.tif")
	stage := testAssetMaterializeStage(
		testComputeStageItem("compute-a", asset, "target-local"),
		testComputeStageItem("compute-b", asset, "target-local"),
	)

	planned, err := PlanAssetMaterializeWorkItems(stage)
	if err != nil {
		t.Fatalf("PlanAssetMaterializeWorkItems() error = %v", err)
	}

	cacheItems := AssetMaterializeItems(planned)
	if len(cacheItems) != 1 {
		t.Fatalf("asset_materialize item count = %d, want 1", len(cacheItems))
	}
	for _, item := range computeDataItems(planned) {
		if len(item.WorkItem.DependsOn) != 1 || item.WorkItem.DependsOn[0] != cacheItems[0].WorkItem.ID {
			t.Fatalf("compute %s depends_on = %+v, want %s", item.WorkItem.ID, item.WorkItem.DependsOn, cacheItems[0].WorkItem.ID)
		}
	}

	payload, ok := cacheItems[0].WorkItem.Parameters["asset_materialize"]
	if !ok || payload.Type != "asset_materialize" {
		t.Fatalf("asset_materialize parameter = %+v, want asset_materialize payload", payload)
	}
	if len(cacheItems[0].ResourceConstraints) != 1 {
		t.Fatalf("resource constraint count = %d, want 1", len(cacheItems[0].ResourceConstraints))
	}
	constraint := cacheItems[0].ResourceConstraints[0]
	if constraint.ResourceKey != "provider:http:example.invalid/download" {
		t.Fatalf("resource key = %q", constraint.ResourceKey)
	}
	if constraint.RequestedUnits != 1 || constraint.Operator != model.WorkItemResourceConstraintOperatorLessEq || constraint.TargetUnits != 1 {
		t.Fatalf("resource constraint = %+v, want source mutex", constraint)
	}
}

func TestPlanAssetMaterializeWorkItemsDeduplicatesSamePhysicalAssetUnderTwoAliases(t *testing.T) {
	cropland := testAssetMaterializeAsset("cropland_year", "2023_30m_cdls.tif")
	inputRaster := cropland
	inputRaster.BindingName = "input_raster"
	stage := testAssetMaterializeStage(
		testComputeStageItem("compute-a", cropland, "target-local"),
		testComputeStageItem("compute-b", inputRaster, "target-local"),
	)

	planned, err := PlanAssetMaterializeWorkItems(stage)
	if err != nil {
		t.Fatalf("PlanAssetMaterializeWorkItems() error = %v", err)
	}

	if got := len(AssetMaterializeItems(planned)); got != 1 {
		t.Fatalf("asset_materialize item count = %d, want 1", got)
	}
}

func TestPlanAssetMaterializeWorkItemsDoesNotDeduplicateDifferentArchiveSelection(t *testing.T) {
	first := testAssetMaterializeAsset("cropland_year", "2023_30m_cdls.tif")
	second := testAssetMaterializeAsset("cropland_year", "metadata.xml")
	stage := testAssetMaterializeStage(
		testComputeStageItem("compute-a", first, "target-local"),
		testComputeStageItem("compute-b", second, "target-local"),
	)

	planned, err := PlanAssetMaterializeWorkItems(stage)
	if err != nil {
		t.Fatalf("PlanAssetMaterializeWorkItems() error = %v", err)
	}

	if got := len(AssetMaterializeItems(planned)); got != 2 {
		t.Fatalf("asset_materialize item count = %d, want 2", got)
	}
}

func TestPlanAssetMaterializeWorkItemsDoesNotDeduplicateDifferentTargetEnvironment(t *testing.T) {
	asset := testAssetMaterializeAsset("cropland_year", "2023_30m_cdls.tif")
	stage := testAssetMaterializeStage(
		testComputeStageItem("compute-a", asset, "target-local"),
		testComputeStageItem("compute-b", asset, "target-hpcc"),
	)

	planned, err := PlanAssetMaterializeWorkItems(stage)
	if err != nil {
		t.Fatalf("PlanAssetMaterializeWorkItems() error = %v", err)
	}

	if got := len(AssetMaterializeItems(planned)); got != 2 {
		t.Fatalf("asset_materialize item count = %d, want 2", got)
	}
}

func TestPlanAssetMaterializeWorkItemsUsesConfiguredSourceCapacityAndTransferLimits(t *testing.T) {
	asset := testAssetMaterializeAsset("cropland_year", "2023_30m_cdls.tif")
	asset.TransferPolicy = model.DataAssetTransferPolicy{
		MaxConcurrentSourceTransfers:   2,
		RequestedBandwidthMiBPerSecond: 25,
		MaxBytesPerSecond:              26214400,
		ProviderArgs: map[string]string{
			"rclone_bwlimit": "25M",
		},
	}
	stage := testAssetMaterializeStage(testComputeStageItem("compute-a", asset, "target-local"))

	planned, err := PlanAssetMaterializeWorkItems(stage)
	if err != nil {
		t.Fatalf("PlanAssetMaterializeWorkItems() error = %v", err)
	}

	cacheItems := AssetMaterializeItems(planned)
	if len(cacheItems) != 1 {
		t.Fatalf("asset_materialize item count = %d, want 1", len(cacheItems))
	}
	constraint := cacheItems[0].ResourceConstraints[0]
	if constraint.TargetUnits != 2 {
		t.Fatalf("target units = %d, want 2", constraint.TargetUnits)
	}

	payload := decodeAssetMaterializePayload(t, cacheItems[0])
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

func TestPlanCommitDataWorkItemsSplitsPublishBindingFromCompute(t *testing.T) {
	asset := testAssetMaterializeAsset("cropland_year", "2023_30m_cdls.tif")
	compute := testComputeStageItem("compute-a", asset, "target-local")
	compute.WorkItem.Parameters["publish"] = model.Parameter{Type: "publish_targets", Value: []model.BoundPublishTarget{testPublishTarget("published_data", "reports/summary.csv")}}
	stage := testAssetMaterializeStage(compute)

	planned, err := PlanCommitDataWorkItems(stage)
	if err != nil {
		t.Fatalf("PlanCommitDataWorkItems() error = %v", err)
	}

	if got := len(planned.WorkItems); got != 2 {
		t.Fatalf("work item count = %d, want 2", got)
	}
	computeItem := planned.WorkItems[0]
	if _, ok := computeItem.WorkItem.Parameters["publish"]; ok {
		t.Fatal("compute item still has publish parameter")
	}
	commitItem := planned.WorkItems[1]
	if commitItem.WorkItem.Type != model.WorkItemTypeCommitData {
		t.Fatalf("commit item type = %q, want commit_data", commitItem.WorkItem.Type)
	}
	if len(commitItem.WorkItem.DependsOn) != 1 || commitItem.WorkItem.DependsOn[0] != compute.WorkItem.ID {
		t.Fatalf("commit depends_on = %+v, want %s", commitItem.WorkItem.DependsOn, compute.WorkItem.ID)
	}
	payload := decodeCommitDataPayload(t, commitItem)
	if payload.Source.FromWorkItemID != compute.WorkItem.ID || payload.Source.FromArtifact != "summary" {
		t.Fatalf("commit source = %+v, want compute summary", payload.Source)
	}
	if len(commitItem.ResourceConstraints) != 1 {
		t.Fatalf("resource constraint count = %d, want 1", len(commitItem.ResourceConstraints))
	}
	constraint := commitItem.ResourceConstraints[0]
	if constraint.ResourceKey != "target:target-local/published-data-write:published_data" {
		t.Fatalf("resource key = %q", constraint.ResourceKey)
	}
	if constraint.RequestedUnits != 1 || constraint.Operator != model.WorkItemResourceConstraintOperatorLessEq || constraint.TargetUnits != 1 {
		t.Fatalf("resource constraint = %+v, want write mutex", constraint)
	}
}

func TestCompileWorkflowStageDoesNotPlanLegacyDataOperators(t *testing.T) {
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
							"data_assets":           {Type: "data_assets", Value: []model.BoundDataAsset{testAssetMaterializeAsset("cropland_year", "2023_30m_cdls.tif")}},
							"publish":               {Type: "publish_targets", Value: []model.BoundPublishTarget{testPublishTarget("published_data", "reports/summary.csv")}},
						},
					},
				},
			},
		},
	}
	plan, err := NormalizeStages(workflow)
	if err != nil {
		t.Fatalf("PlanWorkflow() error = %v", err)
	}

	result, err := CompileWorkflowStage(resolver, workflow, plan, 0)
	if err != nil {
		t.Fatalf("CompileWorkflowStage() error = %v", err)
	}

	if len(result.WorkItems) != 1 {
		t.Fatalf("work item count = %d, want authored compute item only", len(result.WorkItems))
	}
	if len(AssetMaterializeItems(result)) != 0 {
		t.Fatalf("asset_materialize item count = %d, want 0", len(AssetMaterializeItems(result)))
	}
	if len(commitDataItems(result)) != 0 {
		t.Fatalf("commit_data item count = %d, want 0", len(commitDataItems(result)))
	}
	computeItems := computeDataItems(result)
	if len(computeItems) != 1 {
		t.Fatalf("compute item count = %d, want 1", len(computeItems))
	}
	compute := computeItems[0]
	if _, ok := compute.WorkItem.Parameters["data_assets"]; !ok {
		t.Fatal("compute item lost legacy data_assets parameter")
	}
	if _, ok := compute.WorkItem.Parameters["publish"]; !ok {
		t.Fatal("compute item lost legacy publish parameter")
	}
	if len(compute.WorkItem.DependsOn) != 0 {
		t.Fatalf("compute depends_on = %+v, want no hidden cache dependency", compute.WorkItem.DependsOn)
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

package workflow

import (
	"testing"

	"goetl/internal/model"
)

func TestPlanCacheDataWorkItemsDeduplicatesSameProviderParametersForTwoComputeJobs(t *testing.T) {
	asset := testCacheDataAsset("cropland_year", "2023_30m_cdls.tif")
	stage := testCacheDataStage(
		testComputeStageItem("compute-a", asset, "target-local"),
		testComputeStageItem("compute-b", asset, "target-local"),
	)

	planned, err := PlanCacheDataWorkItems(stage)
	if err != nil {
		t.Fatalf("PlanCacheDataWorkItems() error = %v", err)
	}

	cacheItems := cacheDataItems(planned)
	if len(cacheItems) != 1 {
		t.Fatalf("cache_data item count = %d, want 1", len(cacheItems))
	}
	for _, item := range computeDataItems(planned) {
		if len(item.WorkItem.DependsOn) != 1 || item.WorkItem.DependsOn[0] != cacheItems[0].WorkItem.ID {
			t.Fatalf("compute %s depends_on = %+v, want %s", item.WorkItem.ID, item.WorkItem.DependsOn, cacheItems[0].WorkItem.ID)
		}
	}

	payload, ok := cacheItems[0].WorkItem.Parameters["cache_data"]
	if !ok || payload.Type != "cache_data" {
		t.Fatalf("cache_data parameter = %+v, want cache_data payload", payload)
	}
}

func TestPlanCacheDataWorkItemsDeduplicatesSamePhysicalAssetUnderTwoAliases(t *testing.T) {
	cropland := testCacheDataAsset("cropland_year", "2023_30m_cdls.tif")
	inputRaster := cropland
	inputRaster.BindingName = "input_raster"
	stage := testCacheDataStage(
		testComputeStageItem("compute-a", cropland, "target-local"),
		testComputeStageItem("compute-b", inputRaster, "target-local"),
	)

	planned, err := PlanCacheDataWorkItems(stage)
	if err != nil {
		t.Fatalf("PlanCacheDataWorkItems() error = %v", err)
	}

	if got := len(cacheDataItems(planned)); got != 1 {
		t.Fatalf("cache_data item count = %d, want 1", got)
	}
}

func TestPlanCacheDataWorkItemsDoesNotDeduplicateDifferentArchiveSelection(t *testing.T) {
	first := testCacheDataAsset("cropland_year", "2023_30m_cdls.tif")
	second := testCacheDataAsset("cropland_year", "metadata.xml")
	stage := testCacheDataStage(
		testComputeStageItem("compute-a", first, "target-local"),
		testComputeStageItem("compute-b", second, "target-local"),
	)

	planned, err := PlanCacheDataWorkItems(stage)
	if err != nil {
		t.Fatalf("PlanCacheDataWorkItems() error = %v", err)
	}

	if got := len(cacheDataItems(planned)); got != 2 {
		t.Fatalf("cache_data item count = %d, want 2", got)
	}
}

func TestPlanCacheDataWorkItemsDoesNotDeduplicateDifferentTargetEnvironment(t *testing.T) {
	asset := testCacheDataAsset("cropland_year", "2023_30m_cdls.tif")
	stage := testCacheDataStage(
		testComputeStageItem("compute-a", asset, "target-local"),
		testComputeStageItem("compute-b", asset, "target-hpcc"),
	)

	planned, err := PlanCacheDataWorkItems(stage)
	if err != nil {
		t.Fatalf("PlanCacheDataWorkItems() error = %v", err)
	}

	if got := len(cacheDataItems(planned)); got != 2 {
		t.Fatalf("cache_data item count = %d, want 2", got)
	}
}

func testCacheDataStage(items ...CompileStageWorkItem) CompileStageResult {
	return CompileStageResult{
		WorkflowID: "cdl",
		StageIndex: 0,
		Steps: []WorkflowStageStep{
			{StageIndex: 0, StepIndex: 0, StepID: "compute"},
		},
		WorkItems: items,
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

func testCacheDataAsset(bindingName string, archiveMember string) model.BoundDataAsset {
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

func cacheDataItems(result CompileStageResult) []CompileStageWorkItem {
	var items []CompileStageWorkItem
	for _, item := range result.WorkItems {
		if item.WorkItem.Type == model.WorkItemTypeCacheData {
			items = append(items, item)
		}
	}
	return items
}

func computeDataItems(result CompileStageResult) []CompileStageWorkItem {
	var items []CompileStageWorkItem
	for _, item := range result.WorkItems {
		if item.WorkItem.Type != model.WorkItemTypeCacheData {
			items = append(items, item)
		}
	}
	return items
}

const validWorkflowDataSHA256 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

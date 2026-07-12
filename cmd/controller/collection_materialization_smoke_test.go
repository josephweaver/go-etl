package main

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"goetl/internal/model"
	"goetl/internal/persistence"
	"goetl/internal/variable"
	"goetl/internal/workflow"
)

func TestCollectionMaterializationSmokeAggregatesAndHydratesThreeYearFixture(t *testing.T) {
	ctx := context.Background()
	store := openTestWorkflowExecutionStore(t)
	defer store.Close()
	controller := newController()
	controller.workflowStore = store
	run := insertTestPersistenceRunWithStage(t, ctx, store)

	logicalOutput, _, err := aggregateStepOutputJSON(collectionStep(2008, 2009, 2010))
	if err != nil {
		t.Fatalf("aggregateStepOutputJSON() error = %v", err)
	}
	if strings.HasPrefix(logicalOutput, "[") {
		t.Fatalf("logical output = %s, want compact collection object", logicalOutput)
	}
	var collection model.MaterializedAssetCollectionManifest
	if err := json.Unmarshal([]byte(logicalOutput), &collection); err != nil {
		t.Fatalf("decode collection output: %v", err)
	}
	if collection.Schema != model.MaterializedAssetCollectionManifestSchemaV1 ||
		collection.MemberCount != 3 ||
		collection.Path != "/mnt/cache/assets/cdl/${year}.tif" {
		t.Fatalf("collection output = %+v, want three-year cdl descriptor", collection)
	}
	years := collection.Dimensions["year"].Values
	if len(years) != 3 || years[0] != 2008 || years[1] != 2009 || years[2] != 2010 {
		t.Fatalf("collection years = %#v, want 2008, 2009, 2010", years)
	}

	scope, err := workflowStepScope(model.WorkflowDependencyPlan{
		RunID:      run.ID,
		WorkflowID: run.WorkflowID,
		State:      model.WorkflowStateRunning,
		Stages: []model.WorkflowDependencyStage{{
			StageIndex: 0,
			State:      model.WorkflowStageStateCompleted,
			Steps: []model.WorkflowDependencyStep{{
				StageIndex: 0,
				StepIndex:  0,
				StepID:     "materialize-cdl",
				State:      model.WorkflowStepStateCompleted,
				OutputJSON: logicalOutput,
			}},
		}},
	}, 1)
	if err != nil {
		t.Fatalf("workflowStepScope() error = %v", err)
	}
	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})
	workflowStep, err := resolver.Resolve(variable.Reference{Name: variable.Name{Namespace: variable.NamespaceWorkflow, Key: "step"}, Qualified: true})
	if err != nil {
		t.Fatalf("Resolve(workflow.step) error = %v", err)
	}
	secondYear, err := variable.ApplyAccessor(workflowStep, "[0].dimensions.year.values[1]")
	if err != nil {
		t.Fatalf("ApplyAccessor(year) error = %v", err)
	}
	if secondYear.Type != variable.TypeInt || secondYear.Value != 2009 {
		t.Fatalf("second year = %#v, want int 2009", secondYear)
	}

	for index, year := range []int{2008, 2009, 2010} {
		completeCollectionMemberForSmoke(t, ctx, store, run.ID, year, index)
	}

	for _, year := range []int{2008, 2009, 2010} {
		required := testSharedCollectionHydrationAsset("cdl", year)
		computeItem := testSharedHydrationComputeItem("inspect-cdl-"+strconv.Itoa(year), "target-local", []model.BoundDataAsset{required})
		claim := persistence.ClaimedWorkRecord{
			AttemptID: "attempt-inspect-cdl-" + strconv.Itoa(year),
			WorkItem:  persistence.WorkItemRecord{ID: run.ID + ":inspect-cdl-" + strconv.Itoa(year), RunID: run.ID, StageIndex: 1},
		}
		hydrated, err := controller.hydrateAssetMaterializeDependentWorkItem(ctx, claim, computeItem)
		if err != nil {
			t.Fatalf("hydrate year %d: %v", year, err)
		}
		manifest := materializedHydrationParameter(t, hydrated)
		if len(manifest.Assets) != 1 {
			t.Fatalf("year %d manifest assets = %+v, want one", year, manifest.Assets)
		}
		wantPath := "/target/cache/cdl/" + strconv.Itoa(year) + ".tif"
		if manifest.Assets[0].BindingName != "cdl" ||
			manifest.Assets[0].LocalPath != wantPath ||
			manifest.Assets[0].DestinationRelativePath != "cdl/"+strconv.Itoa(year)+".tif" {
			t.Fatalf("year %d hydrated asset = %+v, want concrete member path %s", year, manifest.Assets[0], wantPath)
		}
		projections, err := model.MaterializedDataProjections(manifest)
		if err != nil {
			t.Fatalf("year %d MaterializedDataProjections() error = %v", year, err)
		}
		if got := projections["cdl"].Path; len(got) != 1 || got[0] != wantPath {
			t.Fatalf("year %d data.cdl.path = %+v, want %s", year, got, wantPath)
		}
	}
}

func completeCollectionMemberForSmoke(t *testing.T, ctx context.Context, store *persistence.Store, runID string, year int, index int) {
	t.Helper()
	physical := testSharedCollectionHydrationAsset("cdl_materialize", year)
	assetKey, err := workflow.CanonicalDataAssetInstanceKey("cdl", nil, physical)
	if err != nil {
		t.Fatalf("year %d CanonicalDataAssetInstanceKey() error = %v", year, err)
	}
	destination := "cdl/" + strconv.Itoa(year) + ".tif"
	materializationKey, err := workflow.MaterializationIdentityKey(assetKey, "target-local", destination)
	if err != nil {
		t.Fatalf("year %d MaterializationIdentityKey() error = %v", year, err)
	}
	record := testHydrationCacheRecord(t, runID, "materialize-cdl--year-"+strconv.Itoa(year), 0)
	record.WorkItemIndex = index
	completeHydrationCacheRecord(t, ctx, store, record, testHydrationDestinationManifestJSON(
		t,
		assetKey,
		"target-local",
		"cdl_materialize",
		"/target/cache/cdl/"+strconv.Itoa(year)+".tif",
		destination,
		materializationKey,
		year,
	))
}

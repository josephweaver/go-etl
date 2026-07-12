package workflow

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"goetl/internal/model"
	"goetl/internal/variable"
)

func TestPlanDataAssetCollectionExpandsCDLRangeDeterministically(t *testing.T) {
	definition := testCDLCollectionDefinition()
	resolver := variable.NewResolver(variable.NewSet(), variable.ResolverConfig{})

	first, err := PlanDataAssetCollection(resolver, FanOutItemContext{}, "cdl", definition, nil, nil, "msu-hpcc")
	if err != nil {
		t.Fatalf("PlanDataAssetCollection(first) error = %v", err)
	}
	second, err := PlanDataAssetCollection(resolver, FanOutItemContext{}, "cdl", definition, nil, nil, "msu-hpcc")
	if err != nil {
		t.Fatalf("PlanDataAssetCollection(second) error = %v", err)
	}

	if len(first.Members) != 16 {
		t.Fatalf("member count = %d, want 16", len(first.Members))
	}
	if !reflect.DeepEqual(first.DimensionOrder, []string{"year"}) {
		t.Fatalf("dimension order = %#v", first.DimensionOrder)
	}
	if first.CollectionFingerprint != second.CollectionFingerprint {
		t.Fatalf("collection fingerprint changed: %s != %s", first.CollectionFingerprint, second.CollectionFingerprint)
	}
	for i, member := range first.Members {
		if member.Index != i {
			t.Fatalf("member index = %d, want %d", member.Index, i)
		}
		year := 2008 + i
		if member.Bindings["year"].Value != year {
			t.Fatalf("member %d year = %v, want %d", i, member.Bindings["year"].Value, year)
		}
		if member.DestinationRelativePath != "cdl/"+stringInt(year)+".tif" {
			t.Fatalf("member %d destination = %q", i, member.DestinationRelativePath)
		}
		if member.Instance.BoundAsset.Location.URI != "https://example.invalid/cdl/"+stringInt(year)+".zip" {
			t.Fatalf("member %d uri = %q", i, member.Instance.BoundAsset.Location.URI)
		}
		if member.MaterializationKey != second.Members[i].MaterializationKey {
			t.Fatalf("member %d materialization key changed", i)
		}
		if member.Instance.AssetKey != second.Members[i].Instance.AssetKey {
			t.Fatalf("member %d source asset key changed", i)
		}
	}
}

func TestPlanDataAssetCollectionUsesDeclaredDimensionOrder(t *testing.T) {
	definition := testTwoDimensionCollectionDefinition()
	resolver := variable.NewResolver(variable.NewSet(), variable.ResolverConfig{})

	plan, err := PlanDataAssetCollection(resolver, FanOutItemContext{}, "field_cdl", definition, nil, nil, "msu-hpcc")
	if err != nil {
		t.Fatalf("PlanDataAssetCollection() error = %v", err)
	}

	got := make([]string, 0, len(plan.Members))
	for _, member := range plan.Members {
		got = append(got, member.Bindings["tile"].Value.(string)+"-"+stringInt(member.Bindings["year"].Value.(int)))
	}
	want := []string{"h18v07-2010", "h18v07-2011", "h23v08-2010", "h23v08-2011"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("member order = %#v, want %#v", got, want)
	}
}

func TestPlanDataAssetCollectionResolvesFixedParametersOnce(t *testing.T) {
	definition := testCDLCollectionDefinition()
	definition.Parameters["region"] = model.DataParameterDefinition{Type: "string"}
	definition.Binding.Location.URLTemplate = "https://example.invalid/${region}/cdl/${year}.zip"
	definition.Binding.Materialization.PathTemplate = "cdl/${asset.region}/${asset.year}.tif"
	scope, err := variable.NewScope(variable.Variable{
		Name: variable.Name{Namespace: variable.NamespaceWorkflow, Key: "region"},
		TypedExpression: variable.TypedExpression{
			Type:       variable.TypeString,
			Expression: "midwest",
		},
	})
	if err != nil {
		t.Fatalf("NewScope() error = %v", err)
	}
	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})

	plan, err := PlanDataAssetCollection(resolver, FanOutItemContext{}, "cdl", definition, nil, map[string]variable.TypedExpression{
		"region": {Type: variable.TypeString, Expression: "${workflow.region}"},
	}, "msu-hpcc")
	if err != nil {
		t.Fatalf("PlanDataAssetCollection() error = %v", err)
	}

	if plan.FixedParameters["region"].Value != "midwest" {
		t.Fatalf("fixed region = %v", plan.FixedParameters["region"].Value)
	}
	if plan.PathTemplate != "cdl/midwest/${year}.tif" {
		t.Fatalf("path template = %q", plan.PathTemplate)
	}
	if plan.Members[0].DestinationRelativePath != "cdl/midwest/2008.tif" {
		t.Fatalf("destination = %q", plan.Members[0].DestinationRelativePath)
	}
}

func TestPlanDataAssetCollectionRejectsDimensionOverrideAndMissingFixedParameter(t *testing.T) {
	resolver := variable.NewResolver(variable.NewSet(), variable.ResolverConfig{})

	_, err := PlanDataAssetCollection(resolver, FanOutItemContext{}, "cdl", testCDLCollectionDefinition(), nil, map[string]variable.TypedExpression{
		"year": {Type: variable.TypeInt, Expression: 2017},
	}, "msu-hpcc")
	if err == nil || !strings.Contains(err.Error(), `dimension parameter "year" cannot be overridden`) {
		t.Fatalf("dimension override error = %v", err)
	}

	definition := testCDLCollectionDefinition()
	definition.Parameters["region"] = model.DataParameterDefinition{Type: "string"}
	definition.Binding.Materialization.PathTemplate = "cdl/${asset.region}/${asset.year}.tif"
	_, err = PlanDataAssetCollection(resolver, FanOutItemContext{}, "cdl", definition, nil, nil, "msu-hpcc")
	if err == nil || !strings.Contains(err.Error(), `missing asset parameter "region"`) {
		t.Fatalf("missing fixed parameter error = %v", err)
	}
}

func TestPlanDataAssetCollectionRejectsDestinationCollisions(t *testing.T) {
	definition := testCollisionCollectionDefinition()
	resolver := variable.NewResolver(variable.NewSet(), variable.ResolverConfig{})

	_, err := PlanDataAssetCollection(resolver, FanOutItemContext{}, "collision_asset", definition, nil, nil, "msu-hpcc")
	if err == nil || !strings.Contains(err.Error(), "collision") || !strings.Contains(err.Error(), "source asset keys") {
		t.Fatalf("collision error = %v", err)
	}
}

func TestPlanDataAssetCollectionMaterializationIdentityIncludesDestination(t *testing.T) {
	definition := testScalarMaterializedDefinition("first/${asset.year}.tif")
	resolver := variable.NewResolver(variable.NewSet(), variable.ResolverConfig{})
	bindings := map[string]variable.TypedExpression{
		"year": {Type: variable.TypeInt, Expression: 2024},
	}

	first, err := PlanDataAssetCollection(resolver, FanOutItemContext{}, "cdl", definition, nil, bindings, "msu-hpcc")
	if err != nil {
		t.Fatalf("PlanDataAssetCollection(first) error = %v", err)
	}
	secondDefinition := testScalarMaterializedDefinition("second/${asset.year}.tif")
	second, err := PlanDataAssetCollection(resolver, FanOutItemContext{}, "cdl", secondDefinition, nil, bindings, "msu-hpcc")
	if err != nil {
		t.Fatalf("PlanDataAssetCollection(second) error = %v", err)
	}

	if len(first.Members) != 1 || len(second.Members) != 1 {
		t.Fatalf("scalar member counts = %d and %d", len(first.Members), len(second.Members))
	}
	if first.Members[0].Instance.AssetKey != second.Members[0].Instance.AssetKey {
		t.Fatalf("source asset keys differ: %s != %s", first.Members[0].Instance.AssetKey, second.Members[0].Instance.AssetKey)
	}
	if first.Members[0].MaterializationKey == second.Members[0].MaterializationKey {
		t.Fatalf("materialization keys match despite different destinations: %s", first.Members[0].MaterializationKey)
	}
}

func TestPlanDataAssetCollectionSourceIdentityExcludesStepAlias(t *testing.T) {
	definition := testCDLCollectionDefinition()
	resolver := variable.NewResolver(variable.NewSet(), variable.ResolverConfig{})

	first, err := PlanDataAssetCollection(resolver, FanOutItemContext{}, "cdl", definition, nil, nil, "msu-hpcc")
	if err != nil {
		t.Fatalf("PlanDataAssetCollection(first) error = %v", err)
	}
	second, err := PlanDataAssetCollection(resolver, FanOutItemContext{}, "cdl", definition, nil, nil, "msu-hpcc")
	if err != nil {
		t.Fatalf("PlanDataAssetCollection(second) error = %v", err)
	}
	if first.Members[0].Instance.AssetKey != second.Members[0].Instance.AssetKey {
		t.Fatalf("asset keys differ for same definition: %s != %s", first.Members[0].Instance.AssetKey, second.Members[0].Instance.AssetKey)
	}
}

func testCDLCollectionDefinition() model.DataInputDefinition {
	required := true
	return model.DataInputDefinition{
		Kind:   "raster",
		Format: "geotiff",
		Parameters: map[string]model.DataParameterDefinition{
			"year": {Type: "int"},
		},
		Collection: &model.DataAssetCollectionDefinition{
			Dimensions: []model.DataAssetCollectionDimension{
				{Parameter: "year", Range: &model.DataAssetCollectionRange{From: 2008, Through: 2023}},
			},
		},
		Files: map[string]model.DataFileRoleDefinition{
			"raster": {Member: "${asset.year}_30m_cdls.tif", Required: &required},
		},
		Select: []string{"raster"},
		Binding: model.DataInputBindingDefinition{
			Provider: model.DataProviderHTTP,
			Location: model.DataDefinitionLocation{
				URLTemplate: "https://example.invalid/cdl/${year}.zip",
			},
			Archive: model.DataArchiveBindingDefinition{
				Type:   model.DataAssetArchiveTypeZip,
				Expose: model.DataAssetArchiveExposeSelectedPath,
			},
			Materialization: model.DataDefinitionMaterialization{
				Scope:        "shared",
				Strategy:     model.DataAssetCacheStrategyWorkerCache,
				PathTemplate: "cdl/${asset.year}.tif",
			},
		},
	}
}

func testTwoDimensionCollectionDefinition() model.DataInputDefinition {
	definition := testCDLCollectionDefinition()
	definition.Parameters["tile"] = model.DataParameterDefinition{Type: "string"}
	definition.Collection = &model.DataAssetCollectionDefinition{
		Dimensions: []model.DataAssetCollectionDimension{
			{Parameter: "tile", Values: []any{"h18v07", "h23v08"}},
			{Parameter: "year", Range: &model.DataAssetCollectionRange{From: 2010, Through: 2011}},
		},
	}
	definition.Binding.Location.URLTemplate = "https://example.invalid/${tile}/${year}.zip"
	definition.Files["raster"] = model.DataFileRoleDefinition{Member: "${asset.tile}/${asset.year}.tif"}
	definition.Binding.Materialization.PathTemplate = "cdl/${asset.tile}/${asset.year}.tif"
	return definition
}

func testCollisionCollectionDefinition() model.DataInputDefinition {
	definition := testCDLCollectionDefinition()
	definition.Parameters = map[string]model.DataParameterDefinition{
		"left":  {Type: "string"},
		"right": {Type: "string"},
	}
	definition.Collection = &model.DataAssetCollectionDefinition{
		Dimensions: []model.DataAssetCollectionDimension{
			{Parameter: "left", Values: []any{"a", "ab"}},
			{Parameter: "right", Values: []any{"bc", "c"}},
		},
	}
	definition.Binding.Location.URLTemplate = "https://example.invalid/${left}-${right}.zip"
	definition.Files["raster"] = model.DataFileRoleDefinition{Member: "${asset.left}-${asset.right}.tif"}
	definition.Binding.Materialization.PathTemplate = "out/${asset.left}${asset.right}.tif"
	return definition
}

func testScalarMaterializedDefinition(pathTemplate string) model.DataInputDefinition {
	definition := testCDLCollectionDefinition()
	definition.Collection = nil
	definition.Binding.Location.URLTemplate = "https://example.invalid/cdl/${year}.zip"
	definition.Binding.Materialization.PathTemplate = pathTemplate
	return definition
}

func stringInt(value int) string {
	return strconv.Itoa(value)
}

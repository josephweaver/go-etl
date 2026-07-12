package workflow

import (
	"strings"
	"testing"

	"goetl/internal/model"
	"goetl/internal/variable"
)

func TestInstantiateDataAssetProducesDistinctKeysForTiles(t *testing.T) {
	definition := testYanRoyInstanceDefinition()
	resolver := variable.NewResolver(variable.NewSet(), variable.ResolverConfig{})

	first, err := InstantiateDataAsset(resolver, fanOutTile("h18v07"), "yan_roy_field_segments", definition, nil, map[string]variable.TypedExpression{
		"tile": {Type: variable.TypeString, Expression: "${fanout.tile}"},
	})
	if err != nil {
		t.Fatalf("InstantiateDataAsset(first) error = %v", err)
	}
	second, err := InstantiateDataAsset(resolver, fanOutTile("h18v08"), "yan_roy_field_segments", definition, nil, map[string]variable.TypedExpression{
		"tile": {Type: variable.TypeString, Expression: "${fanout.tile}"},
	})
	if err != nil {
		t.Fatalf("InstantiateDataAsset(second) error = %v", err)
	}

	if first.AssetKey == second.AssetKey {
		t.Fatalf("asset keys match for different tiles: %s", first.AssetKey)
	}
	if got := first.BoundAsset.Archive.Select[0].Member; got != "h18v07/WELD_h18v07_2010_field_segments" {
		t.Fatalf("first raster member = %q", got)
	}
	if got := second.BoundAsset.Archive.Select[0].Member; got != "h18v08/WELD_h18v08_2010_field_segments" {
		t.Fatalf("second raster member = %q", got)
	}
}

func TestInstantiateDataAssetIdentityExcludesStepAlias(t *testing.T) {
	definition := testYanRoyInstanceDefinition()
	resolver := variable.NewResolver(variable.NewSet(), variable.ResolverConfig{})
	bindings := map[string]variable.TypedExpression{
		"tile": {Type: variable.TypeString, Expression: "${fanout.tile}"},
	}

	fieldSegments, err := InstantiateDataAsset(resolver, fanOutTile("h18v07"), "yan_roy_field_segments", definition, nil, bindings)
	if err != nil {
		t.Fatalf("InstantiateDataAsset(field_segments) error = %v", err)
	}
	reusedHeader, err := InstantiateDataAsset(resolver, fanOutTile("h18v07"), "yan_roy_field_segments", definition, nil, bindings)
	if err != nil {
		t.Fatalf("InstantiateDataAsset(reused_header) error = %v", err)
	}

	if fieldSegments.AssetKey != reusedHeader.AssetKey {
		t.Fatalf("same asset instance got different keys: %s != %s", fieldSegments.AssetKey, reusedHeader.AssetKey)
	}
}

func TestInstantiateDataAssetSelectionChangesIdentity(t *testing.T) {
	definition := testYanRoyInstanceDefinition()
	resolver := variable.NewResolver(variable.NewSet(), variable.ResolverConfig{})
	bindings := map[string]variable.TypedExpression{
		"tile": {Type: variable.TypeString, Expression: "${fanout.tile}"},
	}

	headerOnly, err := InstantiateDataAsset(resolver, fanOutTile("h18v07"), "yan_roy_field_segments", definition, []string{"header"}, bindings)
	if err != nil {
		t.Fatalf("InstantiateDataAsset(header) error = %v", err)
	}
	both, err := InstantiateDataAsset(resolver, fanOutTile("h18v07"), "yan_roy_field_segments", definition, []string{"raster", "header"}, bindings)
	if err != nil {
		t.Fatalf("InstantiateDataAsset(both) error = %v", err)
	}

	if headerOnly.AssetKey == both.AssetKey {
		t.Fatalf("asset keys match for different selections: %s", headerOnly.AssetKey)
	}
	if len(headerOnly.BoundAsset.Archive.Select) != 1 {
		t.Fatalf("header-only selected %d members", len(headerOnly.BoundAsset.Archive.Select))
	}
	if headerOnly.BoundAsset.Archive.Select[0].Member != "h18v07/WELD_h18v07_2010_field_segments.hdr" {
		t.Fatalf("header member = %q", headerOnly.BoundAsset.Archive.Select[0].Member)
	}
}

func TestInstantiateDataAssetRejectsMissingAndUnknownParameters(t *testing.T) {
	definition := testYanRoyInstanceDefinition()
	resolver := variable.NewResolver(variable.NewSet(), variable.ResolverConfig{})

	_, err := InstantiateDataAsset(resolver, fanOutTile("h18v07"), "yan_roy_field_segments", definition, nil, nil)
	if err == nil || !strings.Contains(err.Error(), `missing asset parameter "tile"`) {
		t.Fatalf("missing parameter error = %v", err)
	}

	_, err = InstantiateDataAsset(resolver, fanOutTile("h18v07"), "yan_roy_field_segments", definition, nil, map[string]variable.TypedExpression{
		"tile": {Type: variable.TypeString, Expression: "${fanout.tile}"},
		"year": {Type: variable.TypeInt, Expression: 2026},
	})
	if err == nil || !strings.Contains(err.Error(), `unknown asset parameter "year"`) {
		t.Fatalf("unknown parameter error = %v", err)
	}
}

func TestInstantiateDataAssetCanResolveWorkflowParameterReference(t *testing.T) {
	definition := testYanRoyInstanceDefinition()
	scope, err := variable.NewScope(variable.Variable{
		Name: variable.Name{Namespace: variable.NamespaceWorkflow, Key: "tile"},
		TypedExpression: variable.TypedExpression{
			Type:       variable.TypeString,
			Expression: "h18v07",
		},
	})
	if err != nil {
		t.Fatalf("NewScope() error = %v", err)
	}
	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})

	instance, err := InstantiateDataAsset(resolver, variable.ResolvedValue{}, "yan_roy_field_segments", definition, []string{"header"}, map[string]variable.TypedExpression{
		"tile": {Type: variable.TypeString, Expression: "${workflow.tile}"},
	})
	if err != nil {
		t.Fatalf("InstantiateDataAsset() error = %v", err)
	}
	if !strings.Contains(instance.Diagnostic, "tile=h18v07") {
		t.Fatalf("diagnostic = %q", instance.Diagnostic)
	}
}

func TestInstantiateDataAssetCarriesDefinitionNameAndConcreteMaterializationPath(t *testing.T) {
	definition := testScalarMaterializedDefinition("cdl/${asset.year}.tif")
	resolver := variable.NewResolver(variable.NewSet(), variable.ResolverConfig{})

	instance, err := InstantiateDataAsset(resolver, variable.ResolvedValue{}, "cdl", definition, nil, map[string]variable.TypedExpression{
		"year": {Type: variable.TypeInt, Expression: 2024},
	})
	if err != nil {
		t.Fatalf("InstantiateDataAsset() error = %v", err)
	}
	if instance.BoundAsset.DefinitionName != "cdl" {
		t.Fatalf("definition_name = %q, want cdl", instance.BoundAsset.DefinitionName)
	}
	if instance.BoundAsset.Materialization.PathTemplate != "cdl/2024.tif" {
		t.Fatalf("materialization path = %q, want concrete destination", instance.BoundAsset.Materialization.PathTemplate)
	}
}

func testYanRoyInstanceDefinition() model.DataInputDefinition {
	required := true
	immutable := true
	return model.DataInputDefinition{
		Kind: "envi_field_segments",
		Parameters: map[string]model.DataParameterDefinition{
			"tile": {Type: "string"},
		},
		Files: map[string]model.DataFileRoleDefinition{
			"raster": {
				Member:   "${asset.tile}/WELD_${asset.tile}_2010_field_segments",
				As:       "WELD_${asset.tile}_2010_field_segments",
				Required: &required,
			},
			"header": {
				Member:   "${asset.tile}/WELD_${asset.tile}_2010_field_segments.hdr",
				As:       "WELD_${asset.tile}_2010_field_segments.hdr",
				Required: &required,
			},
		},
		Select: []string{"raster", "header"},
		Binding: model.DataInputBindingDefinition{
			ProviderName: "gdrive_release_data",
			Provider:     model.DataProviderGDriveRclone,
			Location: model.DataDefinitionLocation{
				Remote:    "gdrive",
				DrivePath: "Data/Field_Boundaries/ReleaseData.7z",
			},
			Archive: model.DataArchiveBindingDefinition{
				Type:   model.DataAssetArchiveTypeSevenZip,
				Expose: model.DataAssetArchiveExposeSelectedDirectory,
			},
			Cache: model.DataDefinitionCache{
				Strategy:  model.DataAssetCacheStrategyWorkerCache,
				CacheKey:  "gdrive/field_boundaries/release-data/source.7z",
				Immutable: &immutable,
			},
			Materialization: model.DataDefinitionMaterialization{
				Scope:    "shared",
				Strategy: model.DataAssetCacheStrategyWorkerCache,
			},
		},
	}
}

func fanOutTile(tile string) variable.ResolvedValue {
	return variable.ResolvedObject(map[string]variable.ResolvedValue{
		"tile": {Type: variable.TypeString, Value: tile},
	})
}

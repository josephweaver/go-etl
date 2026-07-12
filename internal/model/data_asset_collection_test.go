package model

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestDataInputDefinitionCollectionRangeValidatesAndReportsCardinality(t *testing.T) {
	definition := cdlCollectionDefinition()

	if err := definition.Validate("cdl"); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	cardinality, err := definition.CollectionCardinality()
	if err != nil {
		t.Fatalf("CollectionCardinality() error = %v", err)
	}
	if cardinality != 16 {
		t.Fatalf("cardinality = %d, want 16", cardinality)
	}
	values, err := definition.Collection.Dimensions[0].DomainValues(definition.Parameters["year"])
	if err != nil {
		t.Fatalf("DomainValues() error = %v", err)
	}
	if values[0] != 2008 || values[len(values)-1] != 2023 {
		t.Fatalf("range values endpoints = %v ... %v, want 2008 ... 2023", values[0], values[len(values)-1])
	}
}

func TestDataInputDefinitionNoCollectionRemainsScalar(t *testing.T) {
	definition := cdlCollectionDefinition()
	definition.Collection = nil

	if err := definition.Validate("cdl"); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	cardinality, err := definition.CollectionCardinality()
	if err != nil {
		t.Fatalf("CollectionCardinality() error = %v", err)
	}
	if cardinality != 1 {
		t.Fatalf("cardinality = %d, want scalar cardinality 1", cardinality)
	}
}

func TestDataAssetCollectionExplicitValuesValidateByParameterType(t *testing.T) {
	tests := []struct {
		name      string
		parameter DataParameterDefinition
		values    []any
	}{
		{name: "string", parameter: DataParameterDefinition{Type: "string"}, values: []any{"h18v07", "h23v08"}},
		{name: "bool", parameter: DataParameterDefinition{Type: "bool"}, values: []any{true, false}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			collection := DataAssetCollectionDefinition{
				Dimensions: []DataAssetCollectionDimension{
					{Parameter: "value", Values: test.values},
				},
			}
			cardinality, err := collection.Cardinality(map[string]DataParameterDefinition{"value": test.parameter})
			if err != nil {
				t.Fatalf("Cardinality() error = %v", err)
			}
			if cardinality != uint64(len(test.values)) {
				t.Fatalf("cardinality = %d, want %d", cardinality, len(test.values))
			}
		})
	}
}

func TestDataAssetCollectionPreservesDimensionOrder(t *testing.T) {
	collection := DataAssetCollectionDefinition{
		Dimensions: []DataAssetCollectionDimension{
			{Parameter: "year", Values: []any{2020}},
			{Parameter: "tile", Values: []any{"h18v07"}},
		},
	}

	if _, err := collection.Cardinality(map[string]DataParameterDefinition{
		"year": {Type: "int"},
		"tile": {Type: "string"},
	}); err != nil {
		t.Fatalf("Cardinality() error = %v", err)
	}
	got := []string{collection.Dimensions[0].Parameter, collection.Dimensions[1].Parameter}
	if !reflect.DeepEqual(got, []string{"year", "tile"}) {
		t.Fatalf("dimension order = %#v", got)
	}
}

func TestDataAssetCollectionRejectsInvalidDimensions(t *testing.T) {
	parameters := map[string]DataParameterDefinition{
		"year": {Type: "int"},
		"tile": {Type: "string"},
		"flag": {Type: "bool"},
	}
	tests := []struct {
		name       string
		dimension  DataAssetCollectionDimension
		parameters map[string]DataParameterDefinition
		want       string
	}{
		{
			name:      "unknown parameter",
			dimension: DataAssetCollectionDimension{Parameter: "missing", Values: []any{2020}},
			want:      `parameter "missing" is not defined`,
		},
		{
			name:      "wrong explicit value type",
			dimension: DataAssetCollectionDimension{Parameter: "year", Values: []any{"2020"}},
			want:      "want int",
		},
		{
			name:      "range on non int",
			dimension: DataAssetCollectionDimension{Parameter: "tile", Range: &DataAssetCollectionRange{From: 1, Through: 2}},
			want:      `range requires int parameter`,
		},
		{
			name:      "descending range",
			dimension: DataAssetCollectionDimension{Parameter: "year", Range: &DataAssetCollectionRange{From: 2023, Through: 2008}},
			want:      "range from must be less than or equal to through",
		},
		{
			name:      "both values and range",
			dimension: DataAssetCollectionDimension{Parameter: "year", Values: []any{2020}, Range: &DataAssetCollectionRange{From: 2020, Through: 2021}},
			want:      "must supply values or range, not both",
		},
		{
			name:      "neither values nor range",
			dimension: DataAssetCollectionDimension{Parameter: "year"},
			want:      "must supply values or range",
		},
		{
			name:      "empty values",
			dimension: DataAssetCollectionDimension{Parameter: "year", Values: []any{}},
			want:      "values must not be empty",
		},
		{
			name:      "repeated values",
			dimension: DataAssetCollectionDimension{Parameter: "year", Values: []any{2020, 2020}},
			want:      "duplicates an earlier value",
		},
		{
			name:      "object value",
			dimension: DataAssetCollectionDimension{Parameter: "year", Values: []any{map[string]any{"year": 2020}}},
			want:      "want scalar string, int, or bool",
		},
		{
			name:       "unsupported parameter type",
			dimension:  DataAssetCollectionDimension{Parameter: "flag", Values: []any{true}},
			parameters: map[string]DataParameterDefinition{"flag": {Type: "float"}},
			want:       `unsupported parameter type "float"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testParameters := parameters
			if test.parameters != nil {
				testParameters = test.parameters
			}
			collection := DataAssetCollectionDefinition{Dimensions: []DataAssetCollectionDimension{test.dimension}}
			_, err := collection.Cardinality(testParameters)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Cardinality() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestDataAssetCollectionRejectsDuplicateDimensions(t *testing.T) {
	collection := DataAssetCollectionDefinition{
		Dimensions: []DataAssetCollectionDimension{
			{Parameter: "year", Values: []any{2020}},
			{Parameter: "year", Values: []any{2021}},
		},
	}

	_, err := collection.Cardinality(map[string]DataParameterDefinition{"year": {Type: "int"}})
	if err == nil || !strings.Contains(err.Error(), `duplicates parameter "year"`) {
		t.Fatalf("Cardinality() error = %v, want duplicate parameter error", err)
	}
}

func TestDataAssetCollectionRejectsCardinalityOverflow(t *testing.T) {
	parameters := make(map[string]DataParameterDefinition)
	dimensions := make([]DataAssetCollectionDimension, 0, 64)
	for i := 0; i < 64; i++ {
		name := fmt.Sprintf("p%d", i)
		parameters[name] = DataParameterDefinition{Type: "bool"}
		dimensions = append(dimensions, DataAssetCollectionDimension{Parameter: name, Values: []any{false, true}})
	}
	collection := DataAssetCollectionDefinition{Dimensions: dimensions}

	_, err := collection.Cardinality(parameters)
	if err == nil || !strings.Contains(err.Error(), "collection cardinality overflow") {
		t.Fatalf("Cardinality() error = %v, want overflow error", err)
	}
}

func TestDataAssetCollectionJSONRoundTripPreservesOrderAndValueTypes(t *testing.T) {
	encoded := []byte(`{
		"kind": "raster",
		"parameters": {
			"year": {"type": "int"},
			"tile": {"type": "string"},
			"mask": {"type": "bool"}
		},
		"collection": {
			"dimensions": [
				{"parameter": "year", "values": [2008, 2023]},
				{"parameter": "tile", "values": ["h18v07"]},
				{"parameter": "mask", "values": [true, false]}
			]
		},
		"files": {
			"raster": {"member": "${asset.year}/${asset.tile}.tif", "required": true}
		},
		"select": ["raster"],
		"binding": {
			"provider": "http",
			"location": {"url_template": "https://example.invalid/${year}/${tile}.zip"},
			"materialization": {"scope": "shared", "strategy": "worker_cache"}
		}
	}`)

	var decoded DataInputDefinition
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if err := decoded.Validate("cdl"); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if decoded.Collection.Dimensions[0].Parameter != "year" || decoded.Collection.Dimensions[1].Parameter != "tile" {
		t.Fatalf("dimension order = %#v", decoded.Collection.Dimensions)
	}
	if _, ok := decoded.Collection.Dimensions[0].Values[0].(int); !ok {
		t.Fatalf("year value type = %T, want int", decoded.Collection.Dimensions[0].Values[0])
	}
	if _, ok := decoded.Collection.Dimensions[1].Values[0].(string); !ok {
		t.Fatalf("tile value type = %T, want string", decoded.Collection.Dimensions[1].Values[0])
	}
	if _, ok := decoded.Collection.Dimensions[2].Values[0].(bool); !ok {
		t.Fatalf("mask value type = %T, want bool", decoded.Collection.Dimensions[2].Values[0])
	}

	roundTrip, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decodedAgain DataInputDefinition
	if err := json.Unmarshal(roundTrip, &decodedAgain); err != nil {
		t.Fatalf("round-trip Unmarshal() error = %v", err)
	}
	if !reflect.DeepEqual(decoded.Collection.Dimensions, decodedAgain.Collection.Dimensions) {
		t.Fatalf("round-trip dimensions = %#v, want %#v", decodedAgain.Collection.Dimensions, decoded.Collection.Dimensions)
	}
}

func cdlCollectionDefinition() DataInputDefinition {
	required := true
	return DataInputDefinition{
		Kind:   "raster",
		Format: "geotiff",
		Parameters: map[string]DataParameterDefinition{
			"year": {Type: "int"},
		},
		Collection: &DataAssetCollectionDefinition{
			Dimensions: []DataAssetCollectionDimension{
				{Parameter: "year", Range: &DataAssetCollectionRange{From: 2008, Through: 2023}},
			},
		},
		Files: map[string]DataFileRoleDefinition{
			"raster": {Member: "${asset.year}_30m_cdls.tif", Required: &required},
		},
		Select: []string{"raster"},
		Binding: DataInputBindingDefinition{
			Provider: DataProviderHTTP,
			Location: DataDefinitionLocation{
				URLTemplate: "https://example.invalid/cdl/${year}_30m_cdls.zip",
			},
			Materialization: DataDefinitionMaterialization{
				Scope:    "shared",
				Strategy: DataAssetCacheStrategyWorkerCache,
			},
		},
	}
}

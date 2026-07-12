package workflow

import (
	"strings"
	"testing"

	"goetl/internal/document"
	"goetl/internal/model"
	"goetl/internal/variable"
)

func TestCanonicalCrossproductFanOutPreservesPairTypesAndTemplates(t *testing.T) {
	source := []byte(`
api_version: goet/v1alpha1
kind: Workflow
id: pair-fanout
variables:
  years:
    - 2008
    - 2009
  tiles:
    - h18v07
    - h18v08
  pairs:
    $type: list
    $call: list.crossproduct
    $args:
      - $ref: years
      - $ref: tiles
steps:
  - id: process-pair
    fan_out:
      over: ${workflow.pairs[*]}
      as: pair
      id: ${pair[0]}-${pair[1]}
      output: ${pair[1]}-${pair[0]}
    work:
      type: write_demo_output
      output_prefix: counts
      output_extension: .json
      parameters:
        year: ${pair[0]}
        tile: ${pair[1]}
        generic_tile: ${fanout[1]}
        pair: ${pair}
        label: year-${pair[0]}-tile-${pair[1]}
`)
	items := compileCanonicalFanOutRefinementItems(t, source)
	if len(items) != 4 {
		t.Fatalf("item count = %d, want 4", len(items))
	}

	first := items[0]
	if first.ID != "process-pair-2008-h18v07" {
		t.Fatalf("first id = %q", first.ID)
	}
	if first.OutputFilename != "counts-h18v07-2008.json" {
		t.Fatalf("first output filename = %q", first.OutputFilename)
	}
	assertParameter(t, first.Parameters["year"], "int", 2008)
	assertParameter(t, first.Parameters["tile"], "string", "h18v07")
	assertParameter(t, first.Parameters["generic_tile"], "string", "h18v07")
	assertParameter(t, first.Parameters["label"], "string", "year-2008-tile-h18v07")
	pair, ok := first.Parameters["pair"].Value.([]any)
	if !ok || len(pair) != 2 || pair[0] != 2008 || pair[1] != "h18v07" {
		t.Fatalf("pair parameter = %#v", first.Parameters["pair"])
	}

	wantIDs := []string{
		"process-pair-2008-h18v07",
		"process-pair-2008-h18v08",
		"process-pair-2009-h18v07",
		"process-pair-2009-h18v08",
	}
	for index, want := range wantIDs {
		if items[index].ID != want {
			t.Fatalf("item %d id = %q, want %q", index, items[index].ID, want)
		}
	}
}

func TestCanonicalPairIndexesBindDataAssetParameters(t *testing.T) {
	source := []byte(`
api_version: goet/v1alpha1
kind: Workflow
id: pair-data-binding
variables:
  years:
    - 2008
  tiles:
    - h18v07
  pairs:
    $type: list
    $call: list.crossproduct
    $args:
      - $ref: years
      - $ref: tiles
data:
  inputs:
    field_segments:
      kind: fixture_segments
      parameters:
        year:
          type: int
        tile:
          type: string
      files:
        segment:
          member: ${asset.year}/${asset.tile}/segments.csv
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
  - id: cache-pair
    fan_out:
      over: ${workflow.pairs[*]}
      as: pair
      id: ${pair[0]}-${pair[1]}
    data:
      materialize:
        field_segments:
          asset: field_segments
          with:
            year: ${pair[0]}
            tile: ${fanout[1]}
    work:
      type: asset.materialize
      parameters:
        target_environment_id: target-local
`)
	items := compileCanonicalFanOutRefinementItems(t, source)
	if len(items) != 1 {
		t.Fatalf("item count = %d, want 1", len(items))
	}
	assets, err := boundDataAssetsFromParameters(items[0].Parameters)
	if err != nil {
		t.Fatalf("boundDataAssetsFromParameters() error = %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("asset count = %d, want 1", len(assets))
	}
	tile, tileOK := assets[0].Parameters["tile"].(string)
	if !numericParameterEquals(assets[0].Parameters["year"], 2008) || !tileOK || tile != "h18v07" {
		t.Fatalf("asset parameters = %#v; year type=%T tile type=%T", assets[0].Parameters, assets[0].Parameters["year"], assets[0].Parameters["tile"])
	}
	if got := assets[0].Archive.Select[0].Member; got != "2008/h18v07/segments.csv" {
		t.Fatalf("asset member = %q", got)
	}
}

func TestCompileFanOutWorkItemsRejectsDuplicateRenderedOutput(t *testing.T) {
	resolver := testWorkflowResolver(t, 2024, 2025)
	_, err := CompileFanOutWorkItems(resolver, FanOutWorkItemTemplate{
		FanOutExpression: "${years[*]}",
		IDTemplate:       "${fanout}",
		OutputTemplate:   "same-output",
		Type:             model.WorkItemTypeWriteDemoOutput,
		IDPrefix:         "years",
		OutputPrefix:     "years",
		OutputExtension:  ".txt",
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate output filename") {
		t.Fatalf("CompileFanOutWorkItems() error = %v, want duplicate output filename", err)
	}
}

func TestCompileFanOutWorkItemsRejectsUnsafeRenderedID(t *testing.T) {
	resolver := testWorkflowResolver(t, 2024)
	_, err := CompileFanOutWorkItems(resolver, FanOutWorkItemTemplate{
		FanOutExpression: "${years[*]}",
		IDTemplate:       " ${fanout}",
		Type:             model.WorkItemTypeWriteDemoOutput,
		IDPrefix:         "years",
		OutputPrefix:     "years",
		OutputExtension:  ".txt",
	})
	if err == nil || !strings.Contains(err.Error(), "leading or trailing whitespace") {
		t.Fatalf("CompileFanOutWorkItems() error = %v, want whitespace rejection", err)
	}
}

func TestCompileFanOutWorkItemsRejectsSensitiveRenderedID(t *testing.T) {
	scope, err := variable.NewScope(
		variable.Variable{
			Name: variable.Name{Namespace: variable.NamespaceWorkflow, Key: "tokens"},
			TypedExpression: variable.TypedExpression{Type: variable.TypeList, Expression: []variable.TypedExpression{
				{Type: variable.TypeString, Expression: "${secret_token}"},
			}},
		},
		variable.Variable{
			Name:            variable.Name{Namespace: variable.NamespaceWorkflow, Key: "secret_token"},
			TypedExpression: variable.TypedExpression{Type: variable.TypeString},
			ProtectedRef:    &variable.ProtectedRef{Provider: "worker_env", Key: "SECRET_TOKEN"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})
	_, err = CompileFanOutWorkItems(resolver, FanOutWorkItemTemplate{
		FanOutExpression: "${workflow.tokens[*]}",
		IDTemplate:       "${fanout}",
		Type:             model.WorkItemTypeWriteDemoOutput,
		IDPrefix:         "token",
		OutputPrefix:     "token",
		OutputExtension:  ".txt",
	})
	if err == nil || !strings.Contains(err.Error(), "sensitive value") {
		t.Fatalf("CompileFanOutWorkItems() error = %v, want sensitive rejection", err)
	}
	if strings.Contains(err.Error(), "SECRET_TOKEN") && !strings.Contains(err.Error(), "${worker_env.SECRET_TOKEN}") {
		t.Fatalf("CompileFanOutWorkItems() error leaked unexpected secret detail: %v", err)
	}
}

func compileCanonicalFanOutRefinementItems(t *testing.T, source []byte) []model.WorkItem {
	t.Helper()
	doc, err := document.DecodeCanonicalWorkflowSource(source, document.DecodeOptions{Format: document.SourceFormatYAML})
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
	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{
		FunctionRegistry: variable.DefaultFunctionRegistry(),
	})
	items, err := CompileWorkflow(resolver, workflow)
	if err != nil {
		t.Fatalf("CompileWorkflow() error = %v", err)
	}
	return items
}

func assertParameter(t *testing.T, parameter model.Parameter, wantType string, wantValue any) {
	t.Helper()
	if parameter.Type != wantType || parameter.Value != wantValue {
		t.Fatalf("parameter = %+v, want type=%s value=%#v", parameter, wantType, wantValue)
	}
}

func numericParameterEquals(value any, want int) bool {
	switch typed := value.(type) {
	case int:
		return typed == want
	case float64:
		return typed == float64(want)
	default:
		return false
	}
}

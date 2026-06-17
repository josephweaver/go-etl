package workflow

import (
	"testing"

	"goetl/internal/model"
	"goetl/internal/variable"
)

func TestCompileFanOutWorkItems(t *testing.T) {
	scope, err := variable.NewScope(variable.Variable{
		Name:       variable.Name{Namespace: variable.NamespaceWorkflow, Key: "years"},
		Type:       variable.TypeList(variable.TypeInt),
		Expression: `[2024, 2025]`,
	})
	if err != nil {
		t.Fatal(err)
	}

	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})

	items, err := CompileFanOutWorkItems(resolver, FanOutWorkItemTemplate{
		FanOutExpression: "${years[*]}",
		Type:             model.WorkItemTypeWriteDemoOutput,
		IDPrefix:         "cdl",
		OutputPrefix:     "cdl",
		OutputExtension:  ".txt",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("unexpected item count: %d", len(items))
	}

	if items[0].ID != "cdl-2024" {
		t.Fatalf("unexpected first id: %s", items[0].ID)
	}

	if items[1].OutputFilename != "cdl-2025.txt" {
		t.Fatalf("unexpected second output filename: %s", items[1].OutputFilename)
	}
}

func TestCompileFanOutStep(t *testing.T) {
	scope, err := variable.NewScope(variable.Variable{
		Name:       variable.Name{Namespace: variable.NamespaceWorkflow, Key: "years"},
		Type:       variable.TypeList(variable.TypeInt),
		Expression: `[2024, 2025]`,
	})
	if err != nil {
		t.Fatal(err)
	}

	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})

	items, err := CompileFanOutStep(resolver, FanOutStep{
		ID: "download-cdl",
		WorkItem: FanOutWorkItemTemplate{
			FanOutExpression: "${years[*]}",
			Type:             model.WorkItemTypeWriteDemoOutput,
			IDPrefix:         "download-cdl",
			OutputPrefix:     "cdl",
			OutputExtension:  ".txt",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("unexpected item count: %d", len(items))
	}

	if items[0].ID != "download-cdl-2024" {
		t.Fatalf("unexpected first id: %s", items[0].ID)
	}
}

func TestCompileFanOutWorkItemsCopiesParameters(t *testing.T) {
	scope, err := variable.NewScope(variable.Variable{
		Name:       variable.Name{Namespace: variable.NamespaceWorkflow, Key: "years"},
		Type:       variable.TypeList(variable.TypeInt),
		Expression: `[2024]`,
	})
	if err != nil {
		t.Fatal(err)
	}

	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})
	parameters := model.Parameters{
		"input_root": {Type: "path", Value: "/data/cdl"},
	}

	items, err := CompileFanOutWorkItems(resolver, FanOutWorkItemTemplate{
		FanOutExpression: "${years[*]}",
		Type:             model.WorkItemTypeWriteDemoOutput,
		IDPrefix:         "cdl",
		OutputPrefix:     "cdl",
		OutputExtension:  ".txt",
		Parameters:       parameters,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if items[0].Parameters["input_root"].Value != "/data/cdl" {
		t.Fatalf("unexpected parameter: %+v", items[0].Parameters["input_root"])
	}

	items[0].Parameters["input_root"] = model.Parameter{Type: "path", Value: "/other"}
	if parameters["input_root"].Value != "/data/cdl" {
		t.Fatalf("template parameter was mutated: %+v", parameters["input_root"])
	}
}

func TestCompileFanOutWorkItemsBindsParameterAccessors(t *testing.T) {
	scope, err := variable.NewScope(variable.Variable{
		Name:       variable.Name{Namespace: variable.NamespaceWorkflow, Key: "records"},
		Type:       variable.TypeList(variable.TypeObject),
		Expression: `[{"id": "fixture", "input_path": "demo-summary-input.txt"}, {"id": "fixture-2", "input_path": "demo-summary-input-2.txt"}]`,
	})
	if err != nil {
		t.Fatal(err)
	}

	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})

	items, err := CompileFanOutWorkItems(resolver, FanOutWorkItemTemplate{
		FanOutExpression: "${records[*]}",
		IDTokenAccessor:  ".id",
		OutputAccessor:   ".id",
		Type:             model.WorkItemTypeSummarizeInputFile,
		IDPrefix:         "summary",
		OutputPrefix:     "summary",
		OutputExtension:  ".txt",
		Parameters: model.Parameters{
			"input_path": {Type: "path", Value: "unset"},
		},
		ParameterAccessors: map[string]string{
			"input_path": ".input_path",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if items[0].Parameters["input_path"].Value != "demo-summary-input.txt" {
		t.Fatalf("unexpected input_path parameter: %+v", items[0].Parameters["input_path"])
	}

	if items[1].Parameters["input_path"].Value != "demo-summary-input-2.txt" {
		t.Fatalf("unexpected second input_path parameter: %+v", items[1].Parameters["input_path"])
	}
}

func TestCompileFanOutStepRejectsMissingID(t *testing.T) {
	resolver := variable.NewResolver(variable.NewSet(), variable.ResolverConfig{})

	_, err := CompileFanOutStep(resolver, FanOutStep{
		WorkItem: FanOutWorkItemTemplate{
			FanOutExpression: "${years[*]}",
			Type:             model.WorkItemTypeWriteDemoOutput,
			IDPrefix:         "download-cdl",
			OutputPrefix:     "cdl",
			OutputExtension:  ".txt",
		},
	})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestCompileFanOutWorkItemsUsesObjectTokenAccessor(t *testing.T) {
	scope, err := variable.NewScope(variable.Variable{
		Name:       variable.Name{Namespace: variable.NamespaceWorkflow, Key: "records"},
		Type:       variable.TypeList(variable.TypeObject),
		Expression: `[{"year": 2024, "path": "/data/2024.tif"}, {"year": 2025, "path": "/data/2025.tif"}]`,
	})
	if err != nil {
		t.Fatal(err)
	}

	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})

	items, err := CompileFanOutWorkItems(resolver, FanOutWorkItemTemplate{
		FanOutExpression: "${records[*]}",
		TokenAccessor:    ".year",
		Type:             model.WorkItemTypeWriteDemoOutput,
		IDPrefix:         "cdl",
		OutputPrefix:     "cdl",
		OutputExtension:  ".txt",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if items[0].ID != "cdl-2024" {
		t.Fatalf("unexpected first id: %s", items[0].ID)
	}

	if items[1].OutputFilename != "cdl-2025.txt" {
		t.Fatalf("unexpected second output filename: %s", items[1].OutputFilename)
	}
}

func TestCompileFanOutWorkItemsUsesSeparateTokenAccessors(t *testing.T) {
	scope, err := variable.NewScope(variable.Variable{
		Name:       variable.Name{Namespace: variable.NamespaceWorkflow, Key: "records"},
		Type:       variable.TypeList(variable.TypeObject),
		Expression: `[{"year": 2024, "output": "cdl-iowa-2024"}, {"year": 2025, "output": "cdl-iowa-2025"}]`,
	})
	if err != nil {
		t.Fatal(err)
	}

	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})

	items, err := CompileFanOutWorkItems(resolver, FanOutWorkItemTemplate{
		FanOutExpression: "${records[*]}",
		IDTokenAccessor:  ".year",
		OutputAccessor:   ".output",
		Type:             model.WorkItemTypeWriteDemoOutput,
		IDPrefix:         "cdl",
		OutputPrefix:     "boundary",
		OutputExtension:  ".txt",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if items[0].ID != "cdl-2024" {
		t.Fatalf("unexpected first id: %s", items[0].ID)
	}

	if items[1].OutputFilename != "boundary-cdl-iowa-2025.txt" {
		t.Fatalf("unexpected second output filename: %s", items[1].OutputFilename)
	}
}

func TestCompileFanOutWorkItemsRejectsUnsupportedTokenType(t *testing.T) {
	scope, err := variable.NewScope(variable.Variable{
		Name:       variable.Name{Namespace: variable.NamespaceWorkflow, Key: "records"},
		Type:       variable.TypeList(variable.TypeObject),
		Expression: `[{"year": 2024}]`,
	})
	if err != nil {
		t.Fatal(err)
	}

	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})

	_, err = CompileFanOutWorkItems(resolver, FanOutWorkItemTemplate{
		FanOutExpression: "${records[*]}",
		Type:             model.WorkItemTypeWriteDemoOutput,
		IDPrefix:         "cdl",
		OutputPrefix:     "cdl",
		OutputExtension:  ".txt",
	})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestCompileFanOutWorkItemsRejectsInvalidTemplate(t *testing.T) {
	scope, err := variable.NewScope(variable.Variable{
		Name:       variable.Name{Namespace: variable.NamespaceWorkflow, Key: "years"},
		Type:       variable.TypeList(variable.TypeInt),
		Expression: `[2024]`,
	})
	if err != nil {
		t.Fatal(err)
	}

	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})

	_, err = CompileFanOutWorkItems(resolver, FanOutWorkItemTemplate{
		FanOutExpression: "${years[*]}",
		Type:             model.WorkItemTypeWriteDemoOutput,
		IDPrefix:         "cdl",
		OutputPrefix:     "outputs/cdl",
		OutputExtension:  ".txt",
	})
	if err == nil {
		t.Fatal("expected an error")
	}
}

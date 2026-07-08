package workflow

import (
	"encoding/json"
	"strings"
	"testing"

	"goetl/internal/model"
	"goetl/internal/variable"
)

func TestCompileFanOutWorkItems(t *testing.T) {
	scope, err := variable.NewScope(testIntListVariable("years", 2024, 2025))
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
	scope, err := variable.NewScope(testIntListVariable("years", 2024, 2025))
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
	scope, err := variable.NewScope(testIntListVariable("years", 2024))
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
	scope, err := variable.NewScope(testObjectListVariable("records",
		map[string]variable.TypedExpression{
			"id":         {Type: variable.TypeString, Expression: "fixture"},
			"input_path": {Type: variable.TypeString, Expression: "demo-summary-input.txt"},
		},
		map[string]variable.TypedExpression{
			"id":         {Type: variable.TypeString, Expression: "fixture-2"},
			"input_path": {Type: variable.TypeString, Expression: "demo-summary-input-2.txt"},
		},
	))
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

func TestCompileFanOutWorkItemsPreservesProtectedReferenceParameterAccessor(t *testing.T) {
	scope, err := variable.NewScope(
		variable.Variable{
			Name: variable.Name{Namespace: variable.NamespaceWorkflow, Key: "tokens"},
			TypedExpression: variable.TypedExpression{Type: variable.TypeList, Expression: []variable.TypedExpression{
				{Type: variable.TypeObject, Expression: map[string]variable.TypedExpression{
					"id":     {Type: variable.TypeString, Expression: "fixture"},
					"secret": {Type: variable.TypeString, Expression: "${gdrive_token}"},
				}},
			}},
		},
		variable.Variable{
			Name:            variable.Name{Namespace: variable.NamespaceWorkflow, Key: "gdrive_token"},
			TypedExpression: variable.TypedExpression{Type: variable.TypeString},
			ProtectedRef:    &variable.ProtectedRef{Provider: "worker_env", Key: "GOET_GDRIVE_TOKEN"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})
	items, err := CompileFanOutWorkItems(resolver, FanOutWorkItemTemplate{
		FanOutExpression: "${tokens[*]}",
		Type:             model.WorkItemTypePythonScript,
		IDPrefix:         "download",
		OutputPrefix:     "download",
		OutputExtension:  ".json",
		TokenAccessor:    ".id",
		Parameters: model.Parameters{
			"gdrive_token": {Type: "string"},
		},
		ParameterAccessors: map[string]string{
			"gdrive_token": ".secret",
		},
	})
	if err != nil {
		t.Fatalf("CompileFanOutWorkItems() error = %v", err)
	}

	parameter := items[0].Parameters["gdrive_token"]
	if !parameter.Sensitive {
		t.Fatal("expected protected reference parameter to remain sensitive")
	}
	if parameter.Value != nil {
		t.Fatalf("parameter value = %#v, want no plaintext", parameter.Value)
	}
	if parameter.ProtectedRef == nil {
		t.Fatal("expected protected reference on parameter")
	}
	if parameter.RedactionLabel != "${worker_env.GOET_GDRIVE_TOKEN}" {
		t.Fatalf("redaction label = %q", parameter.RedactionLabel)
	}
}

func TestCompileFanOutWorkItemsResolvesResourceConstraints(t *testing.T) {
	scope, err := variable.NewScope(
		testObjectListVariable("records",
			map[string]variable.TypedExpression{
				"id":                   {Type: variable.TypeString, Expression: "fixture"},
				"memory_allocated_mib": {Type: variable.TypeInt, Expression: 512},
			},
			map[string]variable.TypedExpression{
				"id":                   {Type: variable.TypeString, Expression: "fixture-2"},
				"memory_allocated_mib": {Type: variable.TypeInt, Expression: 1024},
			},
		),
		variable.Variable{
			Name: variable.Name{Namespace: variable.NamespaceControllerConfig, Key: "local_memory_limit_mib"},
			TypedExpression: variable.TypedExpression{
				Type:       variable.TypeInt,
				Expression: 2048,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})
	compiled, err := CompileFanOutWorkItemResults(resolver, FanOutWorkItemTemplate{
		FanOutExpression: "${records[*]}",
		IDTokenAccessor:  ".id",
		OutputAccessor:   ".id",
		Type:             model.WorkItemTypeSummarizeInputFile,
		IDPrefix:         "summary",
		OutputPrefix:     "summary",
		OutputExtension:  ".txt",
		Parameters: model.Parameters{
			"input_path": {Type: "path", Value: "input.txt"},
		},
		ResourceConstraints: []ResourceConstraintDeclaration{
			{
				ResourceKey: variable.TypedExpression{
					Type:       variable.TypeString,
					Expression: "target:local/memory-mib",
				},
				RequestedUnits: variable.TypedExpression{
					Type:       variable.TypeInt,
					Expression: "${step.memory_allocated_mib}",
				},
				Operator: variable.TypedExpression{
					Type:       variable.TypeString,
					Expression: "<=",
				},
				TargetUnits: variable.TypedExpression{
					Type:       variable.TypeInt,
					Expression: "${controller_config.local_memory_limit_mib}",
				},
			},
			{
				ResourceKey: variable.TypedExpression{
					Type:       variable.TypeString,
					Expression: "ctlr/python-env:torch",
				},
				RequestedUnits: variable.TypedExpression{
					Type:       variable.TypeInt,
					Expression: 1,
				},
				Operator: variable.TypedExpression{
					Type:       variable.TypeString,
					Expression: "<=",
				},
				TargetUnits: variable.TypedExpression{
					Type:       variable.TypeInt,
					Expression: 1,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(compiled[0].ResourceConstraints) != 2 {
		t.Fatalf("first resource constraint count = %d, want 2", len(compiled[0].ResourceConstraints))
	}
	first := compiled[0].ResourceConstraints[0]
	if first.WorkItemID != "summary-fixture" || first.ConstraintIndex != 0 || first.ResourceKey != "target:local/memory-mib" {
		t.Fatalf("first constraint identity = %+v", first)
	}
	if first.RequestedUnits != 512 || first.Operator != model.WorkItemResourceConstraintOperatorLessEq || first.TargetUnits != 2048 {
		t.Fatalf("first constraint predicate = %+v", first)
	}
	if compiled[1].ResourceConstraints[0].RequestedUnits != 1024 {
		t.Fatalf("second requested units = %d, want 1024", compiled[1].ResourceConstraints[0].RequestedUnits)
	}
}

func TestResourceConstraintDeclarationUnmarshalAcceptsWorkflowJSONShape(t *testing.T) {
	var declaration ResourceConstraintDeclaration
	if err := json.Unmarshal([]byte(`{
		"resource_key": "target:local/memory-mib",
		"requested_units": "${step.memory_allocated_mib}",
		"operator": "<=",
		"target_units": 2048
	}`), &declaration); err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}
	if declaration.ResourceKey.Type != variable.TypeString || declaration.ResourceKey.Expression != "target:local/memory-mib" {
		t.Fatalf("resource key expression = %+v", declaration.ResourceKey)
	}
	if declaration.RequestedUnits.Type != variable.TypeInt || declaration.RequestedUnits.Expression != "${step.memory_allocated_mib}" {
		t.Fatalf("requested units expression = %+v", declaration.RequestedUnits)
	}
	if declaration.TargetUnits.Type != variable.TypeInt {
		t.Fatalf("target units type = %s, want int", declaration.TargetUnits.Type)
	}
}

func TestCompileFanOutWorkItemsRejectsInvalidResourceConstraints(t *testing.T) {
	tests := []struct {
		name        string
		constraint  ResourceConstraintDeclaration
		wantMessage string
	}{
		{
			name: "invalid operator",
			constraint: testResourceConstraintDeclaration(
				"target:local/memory-mib",
				1,
				"approximately",
				2,
			),
			wantMessage: "unsupported resource constraint operator",
		},
		{
			name: "non-integer requested units",
			constraint: ResourceConstraintDeclaration{
				ResourceKey:    testStringExpression("target:local/memory-mib"),
				RequestedUnits: testStringExpression("1"),
				Operator:       testStringExpression("<="),
				TargetUnits:    testIntExpression(2),
			},
			wantMessage: "requested_units: has type string, want int",
		},
		{
			name: "non-positive requested units",
			constraint: testResourceConstraintDeclaration(
				"target:local/memory-mib",
				0,
				"<=",
				2,
			),
			wantMessage: "requested units must be greater than 0",
		},
		{
			name: "negative target units",
			constraint: testResourceConstraintDeclaration(
				"target:local/memory-mib",
				1,
				"<=",
				-1,
			),
			wantMessage: "target units must be non-negative",
		},
		{
			name: "empty resource key",
			constraint: testResourceConstraintDeclaration(
				"",
				1,
				"<=",
				2,
			),
			wantMessage: "resource_key: is required",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolver := testWorkflowResolver(t, 2024)
			_, err := CompileFanOutWorkItemResults(resolver, FanOutWorkItemTemplate{
				FanOutExpression: "${years[*]}",
				Type:             model.WorkItemTypeWriteDemoOutput,
				IDPrefix:         "cdl",
				OutputPrefix:     "cdl",
				OutputExtension:  ".txt",
				ResourceConstraints: []ResourceConstraintDeclaration{
					test.constraint,
				},
			})
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), test.wantMessage) {
				t.Fatalf("error = %v, want message containing %q", err, test.wantMessage)
			}
		})
	}
}

func TestCompileFanOutWorkItemsRejectsDuplicateResourceKeys(t *testing.T) {
	resolver := testWorkflowResolver(t, 2024)
	_, err := CompileFanOutWorkItemResults(resolver, FanOutWorkItemTemplate{
		FanOutExpression: "${years[*]}",
		Type:             model.WorkItemTypeWriteDemoOutput,
		IDPrefix:         "cdl",
		OutputPrefix:     "cdl",
		OutputExtension:  ".txt",
		ResourceConstraints: []ResourceConstraintDeclaration{
			testResourceConstraintDeclaration("target:local/memory-mib", 1, "<=", 2),
			testResourceConstraintDeclaration("target:local/memory-mib", 1, "<=", 2),
		},
	})
	if err == nil {
		t.Fatal("expected duplicate resource key error")
	}
	if !strings.Contains(err.Error(), "duplicate resource_key") {
		t.Fatalf("error = %v, want duplicate resource_key", err)
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
	scope, err := variable.NewScope(testObjectListVariable("records",
		map[string]variable.TypedExpression{
			"year": {Type: variable.TypeInt, Expression: 2024},
			"path": {Type: variable.TypePath, Expression: "/data/2024.tif"},
		},
		map[string]variable.TypedExpression{
			"year": {Type: variable.TypeInt, Expression: 2025},
			"path": {Type: variable.TypePath, Expression: "/data/2025.tif"},
		},
	))
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
	scope, err := variable.NewScope(testObjectListVariable("records",
		map[string]variable.TypedExpression{
			"year":   {Type: variable.TypeInt, Expression: 2024},
			"output": {Type: variable.TypeString, Expression: "cdl-iowa-2024"},
		},
		map[string]variable.TypedExpression{
			"year":   {Type: variable.TypeInt, Expression: 2025},
			"output": {Type: variable.TypeString, Expression: "cdl-iowa-2025"},
		},
	))
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
	scope, err := variable.NewScope(testObjectListVariable("records", map[string]variable.TypedExpression{
		"year": {Type: variable.TypeInt, Expression: 2024},
	}))
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
	scope, err := variable.NewScope(testIntListVariable("years", 2024))
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

func TestCompileFanOutWorkItemsAllowsPythonScriptWithoutSource(t *testing.T) {
	scope, err := variable.NewScope(testIntListVariable("years", 2024))
	if err != nil {
		t.Fatal(err)
	}

	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})

	items, err := CompileFanOutWorkItems(resolver, FanOutWorkItemTemplate{
		FanOutExpression: "${years[*]}",
		Type:             model.WorkItemTypePythonScript,
		IDPrefix:         "python",
		OutputPrefix:     "python",
		OutputExtension:  ".json",
		Parameters: model.Parameters{
			"python_entrypoint": {Type: "path", Value: "scripts/run.py"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("unexpected item count: %d", len(items))
	}
	if items[0].Source != nil {
		t.Fatalf("compiled python source = %+v, want nil intermediate source", items[0].Source)
	}
	if err := items[0].Validate(); err == nil {
		t.Fatal("strict validation unexpectedly accepted python_script without source")
	}
}

func TestCompileFanOutWorkItemsStillRejectsPythonShapeErrors(t *testing.T) {
	scope, err := variable.NewScope(testIntListVariable("years", 2024))
	if err != nil {
		t.Fatal(err)
	}

	resolver := variable.NewResolver(variable.NewSet(scope), variable.ResolverConfig{})

	_, err = CompileFanOutWorkItems(resolver, FanOutWorkItemTemplate{
		FanOutExpression: "${years[*]}",
		Type:             model.WorkItemTypePythonScript,
		IDPrefix:         "python",
		OutputPrefix:     "python",
		OutputExtension:  ".json",
		Parameters: model.Parameters{
			"python_entrypoint": {Value: "scripts/run.py"},
		},
	})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func testIntListVariable(key string, values ...int) variable.Variable {
	items := make([]variable.TypedExpression, 0, len(values))
	for _, value := range values {
		items = append(items, variable.TypedExpression{Type: variable.TypeInt, Expression: value})
	}
	return variable.Variable{
		Name:            variable.Name{Namespace: variable.NamespaceWorkflow, Key: key},
		TypedExpression: variable.TypedExpression{Type: variable.TypeList, Expression: items},
	}
}

func testObjectListVariable(key string, values ...map[string]variable.TypedExpression) variable.Variable {
	items := make([]variable.TypedExpression, 0, len(values))
	for _, value := range values {
		items = append(items, variable.TypedExpression{Type: variable.TypeObject, Expression: value})
	}
	return variable.Variable{
		Name:            variable.Name{Namespace: variable.NamespaceWorkflow, Key: key},
		TypedExpression: variable.TypedExpression{Type: variable.TypeList, Expression: items},
	}
}

func testResourceConstraintDeclaration(resourceKey string, requestedUnits int, operator string, targetUnits int) ResourceConstraintDeclaration {
	return ResourceConstraintDeclaration{
		ResourceKey:    testStringExpression(resourceKey),
		RequestedUnits: testIntExpression(requestedUnits),
		Operator:       testStringExpression(operator),
		TargetUnits:    testIntExpression(targetUnits),
	}
}

func testStringExpression(value string) variable.TypedExpression {
	return variable.TypedExpression{Type: variable.TypeString, Expression: value}
}

func testIntExpression(value int) variable.TypedExpression {
	return variable.TypedExpression{Type: variable.TypeInt, Expression: value}
}

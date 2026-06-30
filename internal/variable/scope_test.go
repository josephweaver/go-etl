package variable

import "testing"

func TestMerge(t *testing.T) {
	low := Scope{
		"data_dir": {
			Name:            Name{Namespace: NamespaceGlobal, Key: "data_dir"},
			TypedExpression: TypedExpression{Type: TypePath, Expression: "/global/data"},
		},
		"year": {
			Name:            Name{Namespace: NamespaceGlobal, Key: "year"},
			TypedExpression: TypedExpression{Type: TypeInt, Expression: 2025},
		},
	}

	high := Scope{
		"data_dir": {
			Name:            Name{Namespace: NamespaceWorkflow, Key: "data_dir"},
			TypedExpression: TypedExpression{Type: TypePath, Expression: "/workflow/data"},
		},
	}

	merged := Merge(low, high)

	if got := merged["data_dir"].Expression; got != "/workflow/data" {
		t.Fatalf("unexpected data_dir expression: %q", got)
	}

	if got := merged["data_dir"].Name.Namespace; got != NamespaceWorkflow {
		t.Fatalf("unexpected data_dir namespace: %q", got)
	}

	if got := merged["year"].Expression; got != 2025 {
		t.Fatalf("unexpected year expression: %v", got)
	}
}

func TestNewScope(t *testing.T) {
	scope, err := NewScope(
		Variable{Name: Name{Namespace: NamespaceProject, Key: "data_dir"}, TypedExpression: TypedExpression{Type: TypePath, Expression: "/data/project"}},

		Variable{Name: Name{Namespace: NamespaceProject, Key: "year"}, TypedExpression: TypedExpression{Type: TypeInt, Expression: 2025}},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := scope["data_dir"].Expression; got != "/data/project" {
		t.Fatalf("unexpected data_dir expression: %q", got)
	}

	if got := scope["year"].Expression; got != 2025 {
		t.Fatalf("unexpected year expression: %v", got)
	}
}

func TestNewScopeRejectsInvalidVariable(t *testing.T) {
	if _, err := NewScope(Variable{Name: Name{Namespace: NamespaceProject, Key: "data_dir"}, TypedExpression: TypedExpression{Type: Type{Kind: "unknown"}, Expression: "/data/project"}}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestNewScopeRejectsDuplicateKey(t *testing.T) {
	if _, err := NewScope(
		Variable{Name: Name{Namespace: NamespaceProject, Key: "data_dir"}, TypedExpression: TypedExpression{Type: TypePath, Expression: "/data/project"}},

		Variable{Name: Name{Namespace: NamespaceProject, Key: "data_dir"}, TypedExpression: TypedExpression{Type: TypePath, Expression: "/data/other"}},
	); err == nil {
		t.Fatal("expected an error")
	}
}

func TestMergeDoesNotMutateInputs(t *testing.T) {
	low := Scope{
		"data_dir": {
			Name:            Name{Namespace: NamespaceGlobal, Key: "data_dir"},
			TypedExpression: TypedExpression{Type: TypePath, Expression: "/global/data"},
		},
	}

	high := Scope{
		"data_dir": {
			Name:            Name{Namespace: NamespaceWorkflow, Key: "data_dir"},
			TypedExpression: TypedExpression{Type: TypePath, Expression: "/workflow/data"},
		},
	}

	merged := Merge(low, high)
	merged["data_dir"] = Variable{Name: Name{Namespace: NamespaceOverride, Key: "data_dir"}, TypedExpression: TypedExpression{Type: TypePath, Expression: "/override/data"}}

	if got := low["data_dir"].Expression; got != "/global/data" {
		t.Fatalf("low scope was mutated: %q", got)
	}

	if got := high["data_dir"].Expression; got != "/workflow/data" {
		t.Fatalf("high scope was mutated: %q", got)
	}
}

func TestSetLookup(t *testing.T) {
	global, err := NewScope(Variable{Name: Name{Namespace: NamespaceGlobal, Key: "data_dir"}, TypedExpression: TypedExpression{Type: TypePath, Expression: "/global/data"}})
	if err != nil {
		t.Fatal(err)
	}

	workflow, err := NewScope(Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "data_dir"}, TypedExpression: TypedExpression{Type: TypePath, Expression: "/workflow/data"}})
	if err != nil {
		t.Fatal(err)
	}

	set := NewSet(global, workflow)

	variable, ok := set.Lookup("data_dir")
	if !ok {
		t.Fatal("expected variable")
	}

	if variable.Name.Namespace != NamespaceWorkflow {
		t.Fatalf("unexpected namespace: %q", variable.Name.Namespace)
	}

	if variable.Expression != "/workflow/data" {
		t.Fatalf("unexpected expression: %q", variable.Expression)
	}
}

func TestSetLookupMissingKey(t *testing.T) {
	set := NewSet()

	if _, ok := set.Lookup("missing"); ok {
		t.Fatal("unexpected variable")
	}
}

func TestSetLookupName(t *testing.T) {
	global, err := NewScope(Variable{Name: Name{Namespace: NamespaceGlobal, Key: "data_dir"}, TypedExpression: TypedExpression{Type: TypePath, Expression: "/global/data"}})
	if err != nil {
		t.Fatal(err)
	}

	workflow, err := NewScope(Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "data_dir"}, TypedExpression: TypedExpression{Type: TypePath, Expression: "/workflow/data"}})
	if err != nil {
		t.Fatal(err)
	}

	set := NewSet(global, workflow)

	variable, ok := set.LookupName(Name{
		Namespace: NamespaceGlobal,
		Key:       "data_dir",
	})
	if !ok {
		t.Fatal("expected variable")
	}

	if variable.Expression != "/global/data" {
		t.Fatalf("unexpected expression: %q", variable.Expression)
	}
}

func TestSetLookupNameMissingNamespace(t *testing.T) {
	set := NewSet()

	if _, ok := set.LookupName(Name{Namespace: NamespaceWorkflow, Key: "data_dir"}); ok {
		t.Fatal("unexpected variable")
	}
}

func TestSetLookupNameMissingKey(t *testing.T) {
	workflow, err := NewScope(Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "data_dir"}, TypedExpression: TypedExpression{Type: TypePath, Expression: "/workflow/data"}})
	if err != nil {
		t.Fatal(err)
	}

	set := NewSet(workflow)

	if _, ok := set.LookupName(Name{Namespace: NamespaceWorkflow, Key: "missing"}); ok {
		t.Fatal("unexpected variable")
	}
}

func TestSetLookupReference(t *testing.T) {
	global, err := NewScope(Variable{Name: Name{Namespace: NamespaceGlobal, Key: "data_dir"}, TypedExpression: TypedExpression{Type: TypePath, Expression: "/global/data"}})
	if err != nil {
		t.Fatal(err)
	}

	workflow, err := NewScope(Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "data_dir"}, TypedExpression: TypedExpression{Type: TypePath, Expression: "/workflow/data"}})
	if err != nil {
		t.Fatal(err)
	}

	set := NewSet(global, workflow)

	unqualified, err := ParseReference("data_dir")
	if err != nil {
		t.Fatal(err)
	}

	variable, ok := set.LookupReference(unqualified)
	if !ok {
		t.Fatal("expected unqualified variable")
	}

	if variable.Expression != "/workflow/data" {
		t.Fatalf("unexpected unqualified expression: %q", variable.Expression)
	}

	qualified, err := ParseReference("global.data_dir")
	if err != nil {
		t.Fatal(err)
	}

	variable, ok = set.LookupReference(qualified)
	if !ok {
		t.Fatal("expected qualified variable")
	}

	if variable.Expression != "/global/data" {
		t.Fatalf("unexpected qualified expression: %q", variable.Expression)
	}
}

func TestSetLookupReferenceMissing(t *testing.T) {
	set := NewSet()

	reference, err := ParseReference("data_dir")
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := set.LookupReference(reference); ok {
		t.Fatal("unexpected variable")
	}
}

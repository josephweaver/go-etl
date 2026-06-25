package variable

import "testing"

func TestResolverResolveReference(t *testing.T) {
	global, err := NewScope(Variable{
		Name:       Name{Namespace: NamespaceGlobal, Key: "year"},
		Type:       TypeInt,
		Expression: "2024",
	})
	if err != nil {
		t.Fatal(err)
	}

	workflow, err := NewScope(Variable{
		Name:       Name{Namespace: NamespaceWorkflow, Key: "year"},
		Type:       TypeInt,
		Expression: "2025",
	})
	if err != nil {
		t.Fatal(err)
	}

	resolver := NewResolver(NewSet(global, workflow), ResolverConfig{})

	reference, err := ParseReference("year")
	if err != nil {
		t.Fatal(err)
	}

	value, err := resolver.Resolve(reference)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if value.Type != TypeInt {
		t.Fatalf("unexpected type: %s", value.Type)
	}

	if value.Value != 2025 {
		t.Fatalf("unexpected value: %#v", value.Value)
	}
}

func TestResolverOptional(t *testing.T) {
	scope, err := NewScope(Variable{
		Name:       Name{Namespace: NamespaceWorkflow, Key: "year"},
		Type:       TypeInt,
		Expression: "2025",
	})
	if err != nil {
		t.Fatal(err)
	}
	resolver := NewResolver(NewSet(scope), ResolverConfig{})

	value, ok, err := resolver.Optional("year")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected value")
	}
	if value.Value != 2025 {
		t.Fatalf("value = %#v, want 2025", value.Value)
	}

	if _, ok, err := resolver.Optional("missing"); err != nil || ok {
		t.Fatalf("missing optional = ok %v err %v, want false nil", ok, err)
	}
}

func TestResolverTypedAccessors(t *testing.T) {
	scope, err := NewScope(
		Variable{
			Name:       Name{Namespace: NamespaceWorkflow, Key: "name"},
			Type:       TypeString,
			Expression: "goetl",
		},
		Variable{
			Name:       Name{Namespace: NamespaceWorkflow, Key: "root"},
			Type:       TypePath,
			Expression: "/data/goetl",
		},
		Variable{
			Name:       Name{Namespace: NamespaceWorkflow, Key: "settings"},
			Type:       TypeObject,
			Expression: `{"script_path":"/tmp/worker.slurm","args":["--once"]}`,
		},
		Variable{
			Name:       Name{Namespace: NamespaceWorkflow, Key: "args"},
			Type:       TypeList(TypeString),
			Expression: `["--once"]`,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	resolver := NewResolver(NewSet(scope), ResolverConfig{})

	if got, err := resolver.String("name"); err != nil || got != "goetl" {
		t.Fatalf("String = %q err %v, want goetl nil", got, err)
	}
	if got, err := resolver.PathOrString("root"); err != nil || got != "/data/goetl" {
		t.Fatalf("PathOrString = %q err %v, want path nil", got, err)
	}
	if got, ok, err := resolver.OptionalString("missing"); err != nil || ok || got != "" {
		t.Fatalf("OptionalString missing = %q ok %v err %v, want empty false nil", got, ok, err)
	}
	args, err := resolver.StringList("args")
	if err != nil {
		t.Fatalf("StringList: %v", err)
	}
	if len(args) != 1 || args[0] != "--once" {
		t.Fatalf("args = %#v, want --once", args)
	}

	settings, err := resolver.Object("settings")
	if err != nil {
		t.Fatalf("Object: %v", err)
	}
	if got, ok, err := OptionalObjectFieldString(settings, "script_path"); err != nil || !ok || got != "/tmp/worker.slurm" {
		t.Fatalf("ObjectFieldString = %q ok %v err %v, want script path", got, ok, err)
	}
	fieldArgs, ok, err := OptionalObjectFieldStringList(settings, "args")
	if err != nil || !ok {
		t.Fatalf("ObjectFieldStringList ok %v err %v, want true nil", ok, err)
	}
	if len(fieldArgs) != 1 || fieldArgs[0] != "--once" {
		t.Fatalf("field args = %#v, want --once", fieldArgs)
	}
}

func TestResolverTypedAccessorsRejectWrongType(t *testing.T) {
	scope, err := NewScope(Variable{
		Name:       Name{Namespace: NamespaceWorkflow, Key: "year"},
		Type:       TypeInt,
		Expression: "2025",
	})
	if err != nil {
		t.Fatal(err)
	}
	resolver := NewResolver(NewSet(scope), ResolverConfig{})

	if _, err := resolver.String("year"); err == nil {
		t.Fatal("expected wrong-type error")
	}
}

func TestResolverResolveReferenceUsesQualifiedNamespace(t *testing.T) {
	global, err := NewScope(Variable{
		Name:       Name{Namespace: NamespaceGlobal, Key: "year"},
		Type:       TypeInt,
		Expression: "2024",
	})
	if err != nil {
		t.Fatal(err)
	}

	workflow, err := NewScope(Variable{
		Name:       Name{Namespace: NamespaceWorkflow, Key: "year"},
		Type:       TypeInt,
		Expression: "2025",
	})
	if err != nil {
		t.Fatal(err)
	}

	resolver := NewResolver(NewSet(global, workflow), ResolverConfig{})

	reference, err := ParseReference("global.year")
	if err != nil {
		t.Fatal(err)
	}

	value, err := resolver.Resolve(reference)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if value.Value != 2024 {
		t.Fatalf("unexpected value: %#v", value.Value)
	}
}

func TestResolverResolveReferenceRejectsMissingVariable(t *testing.T) {
	resolver := NewResolver(NewSet(), ResolverConfig{})

	reference, err := ParseReference("year")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := resolver.Resolve(reference); err == nil {
		t.Fatal("expected an error")
	}
}

func TestResolverResolveFanOutExpression(t *testing.T) {
	scope, err := NewScope(Variable{
		Name:       Name{Namespace: NamespaceWorkflow, Key: "years"},
		Type:       TypeList(TypeInt),
		Expression: `[2024, 2025]`,
	})
	if err != nil {
		t.Fatal(err)
	}

	resolver := NewResolver(NewSet(scope), ResolverConfig{})

	values, err := resolver.ResolveFanOutExpression("${years[*]}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(values) != 2 {
		t.Fatalf("unexpected value count: %d", len(values))
	}

	if values[1].Value != 2025 {
		t.Fatalf("unexpected second value: %#v", values[1].Value)
	}
}

func TestResolverResolveQualifiedFanOutExpression(t *testing.T) {
	global, err := NewScope(Variable{
		Name:       Name{Namespace: NamespaceGlobal, Key: "years"},
		Type:       TypeList(TypeInt),
		Expression: `[2023, 2024]`,
	})
	if err != nil {
		t.Fatal(err)
	}

	workflow, err := NewScope(Variable{
		Name:       Name{Namespace: NamespaceWorkflow, Key: "years"},
		Type:       TypeList(TypeInt),
		Expression: `[2025, 2026]`,
	})
	if err != nil {
		t.Fatal(err)
	}

	resolver := NewResolver(NewSet(global, workflow), ResolverConfig{})

	values, err := resolver.ResolveFanOutExpression("${global.years[*]}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if values[0].Value != 2023 {
		t.Fatalf("unexpected first value: %#v", values[0].Value)
	}
}

func TestResolverRejectsInvalidFanOutExpression(t *testing.T) {
	scope, err := NewScope(Variable{
		Name:       Name{Namespace: NamespaceWorkflow, Key: "year"},
		Type:       TypeInt,
		Expression: `2025`,
	})
	if err != nil {
		t.Fatal(err)
	}

	resolver := NewResolver(NewSet(scope), ResolverConfig{})

	tests := []string{
		"year[*]",
		"${year}",
		"${year[0]}",
		"${year[*]}",
	}

	for _, expression := range tests {
		t.Run(expression, func(t *testing.T) {
			if _, err := resolver.ResolveFanOutExpression(expression); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestResolverResolveReferenceExpression(t *testing.T) {
	scope, err := NewScope(
		Variable{
			Name:       Name{Namespace: NamespaceWorkflow, Key: "year"},
			Type:       TypeInt,
			Expression: "2025",
		},
		Variable{
			Name:       Name{Namespace: NamespaceWorkflow, Key: "target_year"},
			Type:       TypeInt,
			Expression: "${year}",
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	resolver := NewResolver(NewSet(scope), ResolverConfig{})

	reference, err := ParseReference("target_year")
	if err != nil {
		t.Fatal(err)
	}

	value, err := resolver.Resolve(reference)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if value.Value != 2025 {
		t.Fatalf("unexpected value: %#v", value.Value)
	}
}

func TestResolverResolveReferenceExpressionChain(t *testing.T) {
	scope, err := NewScope(
		Variable{
			Name:       Name{Namespace: NamespaceWorkflow, Key: "year"},
			Type:       TypeInt,
			Expression: "2025",
		},
		Variable{
			Name:       Name{Namespace: NamespaceWorkflow, Key: "target_year"},
			Type:       TypeInt,
			Expression: "${year}",
		},
		Variable{
			Name:       Name{Namespace: NamespaceWorkflow, Key: "final_year"},
			Type:       TypeInt,
			Expression: "${target_year}",
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	resolver := NewResolver(NewSet(scope), ResolverConfig{})

	reference, err := ParseReference("final_year")
	if err != nil {
		t.Fatal(err)
	}

	value, err := resolver.Resolve(reference)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if value.Value != 2025 {
		t.Fatalf("unexpected value: %#v", value.Value)
	}
}

func TestResolverResolveQualifiedReferenceExpression(t *testing.T) {
	global, err := NewScope(Variable{
		Name:       Name{Namespace: NamespaceGlobal, Key: "year"},
		Type:       TypeInt,
		Expression: "2024",
	})
	if err != nil {
		t.Fatal(err)
	}

	workflow, err := NewScope(
		Variable{
			Name:       Name{Namespace: NamespaceWorkflow, Key: "year"},
			Type:       TypeInt,
			Expression: "2025",
		},
		Variable{
			Name:       Name{Namespace: NamespaceWorkflow, Key: "target_year"},
			Type:       TypeInt,
			Expression: "${global.year}",
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	resolver := NewResolver(NewSet(global, workflow), ResolverConfig{})

	reference, err := ParseReference("target_year")
	if err != nil {
		t.Fatal(err)
	}

	value, err := resolver.Resolve(reference)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if value.Value != 2024 {
		t.Fatalf("unexpected value: %#v", value.Value)
	}
}

func TestResolverResolveReferenceExpressionWithFieldAccessor(t *testing.T) {
	scope, err := NewScope(
		Variable{
			Name:       Name{Namespace: NamespaceWorkflow, Key: "record"},
			Type:       TypeObject,
			Expression: `{"year": 2025}`,
		},
		Variable{
			Name:       Name{Namespace: NamespaceWorkflow, Key: "target_year"},
			Type:       TypeInt,
			Expression: "${record.year}",
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	resolver := NewResolver(NewSet(scope), ResolverConfig{})

	reference, err := ParseReference("target_year")
	if err != nil {
		t.Fatal(err)
	}

	value, err := resolver.Resolve(reference)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if value.Value != 2025 {
		t.Fatalf("unexpected value: %#v", value.Value)
	}
}

func TestResolverResolveReferenceExpressionWithIndexAccessor(t *testing.T) {
	scope, err := NewScope(
		Variable{
			Name:       Name{Namespace: NamespaceWorkflow, Key: "years"},
			Type:       TypeList(TypeInt),
			Expression: `[2024, 2025]`,
		},
		Variable{
			Name:       Name{Namespace: NamespaceWorkflow, Key: "target_year"},
			Type:       TypeInt,
			Expression: "${years[1]}",
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	resolver := NewResolver(NewSet(scope), ResolverConfig{})

	reference, err := ParseReference("target_year")
	if err != nil {
		t.Fatal(err)
	}

	value, err := resolver.Resolve(reference)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if value.Value != 2025 {
		t.Fatalf("unexpected value: %#v", value.Value)
	}
}

func TestResolverResolveQualifiedReferenceExpressionWithAccessor(t *testing.T) {
	global, err := NewScope(Variable{
		Name:       Name{Namespace: NamespaceGlobal, Key: "record"},
		Type:       TypeObject,
		Expression: `{"year": 2024}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	workflow, err := NewScope(
		Variable{
			Name:       Name{Namespace: NamespaceWorkflow, Key: "record"},
			Type:       TypeObject,
			Expression: `{"year": 2025}`,
		},
		Variable{
			Name:       Name{Namespace: NamespaceWorkflow, Key: "target_year"},
			Type:       TypeInt,
			Expression: "${global.record.year}",
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	resolver := NewResolver(NewSet(global, workflow), ResolverConfig{})

	reference, err := ParseReference("target_year")
	if err != nil {
		t.Fatal(err)
	}

	value, err := resolver.Resolve(reference)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if value.Value != 2024 {
		t.Fatalf("unexpected value: %#v", value.Value)
	}
}

func TestResolverRejectsFanOutReferenceExpression(t *testing.T) {
	scope, err := NewScope(
		Variable{
			Name:       Name{Namespace: NamespaceWorkflow, Key: "years"},
			Type:       TypeList(TypeInt),
			Expression: `[2024, 2025]`,
		},
		Variable{
			Name:       Name{Namespace: NamespaceWorkflow, Key: "target_year"},
			Type:       TypeInt,
			Expression: "${years[*]}",
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	resolver := NewResolver(NewSet(scope), ResolverConfig{})

	reference, err := ParseReference("target_year")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := resolver.Resolve(reference); err == nil {
		t.Fatal("expected an error")
	}
}

func TestResolverEscapesReferenceExpression(t *testing.T) {
	scope, err := NewScope(Variable{
		Name:       Name{Namespace: NamespaceWorkflow, Key: "template"},
		Type:       TypeString,
		Expression: `\${year}`,
	})
	if err != nil {
		t.Fatal(err)
	}

	resolver := NewResolver(NewSet(scope), ResolverConfig{})

	reference, err := ParseReference("template")
	if err != nil {
		t.Fatal(err)
	}

	value, err := resolver.Resolve(reference)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if value.Value != "${year}" {
		t.Fatalf("unexpected value: %#v", value.Value)
	}
}

func TestResolverEscapesEmbeddedReferenceSyntax(t *testing.T) {
	scope, err := NewScope(Variable{
		Name:       Name{Namespace: NamespaceWorkflow, Key: "template"},
		Type:       TypeString,
		Expression: `before \${year} after`,
	})
	if err != nil {
		t.Fatal(err)
	}

	resolver := NewResolver(NewSet(scope), ResolverConfig{})

	reference, err := ParseReference("template")
	if err != nil {
		t.Fatal(err)
	}

	value, err := resolver.Resolve(reference)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if value.Value != "before ${year} after" {
		t.Fatalf("unexpected value: %#v", value.Value)
	}
}

func TestResolverRejectsMaxDepthExceeded(t *testing.T) {
	scope, err := NewScope(
		Variable{
			Name:       Name{Namespace: NamespaceWorkflow, Key: "a"},
			Type:       TypeString,
			Expression: "${b}",
		},
		Variable{
			Name:       Name{Namespace: NamespaceWorkflow, Key: "b"},
			Type:       TypeString,
			Expression: "${c}",
		},
		Variable{
			Name:       Name{Namespace: NamespaceWorkflow, Key: "c"},
			Type:       TypeString,
			Expression: "done",
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	resolver := NewResolver(NewSet(scope), ResolverConfig{MaxDepth: 2})

	reference, err := ParseReference("a")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := resolver.Resolve(reference); err == nil {
		t.Fatal("expected an error")
	}
}

func TestResolverRejectsMissingNestedReference(t *testing.T) {
	scope, err := NewScope(Variable{
		Name:       Name{Namespace: NamespaceWorkflow, Key: "year"},
		Type:       TypeInt,
		Expression: "${missing}",
	})
	if err != nil {
		t.Fatal(err)
	}

	resolver := NewResolver(NewSet(scope), ResolverConfig{})

	reference, err := ParseReference("year")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := resolver.Resolve(reference); err == nil {
		t.Fatal("expected an error")
	}
}

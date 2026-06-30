package variable

import "testing"

func TestResolverResolveReference(t *testing.T) {
	global, err := NewScope(Variable{Name: Name{Namespace: NamespaceGlobal, Key: "year"}, TypedExpression: TypedExpression{Type: TypeInt, Expression: 2024}})
	if err != nil {
		t.Fatal(err)
	}

	workflow, err := NewScope(Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "year"}, TypedExpression: TypedExpression{Type: TypeInt, Expression: 2025}})
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
	scope, err := NewScope(Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "year"}, TypedExpression: TypedExpression{Type: TypeInt, Expression: 2025}})
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
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "name"}, TypedExpression: TypedExpression{Type: TypeString, Expression: "goetl"}},

		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "root"}, TypedExpression: TypedExpression{Type: TypePath, Expression: "/data/goetl"}},

		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "settings"}, TypedExpression: TypedExpression{Type: TypeObject, Expression: map[string]TypedExpression{
			"script_path": {Type: TypePath, Expression: "/tmp/worker.slurm"},
			"args":        {Type: TypeList, Expression: []TypedExpression{{Type: TypeString, Expression: "--once"}}},
		}}},

		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "args"}, TypedExpression: TypedExpression{Type: TypeList, Expression: []TypedExpression{{Type: TypeString, Expression: "--once"}}}},
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
	scope, err := NewScope(Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "year"}, TypedExpression: TypedExpression{Type: TypeInt, Expression: 2025}})
	if err != nil {
		t.Fatal(err)
	}
	resolver := NewResolver(NewSet(scope), ResolverConfig{})

	if _, err := resolver.String("year"); err == nil {
		t.Fatal("expected wrong-type error")
	}
}

func TestResolverStringListRejectsNonStringItem(t *testing.T) {
	scope, err := NewScope(Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "args"}, TypedExpression: TypedExpression{Type: TypeList, Expression: []TypedExpression{
		{Type: TypeString, Expression: "--once"},
		{Type: TypeInt, Expression: 2},
	}}})
	if err != nil {
		t.Fatal(err)
	}

	resolver := NewResolver(NewSet(scope), ResolverConfig{})
	if _, err := resolver.StringList("args"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestResolverResolveReferenceUsesQualifiedNamespace(t *testing.T) {
	global, err := NewScope(Variable{Name: Name{Namespace: NamespaceGlobal, Key: "year"}, TypedExpression: TypedExpression{Type: TypeInt, Expression: 2024}})
	if err != nil {
		t.Fatal(err)
	}

	workflow, err := NewScope(Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "year"}, TypedExpression: TypedExpression{Type: TypeInt, Expression: 2025}})
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
	scope, err := NewScope(Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "years"}, TypedExpression: TypedExpression{Type: TypeList, Expression: []TypedExpression{
		{Type: TypeInt, Expression: 2024},
		{Type: TypeInt, Expression: 2025},
	}}})
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
	global, err := NewScope(Variable{Name: Name{Namespace: NamespaceGlobal, Key: "years"}, TypedExpression: TypedExpression{Type: TypeList, Expression: []TypedExpression{
		{Type: TypeInt, Expression: 2023},
		{Type: TypeInt, Expression: 2024},
	}}})
	if err != nil {
		t.Fatal(err)
	}

	workflow, err := NewScope(Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "years"}, TypedExpression: TypedExpression{Type: TypeList, Expression: []TypedExpression{
		{Type: TypeInt, Expression: 2025},
		{Type: TypeInt, Expression: 2026},
	}}})
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
	scope, err := NewScope(Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "year"}, TypedExpression: TypedExpression{Type: TypeInt, Expression: 2025}})
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
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "year"}, TypedExpression: TypedExpression{Type: TypeInt, Expression: 2025}},

		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "target_year"}, TypedExpression: TypedExpression{Type: TypeInt, Expression: "${year}"}},
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
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "year"}, TypedExpression: TypedExpression{Type: TypeInt, Expression: 2025}},

		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "target_year"}, TypedExpression: TypedExpression{Type: TypeInt, Expression: "${year}"}},

		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "final_year"}, TypedExpression: TypedExpression{Type: TypeInt, Expression: "${target_year}"}},
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
	global, err := NewScope(Variable{Name: Name{Namespace: NamespaceGlobal, Key: "year"}, TypedExpression: TypedExpression{Type: TypeInt, Expression: 2024}})
	if err != nil {
		t.Fatal(err)
	}

	workflow, err := NewScope(
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "year"}, TypedExpression: TypedExpression{Type: TypeInt, Expression: 2025}},

		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "target_year"}, TypedExpression: TypedExpression{Type: TypeInt, Expression: "${global.year}"}},
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
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "record"}, TypedExpression: TypedExpression{Type: TypeObject, Expression: map[string]TypedExpression{
			"year": {Type: TypeInt, Expression: 2025},
		}}},

		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "target_year"}, TypedExpression: TypedExpression{Type: TypeInt, Expression: "${record.year}"}},
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
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "years"}, TypedExpression: TypedExpression{Type: TypeList, Expression: []TypedExpression{
			{Type: TypeInt, Expression: 2024},
			{Type: TypeInt, Expression: 2025},
		}}},

		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "target_year"}, TypedExpression: TypedExpression{Type: TypeInt, Expression: "${years[1]}"}},
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
	global, err := NewScope(Variable{Name: Name{Namespace: NamespaceGlobal, Key: "record"}, TypedExpression: TypedExpression{Type: TypeObject, Expression: map[string]TypedExpression{
		"year": {Type: TypeInt, Expression: 2024},
	}}})
	if err != nil {
		t.Fatal(err)
	}

	workflow, err := NewScope(
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "record"}, TypedExpression: TypedExpression{Type: TypeObject, Expression: map[string]TypedExpression{
			"year": {Type: TypeInt, Expression: 2025},
		}}},

		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "target_year"}, TypedExpression: TypedExpression{Type: TypeInt, Expression: "${global.record.year}"}},
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

func TestNewScopeRejectsFanOutReferenceExpression(t *testing.T) {
	_, err := NewScope(
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "years"}, TypedExpression: TypedExpression{Type: TypeList, Expression: []TypedExpression{
			{Type: TypeInt, Expression: 2024},
			{Type: TypeInt, Expression: 2025},
		}}},

		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "target_year"}, TypedExpression: TypedExpression{Type: TypeInt, Expression: "${years[*]}"}},
	)
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestResolverEscapesReferenceExpression(t *testing.T) {
	scope, err := NewScope(Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "template"}, TypedExpression: TypedExpression{Type: TypeString, Expression: `\${year}`}})
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
	scope, err := NewScope(Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "template"}, TypedExpression: TypedExpression{Type: TypeString, Expression: `before \${year} after`}})
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
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "a"}, TypedExpression: TypedExpression{Type: TypeString, Expression: "${b}"}},

		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "b"}, TypedExpression: TypedExpression{Type: TypeString, Expression: "${c}"}},

		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "c"}, TypedExpression: TypedExpression{Type: TypeString, Expression: "done"}},
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
	scope, err := NewScope(Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "year"}, TypedExpression: TypedExpression{Type: TypeInt, Expression: "${missing}"}})
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

func TestResolverResolvesWholeValueReferencesInStructuredExpression(t *testing.T) {
	project, err := NewScope(
		Variable{Name: Name{Namespace: NamespaceProjectConfig, Key: "name"}, TypedExpression: TypedExpression{Type: TypeString, Expression: "project-a"}},
		Variable{Name: Name{Namespace: NamespaceProjectConfig, Key: "capacity"}, TypedExpression: TypedExpression{Type: TypeInt, Expression: 2}},
		Variable{Name: Name{Namespace: NamespaceProjectConfig, Key: "record"}, TypedExpression: TypedExpression{Type: TypeObject, Expression: map[string]TypedExpression{
			"year": {Type: TypeInt, Expression: 2025},
		}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := NewScope(Variable{
		Name: Name{Namespace: NamespaceWorkflow, Key: "settings"},
		TypedExpression: TypedExpression{Type: TypeObject, Expression: map[string]TypedExpression{
			"name":     {Type: TypeString, Expression: "${project_config.name}"},
			"capacity": {Type: TypeInt, Expression: "${capacity}"},
			"year":     {Type: TypeInt, Expression: "${project_config.record.year}"},
			"values": {Type: TypeList, Expression: []TypedExpression{
				{Type: TypeInt, Expression: "${capacity}"},
				{Type: TypeObject, Expression: "${project_config.record}"},
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	value, err := NewResolver(NewSet(project, workflow), ResolverConfig{}).Resolve(Reference{
		Name:      Name{Namespace: NamespaceWorkflow, Key: "settings"},
		Qualified: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value.Object["name"].Value != "project-a" || value.Object["year"].Value != 2025 {
		t.Fatalf("unexpected resolved object: %#v", value.Object)
	}
	if value.Object["values"].List[1].Object["year"].Value != 2025 {
		t.Fatalf("unexpected referenced object: %#v", value.Object["values"].List[1])
	}
}

func TestResolverRejectsStructuredReferenceTypeMismatch(t *testing.T) {
	scope, err := NewScope(
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "capacity"}, TypedExpression: TypedExpression{Type: TypeInt, Expression: 2}},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "settings"}, TypedExpression: TypedExpression{Type: TypeObject, Expression: map[string]TypedExpression{
			"capacity": {Type: TypeString, Expression: "${capacity}"},
		}}},
	)
	if err != nil {
		t.Fatal(err)
	}

	resolver := NewResolver(NewSet(scope), ResolverConfig{})
	if _, err := resolver.Object("settings"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestResolverAppliesMaxDepthToStructuredReference(t *testing.T) {
	scope, err := NewScope(
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "a"}, TypedExpression: TypedExpression{Type: TypeString, Expression: "${b}"}},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "b"}, TypedExpression: TypedExpression{Type: TypeString, Expression: "done"}},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "settings"}, TypedExpression: TypedExpression{Type: TypeObject, Expression: map[string]TypedExpression{
			"value": {Type: TypeString, Expression: "${a}"},
		}}},
	)
	if err != nil {
		t.Fatal(err)
	}

	resolver := NewResolver(NewSet(scope), ResolverConfig{MaxDepth: 2})
	if _, err := resolver.Object("settings"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestResolverInterpolatesStringAndPathScalars(t *testing.T) {
	scope, err := NewScope(
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "name"}, TypedExpression: TypedExpression{Type: TypeString, Expression: "orders"}},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "root"}, TypedExpression: TypedExpression{Type: TypePath, Expression: "/data"}},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "year"}, TypedExpression: TypedExpression{Type: TypeInt, Expression: 2026}},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "enabled"}, TypedExpression: TypedExpression{Type: TypeBool, Expression: true}},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "started"}, TypedExpression: TypedExpression{Type: TypeDatetime, Expression: "2026-06-30T12:00:00Z"}},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "record"}, TypedExpression: TypedExpression{Type: TypeObject, Expression: map[string]TypedExpression{
			"parts": {Type: TypeList, Expression: []TypedExpression{{Type: TypeString, Expression: "raw"}}},
		}}},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "label"}, TypedExpression: TypedExpression{Type: TypeString, Expression: `job-${name}-${year}-${enabled}-${started}-\${literal}`}},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "location"}, TypedExpression: TypedExpression{Type: TypePath, Expression: "${root}/${record.parts[0]}/${year}"}},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "settings"}, TypedExpression: TypedExpression{Type: TypeObject, Expression: map[string]TypedExpression{
			"outputs": {Type: TypeList, Expression: []TypedExpression{{Type: TypePath, Expression: "${root}/${name}"}}},
		}}},
	)
	if err != nil {
		t.Fatal(err)
	}

	resolver := NewResolver(NewSet(scope), ResolverConfig{})
	label, err := resolver.String("label")
	if err != nil {
		t.Fatal(err)
	}
	if label != "job-orders-2026-true-2026-06-30T12:00:00Z-${literal}" {
		t.Fatalf("unexpected label: %q", label)
	}

	location, err := resolver.Resolve(Reference{Name: Name{Namespace: NamespaceWorkflow, Key: "location"}, Qualified: true})
	if err != nil {
		t.Fatal(err)
	}
	if location.Type != TypePath || location.Value != "/data/raw/2026" {
		t.Fatalf("unexpected path: %#v", location)
	}

	settings, err := resolver.Object("settings")
	if err != nil {
		t.Fatal(err)
	}
	nested := settings["outputs"].List[0]
	if nested.Type != TypePath || nested.Value != "/data/orders" {
		t.Fatalf("unexpected nested path: %#v", nested)
	}
}

func TestResolverDoesNotReinterpolateReferencedText(t *testing.T) {
	scope, err := NewScope(
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "template"}, TypedExpression: TypedExpression{Type: TypeString, Expression: "${missing}"}},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "output"}, TypedExpression: TypedExpression{Type: TypeString, Expression: "value=${template}"}},
	)
	if err != nil {
		t.Fatal(err)
	}

	value, err := NewResolver(NewSet(scope), ResolverConfig{}).String("output")
	if err == nil {
		t.Fatalf("expected template's whole-value reference to be resolved, got %q", value)
	}

	literalScope, err := NewScope(
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "template"}, TypedExpression: TypedExpression{Type: TypeString, Expression: `\${missing}`}},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "output"}, TypedExpression: TypedExpression{Type: TypeString, Expression: "value=${template}"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	value, err = NewResolver(NewSet(literalScope), ResolverConfig{}).String("output")
	if err != nil {
		t.Fatal(err)
	}
	if value != "value=${missing}" {
		t.Fatalf("unexpected value: %q", value)
	}
}

func TestResolverRejectsStructuredInterpolationValue(t *testing.T) {
	scope, err := NewScope(
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "items"}, TypedExpression: TypedExpression{Type: TypeList, Expression: []TypedExpression{{Type: TypeString, Expression: "one"}}}},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "label"}, TypedExpression: TypedExpression{Type: TypeString, Expression: "items=${items}"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewResolver(NewSet(scope), ResolverConfig{}).String("label"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestResolverCountsEachInterpolationReferenceTowardMaxDepth(t *testing.T) {
	scope, err := NewScope(
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "name"}, TypedExpression: TypedExpression{Type: TypeString, Expression: "orders"}},
		Variable{Name: Name{Namespace: NamespaceWorkflow, Key: "label"}, TypedExpression: TypedExpression{Type: TypeString, Expression: "job-${name}"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewResolver(NewSet(scope), ResolverConfig{MaxDepth: 1}).String("label"); err == nil {
		t.Fatal("expected an error")
	}
}

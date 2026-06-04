package workflow

import (
	"testing"

	"goetl/internal/variable"
)

func TestCompileStepRejectsUnsupportedStep(t *testing.T) {
	resolver := variable.NewResolver(variable.NewSet(), variable.ResolverConfig{})

	_, err := CompileStep(resolver, Step{ID: "download"})
	if err == nil {
		t.Fatal("expected an error")
	}
}

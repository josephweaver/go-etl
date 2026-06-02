package variable

import "testing"

func TestPrecedenceOrder(t *testing.T) {
	want := []Namespace{
		NamespaceClientEnvironment,
		NamespaceControllerEnvironment,
		NamespaceWorkerEnvironment,
		NamespaceGlobal,
		NamespaceBackend,
		NamespaceProject,
		NamespaceWorkflow,
		NamespaceOverride,
	}

	if len(Precedence) != len(want) {
		t.Fatalf("unexpected precedence length: %d", len(Precedence))
	}

	for i := range want {
		if Precedence[i] != want[i] {
			t.Fatalf("unexpected precedence at %d: got %q want %q", i, Precedence[i], want[i])
		}
	}
}

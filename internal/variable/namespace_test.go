package variable

import "testing"

func TestPrecedenceOrder(t *testing.T) {
	want := []Namespace{
		NamespaceGlobalConfig,
		NamespaceClientEnvironment,
		NamespaceControllerEnvironment,
		NamespaceWorkerEnvironment,
		NamespaceClientConfig,
		NamespaceControllerConfig,
		NamespaceWorkerConfig,
		NamespaceProjectConfig,
		NamespaceWorkflow,
		NamespaceOverride,
		NamespaceStep,
		NamespaceFanOut,
		NamespaceAsset,
		NamespaceWorkItem,
		NamespaceRuntime,
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

func TestLegacyNamespacesRemainValidDuringMigration(t *testing.T) {
	for _, namespace := range []Namespace{
		NamespaceGlobal,
		NamespaceBackend,
		NamespaceProject,
	} {
		if !namespace.Valid() {
			t.Fatalf("legacy namespace %q should remain valid during migration", namespace)
		}
	}
}

package reposource

import (
	"context"
	"testing"
)

func TestProviderInterfaceAcceptsConcreteProviders(t *testing.T) {
	var provider Provider = NewLocalProvider(RepositoryIdentity{Value: "local:demo"}, t.TempDir())
	if _, ok := provider.(LocalProvider); !ok {
		t.Fatal("local provider does not satisfy Provider")
	}
	if _, err := provider.Resolve(context.Background(), "working-tree"); err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
}

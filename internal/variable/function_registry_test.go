package variable

import (
	"fmt"
	"testing"
)

type registryTestFunction struct {
	name FunctionName
}

func (function registryTestFunction) Name() FunctionName {
	return function.name
}

func (function registryTestFunction) Evaluate(args []ResolvedValue) (ResolvedValue, error) {
	return ResolvedValue{}, fmt.Errorf("not implemented")
}

func TestFunctionRegistryLookup(t *testing.T) {
	name, err := ParseFunctionName("list.crossproduct")
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewFunctionRegistry(registryTestFunction{name: name})
	if err != nil {
		t.Fatalf("NewFunctionRegistry() error = %v", err)
	}
	if _, ok := registry.Lookup(name); !ok {
		t.Fatal("Lookup() miss, want hit")
	}

	missing, err := ParseFunctionName("list.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Lookup(missing); ok {
		t.Fatal("Lookup() hit, want miss")
	}
}

func TestFunctionRegistryRejectsDuplicateRegistration(t *testing.T) {
	name, err := ParseFunctionName("list.crossproduct")
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewFunctionRegistry(registryTestFunction{name: name}, registryTestFunction{name: name})
	if err == nil {
		t.Fatal("NewFunctionRegistry() expected duplicate error")
	}
}

func TestDefaultFunctionRegistryIsEmpty(t *testing.T) {
	name, err := ParseFunctionName("list.crossproduct")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := DefaultFunctionRegistry().Lookup(name); ok {
		t.Fatal("DefaultFunctionRegistry() unexpectedly has concrete function")
	}
}

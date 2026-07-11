package variable

import "fmt"

type ExpressionFunction interface {
	Name() FunctionName
	Evaluate(args []ResolvedValue) (ResolvedValue, error)
}

type FunctionRegistry struct {
	functions map[FunctionName]ExpressionFunction
}

func NewFunctionRegistry(functions ...ExpressionFunction) (FunctionRegistry, error) {
	registry := FunctionRegistry{functions: map[FunctionName]ExpressionFunction{}}
	for _, function := range functions {
		if function == nil {
			return FunctionRegistry{}, fmt.Errorf("function is required")
		}
		name := function.Name()
		if err := name.Validate(); err != nil {
			return FunctionRegistry{}, err
		}
		if _, exists := registry.functions[name]; exists {
			return FunctionRegistry{}, fmt.Errorf("duplicate function registration: %s", name)
		}
		registry.functions[name] = function
	}
	return registry, nil
}

func DefaultFunctionRegistry() FunctionRegistry {
	registry, _ := NewFunctionRegistry(
		listCrossproductFunction{},
		listZipFunction{},
		listFlattenFunction{},
		listLengthFunction{},
	)
	return registry
}

func (registry FunctionRegistry) Lookup(name FunctionName) (ExpressionFunction, bool) {
	function, ok := registry.functions[name]
	return function, ok
}

package variable

import "fmt"

type listLengthFunction struct{}

func (function listLengthFunction) Name() FunctionName {
	return FunctionName{Namespace: "list", Name: "length"}
}

func (function listLengthFunction) Evaluate(args []ResolvedValue) (ResolvedValue, error) {
	if len(args) != 1 {
		return ResolvedValue{}, fmt.Errorf("arity = %d, want 1", len(args))
	}
	items := args[0]
	if items.Type != TypeList {
		return ResolvedValue{}, fmt.Errorf("argument 0 has type %s, want list", items.Type)
	}
	return ResolvedValue{Type: TypeInt, Value: len(items.List)}, nil
}

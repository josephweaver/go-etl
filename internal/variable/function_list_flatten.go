package variable

import "fmt"

type listFlattenFunction struct{}

func (function listFlattenFunction) Name() FunctionName {
	return FunctionName{Namespace: "list", Name: "flatten"}
}

func (function listFlattenFunction) Evaluate(args []ResolvedValue) (ResolvedValue, error) {
	if len(args) != 1 {
		return ResolvedValue{}, fmt.Errorf("arity = %d, want 1", len(args))
	}
	items := args[0]
	if items.Type != TypeList {
		return ResolvedValue{}, fmt.Errorf("argument 0 has type %s, want list", items.Type)
	}

	flattened := []ResolvedValue{}
	for index, item := range items.List {
		if item.Type != TypeList {
			return ResolvedValue{}, fmt.Errorf("argument 0[%d] has type %s, want list", index, item.Type)
		}
		flattened = append(flattened, item.List...)
	}
	return ResolvedList(flattened), nil
}

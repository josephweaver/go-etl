package variable

import "fmt"

type listCrossproductFunction struct{}

func (function listCrossproductFunction) Name() FunctionName {
	return FunctionName{Namespace: "list", Name: "crossproduct"}
}

func (function listCrossproductFunction) Evaluate(args []ResolvedValue) (ResolvedValue, error) {
	if len(args) != 2 {
		return ResolvedValue{}, fmt.Errorf("arity = %d, want 2", len(args))
	}
	left := args[0]
	right := args[1]
	if left.Type != TypeList {
		return ResolvedValue{}, fmt.Errorf("argument 0 has type %s, want list", left.Type)
	}
	if right.Type != TypeList {
		return ResolvedValue{}, fmt.Errorf("argument 1 has type %s, want list", right.Type)
	}

	pairs := make([]ResolvedValue, 0, len(left.List)*len(right.List))
	for _, leftItem := range left.List {
		for _, rightItem := range right.List {
			pairs = append(pairs, ResolvedList([]ResolvedValue{leftItem, rightItem}))
		}
	}
	return ResolvedList(pairs), nil
}

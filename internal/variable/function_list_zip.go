package variable

import "fmt"

type listZipFunction struct{}

func (function listZipFunction) Name() FunctionName {
	return FunctionName{Namespace: "list", Name: "zip"}
}

func (function listZipFunction) Evaluate(args []ResolvedValue) (ResolvedValue, error) {
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
	if len(left.List) != len(right.List) {
		return ResolvedValue{}, fmt.Errorf("list length mismatch: left=%d right=%d", len(left.List), len(right.List))
	}

	pairs := make([]ResolvedValue, 0, len(left.List))
	for index, leftItem := range left.List {
		pairs = append(pairs, ResolvedList([]ResolvedValue{leftItem, right.List[index]}))
	}
	return ResolvedList(pairs), nil
}

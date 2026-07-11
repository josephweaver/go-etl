package document

import (
	"fmt"

	"goetl/internal/variable"
)

var expressionDirectiveKeys = map[string]struct{}{
	"$args": {},
	"$call": {},
	"$expr": {},
	"$ref":  {},
	"$type": {},
}

func typedExpressionDirectiveFromObject(fields map[string]any) (variable.TypedExpression, bool, error) {
	if !hasExpressionDirectiveKey(fields) {
		return variable.TypedExpression{}, false, nil
	}
	if _, ok := fields["$expr"]; ok {
		return variable.TypedExpression{}, true, fmt.Errorf("$expr is not supported; use $call with $type and $args")
	}
	if _, ok := fields["$ref"]; ok {
		return variable.TypedExpression{}, true, fmt.Errorf("$ref is only valid inside $args")
	}
	if err := requireExactDirectiveKeys(fields, "$type", "$call", "$args"); err != nil {
		return variable.TypedExpression{}, true, err
	}

	typeName, err := requiredDirectiveString(fields, "$type")
	if err != nil {
		return variable.TypedExpression{}, true, err
	}
	resultType, err := variable.ParseType(typeName)
	if err != nil {
		return variable.TypedExpression{}, true, err
	}
	functionNameText, err := requiredDirectiveString(fields, "$call")
	if err != nil {
		return variable.TypedExpression{}, true, err
	}
	functionName, err := variable.ParseFunctionName(functionNameText)
	if err != nil {
		return variable.TypedExpression{}, true, err
	}
	args, err := directiveArgumentReferences(fields["$args"])
	if err != nil {
		return variable.TypedExpression{}, true, err
	}
	call, err := variable.NewFunctionCallExpression(functionName, resultType, args)
	if err != nil {
		return variable.TypedExpression{}, true, err
	}
	return variable.TypedExpression{Type: resultType, Expression: call}, true, nil
}

func hasExpressionDirectiveKey(fields map[string]any) bool {
	for key := range fields {
		if _, ok := expressionDirectiveKeys[key]; ok {
			return true
		}
	}
	return false
}

func requireExactDirectiveKeys(fields map[string]any, keys ...string) error {
	if len(fields) != len(keys) {
		return fmt.Errorf("expression directive must contain exactly %v", keys)
	}
	for _, key := range keys {
		if _, ok := fields[key]; !ok {
			return fmt.Errorf("expression directive missing %s", key)
		}
	}
	return nil
}

func requiredDirectiveString(fields map[string]any, key string) (string, error) {
	value, ok := fields[key]
	if !ok {
		return "", fmt.Errorf("expression directive missing %s", key)
	}
	text, ok := value.(string)
	if !ok || text == "" {
		return "", fmt.Errorf("expression directive %s must be a non-empty string", key)
	}
	return text, nil
}

func directiveArgumentReferences(value any) ([]variable.FunctionArgumentReference, error) {
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("expression directive $args must be a list")
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("expression directive $args must not be empty")
	}
	args := make([]variable.FunctionArgumentReference, 0, len(items))
	for index, item := range items {
		fields, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expression directive $args[%d] must be an object", index)
		}
		if err := requireExactDirectiveKeys(fields, "$ref"); err != nil {
			return nil, fmt.Errorf("expression directive $args[%d]: %w", index, err)
		}
		ref, err := requiredDirectiveString(fields, "$ref")
		if err != nil {
			return nil, fmt.Errorf("expression directive $args[%d]: %w", index, err)
		}
		arg, err := variable.NewFunctionArgumentReference(ref)
		if err != nil {
			return nil, fmt.Errorf("expression directive $args[%d]: %w", index, err)
		}
		args = append(args, arg)
	}
	return args, nil
}

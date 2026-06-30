package variable

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

func ParseLiteral(variable Variable) (ResolvedValue, error) {
	value, err := parseLiteralExpression(variable.TypedExpression)
	if err != nil {
		return ResolvedValue{}, fmt.Errorf("parse literal variable %s: %w", variable.Name.String(), err)
	}
	return value, nil
}

func parseLiteralExpression(expression TypedExpression) (ResolvedValue, error) {
	switch expression.Type.Kind {
	case KindString:
		value, ok := expression.Expression.(string)
		if !ok {
			return ResolvedValue{}, fmt.Errorf("string expression must be a string")
		}
		return ResolvedValue{Type: TypeString, Value: value}, nil
	case KindInt:
		var value int
		switch typed := expression.Expression.(type) {
		case int:
			value = typed
		case json.Number:
			parsed, err := strconv.Atoi(typed.String())
			if err != nil {
				return ResolvedValue{}, fmt.Errorf("parse int expression: %w", err)
			}
			value = parsed
		default:
			return ResolvedValue{}, fmt.Errorf("int expression must be an integer")
		}
		return ResolvedValue{Type: TypeInt, Value: value}, nil
	case KindBool:
		value, ok := expression.Expression.(bool)
		if !ok {
			return ResolvedValue{}, fmt.Errorf("bool expression must be a boolean")
		}
		return ResolvedValue{Type: TypeBool, Value: value}, nil
	case KindDatetime:
		text, ok := expression.Expression.(string)
		if !ok {
			return ResolvedValue{}, fmt.Errorf("datetime expression must be a string")
		}
		value, err := time.Parse(time.RFC3339, text)
		if err != nil {
			return ResolvedValue{}, fmt.Errorf("parse datetime expression: %w", err)
		}
		return ResolvedValue{Type: TypeDatetime, Value: value}, nil
	case KindPath:
		value, ok := expression.Expression.(string)
		if !ok {
			return ResolvedValue{}, fmt.Errorf("path expression must be a string")
		}
		return ResolvedValue{Type: TypePath, Value: value}, nil
	case KindObject:
		children, ok := expression.Expression.(map[string]TypedExpression)
		if !ok {
			return ResolvedValue{}, fmt.Errorf("object expression must be a typed-expression map")
		}
		fields := make(map[string]ResolvedValue, len(children))
		for key, child := range children {
			resolved, err := parseLiteralExpression(child)
			if err != nil {
				return ResolvedValue{}, fmt.Errorf("parse object field %s: %w", key, err)
			}
			fields[key] = resolved
		}
		return ResolvedObject(fields), nil
	case KindList:
		children, ok := expression.Expression.([]TypedExpression)
		if !ok {
			return ResolvedValue{}, fmt.Errorf("list expression must be a typed-expression list")
		}
		values := make([]ResolvedValue, 0, len(children))
		for index, child := range children {
			resolved, err := parseLiteralExpression(child)
			if err != nil {
				return ResolvedValue{}, fmt.Errorf("parse list item %d: %w", index, err)
			}
			values = append(values, resolved)
		}
		return ResolvedList(values), nil
	default:
		return ResolvedValue{}, fmt.Errorf("unsupported expression type: %s", expression.Type)
	}
}

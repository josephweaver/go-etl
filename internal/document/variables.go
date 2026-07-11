package document

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"

	"goetl/internal/variable"
)

func LoadVariables(values map[string]any, namespace variable.Namespace) ([]variable.Variable, error) {
	if err := validateVariableNamespace(namespace); err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	variables := make([]variable.Variable, 0, len(keys))
	for _, key := range keys {
		expression, err := TypedExpressionFromValue(values[key])
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", key, err)
		}
		variables = append(variables, variable.Variable{
			Name: variable.Name{
				Namespace: namespace,
				Key:       key,
			},
			TypedExpression: expression,
		})
	}

	if _, err := variable.NewScope(variables...); err != nil {
		return nil, err
	}
	return variables, nil
}

func TypedExpressionFromValue(value any) (variable.TypedExpression, error) {
	switch typed := value.(type) {
	case string:
		return variable.TypedExpression{Type: variable.TypeString, Expression: typed}, nil
	case int:
		return variable.TypedExpression{Type: variable.TypeInt, Expression: typed}, nil
	case int64:
		integer, err := int64ToInt(typed)
		if err != nil {
			return variable.TypedExpression{}, err
		}
		return variable.TypedExpression{Type: variable.TypeInt, Expression: integer}, nil
	case json.Number:
		integer, err := jsonNumberToInt(typed)
		if err != nil {
			return variable.TypedExpression{}, err
		}
		return variable.TypedExpression{Type: variable.TypeInt, Expression: integer}, nil
	case bool:
		return variable.TypedExpression{Type: variable.TypeBool, Expression: typed}, nil
	case map[string]any:
		if expression, handled, err := typedExpressionDirectiveFromObject(typed); handled || err != nil {
			return expression, err
		}
		fields := make(map[string]variable.TypedExpression, len(typed))
		for key, child := range typed {
			expression, err := TypedExpressionFromValue(child)
			if err != nil {
				return variable.TypedExpression{}, fmt.Errorf("object field %q: %w", key, err)
			}
			fields[key] = expression
		}
		return variable.TypedExpression{Type: variable.TypeObject, Expression: fields}, nil
	case []any:
		items := make([]variable.TypedExpression, 0, len(typed))
		for index, child := range typed {
			expression, err := TypedExpressionFromValue(child)
			if err != nil {
				return variable.TypedExpression{}, fmt.Errorf("list item %d: %w", index, err)
			}
			items = append(items, expression)
		}
		return variable.TypedExpression{Type: variable.TypeList, Expression: items}, nil
	case nil:
		return variable.TypedExpression{}, fmt.Errorf("null is not supported")
	default:
		return variable.TypedExpression{}, fmt.Errorf("unsupported variable value %T", value)
	}
}

func validateVariableNamespace(namespace variable.Namespace) error {
	switch namespace {
	case variable.NamespaceControllerConfig,
		variable.NamespaceProjectConfig,
		variable.NamespaceWorkflow,
		variable.NamespaceOverride:
		return nil
	default:
		return fmt.Errorf("unsupported variable source namespace %q", namespace)
	}
}

func jsonNumberToInt(number json.Number) (int, error) {
	text := number.String()
	if !isJSONIntegerText(text) {
		return 0, fmt.Errorf("number %q is not a supported integer", text)
	}
	integer, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("number %q is not a supported integer: %w", text, err)
	}
	return int64ToInt(integer)
}

func int64ToInt(value int64) (int, error) {
	if strconv.IntSize == 32 && (value > math.MaxInt32 || value < math.MinInt32) {
		return 0, fmt.Errorf("integer %d is outside int range", value)
	}
	return int(value), nil
}

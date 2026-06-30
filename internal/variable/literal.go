package variable

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"
)

func ParseLiteral(variable Variable) (ResolvedValue, error) {
	switch variable.Type.Kind {
	case KindString:
		return ResolvedValue{Type: TypeString, Value: variable.Expression}, nil
	case KindInt:
		value, err := strconv.Atoi(variable.Expression)
		if err != nil {
			return ResolvedValue{}, fmt.Errorf("parse int variable %s: %w", variable.Name.String(), err)
		}
		return ResolvedValue{Type: TypeInt, Value: value}, nil
	case KindBool:
		value, err := strconv.ParseBool(variable.Expression)
		if err != nil {
			return ResolvedValue{}, fmt.Errorf("parse bool variable %s: %w", variable.Name.String(), err)
		}
		return ResolvedValue{Type: TypeBool, Value: value}, nil
	case KindDatetime:
		value, err := time.Parse(time.RFC3339, variable.Expression)
		if err != nil {
			return ResolvedValue{}, fmt.Errorf("parse datetime variable %s: %w", variable.Name.String(), err)
		}
		return ResolvedValue{Type: TypeDatetime, Value: value}, nil
	case KindPath:
		return ResolvedValue{Type: TypePath, Value: variable.Expression}, nil
	case KindObject:
		var decoded map[string]any
		if err := json.Unmarshal([]byte(variable.Expression), &decoded); err != nil {
			return ResolvedValue{}, fmt.Errorf("parse object variable %s: %w", variable.Name.String(), err)
		}

		fields := make(map[string]ResolvedValue, len(decoded))
		for key, value := range decoded {
			resolved, err := resolvedJSONValue(value)
			if err != nil {
				return ResolvedValue{}, fmt.Errorf("parse object field %s.%s: %w", variable.Name.String(), key, err)
			}
			fields[key] = resolved
		}

		return ResolvedObject(fields), nil
	case KindList:
		var decoded []any
		if err := json.Unmarshal([]byte(variable.Expression), &decoded); err != nil {
			return ResolvedValue{}, fmt.Errorf("parse list variable %s: %w", variable.Name.String(), err)
		}

		values := make([]ResolvedValue, 0, len(decoded))
		for index, value := range decoded {
			resolved, err := resolvedJSONValue(value)
			if err != nil {
				return ResolvedValue{}, fmt.Errorf("parse list element %s[%d]: %w", variable.Name.String(), index, err)
			}
			values = append(values, resolved)
		}

		return ResolvedList(values), nil
	default:
		return ResolvedValue{}, fmt.Errorf("unsupported variable type: %s", variable.Type)
	}
}

func resolvedJSONValue(value any) (ResolvedValue, error) {
	switch typed := value.(type) {
	case string:
		return ResolvedValue{Type: TypeString, Value: typed}, nil
	case float64:
		if math.Trunc(typed) != typed {
			return ResolvedValue{}, fmt.Errorf("number is not an int: %v", typed)
		}
		return ResolvedValue{Type: TypeInt, Value: int(typed)}, nil
	case bool:
		return ResolvedValue{Type: TypeBool, Value: typed}, nil
	case map[string]any:
		fields := make(map[string]ResolvedValue, len(typed))
		for key, field := range typed {
			resolved, err := resolvedJSONValue(field)
			if err != nil {
				return ResolvedValue{}, fmt.Errorf("parse object field %s: %w", key, err)
			}
			fields[key] = resolved
		}
		return ResolvedObject(fields), nil
	case []any:
		return resolvedInferredJSONList(typed)
	default:
		return ResolvedValue{}, fmt.Errorf("unsupported JSON value")
	}
}

func resolvedInferredJSONList(values []any) (ResolvedValue, error) {
	resolved := make([]ResolvedValue, 0, len(values))
	for _, value := range values {
		next, err := resolvedJSONValue(value)
		if err != nil {
			return ResolvedValue{}, err
		}
		resolved = append(resolved, next)
	}

	return ResolvedList(resolved), nil
}

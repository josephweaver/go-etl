package variable

import (
	"fmt"
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
	default:
		return ResolvedValue{}, fmt.Errorf("unsupported variable type: %s", variable.Type)
	}
}

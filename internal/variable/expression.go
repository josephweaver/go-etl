package variable

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

type TypedExpression struct {
	Type       Type
	Expression any
}

func (e TypedExpression) MarshalJSON() ([]byte, error) {
	if !e.Type.Valid() {
		return nil, fmt.Errorf("unsupported expression type: %s", e.Type)
	}

	if err := validateExpressionContainer(e.Type, e.Expression); err != nil {
		return nil, err
	}

	return json.Marshal(struct {
		Type       string `json:"type"`
		Expression any    `json:"expression"`
	}{
		Type:       e.Type.String(),
		Expression: e.Expression,
	})
}

func (e *TypedExpression) UnmarshalJSON(data []byte) error {
	var encoded struct {
		Type       string          `json:"type"`
		Expression json.RawMessage `json:"expression"`
	}
	if err := decodeExpressionJSON(data, &encoded); err != nil {
		return err
	}

	if encoded.Type == "" {
		return fmt.Errorf("typed expression type is required")
	}
	if encoded.Expression == nil {
		return fmt.Errorf("typed expression expression is required")
	}

	expressionType, err := expressionType(encoded.Type)
	if err != nil {
		return err
	}

	expression, err := decodeExpressionValue(expressionType, encoded.Expression)
	if err != nil {
		return err
	}

	e.Type = expressionType
	e.Expression = expression
	return nil
}

func expressionType(name string) (Type, error) {
	types := []Type{
		TypeString,
		TypeInt,
		TypeBool,
		TypeDatetime,
		TypePath,
		TypeList,
		TypeObject,
	}
	for _, candidate := range types {
		if candidate.String() == name {
			return candidate, nil
		}
	}

	return Type{}, fmt.Errorf("unsupported expression type: %s", name)
}

func decodeExpressionValue(expressionType Type, data json.RawMessage) (any, error) {
	switch expressionType {
	case TypeObject:
		var fields map[string]TypedExpression
		if err := json.Unmarshal(data, &fields); err != nil {
			return nil, fmt.Errorf("decode object expression: %w", err)
		}
		if fields == nil {
			return nil, fmt.Errorf("object expression must be a JSON object")
		}
		return fields, nil
	case TypeList:
		var items []TypedExpression
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, fmt.Errorf("decode list expression: %w", err)
		}
		if items == nil {
			return nil, fmt.Errorf("list expression must be a JSON array")
		}
		return items, nil
	default:
		var value any
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("decode %s expression: %w", expressionType, err)
		}
		return value, nil
	}
}

func validateExpressionContainer(expressionType Type, expression any) error {
	switch expressionType {
	case TypeObject:
		if _, ok := expression.(map[string]TypedExpression); !ok {
			return fmt.Errorf("object expression must be a typed-expression map")
		}
	case TypeList:
		if _, ok := expression.([]TypedExpression); !ok {
			return fmt.Errorf("list expression must be a typed-expression list")
		}
	}
	return nil
}

func decodeExpressionJSON(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode typed expression: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode typed expression: multiple JSON values")
		}
		return fmt.Errorf("decode typed expression: %w", err)
	}
	return nil
}

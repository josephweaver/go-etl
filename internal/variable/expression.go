package variable

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

type TypedExpression struct {
	Type       Type
	Expression any
}

type interpolationToken struct {
	start     int
	end       int
	reference string
}

func (e TypedExpression) ValidateDefinition() error {
	if !e.Type.Valid() {
		return fmt.Errorf("unsupported expression type: %s", e.Type)
	}
	if text, ok := e.Expression.(string); ok {
		if isReference, err := validateWholeReferenceText(text); isReference || err != nil {
			return err
		}
	}

	switch e.Type {
	case TypeString, TypePath:
		text, ok := e.Expression.(string)
		if !ok {
			return fmt.Errorf("%s expression must be a string", e.Type)
		}
		_, err := parseInterpolationTokens(text)
		return err
	case TypeInt:
		return validateIntExpression(e.Expression)
	case TypeBool:
		if _, ok := e.Expression.(bool); ok {
			return nil
		}
		return validateWholeReferenceExpression(e.Type, e.Expression)
	case TypeDatetime:
		text, ok := e.Expression.(string)
		if !ok {
			return fmt.Errorf("datetime expression must be a string")
		}
		if ok, err := validateWholeReferenceText(text); ok || err != nil {
			return err
		}
		if _, err := time.Parse(time.RFC3339, text); err != nil {
			return fmt.Errorf("parse datetime expression: %w", err)
		}
		return nil
	case TypeObject:
		fields, ok := e.Expression.(map[string]TypedExpression)
		if !ok {
			return fmt.Errorf("object expression must be a typed-expression map")
		}
		for name, field := range fields {
			if err := field.ValidateDefinition(); err != nil {
				return fmt.Errorf("validate object field %s: %w", name, err)
			}
		}
		return nil
	case TypeList:
		items, ok := e.Expression.([]TypedExpression)
		if !ok {
			return fmt.Errorf("list expression must be a typed-expression list")
		}
		for index, item := range items {
			if err := item.ValidateDefinition(); err != nil {
				return fmt.Errorf("validate list item %d: %w", index, err)
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported expression type: %s", e.Type)
	}
}

func validateIntExpression(expression any) error {
	switch value := expression.(type) {
	case int:
		return nil
	case json.Number:
		if _, err := strconv.Atoi(value.String()); err != nil {
			return fmt.Errorf("parse int expression: %w", err)
		}
		return nil
	default:
		return validateWholeReferenceExpression(TypeInt, expression)
	}
}

func validateWholeReferenceExpression(expressionType Type, expression any) error {
	text, ok := expression.(string)
	if !ok {
		return fmt.Errorf("%s expression must be a literal or whole-value reference", expressionType)
	}

	isReference, err := validateWholeReferenceText(text)
	if err != nil {
		return err
	}
	if !isReference {
		return fmt.Errorf("%s expression must be a literal or whole-value reference", expressionType)
	}
	return nil
}

func validateWholeReferenceText(text string) (bool, error) {
	tokens, err := parseInterpolationTokens(text)
	if err != nil {
		return false, err
	}
	if len(tokens) != 1 || tokens[0].start != 0 || tokens[0].end != len(text) {
		return false, nil
	}
	return true, nil
}

func parseInterpolationTokens(text string) ([]interpolationToken, error) {
	tokens := []interpolationToken{}
	for index := 0; index < len(text); {
		if strings.HasPrefix(text[index:], `\${`) {
			index += len(`\${`)
			continue
		}
		if !strings.HasPrefix(text[index:], "${") {
			index++
			continue
		}

		closeOffset := strings.IndexByte(text[index+2:], '}')
		if closeOffset == -1 {
			return nil, fmt.Errorf("unterminated interpolation token")
		}
		end := index + 2 + closeOffset + 1
		referenceText := strings.TrimSpace(text[index+2 : end-1])
		if err := validateReferenceExpression(referenceText); err != nil {
			return nil, err
		}

		tokens = append(tokens, interpolationToken{
			start:     index,
			end:       end,
			reference: referenceText,
		})
		index = end
	}
	return tokens, nil
}

func validateReferenceExpression(text string) error {
	if strings.Contains(text, "${") || strings.ContainsAny(text, "{}") {
		return fmt.Errorf("nested interpolation token is not supported")
	}

	_, accessor, err := parseReferenceExpression(text)
	if err != nil {
		return fmt.Errorf("parse reference expression: %w", err)
	}
	if accessor == "" {
		return nil
	}

	parts, err := parseScalarAccessors(accessor)
	if err != nil {
		return fmt.Errorf("parse reference accessor: %w", err)
	}
	for _, part := range parts {
		switch part[0] {
		case '.':
			if _, err := parseFieldAccessor(part); err != nil {
				return fmt.Errorf("parse reference accessor: %w", err)
			}
		case '[':
			if _, err := parseIndexAccessor(part); err != nil {
				return fmt.Errorf("parse reference accessor: %w", err)
			}
		}
	}
	return nil
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

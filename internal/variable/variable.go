package variable

import (
	"encoding/json"
	"fmt"
)

type Variable struct {
	Name Name
	TypedExpression
	Sensitive bool
}

type variableNameJSON struct {
	Namespace Namespace `json:"namespace"`
	Key       string    `json:"key"`
}

type variableJSON struct {
	Name       variableNameJSON `json:"name"`
	Type       string           `json:"type"`
	Expression json.RawMessage  `json:"expression"`
	Sensitive  bool             `json:"sensitive,omitempty"`
}

func (v Variable) MarshalJSON() ([]byte, error) {
	if err := v.Validate(); err != nil {
		return nil, err
	}

	return json.Marshal(struct {
		Name       variableNameJSON `json:"name"`
		Type       string           `json:"type"`
		Expression any              `json:"expression"`
		Sensitive  bool             `json:"sensitive,omitempty"`
	}{
		Name: variableNameJSON{
			Namespace: v.Name.Namespace,
			Key:       v.Name.Key,
		},
		Type:       v.Type.String(),
		Expression: v.Expression,
		Sensitive:  v.Sensitive,
	})
}

func (v *Variable) UnmarshalJSON(data []byte) error {
	var encoded variableJSON
	if err := decodeExpressionJSON(data, &encoded); err != nil {
		return err
	}
	if encoded.Type == "" {
		return fmt.Errorf("variable type is required")
	}
	if encoded.Expression == nil {
		return fmt.Errorf("variable expression is required")
	}

	expressionType, err := expressionType(encoded.Type)
	if err != nil {
		return err
	}
	expression, err := decodeExpressionValue(expressionType, encoded.Expression)
	if err != nil {
		return err
	}

	v.Name = Name{
		Namespace: encoded.Name.Namespace,
		Key:       encoded.Name.Key,
	}
	v.TypedExpression = TypedExpression{
		Type:       expressionType,
		Expression: expression,
	}
	v.Sensitive = encoded.Sensitive
	return v.Validate()
}

type ResolvedValue struct {
	Type           Type
	Value          any
	Object         map[string]ResolvedValue
	List           []ResolvedValue
	Sensitive      bool
	RedactionLabel string
	Provenance     string
}

func (v Variable) Validate() error {
	if err := v.Name.Validate(); err != nil {
		return err
	}
	return v.TypedExpression.ValidateDefinition()
}

func ResolvedObject(fields map[string]ResolvedValue) ResolvedValue {
	sensitive, label, provenance := aggregateSensitivity(fields, nil)
	return ResolvedValue{
		Type:           TypeObject,
		Object:         fields,
		Sensitive:      sensitive,
		RedactionLabel: label,
		Provenance:     provenance,
	}
}

func ResolvedList(values []ResolvedValue) ResolvedValue {
	sensitive, label, provenance := aggregateSensitivity(nil, values)
	return ResolvedValue{
		Type:           TypeList,
		List:           values,
		Sensitive:      sensitive,
		RedactionLabel: label,
		Provenance:     provenance,
	}
}

func TypedExpressionFromResolved(value ResolvedValue) (TypedExpression, error) {
	switch value.Type {
	case TypeString, TypePath, TypeDatetime:
		text, ok := value.Value.(string)
		if !ok {
			return TypedExpression{}, fmt.Errorf("invalid %s value", value.Type)
		}
		return TypedExpression{Type: value.Type, Expression: text}, nil
	case TypeBool:
		boolean, ok := value.Value.(bool)
		if !ok {
			return TypedExpression{}, fmt.Errorf("invalid bool value")
		}
		return TypedExpression{Type: value.Type, Expression: boolean}, nil
	case TypeInt:
		integer, ok := value.Value.(int)
		if !ok {
			return TypedExpression{}, fmt.Errorf("invalid int value")
		}
		return TypedExpression{Type: value.Type, Expression: integer}, nil
	case TypeObject:
		fields := make(map[string]TypedExpression, len(value.Object))
		for name, field := range value.Object {
			expression, err := TypedExpressionFromResolved(field)
			if err != nil {
				return TypedExpression{}, fmt.Errorf("convert object field %s: %w", name, err)
			}
			fields[name] = expression
		}
		return TypedExpression{Type: TypeObject, Expression: fields}, nil
	case TypeList:
		items := make([]TypedExpression, 0, len(value.List))
		for index, item := range value.List {
			expression, err := TypedExpressionFromResolved(item)
			if err != nil {
				return TypedExpression{}, fmt.Errorf("convert list item %d: %w", index, err)
			}
			items = append(items, expression)
		}
		return TypedExpression{Type: TypeList, Expression: items}, nil
	default:
		return TypedExpression{}, fmt.Errorf("unsupported resolved value type: %s", value.Type)
	}
}

func OptionalObjectFieldObject(fields map[string]ResolvedValue, name string) (map[string]ResolvedValue, bool, error) {
	value, ok, err := optionalObjectFieldType(fields, name, TypeObject)
	if err != nil || !ok {
		return nil, ok, err
	}
	return value.Object, true, nil
}

func OptionalObjectFieldString(fields map[string]ResolvedValue, name string) (string, bool, error) {
	if fields == nil {
		return "", false, nil
	}
	value, ok := fields[name]
	if !ok {
		return "", false, nil
	}
	if value.Type != TypeString && value.Type != TypePath {
		return "", false, fmt.Errorf("%s has type %s, want string or path", name, value.Type)
	}
	text, ok := value.Value.(string)
	if !ok || text == "" {
		return "", false, fmt.Errorf("%s is required", name)
	}
	return text, true, nil
}

func OptionalObjectFieldStringList(fields map[string]ResolvedValue, name string) ([]string, bool, error) {
	value, ok, err := optionalObjectFieldType(fields, name, TypeList)
	if err != nil || !ok {
		return nil, ok, err
	}

	values := make([]string, 0, len(value.List))
	for index, item := range value.List {
		if item.Type != TypeString {
			return nil, false, fmt.Errorf("%s[%d] has type %s, want string", name, index, item.Type)
		}

		text, ok := item.Value.(string)
		if !ok || text == "" {
			return nil, false, fmt.Errorf("%s[%d] is required", name, index)
		}
		values = append(values, text)
	}
	return values, true, nil
}

func optionalObjectFieldType(fields map[string]ResolvedValue, name string, valueType Type) (ResolvedValue, bool, error) {
	if fields == nil {
		return ResolvedValue{}, false, nil
	}
	value, ok := fields[name]
	if !ok {
		return ResolvedValue{}, false, nil
	}
	if value.Type.String() != valueType.String() {
		return ResolvedValue{}, false, fmt.Errorf("%s has type %s, want %s", name, value.Type, valueType)
	}
	return value, true, nil
}

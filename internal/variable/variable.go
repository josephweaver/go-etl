package variable

import "fmt"

type Variable struct {
	Name       Name
	Type       Type
	Expression string
}

type ResolvedValue struct {
	Type   Type
	Value  any
	Object map[string]ResolvedValue
	List   []ResolvedValue
}

func (v Variable) Validate() error {
	if err := v.Name.Validate(); err != nil {
		return err
	}

	if !v.Type.Valid() {
		return fmt.Errorf("unsupported variable type: %s", v.Type)
	}

	if v.Expression == "" {
		return fmt.Errorf("variable expression is required")
	}

	return nil
}

func ResolvedObject(fields map[string]ResolvedValue) ResolvedValue {
	return ResolvedValue{
		Type:   TypeObject,
		Object: fields,
	}
}

func ResolvedList(values []ResolvedValue) ResolvedValue {
	return ResolvedValue{
		Type: TypeList,
		List: values,
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

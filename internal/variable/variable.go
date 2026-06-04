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

func ResolvedList(element Type, values []ResolvedValue) (ResolvedValue, error) {
	listType := TypeList(element)
	if !listType.Valid() {
		return ResolvedValue{}, fmt.Errorf("unsupported list type: %s", listType)
	}

	for index, value := range values {
		if value.Type != element {
			return ResolvedValue{}, fmt.Errorf("list element %d has type %s, want %s", index, value.Type, element)
		}
	}

	return ResolvedValue{
		Type: listType,
		List: values,
	}, nil
}

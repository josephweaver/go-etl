package variable

import "fmt"

type Variable struct {
	Name       Name
	Type       Type
	Expression string
}

type ResolvedValue struct {
	Type  Type
	Value any
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

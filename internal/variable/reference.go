package variable

import (
	"fmt"
	"strings"
)

type Reference struct {
	Name      Name
	Qualified bool
}

func ParseReference(text string) (Reference, error) {
	if text == "" {
		return Reference{}, fmt.Errorf("variable reference is required")
	}

	parts := strings.Split(text, ".")
	switch len(parts) {
	case 1:
		if parts[0] == "" {
			return Reference{}, fmt.Errorf("variable reference key is required")
		}

		return Reference{
			Name: Name{Key: parts[0]},
		}, nil
	case 2:
		name := Name{
			Namespace: Namespace(parts[0]),
			Key:       parts[1],
		}
		if err := name.Validate(); err != nil {
			return Reference{}, err
		}

		return Reference{
			Name:      name,
			Qualified: true,
		}, nil
	default:
		return Reference{}, fmt.Errorf("variable reference must be key or namespace.key: %s", text)
	}
}

func (r Reference) String() string {
	if r.Qualified {
		return r.Name.String()
	}

	return r.Name.Key
}

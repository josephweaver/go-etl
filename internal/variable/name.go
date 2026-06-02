package variable

import "fmt"

type Name struct {
	Namespace Namespace
	Key       string
}

func (n Name) Validate() error {
	if !n.Namespace.Valid() {
		return fmt.Errorf("unsupported namespace: %s", n.Namespace)
	}

	if n.Key == "" {
		return fmt.Errorf("variable key is required")
	}

	return nil
}

func (n Name) String() string {
	return string(n.Namespace) + "." + n.Key
}

package workflow

import (
	"fmt"
	"strings"

	"goetl/internal/variable"
)

type FanOutItemContext struct {
	Alias string
	Index int
	Value variable.ResolvedValue
}

func (c FanOutItemContext) Resolve(resolver variable.Resolver, referenceText string) (variable.ResolvedValue, bool, error) {
	root, accessor := splitFanOutReferenceAccessor(referenceText)
	switch root {
	case string(variable.NamespaceFanOut), c.Alias, string(variable.NamespaceStep):
		if root == "" {
			break
		}
		if accessor == "" {
			return c.Value, true, nil
		}
		value, err := variable.ApplyAccessor(c.Value, accessor)
		return value, true, err
	}

	reference, err := variable.ParseReference(root)
	if err != nil {
		return variable.ResolvedValue{}, false, err
	}
	value, err := resolver.Resolve(reference)
	if err != nil {
		return variable.ResolvedValue{}, false, err
	}
	if accessor != "" {
		value, err = variable.ApplyAccessor(value, accessor)
		if err != nil {
			return variable.ResolvedValue{}, false, err
		}
	}
	return value, false, nil
}

func splitFanOutReferenceAccessor(text string) (string, string) {
	parts := strings.Split(text, ".")
	if len(parts) >= 2 && (parts[0] == string(variable.NamespaceFanOut) || parts[0] == string(variable.NamespaceStep)) {
		return parts[0], strings.TrimPrefix(text, parts[0])
	}
	if len(parts) >= 2 && variable.Namespace(parts[0]).Valid() {
		key := parts[1]
		if index := strings.Index(key, "["); index != -1 {
			key = key[:index]
		}
		referenceText := strings.Join([]string{parts[0], key}, ".")
		accessor := strings.TrimPrefix(text, referenceText)
		return referenceText, accessor
	}

	index := strings.IndexAny(text, ".[")
	if index == -1 {
		return text, ""
	}
	return text[:index], text[index:]
}

func (c FanOutItemContext) Validate() error {
	if c.Index < 0 {
		return fmt.Errorf("fan-out item index must be non-negative")
	}
	if c.Alias == string(variable.NamespaceFanOut) || c.Alias == string(variable.NamespaceStep) {
		return fmt.Errorf("fan-out alias %q is reserved", c.Alias)
	}
	return nil
}

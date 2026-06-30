package variable

import (
	"fmt"
	"strings"
)

const DefaultMaxDepth = 10

type ResolverConfig struct {
	MaxDepth int
}

type Resolver struct {
	set    Set
	config ResolverConfig
}

func NewResolver(set Set, config ResolverConfig) Resolver {
	if config.MaxDepth == 0 {
		config.MaxDepth = DefaultMaxDepth
	}

	return Resolver{set: set, config: config}
}

func (r Resolver) Resolve(reference Reference) (ResolvedValue, error) {
	return r.resolve(reference, 0)
}

func (r Resolver) Optional(referenceText string) (ResolvedValue, bool, error) {
	reference, err := ParseReference(referenceText)
	if err != nil {
		return ResolvedValue{}, false, err
	}

	if _, ok := r.set.LookupReference(reference); !ok {
		return ResolvedValue{}, false, nil
	}

	value, err := r.resolve(reference, 0)
	if err != nil {
		return ResolvedValue{}, false, err
	}
	return value, true, nil
}

func (r Resolver) String(referenceText string) (string, error) {
	value, err := r.requiredType(referenceText, TypeString)
	if err != nil {
		return "", err
	}
	text, ok := value.Value.(string)
	if !ok || text == "" {
		return "", fmt.Errorf("%s is required", referenceText)
	}
	return text, nil
}

func (r Resolver) OptionalString(referenceText string) (string, bool, error) {
	value, ok, err := r.optionalType(referenceText, TypeString)
	if err != nil || !ok {
		return "", ok, err
	}
	text, ok := value.Value.(string)
	if !ok || text == "" {
		return "", false, fmt.Errorf("%s is required", referenceText)
	}
	return text, true, nil
}

func (r Resolver) PathOrString(referenceText string) (string, error) {
	value, err := r.requiredPathOrString(referenceText)
	if err != nil {
		return "", err
	}
	text, ok := value.Value.(string)
	if !ok || text == "" {
		return "", fmt.Errorf("%s is required", referenceText)
	}
	return text, nil
}

func (r Resolver) OptionalPathOrString(referenceText string) (string, bool, error) {
	value, ok, err := r.optionalPathOrString(referenceText)
	if err != nil || !ok {
		return "", ok, err
	}
	text, ok := value.Value.(string)
	if !ok || text == "" {
		return "", false, fmt.Errorf("%s is required", referenceText)
	}
	return text, true, nil
}

func (r Resolver) Object(referenceText string) (map[string]ResolvedValue, error) {
	value, err := r.requiredType(referenceText, TypeObject)
	if err != nil {
		return nil, err
	}
	return value.Object, nil
}

func (r Resolver) OptionalObject(referenceText string) (map[string]ResolvedValue, bool, error) {
	value, ok, err := r.optionalType(referenceText, TypeObject)
	if err != nil || !ok {
		return nil, ok, err
	}
	return value.Object, true, nil
}

func (r Resolver) StringList(referenceText string) ([]string, error) {
	value, err := r.requiredType(referenceText, TypeList)
	if err != nil {
		return nil, err
	}
	return stringListValue(referenceText, value.List)
}

func (r Resolver) OptionalStringList(referenceText string) ([]string, bool, error) {
	value, ok, err := r.optionalType(referenceText, TypeList)
	if err != nil || !ok {
		return nil, ok, err
	}
	values, err := stringListValue(referenceText, value.List)
	if err != nil {
		return nil, false, err
	}
	return values, true, nil
}

func (r Resolver) ResolveFanOutExpression(expression string) ([]ResolvedValue, error) {
	refText, ok := referenceExpression(expression)
	if !ok {
		return nil, fmt.Errorf("fan-out expression must be a reference expression")
	}

	reference, accessor, err := parseReferenceExpression(refText)
	if err != nil {
		return nil, fmt.Errorf("parse fan-out expression: %w", err)
	}

	if accessor != "[*]" {
		return nil, fmt.Errorf("fan-out expression must end with [*]")
	}

	resolved, err := r.resolve(reference, 0)
	if err != nil {
		return nil, err
	}

	return ApplyFanOutAccessor(resolved, accessor)
}

func (r Resolver) requiredType(referenceText string, valueType Type) (ResolvedValue, error) {
	reference, err := ParseReference(referenceText)
	if err != nil {
		return ResolvedValue{}, err
	}
	value, err := r.Resolve(reference)
	if err != nil {
		return ResolvedValue{}, err
	}
	if value.Type.String() != valueType.String() {
		return ResolvedValue{}, fmt.Errorf("%s has type %s, want %s", referenceText, value.Type, valueType)
	}
	return value, nil
}

func (r Resolver) optionalType(referenceText string, valueType Type) (ResolvedValue, bool, error) {
	value, ok, err := r.Optional(referenceText)
	if err != nil || !ok {
		return ResolvedValue{}, ok, err
	}
	if value.Type.String() != valueType.String() {
		return ResolvedValue{}, false, fmt.Errorf("%s has type %s, want %s", referenceText, value.Type, valueType)
	}
	return value, true, nil
}

func (r Resolver) requiredPathOrString(referenceText string) (ResolvedValue, error) {
	reference, err := ParseReference(referenceText)
	if err != nil {
		return ResolvedValue{}, err
	}
	value, err := r.Resolve(reference)
	if err != nil {
		return ResolvedValue{}, err
	}
	if value.Type != TypePath && value.Type != TypeString {
		return ResolvedValue{}, fmt.Errorf("%s has type %s, want path or string", referenceText, value.Type)
	}
	return value, nil
}

func (r Resolver) optionalPathOrString(referenceText string) (ResolvedValue, bool, error) {
	value, ok, err := r.Optional(referenceText)
	if err != nil || !ok {
		return ResolvedValue{}, ok, err
	}
	if value.Type != TypePath && value.Type != TypeString {
		return ResolvedValue{}, false, fmt.Errorf("%s has type %s, want path or string", referenceText, value.Type)
	}
	return value, true, nil
}

func stringListValue(referenceText string, list []ResolvedValue) ([]string, error) {
	values := make([]string, 0, len(list))
	for index, item := range list {
		if item.Type != TypeString {
			return nil, fmt.Errorf("%s[%d] has type %s, want string", referenceText, index, item.Type)
		}

		text, ok := item.Value.(string)
		if !ok || text == "" {
			return nil, fmt.Errorf("%s[%d] is required", referenceText, index)
		}
		values = append(values, text)
	}
	return values, nil
}

func (r Resolver) resolve(reference Reference, depth int) (ResolvedValue, error) {
	if depth >= r.config.MaxDepth {
		return ResolvedValue{}, fmt.Errorf("maximum variable resolution depth exceeded at %s", reference.String())
	}

	variable, ok := r.set.LookupReference(reference)
	if !ok {
		return ResolvedValue{}, fmt.Errorf("variable not found: %s", reference.String())
	}

	if refText, ok := referenceExpression(variable.Expression); ok {
		next, accessor, err := parseReferenceExpression(refText)
		if err != nil {
			return ResolvedValue{}, fmt.Errorf("parse reference expression for %s: %w", variable.Name.String(), err)
		}

		resolved, err := r.resolve(next, depth+1)
		if err != nil {
			return ResolvedValue{}, err
		}

		if accessor == "" {
			return resolved, nil
		}

		return ApplyAccessor(resolved, accessor)
	}

	variable.Expression = unescapeExpression(variable.Expression)
	return ParseLiteral(variable)
}

func referenceExpression(expression string) (string, bool) {
	if strings.HasPrefix(expression, `\${`) {
		return "", false
	}

	if !strings.HasPrefix(expression, "${") || !strings.HasSuffix(expression, "}") {
		return "", false
	}

	return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(expression, "${"), "}")), true
}

func parseReferenceExpression(text string) (Reference, string, error) {
	referenceText, accessor := splitReferenceAccessor(text)

	reference, err := ParseReference(referenceText)
	if err != nil {
		return Reference{}, "", err
	}

	return reference, accessor, nil
}

func splitReferenceAccessor(text string) (string, string) {
	parts := strings.Split(text, ".")
	if len(parts) >= 2 && Namespace(parts[0]).Valid() {
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

func unescapeExpression(expression string) string {
	return strings.ReplaceAll(expression, `\${`, "${")
}

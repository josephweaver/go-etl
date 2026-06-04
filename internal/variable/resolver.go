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

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

func (r Resolver) resolve(reference Reference, depth int) (ResolvedValue, error) {
	if depth >= r.config.MaxDepth {
		return ResolvedValue{}, fmt.Errorf("maximum variable resolution depth exceeded at %s", reference.String())
	}

	variable, ok := r.set.LookupReference(reference)
	if !ok {
		return ResolvedValue{}, fmt.Errorf("variable not found: %s", reference.String())
	}

	if refText, ok := referenceExpression(variable.Expression); ok {
		next, err := ParseReference(refText)
		if err != nil {
			return ResolvedValue{}, fmt.Errorf("parse reference expression for %s: %w", variable.Name.String(), err)
		}

		return r.resolve(next, depth+1)
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

func unescapeExpression(expression string) string {
	return strings.ReplaceAll(expression, `\${`, "${")
}

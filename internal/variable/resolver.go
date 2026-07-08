package variable

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

const DefaultMaxDepth = 10

type ResolverConfig struct {
	MaxDepth                    int
	ControllerEnvironmentLookup func(string) (string, bool)
}

type Resolver struct {
	set    Set
	config ResolverConfig
	state  *resolverState
}

type resolverState struct {
	mu                    sync.Mutex
	controllerEnvironment map[string]environmentLookupResult
}

type environmentLookupResult struct {
	value string
	ok    bool
}

func NewResolver(set Set, config ResolverConfig) Resolver {
	if config.MaxDepth == 0 {
		config.MaxDepth = DefaultMaxDepth
	}

	return Resolver{
		set:    set,
		config: config,
		state: &resolverState{
			controllerEnvironment: make(map[string]environmentLookupResult),
		},
	}
}

func (r Resolver) Resolve(reference Reference) (ResolvedValue, error) {
	return r.resolveRoot(reference)
}

func (r Resolver) Optional(referenceText string) (ResolvedValue, bool, error) {
	reference, err := ParseReference(referenceText)
	if err != nil {
		return ResolvedValue{}, false, err
	}

	if _, ok := r.lookupVariable(reference); !ok {
		return ResolvedValue{}, false, nil
	}

	value, err := r.resolveRoot(reference)
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

	resolved, err := r.resolveRoot(reference)
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

type resolutionContext struct {
	root  string
	path  string
	chain []Name
}

func (r Resolver) resolveRoot(reference Reference) (ResolvedValue, error) {
	variable, ok := r.lookupVariable(reference)
	if !ok {
		return ResolvedValue{}, fmt.Errorf("resolve %s at /: variable not found: %s", reference.String(), reference.String())
	}

	context := resolutionContext{root: variable.Name.String(), path: "/"}
	value, err := r.resolveVariable(variable, 0, context)
	if err != nil {
		return ResolvedValue{}, fmt.Errorf("resolve %s at %s: %w", context.root, errorPath(err, context.path), err)
	}
	return value, nil
}

func (r Resolver) resolveVariable(variable Variable, depth int, context resolutionContext) (ResolvedValue, error) {
	for index, name := range context.chain {
		if name == variable.Name {
			chain := append(append([]Name{}, context.chain[index:]...), variable.Name)
			parts := make([]string, len(chain))
			for i, item := range chain {
				parts[i] = item.String()
			}
			return ResolvedValue{}, locatedError{path: context.path, cause: fmt.Errorf("reference cycle: %s", strings.Join(parts, " -> "))}
		}
	}
	if depth >= r.config.MaxDepth {
		return ResolvedValue{}, locatedError{path: context.path, cause: fmt.Errorf("maximum variable resolution depth exceeded at %s", variable.Name.String())}
	}

	context.chain = append(append([]Name{}, context.chain...), variable.Name)
	value, err := r.resolveExpression(variable.TypedExpression, depth, context)
	if err != nil {
		return ResolvedValue{}, err
	}
	return mergeSensitivity(value, variable.Sensitive, variable.Name.String()), nil
}

func (r Resolver) resolveReference(reference Reference, depth int, context resolutionContext) (ResolvedValue, error) {
	variable, ok := r.lookupVariable(reference)
	if !ok {
		return ResolvedValue{}, locatedError{path: context.path, cause: fmt.Errorf("variable not found: %s", reference.String())}
	}
	return r.resolveVariable(variable, depth, context)
}

func (r Resolver) lookupVariable(reference Reference) (Variable, bool) {
	if reference.Qualified && reference.Name.Namespace == NamespaceControllerEnvironment {
		return r.lookupControllerEnvironment(reference.Name.Key)
	}
	if item, ok := r.set.LookupReference(reference); ok {
		return item, true
	}
	if !reference.Qualified {
		return r.lookupControllerEnvironment(reference.Name.Key)
	}

	return Variable{}, false
}

func (r Resolver) lookupControllerEnvironment(key string) (Variable, bool) {
	if r.config.ControllerEnvironmentLookup == nil || r.state == nil {
		return Variable{}, false
	}

	r.state.mu.Lock()
	defer r.state.mu.Unlock()

	result, cached := r.state.controllerEnvironment[key]
	if !cached {
		result.value, result.ok = r.config.ControllerEnvironmentLookup(key)
		r.state.controllerEnvironment[key] = result
	}
	if !result.ok {
		return Variable{}, false
	}

	return Variable{
		Name: Name{Namespace: NamespaceControllerEnvironment, Key: key},
		TypedExpression: TypedExpression{
			Type:       TypeString,
			Expression: result.value,
		},
	}, true
}

func (r Resolver) resolveExpression(expression TypedExpression, depth int, context resolutionContext) (value ResolvedValue, err error) {
	defer func() {
		var located locatedError
		if err != nil && !errors.As(err, &located) {
			err = locatedError{path: context.path, cause: err}
		}
	}()
	if expressionText, isText := expression.Expression.(string); isText {
		if refText, ok := wholeValueReferenceExpression(expressionText); ok {
			next, accessor, err := parseReferenceExpression(refText)
			if err != nil {
				return ResolvedValue{}, fmt.Errorf("parse reference expression: %w", err)
			}

			resolved, err := r.resolveReference(next, depth+1, context)
			if err != nil {
				return ResolvedValue{}, err
			}

			if accessor != "" {
				resolved, err = ApplyAccessor(resolved, accessor)
				if err != nil {
					return ResolvedValue{}, err
				}
			}

			if resolved.Type != expression.Type {
				return ResolvedValue{}, fmt.Errorf("reference has type %s, want %s", resolved.Type, expression.Type)
			}
			return resolved, nil
		}
		if expression.Type == TypeString || expression.Type == TypePath {
			interpolated, sensitive, label, provenance, err := r.interpolate(expressionText, depth, context)
			if err != nil {
				return ResolvedValue{}, err
			}
			expression.Expression = interpolated
			defer func() {
				if err == nil {
					value = mergeSensitivity(value, sensitive, provenance)
					if value.RedactionLabel == "" && label != "" {
						value.RedactionLabel = label
					}
				}
			}()
		} else {
			expression.Expression = unescapeExpression(expressionText)
		}
	}

	switch expression.Type {
	case TypeObject:
		children, ok := expression.Expression.(map[string]TypedExpression)
		if !ok {
			return ResolvedValue{}, locatedError{path: context.path, cause: fmt.Errorf("object expression must be a typed-expression map")}
		}
		fields := make(map[string]ResolvedValue, len(children))
		for name, child := range children {
			childContext := context
			childContext.path = appendJSONPointer(context.path, name)
			resolved, err := r.resolveExpression(child, depth, childContext)
			if err != nil {
				return ResolvedValue{}, err
			}
			fields[name] = resolved
		}
		return ResolvedObject(fields), nil
	case TypeList:
		children, ok := expression.Expression.([]TypedExpression)
		if !ok {
			return ResolvedValue{}, locatedError{path: context.path, cause: fmt.Errorf("list expression must be a typed-expression list")}
		}
		values := make([]ResolvedValue, 0, len(children))
		for index, child := range children {
			childContext := context
			childContext.path = appendJSONPointer(context.path, strconv.Itoa(index))
			resolved, err := r.resolveExpression(child, depth, childContext)
			if err != nil {
				return ResolvedValue{}, err
			}
			values = append(values, resolved)
		}
		return ResolvedList(values), nil
	default:
		value, err := parseLiteralExpression(expression)
		if err != nil {
			return ResolvedValue{}, locatedError{path: context.path, cause: err}
		}
		return value, nil
	}
}

func (r Resolver) interpolate(expression string, depth int, context resolutionContext) (string, bool, string, string, error) {
	tokens, err := parseInterpolationTokens(expression)
	if err != nil {
		return "", false, "", "", err
	}

	var result strings.Builder
	sensitive := false
	label := ""
	provenance := ""
	previous := 0
	for _, token := range tokens {
		result.WriteString(unescapeExpression(expression[previous:token.start]))

		reference, accessor, err := parseReferenceExpression(token.reference)
		if err != nil {
			return "", false, "", "", fmt.Errorf("parse reference expression: %w", err)
		}
		resolved, err := r.resolveReference(reference, depth+1, context)
		if err != nil {
			return "", false, "", "", err
		}
		if accessor != "" {
			resolved, err = ApplyAccessor(resolved, accessor)
			if err != nil {
				return "", false, "", "", err
			}
		}
		if resolved.Sensitive && !sensitive {
			sensitive = true
			label = resolved.RedactionLabel
			provenance = resolved.Provenance
		}

		text, err := interpolationText(resolved)
		if err != nil {
			return "", false, "", "", fmt.Errorf("interpolate %s: %w", token.reference, err)
		}
		result.WriteString(text)
		previous = token.end
	}
	result.WriteString(unescapeExpression(expression[previous:]))
	return result.String(), sensitive, label, provenance, nil
}

type locatedError struct {
	path  string
	cause error
}

func (e locatedError) Error() string { return e.cause.Error() }
func (e locatedError) Unwrap() error { return e.cause }

func errorPath(err error, fallback string) string {
	var located locatedError
	if errors.As(err, &located) {
		return located.path
	}
	return fallback
}

func appendJSONPointer(path, segment string) string {
	segment = strings.ReplaceAll(segment, "~", "~0")
	segment = strings.ReplaceAll(segment, "/", "~1")
	if path == "/" {
		return "/" + segment
	}
	return path + "/" + segment
}

func interpolationText(value ResolvedValue) (string, error) {
	switch value.Type {
	case TypeString, TypePath:
		text, ok := value.Value.(string)
		if !ok {
			return "", fmt.Errorf("invalid %s value", value.Type)
		}
		return text, nil
	case TypeInt:
		integer, ok := value.Value.(int)
		if !ok {
			return "", fmt.Errorf("invalid int value")
		}
		return strconv.Itoa(integer), nil
	case TypeBool:
		boolean, ok := value.Value.(bool)
		if !ok {
			return "", fmt.Errorf("invalid bool value")
		}
		return strconv.FormatBool(boolean), nil
	case TypeDatetime:
		datetime, ok := value.Value.(time.Time)
		if !ok {
			return "", fmt.Errorf("invalid datetime value")
		}
		return datetime.Format(time.RFC3339), nil
	default:
		return "", fmt.Errorf("value has type %s, want scalar", value.Type)
	}
}

func wholeValueReferenceExpression(expression string) (string, bool) {
	tokens, err := parseInterpolationTokens(expression)
	if err != nil || len(tokens) != 1 || tokens[0].start != 0 || tokens[0].end != len(expression) {
		return "", false
	}
	return tokens[0].reference, true
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

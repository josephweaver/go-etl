package variable

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var functionIdentifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type FunctionName struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

type FunctionArgumentReference struct {
	Expression string    `json:"expression"`
	Reference  Reference `json:"reference"`
	Accessor   string    `json:"accessor,omitempty"`
}

type FunctionCallExpression struct {
	Name       FunctionName                `json:"name"`
	ResultType Type                        `json:"result_type"`
	Arguments  []FunctionArgumentReference `json:"arguments"`
}

func ParseType(name string) (Type, error) {
	return expressionType(name)
}

func ParseFunctionName(text string) (FunctionName, error) {
	parts := strings.Split(text, ".")
	if len(parts) != 2 {
		return FunctionName{}, fmt.Errorf("function name must be namespace.name: %s", text)
	}
	return NewFunctionName(parts[0], parts[1])
}

func NewFunctionName(namespace string, name string) (FunctionName, error) {
	if !functionIdentifierPattern.MatchString(namespace) {
		return FunctionName{}, fmt.Errorf("function namespace must be an identifier: %s", namespace)
	}
	if !functionIdentifierPattern.MatchString(name) {
		return FunctionName{}, fmt.Errorf("function name must be an identifier: %s", name)
	}
	return FunctionName{Namespace: namespace, Name: name}, nil
}

func (name FunctionName) String() string {
	if name.Namespace == "" || name.Name == "" {
		return ""
	}
	return name.Namespace + "." + name.Name
}

func (name FunctionName) Validate() error {
	_, err := NewFunctionName(name.Namespace, name.Name)
	return err
}

func NewFunctionArgumentReference(text string) (FunctionArgumentReference, error) {
	if strings.TrimSpace(text) == "" {
		return FunctionArgumentReference{}, fmt.Errorf("function argument reference is required")
	}
	if strings.TrimSpace(text) != text {
		return FunctionArgumentReference{}, fmt.Errorf("function argument reference must not contain leading or trailing whitespace")
	}
	if err := validateFunctionArgumentReferenceText(text); err != nil {
		return FunctionArgumentReference{}, err
	}
	if err := validateReferenceExpression(text); err != nil {
		return FunctionArgumentReference{}, err
	}
	reference, accessor, err := parseReferenceExpression(text)
	if err != nil {
		return FunctionArgumentReference{}, err
	}
	return FunctionArgumentReference{
		Expression: text,
		Reference:  reference,
		Accessor:   accessor,
	}, nil
}

func validateFunctionArgumentReferenceText(text string) error {
	if strings.ContainsAny(text, "\"'()+, ") {
		return fmt.Errorf("function argument must be a reference with optional accessors: %s", text)
	}
	referenceText, accessor := splitReferenceAccessor(text)
	reference, err := ParseReference(referenceText)
	if err != nil {
		return err
	}
	if reference.Name.Key == "true" || reference.Name.Key == "false" || reference.Name.Key == "null" {
		return fmt.Errorf("function argument must not be a literal: %s", text)
	}
	if !functionIdentifierPattern.MatchString(reference.Name.Key) {
		return fmt.Errorf("function argument reference key must be an identifier: %s", reference.Name.Key)
	}
	if accessor == "" {
		return nil
	}
	parts, err := parseScalarAccessors(accessor)
	if err != nil {
		return err
	}
	for _, part := range parts {
		if strings.HasPrefix(part, ".") {
			field := strings.TrimPrefix(part, ".")
			if !functionIdentifierPattern.MatchString(field) {
				return fmt.Errorf("function argument field accessor must be an identifier: %s", field)
			}
		}
	}
	return nil
}

func (argument FunctionArgumentReference) Validate() error {
	parsed, err := NewFunctionArgumentReference(argument.Expression)
	if err != nil {
		return err
	}
	if parsed.Reference != argument.Reference || parsed.Accessor != argument.Accessor {
		return fmt.Errorf("function argument reference fields do not match expression %q", argument.Expression)
	}
	return nil
}

func NewFunctionCallExpression(name FunctionName, resultType Type, arguments []FunctionArgumentReference) (FunctionCallExpression, error) {
	call := FunctionCallExpression{Name: name, ResultType: resultType, Arguments: append([]FunctionArgumentReference(nil), arguments...)}
	if err := call.Validate(); err != nil {
		return FunctionCallExpression{}, err
	}
	return call, nil
}

func (call FunctionCallExpression) Validate() error {
	if err := call.Name.Validate(); err != nil {
		return err
	}
	if !call.ResultType.Valid() {
		return fmt.Errorf("function result type is unsupported: %s", call.ResultType)
	}
	if len(call.Arguments) == 0 {
		return fmt.Errorf("function call requires at least one argument")
	}
	for index, argument := range call.Arguments {
		if err := argument.Validate(); err != nil {
			return fmt.Errorf("function argument %d: %w", index, err)
		}
	}
	return nil
}

func (call FunctionCallExpression) MarshalJSON() ([]byte, error) {
	if err := call.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Name       string                      `json:"name"`
		ResultType string                      `json:"result_type"`
		Arguments  []FunctionArgumentReference `json:"arguments"`
	}{
		Name:       call.Name.String(),
		ResultType: call.ResultType.String(),
		Arguments:  call.Arguments,
	})
}

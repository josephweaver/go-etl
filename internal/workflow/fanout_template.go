package workflow

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"goetl/internal/variable"
)

type fanOutTemplateSegment struct {
	start     int
	end       int
	reference string
}

func renderFanOutTemplate(resolver variable.Resolver, context FanOutItemContext, template string, rejectSensitive bool) (string, error) {
	tokens, err := parseFanOutTemplateTokens(template)
	if err != nil {
		return "", err
	}

	var result strings.Builder
	previous := 0
	for _, token := range tokens {
		result.WriteString(unescapeFanOutTemplate(template[previous:token.start]))
		value, _, err := context.Resolve(resolver, token.reference)
		if err != nil {
			return "", fmt.Errorf("%s: %w", token.reference, err)
		}
		if rejectSensitive && fanOutValueIsSensitive(value) {
			return "", fmt.Errorf("%s resolves to sensitive value %s", token.reference, fanOutRedactionLabel(value))
		}
		text, err := renderFanOutScalar(value)
		if err != nil {
			return "", fmt.Errorf("%s: %w", token.reference, err)
		}
		result.WriteString(text)
		previous = token.end
	}
	result.WriteString(unescapeFanOutTemplate(template[previous:]))
	return result.String(), nil
}

func resolveFanOutWholeReference(resolver variable.Resolver, context FanOutItemContext, expression string) (variable.ResolvedValue, bool, error) {
	tokens, err := parseFanOutTemplateTokens(expression)
	if err != nil || len(tokens) != 1 || tokens[0].start != 0 || tokens[0].end != len(expression) {
		return variable.ResolvedValue{}, false, err
	}
	value, _, err := context.Resolve(resolver, tokens[0].reference)
	return value, true, err
}

func parseFanOutTemplateTokens(template string) ([]fanOutTemplateSegment, error) {
	tokens := []fanOutTemplateSegment{}
	for index := 0; index < len(template); {
		if strings.HasPrefix(template[index:], `\${`) {
			index += len(`\${`)
			continue
		}
		if !strings.HasPrefix(template[index:], "${") {
			_, size := utf8.DecodeRuneInString(template[index:])
			if size == 0 {
				size = 1
			}
			index += size
			continue
		}

		closeOffset := strings.IndexByte(template[index+2:], '}')
		if closeOffset == -1 {
			return nil, fmt.Errorf("unterminated template placeholder")
		}
		end := index + 2 + closeOffset + 1
		referenceText := strings.TrimSpace(template[index+2 : end-1])
		if referenceText == "" {
			return nil, fmt.Errorf("template placeholder reference is required")
		}
		if strings.Contains(referenceText, "${") || strings.ContainsAny(referenceText, "{}") {
			return nil, fmt.Errorf("nested template placeholder is not supported")
		}
		root, _ := splitFanOutReferenceAccessor(referenceText)
		if _, err := variable.ParseReference(root); err != nil {
			return nil, fmt.Errorf("parse template placeholder %q: %w", referenceText, err)
		}
		tokens = append(tokens, fanOutTemplateSegment{
			start:     index,
			end:       end,
			reference: referenceText,
		})
		index = end
	}
	return tokens, nil
}

func renderFanOutScalar(value variable.ResolvedValue) (string, error) {
	switch value.Type {
	case variable.TypeString, variable.TypePath:
		text, ok := value.Value.(string)
		if !ok {
			return "", fmt.Errorf("invalid %s value", value.Type)
		}
		return text, nil
	case variable.TypeInt:
		integer, ok := value.Value.(int)
		if !ok {
			return "", fmt.Errorf("invalid int value")
		}
		return strconv.Itoa(integer), nil
	case variable.TypeBool:
		boolean, ok := value.Value.(bool)
		if !ok {
			return "", fmt.Errorf("invalid bool value")
		}
		return strconv.FormatBool(boolean), nil
	default:
		return "", fmt.Errorf("value has type %s, select a scalar field or index", value.Type)
	}
}

func fanOutValueIsSensitive(value variable.ResolvedValue) bool {
	return value.Sensitive || value.ProtectedRef != nil
}

func fanOutRedactionLabel(value variable.ResolvedValue) string {
	if value.RedactionLabel != "" {
		return value.RedactionLabel
	}
	if value.ProtectedRef != nil {
		return value.ProtectedRef.RedactionLabelValue()
	}
	return "[REDACTED]"
}

func unescapeFanOutTemplate(template string) string {
	return strings.ReplaceAll(template, `\${`, "${")
}

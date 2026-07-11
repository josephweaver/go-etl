package document

import (
	"fmt"
	"strings"
)

type SourceFormat string

const (
	SourceFormatJSON SourceFormat = "json"
	SourceFormatYAML SourceFormat = "yaml"
)

type DecodeOptions struct {
	Path      string
	MediaType string
	Format    SourceFormat
}

type SourceError struct {
	Path   string
	Line   int
	Column int
	Err    error
}

func (err SourceError) Error() string {
	location := err.Path
	if location == "" {
		location = "<source>"
	}
	if err.Line > 0 {
		location = fmt.Sprintf("%s:%d:%d", location, err.Line, err.Column)
	}
	return fmt.Sprintf("%s: %v", location, err.Err)
}

func (err SourceError) Unwrap() error {
	return err.Err
}

func sourceError(path string, line int, column int, format string, err error) error {
	if err == nil {
		return nil
	}
	if format != "" {
		err = fmt.Errorf("%s: %w", format, err)
	}
	return SourceError{
		Path:   path,
		Line:   line,
		Column: column,
		Err:    err,
	}
}

func valuePath(parent string, field string) string {
	if parent == "" {
		parent = "$"
	}
	if field == "" {
		return parent
	}
	if parent == "$" {
		return "$." + field
	}
	return parent + "." + field
}

func indexPath(parent string, index int) string {
	if parent == "" {
		parent = "$"
	}
	return fmt.Sprintf("%s[%d]", parent, index)
}

func isJSONIntegerText(text string) bool {
	if text == "" || strings.ContainsAny(text, ".eE") {
		return false
	}
	if strings.HasPrefix(text, "-") {
		text = text[1:]
		if text == "" {
			return false
		}
	}
	if len(text) > 1 && text[0] == '0' {
		return false
	}
	for _, char := range text {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func offsetLineColumn(data []byte, offset int64) (int, int) {
	if offset < 0 {
		offset = 0
	}
	if offset > int64(len(data)) {
		offset = int64(len(data))
	}

	line := 1
	column := 1
	for _, char := range string(data[:offset]) {
		if char == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}
	return line, column
}

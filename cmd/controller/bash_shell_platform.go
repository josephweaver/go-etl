package main

import (
	"fmt"
	"strings"
)

type BashShellPlatform struct{}

func (BashShellPlatform) Newline() string {
	return "\n"
}

func (BashShellPlatform) QuoteArg(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func (p BashShellPlatform) LocalizePath(value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("path is required")
	}
	if containsNewline(value) {
		return "", fmt.Errorf("path must not contain newlines")
	}
	return value, nil
}

func (p BashShellPlatform) CopyCommand(src string, dest string) (string, error) {
	src, err := p.LocalizePath(src)
	if err != nil {
		return "", fmt.Errorf("copy source: %w", err)
	}
	dest, err = p.LocalizePath(dest)
	if err != nil {
		return "", fmt.Errorf("copy destination: %w", err)
	}
	return "cp " + p.QuoteArg(src) + " " + p.QuoteArg(dest), nil
}

func (p BashShellPlatform) RemoveFileCommand(path string) (string, error) {
	path, err := p.LocalizePath(path)
	if err != nil {
		return "", err
	}
	return "rm -f " + p.QuoteArg(path), nil
}

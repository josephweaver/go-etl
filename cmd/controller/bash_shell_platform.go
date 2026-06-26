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

func (p BashShellPlatform) MakeDirectoryCommand(path string) (string, error) {
	path, err := p.LocalizePath(path)
	if err != nil {
		return "", err
	}
	return "mkdir -p " + p.QuoteArg(path), nil
}

func (p BashShellPlatform) MoveCommand(src string, dest string) (string, error) {
	src, err := p.LocalizePath(src)
	if err != nil {
		return "", fmt.Errorf("move source: %w", err)
	}
	dest, err = p.LocalizePath(dest)
	if err != nil {
		return "", fmt.Errorf("move destination: %w", err)
	}
	return "mv " + p.QuoteArg(src) + " " + p.QuoteArg(dest), nil
}

func (p BashShellPlatform) RemoveFileCommand(path string) (string, error) {
	path, err := p.LocalizePath(path)
	if err != nil {
		return "", err
	}
	return "rm -f " + p.QuoteArg(path), nil
}

func (p BashShellPlatform) RemoveTreeCommand(path string) (string, error) {
	path, err := p.LocalizePath(path)
	if err != nil {
		return "", err
	}
	if path == "/" {
		return "", fmt.Errorf("recursive remove path must not be root")
	}
	return "rm -rf " + p.QuoteArg(path), nil
}

func (p BashShellPlatform) ChmodCommand(mode string, path string) (string, error) {
	if mode == "" {
		return "", fmt.Errorf("chmod mode is required")
	}
	if containsNewline(mode) {
		return "", fmt.Errorf("chmod mode must not contain newlines")
	}
	path, err := p.LocalizePath(path)
	if err != nil {
		return "", err
	}
	return "chmod " + p.QuoteArg(mode) + " " + p.QuoteArg(path), nil
}

func (p BashShellPlatform) ChownCommand(owner string, path string) (string, error) {
	if owner == "" {
		return "", fmt.Errorf("chown owner is required")
	}
	if containsNewline(owner) {
		return "", fmt.Errorf("chown owner must not contain newlines")
	}
	path, err := p.LocalizePath(path)
	if err != nil {
		return "", err
	}
	return "chown " + p.QuoteArg(owner) + " " + p.QuoteArg(path), nil
}

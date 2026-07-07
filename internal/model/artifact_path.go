package model

import (
	"fmt"
	"path"
	"strings"
	"unicode"
)

func ValidateArtifactRelativePath(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("artifact path is required")
	}
	if trimmed != value {
		return "", fmt.Errorf("artifact path must not contain leading or trailing whitespace")
	}
	if strings.Contains(value, "\\") {
		return "", fmt.Errorf("artifact path must use forward slashes")
	}
	if strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return "", fmt.Errorf("artifact path must be relative")
	}
	if hasArtifactDrivePrefix(value) {
		return "", fmt.Errorf("artifact path must be relative")
	}

	segments := strings.Split(value, "/")
	for _, segment := range segments {
		switch segment {
		case "":
			return "", fmt.Errorf("artifact path must not contain empty segments")
		case ".":
			return "", fmt.Errorf("artifact path must not contain . segments")
		case "..":
			return "", fmt.Errorf("artifact path must not contain .. segments")
		}
	}

	clean := path.Clean(value)
	if clean == "." || clean == "/" || strings.HasPrefix(clean, "/") {
		return "", fmt.Errorf("artifact path must be relative")
	}
	if strings.Contains(clean, "\\") {
		return "", fmt.Errorf("artifact path must use forward slashes")
	}
	if hasArtifactDrivePrefix(clean) {
		return "", fmt.Errorf("artifact path must be relative")
	}
	if clean != value {
		return "", fmt.Errorf("artifact path must be a clean relative path")
	}

	return clean, nil
}

func hasArtifactDrivePrefix(value string) bool {
	if len(value) < 2 {
		return false
	}
	if value[1] != ':' {
		return false
	}
	return unicode.IsLetter(rune(value[0]))
}

package reposource

import (
	"fmt"
	"path"
	"strings"
	"unicode"
)

func ValidateRepositoryRelativePath(value string) (string, error) {
	return validateSlashPath(value)
}

func ValidateCacheRelativePath(value string) (string, error) {
	return validateSlashPath(value)
}

func validateSlashPath(value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("path is required")
	}
	if value == "." {
		return "", fmt.Errorf("path is required")
	}
	if strings.Contains(value, "\\") {
		return "", fmt.Errorf("path must use forward slashes")
	}
	if strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return "", fmt.Errorf("path must be repository-relative")
	}
	if hasDrivePrefix(value) {
		return "", fmt.Errorf("path must be repository-relative")
	}
	if hasOriginalDotDotSegment(value) {
		return "", fmt.Errorf("path must not contain .. segments")
	}

	clean := path.Clean(value)
	if clean == "." || clean == "/" || strings.HasPrefix(clean, "/") {
		return "", fmt.Errorf("path must be repository-relative")
	}
	if strings.Contains(clean, "\\") {
		return "", fmt.Errorf("path must use forward slashes")
	}
	if hasDrivePrefix(clean) {
		return "", fmt.Errorf("path must be repository-relative")
	}
	return clean, nil
}

func hasOriginalDotDotSegment(value string) bool {
	segments := strings.Split(value, "/")
	for _, segment := range segments {
		if segment == ".." {
			return true
		}
	}
	return false
}

func hasDrivePrefix(value string) bool {
	if len(value) < 2 {
		return false
	}
	if value[1] != ':' {
		return false
	}
	return unicode.IsLetter(rune(value[0]))
}

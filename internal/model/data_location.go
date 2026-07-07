package model

import (
	"fmt"
	"strings"
	"unicode"
)

const (
	DataLocationTypeRegistered = "registered_location"

	DataLocationAccessReadOnly  = "read_only"
	DataLocationAccessWriteOnly = "write_only"
	DataLocationAccessReadWrite = "read_write"
)

type DataLocation struct {
	Name     string         `json:"name"`
	Type     string         `json:"type"`
	Access   string         `json:"access,omitempty"`
	RootRef  string         `json:"root_ref,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type DataLocationPathTemplate struct {
	Name         string `json:"name"`
	PathTemplate string `json:"path_template"`
}

func (location DataLocation) Validate() error {
	if err := validateDataName(location.Name, "data location name"); err != nil {
		return err
	}
	if location.Type != DataLocationTypeRegistered {
		return fmt.Errorf("unsupported data location type %q", location.Type)
	}
	if location.Access != "" && !isSupportedDataLocationAccess(location.Access) {
		return fmt.Errorf("unsupported data location access %q", location.Access)
	}
	if strings.TrimSpace(location.RootRef) != location.RootRef {
		return fmt.Errorf("data location root_ref must not contain leading or trailing whitespace")
	}
	return nil
}

func (location DataLocationPathTemplate) Validate() error {
	if err := validateDataName(location.Name, "data location name"); err != nil {
		return err
	}
	if _, err := validateDataRelativePath(location.PathTemplate, "data location path_template"); err != nil {
		return err
	}
	return nil
}

func isSupportedDataLocationAccess(access string) bool {
	switch access {
	case DataLocationAccessReadOnly, DataLocationAccessWriteOnly, DataLocationAccessReadWrite:
		return true
	default:
		return false
	}
}

func validateDataName(value, field string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must not contain leading or trailing whitespace", field)
	}
	for _, r := range value {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '_', r == '-', r == '.':
			continue
		default:
			return fmt.Errorf("%s contains unsupported character %q", field, r)
		}
	}
	return nil
}

func validateDataRelativePath(value, field string) (string, error) {
	clean, err := ValidateArtifactRelativePath(value)
	if err != nil {
		return "", fmt.Errorf("%s: %w", field, err)
	}
	return clean, nil
}

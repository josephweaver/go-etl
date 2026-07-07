package model

import "fmt"

const (
	DataAssetArchiveTypeZip      = "zip"
	DataAssetArchiveTypeSevenZip = "seven_zip"

	DataAssetArchiveExposeSelectedPath      = "selected_path"
	DataAssetArchiveExposeSelectedDirectory = "selected_directory"
)

type DataAssetArchiveTemplate struct {
	Type   string                           `json:"type"`
	Select []DataAssetArchiveSelectTemplate `json:"select,omitempty"`
	Expose string                           `json:"expose,omitempty"`
}

type DataAssetArchiveSelectTemplate struct {
	MemberTemplate string `json:"member_template"`
	As             string `json:"as,omitempty"`
	Required       *bool  `json:"required,omitempty"`
}

type DataAssetArchive struct {
	Type   string                   `json:"type"`
	Select []DataAssetArchiveSelect `json:"select,omitempty"`
	Expose string                   `json:"expose,omitempty"`
}

type DataAssetArchiveSelect struct {
	Member   string `json:"member"`
	As       string `json:"as,omitempty"`
	Required *bool  `json:"required,omitempty"`
}

func (archive DataAssetArchiveTemplate) Validate() error {
	if err := validateArchiveType(archive.Type); err != nil {
		return err
	}
	if err := validateArchiveExpose(archive.Expose); err != nil {
		return err
	}
	requiredCount := 0
	for i, selector := range archive.Select {
		if err := selector.Validate(); err != nil {
			return fmt.Errorf("archive select %d: %w", i, err)
		}
		if selector.effectiveRequired() {
			requiredCount++
		}
	}
	return validateArchiveExposeSelection(archive.Expose, requiredCount)
}

func (selector DataAssetArchiveSelectTemplate) Validate() error {
	if _, err := validateDataRelativePath(selector.MemberTemplate, "archive member_template"); err != nil {
		return err
	}
	if selector.As != "" {
		if _, err := validateDataRelativePath(selector.As, "archive as path"); err != nil {
			return err
		}
	}
	return nil
}

func (archive DataAssetArchive) Validate() error {
	if err := validateArchiveType(archive.Type); err != nil {
		return err
	}
	if err := validateArchiveExpose(archive.Expose); err != nil {
		return err
	}
	requiredCount := 0
	for i, selector := range archive.Select {
		if err := selector.Validate(); err != nil {
			return fmt.Errorf("archive select %d: %w", i, err)
		}
		if selector.effectiveRequired() {
			requiredCount++
		}
	}
	return validateArchiveExposeSelection(archive.Expose, requiredCount)
}

func (selector DataAssetArchiveSelect) Validate() error {
	if _, err := validateDataRelativePath(selector.Member, "archive member"); err != nil {
		return err
	}
	if selector.As != "" {
		if _, err := validateDataRelativePath(selector.As, "archive as path"); err != nil {
			return err
		}
	}
	return nil
}

func validateArchiveType(archiveType string) error {
	switch archiveType {
	case DataAssetArchiveTypeZip, DataAssetArchiveTypeSevenZip:
		return nil
	default:
		return fmt.Errorf("unsupported archive type %q", archiveType)
	}
}

func validateArchiveExpose(expose string) error {
	switch expose {
	case "", DataAssetArchiveExposeSelectedPath, DataAssetArchiveExposeSelectedDirectory:
		return nil
	default:
		return fmt.Errorf("unsupported archive expose %q", expose)
	}
}

func validateArchiveExposeSelection(expose string, requiredCount int) error {
	if expose == DataAssetArchiveExposeSelectedPath && requiredCount != 1 {
		return fmt.Errorf("archive expose selected_path requires exactly one required member")
	}
	return nil
}

func (selector DataAssetArchiveSelectTemplate) effectiveRequired() bool {
	if selector.Required == nil {
		return true
	}
	return *selector.Required
}

func (selector DataAssetArchiveSelect) effectiveRequired() bool {
	if selector.Required == nil {
		return true
	}
	return *selector.Required
}

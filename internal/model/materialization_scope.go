package model

import (
	"errors"
	"fmt"
	"strings"

	fp "goetl/internal/fingerprint"
)

const (
	DataMaterializationScopeShared = "shared"
	DataMaterializationScopeWorker = "worker"
)

var ErrMaterializationScopeNotImplemented = errors.New("materialization scope is not implemented")

type MaterializationDomain struct {
	Scope string `json:"scope"`
	ID    string `json:"id"`
}

func ValidateMaterializationScope(scope string) error {
	switch scope {
	case "", DataMaterializationScopeShared, DataMaterializationScopeWorker:
		return nil
	default:
		return fmt.Errorf("unsupported materialization scope %q", scope)
	}
}

func SharedMaterializationDomain(targetEnvironmentID string) (MaterializationDomain, error) {
	if strings.TrimSpace(targetEnvironmentID) == "" {
		return MaterializationDomain{}, fmt.Errorf("shared materialization domain id is required")
	}
	if strings.TrimSpace(targetEnvironmentID) != targetEnvironmentID {
		return MaterializationDomain{}, fmt.Errorf("shared materialization domain id must not contain leading or trailing whitespace")
	}
	return MaterializationDomain{Scope: DataMaterializationScopeShared, ID: targetEnvironmentID}, nil
}

func ResolveMaterializationDomain(materialization DataAssetMaterialization, targetEnvironmentID string) (MaterializationDomain, error) {
	if err := materialization.Validate(); err != nil {
		return MaterializationDomain{}, err
	}
	switch materialization.Scope {
	case DataMaterializationScopeShared:
		return SharedMaterializationDomain(targetEnvironmentID)
	case DataMaterializationScopeWorker:
		return MaterializationDomain{}, fmt.Errorf("%w: %s", ErrMaterializationScopeNotImplemented, DataMaterializationScopeWorker)
	case "":
		return MaterializationDomain{}, fmt.Errorf("materialization scope is required")
	default:
		return MaterializationDomain{}, fmt.Errorf("unsupported materialization scope %q", materialization.Scope)
	}
}

func MaterializationLookupIdentity(assetKey string, materialization DataAssetMaterialization, targetEnvironmentID string) (map[string]any, error) {
	if strings.TrimSpace(assetKey) == "" {
		return nil, fmt.Errorf("asset_key is required")
	}
	if !strings.HasPrefix(assetKey, "sha256:") {
		return nil, fmt.Errorf("asset_key must use sha256: prefix")
	}
	if err := validateOptionalSHA256("asset_key", strings.TrimPrefix(assetKey, "sha256:")); err != nil {
		return nil, err
	}
	domain, err := ResolveMaterializationDomain(materialization, targetEnvironmentID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"asset_key":                 assetKey,
		"materialization_scope":     domain.Scope,
		"materialization_domain_id": domain.ID,
	}, nil
}

func MaterializationLookupKey(assetKey string, materialization DataAssetMaterialization, targetEnvironmentID string) (string, error) {
	identity, err := MaterializationLookupIdentity(assetKey, materialization, targetEnvironmentID)
	if err != nil {
		return "", err
	}
	_, hash, err := fp.CanonicalJSONSHA256(identity)
	if err != nil {
		return "", err
	}
	return "sha256:" + hash, nil
}

package model

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

type MaterializedDataProjection struct {
	AssetKey                string                                    `json:"asset_key,omitempty"`
	MaterializationDomainID string                                    `json:"materialization_domain_id,omitempty"`
	Path                    []string                                  `json:"path"`
	Files                   map[string]MaterializedDataFileProjection `json:"files,omitempty"`
	Metadata                map[string]any                            `json:"metadata,omitempty"`
}

type MaterializedDataFileProjection struct {
	Path      string `json:"path"`
	Member    string `json:"member,omitempty"`
	SizeBytes *int64 `json:"size_bytes,omitempty"`
	SHA256    string `json:"sha256,omitempty"`
}

func MaterializedDataProjections(manifest MaterializedDataAssetManifest) (map[string]MaterializedDataProjection, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}

	projections := make(map[string]MaterializedDataProjection, len(manifest.Assets))
	for _, asset := range manifest.Assets {
		if _, exists := projections[asset.BindingName]; exists {
			return nil, fmt.Errorf("duplicate materialized data binding %q", asset.BindingName)
		}
		projection, err := MaterializedDataProjectionForAsset(manifest, asset)
		if err != nil {
			return nil, fmt.Errorf("project materialized data binding %s: %w", asset.BindingName, err)
		}
		projections[asset.BindingName] = projection
	}
	return projections, nil
}

func MaterializedDataProjectionForAsset(manifest MaterializedDataAssetManifest, asset MaterializedDataAsset) (MaterializedDataProjection, error) {
	if err := asset.Validate(); err != nil {
		return MaterializedDataProjection{}, err
	}

	projection := MaterializedDataProjection{
		AssetKey:                manifest.AssetKey,
		MaterializationDomainID: manifest.TargetEnvironmentID,
		Path:                    []string{},
		Files:                   map[string]MaterializedDataFileProjection{},
		Metadata:                asset.Metadata,
	}

	if len(asset.ArchiveMembers) == 0 {
		projection.Path = append(projection.Path, asset.LocalPath)
		return projection, nil
	}

	for _, member := range asset.ArchiveMembers {
		if strings.TrimSpace(member.LocalPath) == "" {
			return MaterializedDataProjection{}, fmt.Errorf("archive member %q local_path is required", member.Member)
		}
		projection.Path = append(projection.Path, member.LocalPath)

		role := materializedArchiveMemberProjectionRole(member.Member)
		if role == "" {
			continue
		}
		if _, exists := projection.Files[role]; exists {
			return MaterializedDataProjection{}, fmt.Errorf("duplicate projected file role %q", role)
		}
		projection.Files[role] = MaterializedDataFileProjection{
			Path:      member.LocalPath,
			Member:    member.Member,
			SizeBytes: member.SizeBytes,
			SHA256:    member.SHA256,
		}
	}
	return projection, nil
}

func materializedArchiveMemberProjectionRole(member string) string {
	if validMaterializedProjectionRole(member) {
		return member
	}

	base := path.Base(member)
	ext := path.Ext(base)
	if strings.EqualFold(ext, ".hdr") {
		return "header"
	}

	stem := strings.TrimSuffix(base, ext)
	if validMaterializedProjectionRole(stem) {
		return stem
	}
	return ""
}

func validMaterializedProjectionRole(name string) bool {
	return materializedProjectionRolePattern.MatchString(name)
}

var materializedProjectionRolePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

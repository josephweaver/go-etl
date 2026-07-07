package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"goetl/internal/model"
)

type gdriveRcloneProvider struct {
	executable string
	configPath string
}

func (m assetMaterializer) acquireGDriveRclone(asset model.BoundDataAsset, destination string) (assetEvidence, error) {
	if !m.config.EnableGDriveRcloneProvider {
		return assetEvidence{}, fmt.Errorf("gdrive_rclone provider is disabled")
	}
	if strings.TrimSpace(m.config.RcloneExecutable) == "" {
		return assetEvidence{}, fmt.Errorf("gdrive_rclone provider requires configured rclone_executable")
	}
	if asset.Location.FileID != "" {
		return assetEvidence{}, fmt.Errorf("gdrive_rclone file_id access is not implemented; use drive_path")
	}
	if err := asset.Location.Validate(); err != nil {
		return assetEvidence{}, err
	}

	provider := gdriveRcloneProvider{
		executable: m.config.RcloneExecutable,
		configPath: m.config.RcloneConfigPath,
	}
	return provider.copyTo(context.Background(), asset.Location.Remote, asset.Location.DrivePath, destination, m.config.effectiveMaxAssetBytes())
}

func (p gdriveRcloneProvider) copyTo(ctx context.Context, remote string, drivePath string, destination string, maxBytes int64) (assetEvidence, error) {
	if strings.TrimSpace(destination) == "" {
		return assetEvidence{}, fmt.Errorf("rclone destination is required")
	}
	safePath, err := cleanDataRelativePath(drivePath)
	if err != nil {
		return assetEvidence{}, err
	}
	remotePath := remote + ":" + safePath

	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return assetEvidence{}, fmt.Errorf("create parent directory for %s: %w", destination, err)
	}
	tmp := destination + ".tmp-" + randomHex(8)
	args := p.copyToArgs(remotePath, tmp)
	command := exec.CommandContext(ctx, p.executable, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		_ = os.Remove(tmp)
		return assetEvidence{}, fmt.Errorf("run rclone copyto: %w: %s", err, p.redactOutput(output))
	}

	evidence, err := hashFileWithLimit(tmp, maxBytes)
	if err != nil {
		_ = os.Remove(tmp)
		return assetEvidence{}, err
	}
	if err := os.Rename(tmp, destination); err != nil {
		_ = os.Remove(tmp)
		return assetEvidence{}, fmt.Errorf("move rclone data asset from %s to %s: %w", tmp, destination, err)
	}
	return evidence, nil
}

func (p gdriveRcloneProvider) copyToArgs(remotePath string, destination string) []string {
	args := []string{}
	if strings.TrimSpace(p.configPath) != "" {
		args = append(args, "--config", p.configPath)
	}
	return append(args, "copyto", remotePath, destination)
}

func (p gdriveRcloneProvider) redactOutput(output []byte) string {
	text := strings.TrimSpace(string(output))
	if text == "" {
		return ""
	}
	replacements := []string{p.configPath}
	if p.configPath != "" {
		replacements = append(replacements, filepath.Clean(p.configPath))
	}
	for _, value := range replacements {
		if value != "" {
			text = strings.ReplaceAll(text, value, "[redacted-rclone-config]")
		}
	}
	for _, pattern := range []*regexp.Regexp{
		regexp.MustCompile(`(?i)(access_token|refresh_token|token)(=|:)[^\s]+`),
		regexp.MustCompile(`(?i)(bearer)\s+[^\s]+`),
	} {
		text = pattern.ReplaceAllString(text, "$1$2[redacted]")
	}
	const maxOutput = 4096
	if len(text) > maxOutput {
		text = text[:maxOutput] + "...[truncated]"
	}
	return text
}

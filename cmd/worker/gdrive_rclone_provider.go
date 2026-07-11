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
	return provider.copyTo(context.Background(), asset.Location.Remote, asset.Location.DrivePath, destination, m.config.effectiveMaxAssetBytes(), asset.TransferPolicy)
}

func (p gdriveRcloneProvider) copyTo(ctx context.Context, remote string, drivePath string, destination string, maxBytes int64, transferPolicy model.DataAssetTransferPolicy) (assetEvidence, error) {
	if strings.TrimSpace(destination) == "" {
		return assetEvidence{}, fmt.Errorf("rclone destination is required")
	}
	if err := transferPolicy.Validate(); err != nil {
		return assetEvidence{}, err
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
	args := p.copyToArgs(remotePath, tmp, transferPolicy)
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

func (p gdriveRcloneProvider) uploadFile(ctx context.Context, sourcePath string, remote string, drivePath string, transferPolicy model.DataAssetTransferPolicy) error {
	if strings.TrimSpace(sourcePath) == "" {
		return fmt.Errorf("rclone source is required")
	}
	if err := transferPolicy.Validate(); err != nil {
		return err
	}
	safePath, err := cleanDataRelativePath(drivePath)
	if err != nil {
		return err
	}
	remotePath := remote + ":" + safePath
	args := p.copyToArgs(sourcePath, remotePath, transferPolicy)
	command := exec.CommandContext(ctx, p.executable, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("run rclone copyto: %w: %s", err, p.redactOutput(output))
	}
	return nil
}

func (p gdriveRcloneProvider) exists(ctx context.Context, remote string, drivePath string) (bool, error) {
	safePath, err := cleanDataRelativePath(drivePath)
	if err != nil {
		return false, err
	}
	remotePath := remote + ":" + safePath
	args := p.lsfArgs(remotePath)
	command := exec.CommandContext(ctx, p.executable, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return false, nil
	}
	_ = output
	return true, nil
}

func (p gdriveRcloneProvider) copyToArgs(source string, destination string, transferPolicy model.DataAssetTransferPolicy) []string {
	args := []string{}
	if strings.TrimSpace(p.configPath) != "" {
		args = append(args, "--config", p.configPath)
	}
	args = append(args, "copyto", source, destination)
	if bwlimit := rcloneBwlimit(transferPolicy); bwlimit != "" {
		args = append(args, "--bwlimit", bwlimit)
	}
	return args
}

func (p gdriveRcloneProvider) lsfArgs(remotePath string) []string {
	args := []string{}
	if strings.TrimSpace(p.configPath) != "" {
		args = append(args, "--config", p.configPath)
	}
	return append(args, "lsf", remotePath)
}

func rcloneBwlimit(transferPolicy model.DataAssetTransferPolicy) string {
	if transferPolicy.ProviderArgs != nil {
		if value := strings.TrimSpace(transferPolicy.ProviderArgs["rclone_bwlimit"]); value != "" {
			return value
		}
	}
	if transferPolicy.RequestedBandwidthMiBPerSecond > 0 {
		return fmt.Sprintf("%dM", transferPolicy.RequestedBandwidthMiBPerSecond)
	}
	if transferPolicy.MaxBytesPerSecond > 0 {
		mib := transferPolicy.MaxBytesPerSecond / (1024 * 1024)
		if mib > 0 {
			return fmt.Sprintf("%dM", mib)
		}
	}
	return ""
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

package client

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"goetl/internal/controllerhttp"
)

const (
	ControllerTokenFileEnv  = "GOET_CONTROLLER_TOKEN_FILE"
	ControllerTokenEnv      = "GOET_CONTROLLER_TOKEN"
	MaxControllerTokenBytes = 64 * 1024
)

type ControllerCredentialConfig struct {
	TokenFile string
	LookupEnv func(string) (string, bool)
}

func LoadControllerTokenProvider(config ControllerCredentialConfig) (controllerhttp.TokenProvider, error) {
	lookupEnv := config.LookupEnv
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}

	if config.TokenFile != "" {
		return tokenProviderFromFile(config.TokenFile)
	}
	if envFile, ok := lookupEnv(ControllerTokenFileEnv); ok && envFile != "" {
		return tokenProviderFromFile(envFile)
	}
	if envToken, ok := lookupEnv(ControllerTokenEnv); ok {
		return tokenProviderFromValue("environment controller token", envToken)
	}
	return nil, nil
}

func tokenProviderFromFile(path string) (controllerhttp.TokenProvider, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("controller token file %q stat failed: %w", path, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("controller token file %q must not be a directory", path)
	}
	if info.Size() > MaxControllerTokenBytes {
		return nil, fmt.Errorf("controller token file %q exceeds %d bytes", path, MaxControllerTokenBytes)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("controller token file %q permissions must not grant group or other access", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("controller token file %q read failed: %w", path, err)
	}
	return tokenProviderFromValue("controller token file", string(data))
}

func tokenProviderFromValue(source string, value string) (controllerhttp.TokenProvider, error) {
	if len(value) > MaxControllerTokenBytes {
		return nil, fmt.Errorf("%s exceeds %d bytes", source, MaxControllerTokenBytes)
	}
	token, err := controllerhttp.NewSensitiveToken(trimOneTrailingLineEnding(value))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	return controllerhttp.NewStaticTokenProvider(token), nil
}

func trimOneTrailingLineEnding(value string) string {
	if strings.HasSuffix(value, "\r\n") {
		return strings.TrimSuffix(value, "\r\n")
	}
	if strings.HasSuffix(value, "\n") {
		return strings.TrimSuffix(value, "\n")
	}
	if strings.HasSuffix(value, "\r") {
		return strings.TrimSuffix(value, "\r")
	}
	return value
}

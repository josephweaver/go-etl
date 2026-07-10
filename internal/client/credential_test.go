package client

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestLoadControllerTokenProviderPrecedence(t *testing.T) {
	dir := t.TempDir()
	flagFile := writeCredentialTestFile(t, dir, "flag-token", "flag-secret\n", 0o600)
	envFile := writeCredentialTestFile(t, dir, "env-token", "env-file-secret\n", 0o600)

	provider, err := LoadControllerTokenProvider(ControllerCredentialConfig{
		TokenFile: flagFile,
		LookupEnv: func(name string) (string, bool) {
			switch name {
			case ControllerTokenFileEnv:
				return envFile, true
			case ControllerTokenEnv:
				return "env-secret", true
			default:
				return "", false
			}
		},
	})
	if err != nil {
		t.Fatalf("LoadControllerTokenProvider() error = %v", err)
	}
	token, err := provider.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if token.Plaintext() != "flag-secret" {
		t.Fatalf("token = %q, want flag-secret", token.Plaintext())
	}
}

func TestLoadControllerTokenProviderUsesEnvironmentTokenFileBeforeEnvironmentToken(t *testing.T) {
	dir := t.TempDir()
	envFile := writeCredentialTestFile(t, dir, "env-token", "env-file-secret\r\n", 0o600)

	provider, err := LoadControllerTokenProvider(ControllerCredentialConfig{
		LookupEnv: func(name string) (string, bool) {
			switch name {
			case ControllerTokenFileEnv:
				return envFile, true
			case ControllerTokenEnv:
				return "env-secret", true
			default:
				return "", false
			}
		},
	})
	if err != nil {
		t.Fatalf("LoadControllerTokenProvider() error = %v", err)
	}
	token, err := provider.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if token.Plaintext() != "env-file-secret" {
		t.Fatalf("token = %q, want env-file-secret", token.Plaintext())
	}
}

func TestLoadControllerTokenProviderUsesEnvironmentToken(t *testing.T) {
	provider, err := LoadControllerTokenProvider(ControllerCredentialConfig{
		LookupEnv: func(name string) (string, bool) {
			if name == ControllerTokenEnv {
				return "env-secret\n", true
			}
			return "", false
		},
	})
	if err != nil {
		t.Fatalf("LoadControllerTokenProvider() error = %v", err)
	}
	token, err := provider.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if token.Plaintext() != "env-secret" {
		t.Fatalf("token = %q, want env-secret", token.Plaintext())
	}
}

func TestLoadControllerTokenProviderReturnsNilWithoutCredential(t *testing.T) {
	provider, err := LoadControllerTokenProvider(ControllerCredentialConfig{
		LookupEnv: func(string) (string, bool) { return "", false },
	})
	if err != nil {
		t.Fatalf("LoadControllerTokenProvider() error = %v", err)
	}
	if provider != nil {
		t.Fatalf("provider = %T, want nil", provider)
	}
}

func TestLoadControllerTokenProviderRejectsEmptyAndOversizedTokens(t *testing.T) {
	_, err := LoadControllerTokenProvider(ControllerCredentialConfig{
		LookupEnv: func(name string) (string, bool) {
			if name == ControllerTokenEnv {
				return "\n", true
			}
			return "", false
		},
	})
	if err == nil {
		t.Fatal("expected empty token error")
	}

	_, err = LoadControllerTokenProvider(ControllerCredentialConfig{
		LookupEnv: func(name string) (string, bool) {
			if name == ControllerTokenEnv {
				return strings.Repeat("x", MaxControllerTokenBytes+1), true
			}
			return "", false
		},
	})
	if err == nil {
		t.Fatal("expected oversized token error")
	}
}

func TestLoadControllerTokenProviderRejectsOversizedFile(t *testing.T) {
	dir := t.TempDir()
	path := writeCredentialTestFile(t, dir, "large-token", strings.Repeat("x", MaxControllerTokenBytes+1), 0o600)

	_, err := LoadControllerTokenProvider(ControllerCredentialConfig{TokenFile: path})
	if err == nil {
		t.Fatal("expected oversized file error")
	}
}

func TestLoadControllerTokenProviderRejectsBroadUnixFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows relies on ACLs rather than POSIX mode bits")
	}
	dir := t.TempDir()
	path := writeCredentialTestFile(t, dir, "broad-token", "secret", 0o644)

	_, err := LoadControllerTokenProvider(ControllerCredentialConfig{TokenFile: path})
	if err == nil {
		t.Fatal("expected broad permission error")
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("error leaked token: %v", err)
	}
}

func writeCredentialTestFile(t *testing.T, dir string, name string, content string, mode os.FileMode) string {
	t.Helper()
	path := fmt.Sprintf("%s/%s", dir, name)
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	return path
}

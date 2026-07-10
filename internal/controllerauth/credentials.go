package controllerauth

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

type CredentialSources struct {
	LookupEnv func(string) (string, bool)
	ReadFile  func(string) ([]byte, error)
	StatFile  func(string) (fs.FileInfo, error)
}

type Principal struct {
	ID   string
	Role Role
}

type Store struct {
	credentials []loadedCredential
}

type loadedCredential struct {
	principal Principal
	digest    [sha256.Size]byte
}

func DefaultCredentialSources() CredentialSources {
	return CredentialSources{
		LookupEnv: os.LookupEnv,
		ReadFile:  os.ReadFile,
		StatFile:  os.Stat,
	}
}

func LoadCredentials(config Config, sources CredentialSources) (Store, error) {
	if config.Mode == ModeDisabled {
		return Store{}, nil
	}
	if config.Mode != ModeBearer {
		return Store{}, fmt.Errorf("unsupported authentication mode %q", config.Mode)
	}
	if len(config.Credentials) == 0 {
		return Store{}, fmt.Errorf("bearer authentication requires at least one credential")
	}

	if sources.LookupEnv == nil {
		sources.LookupEnv = os.LookupEnv
	}
	if sources.ReadFile == nil {
		sources.ReadFile = os.ReadFile
	}
	if sources.StatFile == nil {
		sources.StatFile = os.Stat
	}

	store := Store{credentials: make([]loadedCredential, 0, len(config.Credentials))}
	seenDigest := make(map[[sha256.Size]byte]string, len(config.Credentials))
	for _, credential := range config.Credentials {
		token, err := readCredentialToken(credential, sources)
		if err != nil {
			return Store{}, err
		}
		digest := sha256.Sum256([]byte(token))
		if priorID, ok := seenDigest[digest]; ok {
			return Store{}, fmt.Errorf("credential %q duplicates token material from credential %q", credential.ID, priorID)
		}
		seenDigest[digest] = credential.ID
		store.credentials = append(store.credentials, loadedCredential{
			principal: Principal{
				ID:   credential.ID,
				Role: credential.Role,
			},
			digest: digest,
		})
	}

	return store, nil
}

func (s Store) MatchBearer(token string) (Principal, bool) {
	digest := sha256.Sum256([]byte(token))
	for _, credential := range s.credentials {
		if subtle.ConstantTimeCompare(digest[:], credential.digest[:]) == 1 {
			return credential.principal, true
		}
	}
	return Principal{}, false
}

func readCredentialToken(credential Credential, sources CredentialSources) (string, error) {
	switch {
	case credential.TokenEnv != "":
		value, ok := sources.LookupEnv(credential.TokenEnv)
		if !ok {
			return "", fmt.Errorf("credential %q token_env %q is not set", credential.ID, credential.TokenEnv)
		}
		return normalizedToken(credential.ID, fmt.Sprintf("token_env %q", credential.TokenEnv), value)
	case credential.TokenFile != "":
		info, err := sources.StatFile(credential.TokenFile)
		if err != nil {
			return "", fmt.Errorf("credential %q token_file %q stat failed: %w", credential.ID, credential.TokenFile, err)
		}
		if err := validateRestrictiveFileMode(info.Mode()); err != nil {
			return "", fmt.Errorf("credential %q token_file %q: %w", credential.ID, credential.TokenFile, err)
		}
		data, err := sources.ReadFile(credential.TokenFile)
		if err != nil {
			return "", fmt.Errorf("credential %q token_file %q read failed: %w", credential.ID, credential.TokenFile, err)
		}
		return normalizedToken(credential.ID, fmt.Sprintf("token_file %q", credential.TokenFile), string(data))
	default:
		return "", fmt.Errorf("credential %q has no token source", credential.ID)
	}
}

func validateRestrictiveFileMode(mode fs.FileMode) error {
	if mode.IsDir() {
		return fmt.Errorf("token file must not be a directory")
	}
	perm := mode.Perm()
	if perm == 0 {
		return nil
	}
	if perm&0o077 != 0 {
		return fmt.Errorf("token file permissions must not grant group or other access")
	}
	return nil
}

func normalizedToken(credentialID string, source string, value string) (string, error) {
	value = trimOneTrailingLineEnding(value)
	if value == "" {
		return "", fmt.Errorf("credential %q %s is empty", credentialID, source)
	}
	return value, nil
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

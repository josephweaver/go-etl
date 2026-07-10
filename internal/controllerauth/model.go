package controllerauth

import (
	"fmt"

	"goetl/internal/variable"
)

type Mode string

const (
	ModeDisabled Mode = "disabled"
	ModeBearer   Mode = "bearer"
)

type Role string

const (
	RoleClient Role = "client"
	RoleWorker Role = "worker"
	RoleAdmin  Role = "admin"
)

type Config struct {
	Mode        Mode
	Credentials []Credential
}

type Credential struct {
	ID        string
	Role      Role
	TokenEnv  string
	TokenFile string
}

func ConfigFromResolved(value variable.ResolvedValue) (Config, error) {
	if value.Type != variable.TypeObject {
		return Config{}, fmt.Errorf("controller_config.authentication has type %s, want object", value.Type)
	}
	return configFromFields(value.Object)
}

func configFromFields(fields map[string]variable.ResolvedValue) (Config, error) {
	if err := rejectUnknownFields("controller_config.authentication", fields, "mode", "credentials"); err != nil {
		return Config{}, err
	}

	modeText, err := requiredStringField(fields, "mode")
	if err != nil {
		return Config{}, err
	}
	mode := Mode(modeText)
	if !mode.Valid() {
		return Config{}, fmt.Errorf("unknown authentication mode %q", modeText)
	}

	credentials, found, err := credentialListField(fields, "credentials")
	if err != nil {
		return Config{}, err
	}
	if mode == ModeBearer && (!found || len(credentials) == 0) {
		return Config{}, fmt.Errorf("bearer authentication requires at least one credential")
	}

	return Config{
		Mode:        mode,
		Credentials: credentials,
	}, nil
}

func (m Mode) Valid() bool {
	switch m {
	case ModeDisabled, ModeBearer:
		return true
	default:
		return false
	}
}

func (r Role) Valid() bool {
	switch r {
	case RoleClient, RoleWorker, RoleAdmin:
		return true
	default:
		return false
	}
}

func credentialListField(fields map[string]variable.ResolvedValue, name string) ([]Credential, bool, error) {
	value, ok := fields[name]
	if !ok {
		return nil, false, nil
	}
	if value.Type != variable.TypeList {
		return nil, false, fmt.Errorf("%s has type %s, want list", name, value.Type)
	}

	credentials := make([]Credential, 0, len(value.List))
	seen := make(map[string]struct{}, len(value.List))
	for index, item := range value.List {
		if item.Type != variable.TypeObject {
			return nil, false, fmt.Errorf("%s[%d] has type %s, want object", name, index, item.Type)
		}
		credential, err := credentialFromFields(index, item.Object)
		if err != nil {
			return nil, false, err
		}
		if _, ok := seen[credential.ID]; ok {
			return nil, false, fmt.Errorf("duplicate credential id %q", credential.ID)
		}
		seen[credential.ID] = struct{}{}
		credentials = append(credentials, credential)
	}
	return credentials, true, nil
}

func credentialFromFields(index int, fields map[string]variable.ResolvedValue) (Credential, error) {
	prefix := fmt.Sprintf("credentials[%d]", index)
	if err := rejectUnknownFields(prefix, fields, "id", "role", "token_env", "token_file"); err != nil {
		return Credential{}, err
	}

	id, err := requiredStringField(fields, "id")
	if err != nil {
		return Credential{}, fmt.Errorf("%s: %w", prefix, err)
	}
	roleText, err := requiredStringField(fields, "role")
	if err != nil {
		return Credential{}, fmt.Errorf("%s: %w", prefix, err)
	}
	role := Role(roleText)
	if !role.Valid() {
		return Credential{}, fmt.Errorf("%s: unknown role %q", prefix, roleText)
	}

	tokenEnv, hasTokenEnv, err := optionalStringField(fields, "token_env")
	if err != nil {
		return Credential{}, fmt.Errorf("%s: %w", prefix, err)
	}
	tokenFile, hasTokenFile, err := optionalPathOrStringField(fields, "token_file")
	if err != nil {
		return Credential{}, fmt.Errorf("%s: %w", prefix, err)
	}
	if hasTokenEnv == hasTokenFile {
		return Credential{}, fmt.Errorf("%s: exactly one token source is required", prefix)
	}

	return Credential{
		ID:        id,
		Role:      role,
		TokenEnv:  tokenEnv,
		TokenFile: tokenFile,
	}, nil
}

func rejectUnknownFields(context string, fields map[string]variable.ResolvedValue, allowed ...string) error {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = struct{}{}
	}
	for name := range fields {
		if _, ok := allowedSet[name]; !ok {
			return fmt.Errorf("%s: unknown field %q", context, name)
		}
	}
	return nil
}

func requiredStringField(fields map[string]variable.ResolvedValue, name string) (string, error) {
	text, found, err := optionalStringField(fields, name)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("%s is required", name)
	}
	return text, nil
}

func optionalStringField(fields map[string]variable.ResolvedValue, name string) (string, bool, error) {
	value, ok := fields[name]
	if !ok {
		return "", false, nil
	}
	if value.Type != variable.TypeString {
		return "", false, fmt.Errorf("%s has type %s, want string", name, value.Type)
	}
	text, ok := value.Value.(string)
	if !ok || text == "" {
		return "", false, fmt.Errorf("%s is required", name)
	}
	return text, true, nil
}

func optionalPathOrStringField(fields map[string]variable.ResolvedValue, name string) (string, bool, error) {
	value, ok := fields[name]
	if !ok {
		return "", false, nil
	}
	if value.Type != variable.TypeString && value.Type != variable.TypePath {
		return "", false, fmt.Errorf("%s has type %s, want string or path", name, value.Type)
	}
	text, ok := value.Value.(string)
	if !ok || text == "" {
		return "", false, fmt.Errorf("%s is required", name)
	}
	return text, true, nil
}

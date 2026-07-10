package controllerauth

import (
	"strings"
	"testing"

	"goetl/internal/variable"
)

func TestConfigFromResolvedBearer(t *testing.T) {
	cfg, err := ConfigFromResolved(object(map[string]variable.ResolvedValue{
		"mode": stringValue("bearer"),
		"credentials": list(
			credentialObject("primary-client", "client", "token_env", stringValue("GOET_CONTROLLER_CLIENT_TOKEN")),
			credentialObject("worker-pool", "worker", "token_file", pathValue("/etc/goet/secrets/controller-worker-token")),
			credentialObject("operator", "admin", "token_file", pathValue("/etc/goet/secrets/controller-admin-token")),
		),
	}))
	if err != nil {
		t.Fatalf("ConfigFromResolved() error = %v", err)
	}

	if cfg.Mode != ModeBearer {
		t.Fatalf("mode = %q, want %q", cfg.Mode, ModeBearer)
	}
	if len(cfg.Credentials) != 3 {
		t.Fatalf("credential count = %d, want 3", len(cfg.Credentials))
	}
	if cfg.Credentials[0].ID != "primary-client" || cfg.Credentials[0].Role != RoleClient || cfg.Credentials[0].TokenEnv != "GOET_CONTROLLER_CLIENT_TOKEN" {
		t.Fatalf("first credential = %+v, want client token env", cfg.Credentials[0])
	}
	if cfg.Credentials[1].TokenFile != "/etc/goet/secrets/controller-worker-token" {
		t.Fatalf("worker token file = %q", cfg.Credentials[1].TokenFile)
	}
}

func TestConfigFromResolvedDisabledDefault(t *testing.T) {
	cfg, err := ConfigFromResolved(object(map[string]variable.ResolvedValue{
		"mode":        stringValue("disabled"),
		"credentials": list(),
	}))
	if err != nil {
		t.Fatalf("ConfigFromResolved() error = %v", err)
	}

	if cfg.Mode != ModeDisabled {
		t.Fatalf("mode = %q, want %q", cfg.Mode, ModeDisabled)
	}
	if len(cfg.Credentials) != 0 {
		t.Fatalf("credential count = %d, want 0", len(cfg.Credentials))
	}
}

func TestConfigFromResolvedRejectsInvalidShape(t *testing.T) {
	tests := []struct {
		name  string
		value variable.ResolvedValue
		want  string
	}{
		{
			name:  "non object root",
			value: stringValue("disabled"),
			want:  "controller_config.authentication has type string, want object",
		},
		{
			name: "unknown authentication field",
			value: object(map[string]variable.ResolvedValue{
				"mode":        stringValue("disabled"),
				"credentials": list(),
				"token":       stringValue("raw-secret"),
			}),
			want: "unknown field \"token\"",
		},
		{
			name: "missing mode",
			value: object(map[string]variable.ResolvedValue{
				"credentials": list(),
			}),
			want: "mode is required",
		},
		{
			name: "unknown mode",
			value: object(map[string]variable.ResolvedValue{
				"mode":        stringValue("oauth"),
				"credentials": list(),
			}),
			want: "unknown authentication mode \"oauth\"",
		},
		{
			name: "bearer without credentials",
			value: object(map[string]variable.ResolvedValue{
				"mode":        stringValue("bearer"),
				"credentials": list(),
			}),
			want: "bearer authentication requires at least one credential",
		},
		{
			name: "credentials wrong type",
			value: object(map[string]variable.ResolvedValue{
				"mode":        stringValue("bearer"),
				"credentials": stringValue("not-a-list"),
			}),
			want: "credentials has type string, want list",
		},
		{
			name: "credential item wrong type",
			value: object(map[string]variable.ResolvedValue{
				"mode":        stringValue("bearer"),
				"credentials": list(stringValue("not-an-object")),
			}),
			want: "credentials[0] has type string, want object",
		},
		{
			name: "unknown credential field",
			value: object(map[string]variable.ResolvedValue{
				"mode": stringValue("bearer"),
				"credentials": list(object(map[string]variable.ResolvedValue{
					"id":        stringValue("client"),
					"role":      stringValue("client"),
					"token_env": stringValue("GOET_CONTROLLER_CLIENT_TOKEN"),
					"token":     stringValue("raw-secret"),
				})),
			}),
			want: "credentials[0]: unknown field \"token\"",
		},
		{
			name: "unknown role",
			value: object(map[string]variable.ResolvedValue{
				"mode":        stringValue("bearer"),
				"credentials": list(credentialObject("client", "superuser", "token_env", stringValue("GOET_CONTROLLER_CLIENT_TOKEN"))),
			}),
			want: "credentials[0]: unknown role \"superuser\"",
		},
		{
			name: "duplicate credential id",
			value: object(map[string]variable.ResolvedValue{
				"mode": stringValue("bearer"),
				"credentials": list(
					credentialObject("client", "client", "token_env", stringValue("GOET_CONTROLLER_CLIENT_TOKEN")),
					credentialObject("client", "admin", "token_env", stringValue("GOET_CONTROLLER_ADMIN_TOKEN")),
				),
			}),
			want: "duplicate credential id \"client\"",
		},
		{
			name: "credential without token source",
			value: object(map[string]variable.ResolvedValue{
				"mode": stringValue("bearer"),
				"credentials": list(object(map[string]variable.ResolvedValue{
					"id":   stringValue("client"),
					"role": stringValue("client"),
				})),
			}),
			want: "credentials[0]: exactly one token source is required",
		},
		{
			name: "credential with both token sources",
			value: object(map[string]variable.ResolvedValue{
				"mode": stringValue("bearer"),
				"credentials": list(object(map[string]variable.ResolvedValue{
					"id":         stringValue("client"),
					"role":       stringValue("client"),
					"token_env":  stringValue("GOET_CONTROLLER_CLIENT_TOKEN"),
					"token_file": pathValue("/etc/goet/secrets/client-token"),
				})),
			}),
			want: "credentials[0]: exactly one token source is required",
		},
		{
			name: "empty token source name",
			value: object(map[string]variable.ResolvedValue{
				"mode":        stringValue("bearer"),
				"credentials": list(credentialObject("client", "client", "token_env", stringValue(""))),
			}),
			want: "credentials[0]: token_env is required",
		},
		{
			name: "role has wrong type",
			value: object(map[string]variable.ResolvedValue{
				"mode": stringValue("bearer"),
				"credentials": list(object(map[string]variable.ResolvedValue{
					"id":        stringValue("client"),
					"role":      pathValue("/not-a-role"),
					"token_env": stringValue("GOET_CONTROLLER_CLIENT_TOKEN"),
				})),
			}),
			want: "credentials[0]: role has type path, want string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ConfigFromResolved(tt.value)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ConfigFromResolved() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func credentialObject(id string, role string, sourceName string, sourceValue variable.ResolvedValue) variable.ResolvedValue {
	fields := map[string]variable.ResolvedValue{
		"id":       stringValue(id),
		"role":     stringValue(role),
		sourceName: sourceValue,
	}
	return object(fields)
}

func object(fields map[string]variable.ResolvedValue) variable.ResolvedValue {
	return variable.ResolvedObject(fields)
}

func list(values ...variable.ResolvedValue) variable.ResolvedValue {
	return variable.ResolvedList(values)
}

func stringValue(value string) variable.ResolvedValue {
	return variable.ResolvedValue{Type: variable.TypeString, Value: value}
}

func pathValue(value string) variable.ResolvedValue {
	return variable.ResolvedValue{Type: variable.TypePath, Value: value}
}

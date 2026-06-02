package variable

import "testing"

func TestNameString(t *testing.T) {
	name := Name{
		Namespace: NamespaceWorkerEnvironment,
		Key:       "GDAL_DATA",
	}

	if got := name.String(); got != "worker_env.GDAL_DATA" {
		t.Fatalf("unexpected name: %q", got)
	}
}

func TestNameValidate(t *testing.T) {
	tests := []struct {
		name    string
		value   Name
		wantErr bool
	}{
		{
			name: "valid name",
			value: Name{
				Namespace: NamespaceProject,
				Key:       "data_dir",
			},
		},
		{
			name: "missing namespace",
			value: Name{
				Key: "data_dir",
			},
			wantErr: true,
		},
		{
			name: "unsupported namespace",
			value: Name{
				Namespace: "unknown",
				Key:       "data_dir",
			},
			wantErr: true,
		},
		{
			name: "missing key",
			value: Name{
				Namespace: NamespaceProject,
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.value.Validate()

			if test.wantErr && err == nil {
				t.Fatal("expected an error")
			}

			if !test.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

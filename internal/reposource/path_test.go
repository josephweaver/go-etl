package reposource

import "testing"

func TestValidateRepositoryRelativePathAcceptsCleanPaths(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{name: "project", path: "project.json", want: "project.json"},
		{name: "workflow", path: "workflows/train.json", want: "workflows/train.json"},
		{name: "script", path: "scripts/lib/helpers.py", want: "scripts/lib/helpers.py"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidateRepositoryRelativePath(tc.path)
			if err != nil {
				t.Fatalf("ValidateRepositoryRelativePath() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("ValidateRepositoryRelativePath() = %q, want %q", got, tc.want)
			}
			cacheGot, err := ValidateCacheRelativePath(tc.path)
			if err != nil {
				t.Fatalf("ValidateCacheRelativePath() error = %v", err)
			}
			if cacheGot != tc.want {
				t.Fatalf("ValidateCacheRelativePath() = %q, want %q", cacheGot, tc.want)
			}
		})
	}
}

func TestValidateRepositoryRelativePathRejectsUnsafePaths(t *testing.T) {
	cases := []struct {
		name string
		path string
	}{
		{name: "empty", path: ""},
		{name: "dot", path: "."},
		{name: "parent", path: "../project.json"},
		{name: "nested-parent", path: "workflows/../project.json"},
		{name: "absolute", path: "/project.json"},
		{name: "drive-qualified", path: "C:/project.json"},
		{name: "backslash", path: `workflows\train.json`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ValidateRepositoryRelativePath(tc.path); err == nil {
				t.Fatal("expected an error")
			}
			if _, err := ValidateCacheRelativePath(tc.path); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

package model

import "testing"

func TestValidateArtifactRelativePathAcceptsCleanPaths(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{name: "file", path: "output.json", want: "output.json"},
		{name: "nested file", path: "tables/year-2024/output.parquet", want: "tables/year-2024/output.parquet"},
		{name: "directory-like path", path: "tiles/tile-001", want: "tiles/tile-001"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidateArtifactRelativePath(tc.path)
			if err != nil {
				t.Fatalf("ValidateArtifactRelativePath() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("ValidateArtifactRelativePath() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestValidateArtifactRelativePathRejectsUnsafePaths(t *testing.T) {
	cases := []struct {
		name string
		path string
	}{
		{name: "empty", path: ""},
		{name: "whitespace", path: " "},
		{name: "absolute", path: "/output.json"},
		{name: "double slash absolute", path: "//server/share/output.json"},
		{name: "backslash", path: `tiles\output.json`},
		{name: "drive qualified", path: "C:/tiles/output.json"},
		{name: "dot segment", path: "./output.json"},
		{name: "nested dot segment", path: "tiles/./output.json"},
		{name: "parent segment", path: "../output.json"},
		{name: "nested parent segment", path: "tiles/../output.json"},
		{name: "empty middle segment", path: "tiles//output.json"},
		{name: "trailing slash", path: "tiles/output/"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ValidateArtifactRelativePath(tc.path); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

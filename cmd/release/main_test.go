package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNextVersion(t *testing.T) {
	t.Parallel()
	cases := []struct {
		current, bump, explicit, want string
	}{
		{"", "patch", "", "v0.0.1"},
		{"v0.1.0", "patch", "", "v0.1.1"},
		{"v0.1.0", "minor", "", "v0.2.0"},
		{"v0.1.0", "major", "", "v1.0.0"},
		{"v1.2.3", "patch", "", "v1.2.4"},
		{"v0.1.0", "patch", "v9.8.7", "v9.8.7"},
		{"v0.1.0", "patch", "1.0.0", "v1.0.0"},
	}
	for _, tc := range cases {
		got, err := nextVersion(tc.current, tc.bump, tc.explicit)
		if err != nil {
			t.Fatalf("%+v: %v", tc, err)
		}
		if got != tc.want {
			t.Fatalf("%+v: got %s want %s", tc, got, tc.want)
		}
	}
}

func TestDisplayTagAndModuleRoot(t *testing.T) {
	if displayTag("") != "(none)" || displayTag("v1.0.0") != "v1.0.0" {
		t.Fatal("displayTag")
	}
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("moduleRoot=%s: %v", root, err)
	}
}

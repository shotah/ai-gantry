package mcp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shotah/ai-gantry/internal/mcp"
)

func TestLoadManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.toml")
	content := `
[[server]]
name = "strava"
command = "strava-mcp"

[[server]]
name = "gws"
command = "gws-mcp"
args = ["--tools", "gmail"]
env = ["FOO=bar"]
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := mcp.LoadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Servers) != 2 {
		t.Fatalf("%d", len(m.Servers))
	}
	if m.Servers[1].Args[0] != "--tools" || m.Servers[1].Env[0] != "FOO=bar" {
		t.Fatalf("%+v", m.Servers[1])
	}
}

func TestLoadManifest_DuplicateName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.toml")
	_ = os.WriteFile(path, []byte(`
[[server]]
name = "a"
command = "x"
[[server]]
name = "a"
command = "y"
`), 0o644)
	_, err := mcp.LoadManifest(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadManifest_Missing(t *testing.T) {
	_, err := mcp.LoadManifest(filepath.Join(t.TempDir(), "nope.toml"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadManifest_ToolsFilterFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.toml")
	content := `
[[server]]
name = "garmin"
command = "garmin"
args = ["mcp"]
tools = ["get_sleep", "get_weight"]
exclude = ["raw_*"]
tools_prefix = "garm"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := mcp.LoadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	s := m.Servers[0]
	if len(s.Tools) != 2 || s.Tools[0] != "get_sleep" {
		t.Fatalf("tools=%v", s.Tools)
	}
	if len(s.Exclude) != 1 || s.Exclude[0] != "raw_*" {
		t.Fatalf("exclude=%v", s.Exclude)
	}
	if s.ToolsPrefix != "garm" {
		t.Fatalf("prefix=%q", s.ToolsPrefix)
	}
}

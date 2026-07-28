package mcp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/mcp"
)

func TestResolveDownload_Placeholders(t *testing.T) {
	s := mcp.ServerSpec{
		Name:        "math",
		Command:     "mcp-go-math",
		DownloadURL: "https://example.com/math/mcp-go-math_0.0.1_{os}_{arch}.tar.gz",
	}
	u, file, err := s.ResolveDownload("linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://example.com/math/mcp-go-math_0.0.1_linux_amd64.tar.gz"
	if u != want {
		t.Fatalf("url=%q want %q", u, want)
	}
	if file != "mcp-go-math_0.0.1_linux_amd64.tar.gz" {
		t.Fatalf("file=%q", file)
	}
}

func TestResolveDownload_TagAndVersion(t *testing.T) {
	s := mcp.ServerSpec{
		Name:        "math",
		Command:     "mcp-go-math",
		DownloadURL: "https://github.com/shotah/mcp-go-math/releases/download/{tag}/mcp-go-math_{version}_{os}_{arch}.tar.gz",
		DownloadTag: "v0.0.2",
	}
	u, file, tag, err := s.ResolveDownloadLookup("linux", "amd64", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "https://github.com/shotah/mcp-go-math/releases/download/v0.0.2/mcp-go-math_0.0.2_linux_amd64.tar.gz"
	if u != want {
		t.Fatalf("url=%q want %q", u, want)
	}
	if file != "mcp-go-math_0.0.2_linux_amd64.tar.gz" {
		t.Fatalf("file=%q", file)
	}
	if tag != "v0.0.2" {
		t.Fatalf("tag=%q", tag)
	}
}

func TestResolveDownload_Latest(t *testing.T) {
	s := mcp.ServerSpec{
		Name:        "math",
		Command:     "mcp-go-math",
		DownloadURL: "https://github.com/shotah/mcp-go-math/releases/download/{tag}/mcp-go-math_{version}_{os}_{arch}.tar.gz",
		DownloadTag: "latest",
	}
	lookup := func(owner, repo string) (string, error) {
		if owner != "shotah" || repo != "mcp-go-math" {
			t.Fatalf("lookup %s/%s", owner, repo)
		}
		return "v9.9.9", nil
	}
	u, _, tag, err := s.ResolveDownloadLookup("linux", "amd64", lookup)
	if err != nil {
		t.Fatal(err)
	}
	if tag != "v9.9.9" {
		t.Fatalf("tag=%q", tag)
	}
	if !strings.Contains(u, "/v9.9.9/") || !strings.Contains(u, "mcp-go-math_9.9.9_linux_amd64") {
		t.Fatalf("url=%q", u)
	}
}

func TestResolveDownload_LatestNeedsGitHub(t *testing.T) {
	s := mcp.ServerSpec{
		Name:        "x",
		Command:     "x",
		DownloadURL: "https://cdn.example/{tag}/x_{version}.tar.gz",
		DownloadTag: "latest",
	}
	if _, _, _, err := s.ResolveDownloadLookup("linux", "amd64", nil); err == nil {
		t.Fatal("want error for non-github latest")
	}
}

func TestResolveDownload_RejectsRelative(t *testing.T) {
	s := mcp.ServerSpec{Name: "x", Command: "x", DownloadURL: "not-a-url"}
	if _, _, err := s.ResolveDownload("linux", "amd64"); err == nil {
		t.Fatal("want error")
	}
}

func TestDownloadPlans_FromManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.toml")
	content := `
[[server]]
name = "strava"
command = "strava-mcp"

[[server]]
name = "math"
command = "mcp-go-math"
download_url = "https://gitlab.example/pkg/math_{os}_{arch}.tar.gz"

[[server]]
name = "cast"
command = "mcp-beam"
download_url = "https://cdn.example/mcp-beam.tar.gz"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := mcp.LoadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	plans, err := mcp.DownloadPlansLookup(m, "linux", "arm64", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 2 {
		t.Fatalf("plans=%d want 2", len(plans))
	}
	if plans[0].URL != "https://gitlab.example/pkg/math_linux_arm64.tar.gz" {
		t.Fatalf("math url=%q", plans[0].URL)
	}
	if plans[1].Command != "mcp-beam" {
		t.Fatalf("%+v", plans[1])
	}
	if len(mcp.Commands(m)) != 3 {
		t.Fatalf("commands=%v", mcp.Commands(m))
	}
}

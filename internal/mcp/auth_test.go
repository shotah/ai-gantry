package mcp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shotah/ai-gantry/internal/mcp"
)

func TestServerSpec_AuthCmd(t *testing.T) {
	s := mcp.ServerSpec{Name: "strava", Command: "strava-mcp", AuthArgs: []string{"auth"}}
	cmd, args, ok := s.AuthCmd()
	if !ok || cmd != "strava-mcp" || len(args) != 1 || args[0] != "auth" {
		t.Fatalf("got cmd=%q args=%v ok=%v", cmd, args, ok)
	}

	s2 := mcp.ServerSpec{Name: "x", Command: "run", AuthCommand: "auth-bin", AuthArgs: []string{"login"}}
	cmd, args, ok = s2.AuthCmd()
	if !ok || cmd != "auth-bin" || args[0] != "login" {
		t.Fatalf("got cmd=%q args=%v ok=%v", cmd, args, ok)
	}

	if (mcp.ServerSpec{Name: "cast", Command: "mcp-beam"}).AuthConfigured() {
		t.Fatal("cast should have no auth")
	}
}

func TestExpandAuthArgs(t *testing.T) {
	t.Setenv("YTMUSIC_HEADERS_PATH", "/data/.config/ytmusic/headers.json")
	got := mcp.ExpandAuthArgs([]string{"auth", "--out", "${YTMUSIC_HEADERS_PATH}"})
	if got[2] != "/data/.config/ytmusic/headers.json" {
		t.Fatalf("%v", got)
	}
}

func TestLoadManifest_AuthFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.toml")
	content := `
[[server]]
name = "garmin"
command = "garmin"
args = ["mcp"]
auth_args = ["login"]

[[server]]
name = "cast"
command = "mcp-beam"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := mcp.LoadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	auth := mcp.AuthServers(m)
	if len(auth) != 1 || auth[0].Name != "garmin" {
		t.Fatalf("%+v", auth)
	}
	s, ok := mcp.FindServer(m, "garmin")
	if !ok || !s.AuthConfigured() {
		t.Fatal("garmin missing")
	}
}

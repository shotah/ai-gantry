package doctor_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/shotah/ai-gantry/internal/doctor"
	"github.com/shotah/ai-gantry/internal/heartbeat"
	"github.com/shotah/ai-gantry/internal/mcp"
	"github.com/shotah/ai-gantry/internal/session"
)

func TestCollect_AllSkippedIsNotOKButAlive(t *testing.T) {
	dir := t.TempDir()
	personaDir := filepath.Join(dir, "persona")
	if err := os.Mkdir(personaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(personaDir, "PERSONA.md"), []byte("# p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	touchHeartbeat(t, dir)

	if err := doctor.WriteSnapshot(dir, []mcp.ServerStatus{
		{Name: "google", State: mcp.ServerSkipped, Reason: mcp.ReasonNoOAuth, Note: "no token", Auth: true},
		{Name: "math", State: mcp.ServerSkipped, Reason: mcp.ReasonNoBinary, Note: "not found", Auth: false},
	}); err != nil {
		t.Fatal(err)
	}

	rep := doctor.Collect(doctor.Paths{
		DataDir:    dir,
		PersonaDir: personaDir,
		Manifest:   filepath.Join(dir, "missing.toml"),
		Channel:    "telegram",
	})
	if !rep.Alive {
		t.Fatal("want alive")
	}
	if rep.OK {
		t.Fatal("all skipped must not be operator-ok")
	}
	if rep.Reason != "mcp_all_skipped" {
		t.Fatalf("reason=%q", rep.Reason)
	}
	if !rep.Persona.PersonaMD || rep.Persona.SelfMD {
		t.Fatalf("persona=%+v", rep.Persona)
	}
	if rep.Channel != "telegram" || rep.MCP.Listed != 2 || rep.MCP.Connected != 0 || rep.MCP.Skipped != 2 {
		t.Fatalf("%+v", rep.MCP)
	}
	if rep.MCP.Servers[0].Reason != "no_oauth" || !rep.MCP.Servers[0].Auth {
		t.Fatalf("%+v", rep.MCP.Servers[0])
	}
}

func TestCollect_EmptyManifestOK(t *testing.T) {
	dir := t.TempDir()
	touchHeartbeat(t, dir)
	if err := doctor.WriteSnapshot(dir, nil); err != nil {
		t.Fatal(err)
	}
	rep := doctor.Collect(doctor.Paths{DataDir: dir, PersonaDir: dir, Channel: "stdio"})
	if !rep.Alive || !rep.OK {
		t.Fatalf("chat-only should be ok: %+v", rep)
	}
	if rep.MCP.Listed != 0 {
		t.Fatalf("%+v", rep.MCP)
	}
}

func TestCollect_PartialSkipStillOK(t *testing.T) {
	dir := t.TempDir()
	touchHeartbeat(t, dir)
	if err := doctor.WriteSnapshot(dir, []mcp.ServerStatus{
		{Name: "math", State: mcp.ServerIdle},
		{Name: "google", State: mcp.ServerSkipped, Reason: mcp.ReasonNoOAuth, Auth: true},
	}); err != nil {
		t.Fatal(err)
	}
	rep := doctor.Collect(doctor.Paths{DataDir: dir, PersonaDir: dir, Channel: "telegram"})
	if !rep.Alive || !rep.OK {
		t.Fatalf("%+v", rep)
	}
	if rep.MCP.Connected != 1 || rep.MCP.Skipped != 1 {
		t.Fatalf("%+v", rep.MCP)
	}
}

func TestCollect_NoHeartbeat(t *testing.T) {
	dir := t.TempDir()
	rep := doctor.Collect(doctor.Paths{DataDir: dir, PersonaDir: dir, Channel: "telegram"})
	if rep.Alive || rep.OK || rep.Reason != "no_heartbeat" {
		t.Fatalf("%+v", rep)
	}
}

func TestCollect_ManifestFallbackUnknown(t *testing.T) {
	dir := t.TempDir()
	touchHeartbeat(t, dir)
	man := filepath.Join(dir, "mcp.toml")
	if err := os.WriteFile(man, []byte(`
[[server]]
name = "math"
command = "mcp-go-math"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	rep := doctor.Collect(doctor.Paths{DataDir: dir, PersonaDir: dir, Manifest: man, Channel: "discord"})
	if !rep.Alive {
		t.Fatal("alive")
	}
	if rep.OK {
		t.Fatal("listed but none connected (unknown) is not operator-ok")
	}
	if len(rep.MCP.Servers) != 1 || rep.MCP.Servers[0].State != "unknown" {
		t.Fatalf("%+v", rep.MCP)
	}
	if rep.Channel != "discord" {
		t.Fatalf("channel=%q", rep.Channel)
	}
}

func TestWriteSnapshot_RoundTripJSON(t *testing.T) {
	dir := t.TempDir()
	if err := doctor.WriteSnapshot(dir, []mcp.ServerStatus{
		{Name: "math", State: mcp.ServerOK, Auth: false},
	}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, doctor.SnapshotFile))
	if err != nil {
		t.Fatal(err)
	}
	var m doctor.MCPReport
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m.Listed != 1 || m.Connected != 1 || m.Servers[0].State != "connected" {
		t.Fatalf("%+v", m)
	}
}

func TestPathsFromEnv_Defaults(t *testing.T) {
	t.Setenv("DATA_DIR", "")
	t.Setenv("PERSONA_DIR", "")
	t.Setenv("MCP_MANIFEST", "")
	t.Setenv("CHANNEL", "")
	p := doctor.PathsFromEnv()
	if p.DataDir != "/data" || p.PersonaDir != "/persona" || p.Manifest != "/etc/gantry/mcp.toml" || p.Channel != "telegram" {
		t.Fatalf("%+v", p)
	}
	t.Setenv("CHANNEL", " Slack ")
	p = doctor.PathsFromEnv()
	if p.Channel != "slack" {
		t.Fatalf("%q", p.Channel)
	}
}

func touchHeartbeat(t *testing.T, dir string) {
	t.Helper()
	store, err := session.Open(dir, 10, 1000)
	if err != nil {
		t.Fatal(err)
	}
	hb, err := heartbeat.OpenDB(store.DB())
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := hb.Touch(context.Background(), "test"); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}

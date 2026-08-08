package agent_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/channel"
)

func TestAgent_AuthListAndGarminRefuse(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "mcp.toml")
	if err := os.WriteFile(manifest, []byte(`
[[server]]
name = "strava"
command = "echo"
auth_args = ["auth"]

[[server]]
name = "garmin"
command = "echo"
auth_args = ["login"]
`), 0o644); err != nil {
		t.Fatal(err)
	}

	a, err := agent.New(agent.Options{
		Completer:   &fakeCompleter{},
		Sessions:    newMemHistory(),
		Model:       "m",
		MCPManifest: manifest,
	})
	if err != nil {
		t.Fatal(err)
	}

	list, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "/auth"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(list, "strava") || !strings.Contains(list, "garmin") || !strings.Contains(list, "TTY only") {
		t.Fatalf("list = %q", list)
	}
	if !strings.Contains(list, agent.AuthGuideURL) {
		t.Fatalf("missing guide: %q", list)
	}

	refuse, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "/auth garmin"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(refuse, "not supported") || !strings.Contains(strings.ToLower(refuse), "password") {
		t.Fatalf("refuse = %q", refuse)
	}
}

func TestAgent_AuthURLAndExchange(t *testing.T) {
	dir := t.TempDir()
	bin := writeAuthStub(t, dir)

	manifest := filepath.Join(dir, "mcp.toml")
	content := "[[server]]\nname = \"demo\"\ncommand = " + tomlQuote(bin) + "\nauth_args = [\"auth\"]\n"
	if err := os.WriteFile(manifest, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	a, err := agent.New(agent.Options{
		Completer:   &fakeCompleter{},
		Sessions:    newMemHistory(),
		Model:       "m",
		MCPManifest: manifest,
	})
	if err != nil {
		t.Fatal(err)
	}

	urlOut, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "/auth demo"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(urlOut, "open https://example.test/auth") || !strings.Contains(urlOut, "/auth demo <code>") {
		t.Fatalf("url = %q", urlOut)
	}

	exOut, err := a.Handle(context.Background(), channel.Message{SessionID: "s", Text: "/auth demo abc123"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(exOut, "demo: authorized") {
		t.Fatalf("exchange = %q", exOut)
	}
}

func tomlQuote(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}

func writeAuthStub(t *testing.T, dir string) string {
	t.Helper()
	src := filepath.Join(dir, "authstub.go")
	code := `package main
import (
  "fmt"
  "os"
)
func main() {
  args := os.Args[1:]
  if len(args) >= 2 && args[0] == "auth" && args[1] == "url" {
    fmt.Println("open https://example.test/auth")
    fmt.Println("then paste the code: /auth demo <code>")
    return
  }
  if len(args) >= 3 && args[0] == "auth" && args[1] == "exchange" {
    fmt.Printf("demo: authorized ✓ (tokens → /tmp/demo.json) code=%s\n", args[2])
    return
  }
  fmt.Fprintln(os.Stderr, "usage: auth url|exchange")
  os.Exit(2)
}
`
	if err := os.WriteFile(src, []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "authstub")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", out, src)
	cmd.Env = os.Environ()
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, b)
	}
	return out
}

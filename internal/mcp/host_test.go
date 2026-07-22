package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/shotah/ai-gantry/internal/mcp"
)

type fakeConn struct {
	tools []mcp.Tool
	calls int
	fail  bool
}

func (f *fakeConn) ListTools(context.Context) ([]mcp.Tool, error) {
	out := make([]mcp.Tool, len(f.tools))
	copy(out, f.tools)
	return out, nil
}

func (f *fakeConn) CallTool(_ context.Context, name string, args map[string]any) (string, error) {
	f.calls++
	if f.fail {
		f.fail = false
		return "", fmt.Errorf("boom")
	}
	b, _ := json.Marshal(args)
	return name + ":" + string(b), nil
}

func (f *fakeConn) Close() error { return nil }

func writeManifest(t *testing.T, servers string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mcp.toml")
	if err := os.WriteFile(path, []byte(servers), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHost_StartCallRestart(t *testing.T) {
	path := writeManifest(t, `
[[server]]
name = "demo"
command = "unused"
`)
	conn := &fakeConn{tools: []mcp.Tool{{
		OriginalName: "echo",
		Description:  "echo args",
		InputSchema:  map[string]any{"type": "object"},
	}}}
	dials := 0
	host, err := mcp.Start(context.Background(), mcp.Options{
		ManifestPath:   path,
		ResultMaxChars: 1000,
		Dial: func(_ context.Context, spec mcp.ServerSpec, _ io.Writer) (mcp.Conn, error) {
			dials++
			if spec.Name != "demo" {
				t.Fatalf("spec %#v", spec)
			}
			return conn, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = host.Close() })

	if host.ToolCount() != 1 {
		t.Fatalf("tools=%d", host.ToolCount())
	}
	defs := host.Tools()
	if defs[0].Name != "demo__echo" {
		t.Fatalf("%q", defs[0].Name)
	}

	out, err := host.Call(context.Background(), "demo__echo", json.RawMessage(`{"q":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "echo:") {
		t.Fatalf("%q", out)
	}

	// Force failure then successful restart re-dial.
	conn.fail = true
	out, err = host.Call(context.Background(), "demo__echo", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if dials < 2 {
		t.Fatalf("dials=%d, want restart", dials)
	}
	if out == "" {
		t.Fatal("empty result after restart")
	}
}

func TestHost_BootFail(t *testing.T) {
	path := writeManifest(t, `
[[server]]
name = "demo"
command = "unused"
`)
	_, err := mcp.Start(context.Background(), mcp.Options{
		ManifestPath: path,
		Dial: func(context.Context, mcp.ServerSpec, io.Writer) (mcp.Conn, error) {
			return nil, fmt.Errorf("cannot spawn")
		},
	})
	if err == nil || !strings.Contains(err.Error(), "boot server") {
		t.Fatalf("err = %v", err)
	}
}

func TestHost_Truncate(t *testing.T) {
	path := writeManifest(t, `
[[server]]
name = "demo"
command = "x"
`)
	host, err := mcp.Start(context.Background(), mcp.Options{
		ManifestPath:   path,
		ResultMaxChars: 40,
		Dial: func(context.Context, mcp.ServerSpec, io.Writer) (mcp.Conn, error) {
			return &longConn{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = host.Close() })

	out, err := host.Call(context.Background(), "demo__big", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "truncated") {
		t.Fatalf("%q", out)
	}
}

type longConn struct{}

func (longConn) ListTools(context.Context) ([]mcp.Tool, error) {
	return []mcp.Tool{{OriginalName: "big", InputSchema: map[string]any{"type": "object"}}}, nil
}

func (longConn) CallTool(context.Context, string, map[string]any) (string, error) {
	return strings.Repeat("z", 200), nil
}

func (longConn) Close() error { return nil }

func TestHost_InMemorySDK(t *testing.T) {
	// Real SDK server over in-memory transport.
	path := writeManifest(t, `
[[server]]
name = "mem"
command = "unused"
`)

	type in struct {
		Name string `json:"name"`
	}
	type out struct {
		Greeting string `json:"greeting"`
	}

	host, err := mcp.Start(context.Background(), mcp.Options{
		ManifestPath: path,
		Dial: func(ctx context.Context, _ mcp.ServerSpec, _ io.Writer) (mcp.Conn, error) {
			server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "mem", Version: "v1"}, nil)
			mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "greet", Description: "hi"}, func(_ context.Context, _ *mcpsdk.CallToolRequest, input in) (*mcpsdk.CallToolResult, out, error) {
				return nil, out{Greeting: "Hello " + input.Name}, nil
			})
			t1, t2 := mcpsdk.NewInMemoryTransports()
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = server.Connect(ctx, t1, nil)
			}()
			client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "gantry-test", Version: "v1"}, nil)
			session, err := client.Connect(ctx, t2, nil)
			if err != nil {
				return nil, err
			}
			// Adapt session via a thin wrapper using the package's default shape:
			// we need mcp.Conn — use a local adapter.
			return &sdkAdapter{session: session}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = host.Close() })

	got, err := host.Call(context.Background(), "mem__greet", json.RawMessage(`{"name":"gantry"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Hello gantry") {
		t.Fatalf("%q", got)
	}
}

// sdkAdapter exposes ClientSession as mcp.Conn for tests (mirrors production sdkConn).
type sdkAdapter struct {
	session *mcpsdk.ClientSession
}

func (a *sdkAdapter) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	var out []mcp.Tool
	for tool, err := range a.session.Tools(ctx, nil) {
		if err != nil {
			return nil, err
		}
		schema, _ := tool.InputSchema.(map[string]any)
		out = append(out, mcp.Tool{
			OriginalName: tool.Name,
			Description:  tool.Description,
			InputSchema:  schema,
		})
	}
	return out, nil
}

func (a *sdkAdapter) CallTool(ctx context.Context, name string, arguments map[string]any) (string, error) {
	res, err := a.session.CallTool(ctx, &mcpsdk.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		return "", err
	}
	var parts []string
	for _, c := range res.Content {
		if tc, ok := c.(*mcpsdk.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	if len(parts) == 0 && res.StructuredContent != nil {
		b, _ := json.Marshal(res.StructuredContent)
		return string(b), nil
	}
	return strings.Join(parts, "\n"), nil
}

func (a *sdkAdapter) Close() error { return a.session.Close() }

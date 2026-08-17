package mcp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/shotah/ai-gantry/internal/mcp"
	"github.com/shotah/ai-gantry/internal/provider"
)

type fakeConn struct {
	tools      []mcp.Tool
	calls      int
	fail       bool
	failErr    error // when fail; default restartable EOF
	blockUntil <-chan struct{}
	closed     bool
}

func (f *fakeConn) ListTools(context.Context) ([]mcp.Tool, error) {
	out := make([]mcp.Tool, len(f.tools))
	copy(out, f.tools)
	return out, nil
}

func (f *fakeConn) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	f.calls++
	if f.blockUntil != nil {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-f.blockUntil:
		}
	}
	if f.fail {
		f.fail = false
		if f.failErr != nil {
			return "", f.failErr
		}
		return "", fmt.Errorf("EOF")
	}
	b, _ := json.Marshal(args)
	return name + ":" + string(b), nil
}

func (f *fakeConn) Close() error {
	f.closed = true
	return nil
}

func writeManifest(t *testing.T, servers string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mcp.toml")
	if err := os.WriteFile(path, []byte(servers), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHost_UnknownToolSuggestsCatalog(t *testing.T) {
	path := writeManifest(t, `
[[server]]
name = "google-workspace"
command = "unused"
`)
	host, err := mcp.Start(context.Background(), mcp.Options{
		ManifestPath: path,
		Dial: func(context.Context, mcp.ServerSpec, io.Writer) (mcp.Conn, error) {
			return &fakeConn{tools: []mcp.Tool{
				{OriginalName: "get_events"},
				{OriginalName: "list_calendars"},
			}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = host.Close() })

	// Hallucinated tool on a real server: list that server's real tools.
	_, err = host.Call(context.Background(), "google-workspace__get_calendar_event", nil)
	if err == nil {
		t.Fatal("want error for unknown tool")
	}
	for _, want := range []string{
		`unknown tool "google-workspace__get_calendar_event"`,
		"valid google-workspace tools include",
		"google-workspace__get_events",
		"google-workspace__list_calendars",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err = %v, want substring %q", err, want)
		}
	}

	// Unknown server prefix: list available prefixes.
	_, err = host.Call(context.Background(), "bogus__thing", nil)
	if err == nil || !strings.Contains(err.Error(), "available server prefixes are: google-workspace") {
		t.Fatalf("err = %v, want prefix list", err)
	}
}

// A failed tool call costs a full model round-trip — the most expensive thing in
// a local-model turn — so a real tool name wearing an invented or missing prefix
// is repaired in place instead of bounced back as a hint.
func TestHost_RepairsPrefixOnRealToolName(t *testing.T) {
	path := writeManifest(t, `
[[server]]
name = "garmin"
command = "unused"

[[server]]
name = "strava"
command = "unused"
`)
	host, err := mcp.Start(context.Background(), mcp.Options{
		ManifestPath: path,
		Dial: func(_ context.Context, spec mcp.ServerSpec, _ io.Writer) (mcp.Conn, error) {
			if spec.Name == "garmin" {
				return &fakeConn{tools: []mcp.Tool{
					{OriginalName: "hrv_get"},
					{OriginalName: "sleep_get"},
					{OriginalName: "activities_get"},
				}}, nil
			}
			return &fakeConn{tools: []mcp.Tool{
				{OriginalName: "activities_get"},
				{OriginalName: "athlete_get"},
			}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = host.Close() })
	ctx := context.Background()

	// Invented prefix, one server publishes the name: call it.
	for _, name := range []string{"mcp__hrv_get", "sleep_get", "GARMIN__hrv_get"} {
		out, err := host.Call(ctx, name, nil)
		if err != nil {
			t.Fatalf("Call(%q) = %v, want repair", name, err)
		}
		if !strings.Contains(out, "hrv_get") && !strings.Contains(out, "sleep_get") {
			t.Fatalf("Call(%q) out = %q, want the tool result", name, out)
		}
	}

	// Both servers publish activities_get: guessing would be a coin flip, so hand
	// the model both real names instead.
	_, err = host.Call(ctx, "mcp__activities_get", nil)
	if err == nil {
		t.Fatal("want error for an ambiguous tool name")
	}
	for _, want := range []string{"garmin__activities_get", "strava__activities_get"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err = %v, want substring %q", err, want)
		}
	}

	// Real prefix: the model picked that server deliberately, so list its
	// catalog rather than silently crossing to the server that has the name.
	_, err = host.Call(ctx, "garmin__athlete_get", nil)
	if err == nil {
		t.Fatal("want error, not a cross-server repair")
	}
	if !strings.Contains(err.Error(), "valid garmin tools include") {
		t.Fatalf("err = %v, want the garmin catalog", err)
	}
}

// Calendar ask + hallucinated strava__*event* must not put every Strava tool in
// Candidates — that constrained the retry so the model could not call google__.
func TestHost_WrongServerGuessCandidatesAreNearestNotWholeCatalog(t *testing.T) {
	path := writeManifest(t, `
[[server]]
name = "google"
command = "unused"

[[server]]
name = "strava"
command = "unused"
`)
	host, err := mcp.Start(context.Background(), mcp.Options{
		ManifestPath: path,
		Dial: func(_ context.Context, spec mcp.ServerSpec, _ io.Writer) (mcp.Conn, error) {
			switch spec.Name {
			case "google":
				return &fakeConn{tools: []mcp.Tool{
					{OriginalName: "calendar_list_events"},
					{OriginalName: "calendar_get_event"},
					{OriginalName: "calendar_delete_event"},
				}}, nil
			case "strava":
				return &fakeConn{tools: []mcp.Tool{
					{OriginalName: "activities_list"},
					{OriginalName: "activities_get"},
					{OriginalName: "activities_get_streams"},
					{OriginalName: "athlete_get"},
					{OriginalName: "athlete_get_stats"},
					{OriginalName: "activities_get_zones"},
				}}, nil
			default:
				return nil, fmt.Errorf("unexpected server %q", spec.Name)
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = host.Close() })

	_, err = host.Call(context.Background(), "strava__activities_get_event", nil)
	if err == nil {
		t.Fatal("want unknown tool error")
	}
	var unknown *mcp.UnknownToolError
	if !errors.As(err, &unknown) {
		t.Fatalf("err is %T, want *mcp.UnknownToolError", err)
	}
	if !strings.Contains(unknown.Hint, "valid strava tools include") {
		t.Fatalf("hint should still list the strava catalog: %s", unknown.Hint)
	}
	for _, c := range unknown.Candidates {
		if strings.HasPrefix(c, "strava__") && len(unknown.Candidates) >= 6 {
			t.Fatalf("Candidates = %v; must not be the whole strava catalog (traps calendar retries)", unknown.Candidates)
		}
	}
	// Prefer pointing at calendar when the invented name ends in get_event.
	joined := strings.Join(unknown.Candidates, " ")
	if !strings.Contains(joined, "google__calendar_") && len(unknown.Candidates) > 0 {
		// nearest may be empty if tokens don't match — also OK (unconstrained retry).
		t.Logf("Candidates = %v (ok if empty; must not be full strava dump)", unknown.Candidates)
	}
	if len(unknown.Candidates) >= 6 {
		t.Fatalf("Candidates = %v, want at most a short nearest list", unknown.Candidates)
	}
}

// Observed on Qwen: it invented mcp__get_hrv_and_body_battery — a fake prefix
// stitched onto two real tool names merged together — then gave up when the
// error only listed server prefixes. The fragments name the tools it wanted, so
// the hint must lead with those.
func TestHost_InventedToolNameSuggestsRealNeighbors(t *testing.T) {
	path := writeManifest(t, `
[[server]]
name = "garmin"
command = "unused"
`)
	host, err := mcp.Start(context.Background(), mcp.Options{
		ManifestPath: path,
		Dial: func(context.Context, mcp.ServerSpec, io.Writer) (mcp.Conn, error) {
			return &fakeConn{tools: []mcp.Tool{
				{OriginalName: "hrv_get"},
				{OriginalName: "wellness_get_body_battery"},
				{OriginalName: "sleep_get"},
				{OriginalName: "activities_list"},
			}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = host.Close() })

	_, err = host.Call(context.Background(), "mcp__get_hrv_and_body_battery", nil)
	if err == nil {
		t.Fatal("want error for invented tool")
	}
	got := err.Error()
	if !strings.Contains(got, "closest real names are") {
		t.Fatalf("err = %v, want closest-name hint", err)
	}
	// Two shared tokens (body, battery) must outrank one (hrv).
	body := strings.Index(got, "garmin__wellness_get_body_battery")
	hrv := strings.Index(got, "garmin__hrv_get")
	if body < 0 || hrv < 0 || body > hrv {
		t.Fatalf("err = %v, want wellness_get_body_battery ranked before hrv_get", err)
	}
	// Tools sharing nothing but the generic "get"/"list" verb are noise.
	for _, unwanted := range []string{"garmin__sleep_get", "garmin__activities_list"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("err = %v, must not suggest unrelated %q", err, unwanted)
		}
	}
	// The same names must be machine-readable, so the agent can constrain the
	// retry instead of trusting the model to read the hint.
	var unknown *mcp.UnknownToolError
	if !errors.As(err, &unknown) {
		t.Fatalf("err is %T, want *mcp.UnknownToolError", err)
	}
	want := []string{"garmin__wellness_get_body_battery", "garmin__hrv_get"}
	if !slices.Equal(unknown.Candidates, want) {
		t.Fatalf("candidates = %v, want %v", unknown.Candidates, want)
	}

}

// The schema block is the biggest slice of the prompt and leads the system
// message, so a randomized order would rewrite the prefix every turn and cost a
// full re-prefill instead of a prompt-cache hit. Order must be stable and sorted.
func TestHost_ToolsOrderIsStable(t *testing.T) {
	path := writeManifest(t, `
[[server]]
name = "garmin"
command = "unused"
`)
	// Enough tools that map iteration order would differ between calls.
	var tools []mcp.Tool
	for _, n := range []string{
		"list_activities", "get_activity", "get_sleep", "get_weight",
		"get_body_battery", "get_training_readiness", "get_hrv", "get_steps",
		"get_stress", "get_vo2max", "get_races", "get_gear",
	} {
		tools = append(tools, mcp.Tool{OriginalName: n})
	}
	host, err := mcp.Start(context.Background(), mcp.Options{
		ManifestPath: path,
		Dial: func(context.Context, mcp.ServerSpec, io.Writer) (mcp.Conn, error) {
			return &fakeConn{tools: tools}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = host.Close() })

	first := toolNames(host.Tools())
	if len(first) != len(tools) {
		t.Fatalf("got %d tools, want %d", len(first), len(tools))
	}
	if !sort.StringsAreSorted(first) {
		t.Fatalf("tools not sorted by name: %v", first)
	}
	// Repeat: randomized map iteration shows up as a differing order here.
	for i := 0; i < 20; i++ {
		if got := toolNames(host.Tools()); !slices.Equal(got, first) {
			t.Fatalf("call %d order = %v, want %v", i, got, first)
		}
	}
}

func toolNames(defs []provider.ToolDef) []string {
	out := make([]string, len(defs))
	for i, d := range defs {
		out[i] = d.Name
	}
	return out
}

func TestHost_CallAliasesUnderscoredPrefix(t *testing.T) {
	path := writeManifest(t, `
[[server]]
name = "google-search"
command = "unused"
`)
	conn := &fakeConn{tools: []mcp.Tool{{OriginalName: "web_search"}}}
	host, err := mcp.Start(context.Background(), mcp.Options{
		ManifestPath: path,
		Dial: func(context.Context, mcp.ServerSpec, io.Writer) (mcp.Conn, error) {
			return conn, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = host.Close() })

	// Exact name still works.
	got, err := host.Call(context.Background(), "google-search__web_search", json.RawMessage(`{"query":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "web_search:") {
		t.Fatalf("got %q, want web_search:…", got)
	}

	// Local models often turn the hyphenated server prefix into underscores.
	got, err = host.Call(context.Background(), "google_search__web_search", json.RawMessage(`{"query":"edgeworks"}`))
	if err != nil {
		t.Fatalf("aliased call failed: %v", err)
	}
	if !strings.Contains(got, "edgeworks") {
		t.Fatalf("got %q, want args echoed", got)
	}
	if conn.calls != 2 {
		t.Fatalf("calls = %d, want 2", conn.calls)
	}
}

func TestHost_UnknownToolSuggestsHyphenPrefix(t *testing.T) {
	path := writeManifest(t, `
[[server]]
name = "google-search"
command = "unused"
`)
	host, err := mcp.Start(context.Background(), mcp.Options{
		ManifestPath: path,
		Dial: func(context.Context, mcp.ServerSpec, io.Writer) (mcp.Conn, error) {
			return &fakeConn{tools: []mcp.Tool{{OriginalName: "web_search"}}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = host.Close() })

	// Wrong tool + underscored prefix: keep listing the real catalog (not only prefixes).
	_, err = host.Call(context.Background(), "google_search__google_search", nil)
	if err == nil {
		t.Fatal("want error for unknown tool")
	}
	for _, want := range []string{
		`unknown tool "google_search__google_search"`,
		`did you mean "google-search"?`,
		"valid google-search tools include",
		"google-search__web_search",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err = %v, want substring %q", err, want)
		}
	}
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
	if rows := host.ServerHealth(); len(rows) != 1 || rows[0].State != mcp.ServerOK || rows[0].Tool != "demo__echo" {
		t.Fatalf("health after ok = %#v", rows)
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

func TestHost_ToolsAllowlistAndPrefix(t *testing.T) {
	path := writeManifest(t, `
[[server]]
name = "garmin"
command = "unused"
tools = ["sleep_get", "raw_dump"]
exclude = ["raw_*"]
tools_prefix = "garm"
`)
	host, err := mcp.Start(context.Background(), mcp.Options{
		ManifestPath: path,
		Dial: func(context.Context, mcp.ServerSpec, io.Writer) (mcp.Conn, error) {
			return &fakeConn{tools: []mcp.Tool{
				{OriginalName: "sleep_get"},
				{OriginalName: "weight_get"},
				{OriginalName: "raw_dump"},
			}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = host.Close() })
	if host.ToolCount() != 1 {
		t.Fatalf("tools=%d", host.ToolCount())
	}
	if host.Tools()[0].Name != "garm__sleep_get" {
		t.Fatalf("%q", host.Tools()[0].Name)
	}
}

func TestHost_BootSoftSkipsFailedServer(t *testing.T) {
	path := writeManifest(t, `
[[server]]
name = "broken"
command = "unused"

[[server]]
name = "ok"
command = "unused"
`)
	host, err := mcp.Start(context.Background(), mcp.Options{
		ManifestPath: path,
		Dial: func(_ context.Context, spec mcp.ServerSpec, _ io.Writer) (mcp.Conn, error) {
			if spec.Name == "broken" {
				return nil, fmt.Errorf("cannot spawn")
			}
			return &fakeConn{tools: []mcp.Tool{{OriginalName: "ping"}}}, nil
		},
	})
	if err != nil {
		t.Fatalf("Start should fail-soft, got %v", err)
	}
	t.Cleanup(func() { _ = host.Close() })
	if host.ToolCount() != 1 {
		t.Fatalf("tools=%d want 1 (broken skipped)", host.ToolCount())
	}
	if host.Tools()[0].Name != "ok__ping" {
		t.Fatalf("got %q", host.Tools()[0].Name)
	}
	health := host.ServerHealth()
	if len(health) != 2 {
		t.Fatalf("health=%#v", health)
	}
	if health[0].Name != "broken" || health[0].State != mcp.ServerSkipped || !strings.Contains(health[0].Note, "cannot spawn") {
		t.Fatalf("skipped=%#v", health[0])
	}
	if health[1].Name != "ok" || health[1].State != mcp.ServerIdle {
		t.Fatalf("ok=%#v", health[1])
	}
}

func TestHost_BootSoftAllFailedStillStarts(t *testing.T) {
	path := writeManifest(t, `
[[server]]
name = "demo"
command = "unused"
`)
	host, err := mcp.Start(context.Background(), mcp.Options{
		ManifestPath: path,
		Dial: func(context.Context, mcp.ServerSpec, io.Writer) (mcp.Conn, error) {
			return nil, fmt.Errorf("cannot spawn")
		},
	})
	if err != nil {
		t.Fatalf("Start should fail-soft even if all servers fail, got %v", err)
	}
	t.Cleanup(func() { _ = host.Close() })
	if host.ToolCount() != 0 {
		t.Fatalf("tools=%d want 0", host.ToolCount())
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

	raw, err := host.CallRaw(context.Background(), "demo__big", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "truncated") || raw != strings.Repeat("z", 200) {
		t.Fatalf("CallRaw should be untruncated, got %q", raw)
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

func TestHost_CancelDoesNotRestart(t *testing.T) {
	path := writeManifest(t, `
[[server]]
name = "demo"
command = "unused"
`)
	conn := &fakeConn{tools: []mcp.Tool{{OriginalName: "echo", InputSchema: map[string]any{"type": "object"}}}}
	dials := 0
	host, err := mcp.Start(context.Background(), mcp.Options{
		ManifestPath: path,
		Dial: func(context.Context, mcp.ServerSpec, io.Writer) (mcp.Conn, error) {
			dials++
			return conn, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = host.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	conn.blockUntil = make(chan struct{}) // never closed; wait on ctx only
	go func() {
		close(started)
		_, _ = host.Call(ctx, "demo__echo", json.RawMessage(`{}`))
	}()
	<-started
	// Give CallTool a moment to enter the block.
	time.Sleep(20 * time.Millisecond)
	cancel()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && conn.calls < 1 {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	if dials != 1 {
		t.Fatalf("dials=%d want 1 (no restart on cancel)", dials)
	}
	if conn.closed {
		t.Fatal("cancel must not Close the MCP connection")
	}
}

func TestHost_NonTransportErrorDoesNotRestart(t *testing.T) {
	path := writeManifest(t, `
[[server]]
name = "demo"
command = "unused"
`)
	conn := &fakeConn{
		tools:   []mcp.Tool{{OriginalName: "echo", InputSchema: map[string]any{"type": "object"}}},
		fail:    true,
		failErr: fmt.Errorf("invalid argument: bad date"),
	}
	dials := 0
	host, err := mcp.Start(context.Background(), mcp.Options{
		ManifestPath: path,
		Dial: func(context.Context, mcp.ServerSpec, io.Writer) (mcp.Conn, error) {
			dials++
			return conn, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = host.Close() })

	_, err = host.Call(context.Background(), "demo__echo", json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "invalid argument") {
		t.Fatalf("err=%v", err)
	}
	if dials != 1 {
		t.Fatalf("dials=%d want 1 (no restart on arg error)", dials)
	}
}

func TestHost_CallStats(t *testing.T) {
	path := writeManifest(t, `
[[server]]
name = "google-search"
command = "unused"

[[server]]
name = "slow"
command = "unused"
`)
	host, err := mcp.Start(context.Background(), mcp.Options{
		ManifestPath: path,
		Dial: func(_ context.Context, spec mcp.ServerSpec, _ io.Writer) (mcp.Conn, error) {
			switch spec.Name {
			case "google-search":
				return &fakeConn{tools: []mcp.Tool{{OriginalName: "web_search"}}}, nil
			case "slow":
				return &fakeConn{
					tools:   []mcp.Tool{{OriginalName: "work"}},
					fail:    true,
					failErr: fmt.Errorf("invalid argument: nope"),
				}, nil
			default:
				return nil, fmt.Errorf("unknown %s", spec.Name)
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = host.Close() })

	empty := host.CallStats()
	if empty.TotalCalls != 0 {
		t.Fatalf("empty stats = %#v", empty)
	}

	_, _ = host.Call(context.Background(), "google-search__web_search", json.RawMessage(`{}`))
	_, _ = host.Call(context.Background(), "google_search__web_search", json.RawMessage(`{}`)) // alias
	_, _ = host.Call(context.Background(), "slow__work", json.RawMessage(`{}`))                // error
	_, _ = host.Call(context.Background(), "bogus__thing", nil)                                // unknown

	stats := host.CallStats()
	if stats.TotalCalls != 3 {
		t.Fatalf("totalCalls=%d want 3", stats.TotalCalls)
	}
	if stats.PrefixAlias != 1 {
		t.Fatalf("prefixAlias=%d want 1", stats.PrefixAlias)
	}
	if stats.UnknownTool < 1 {
		t.Fatalf("unknownTool=%d", stats.UnknownTool)
	}
	if len(stats.ByTool) < 2 {
		t.Fatalf("byTool=%#v", stats.ByTool)
	}
	// Sorted by total duration desc — both tools present; slow has the error.
	var slow, search *mcp.ToolCallStat
	for i := range stats.ByTool {
		switch stats.ByTool[i].Name {
		case "slow__work":
			slow = &stats.ByTool[i]
		case "google-search__web_search":
			search = &stats.ByTool[i]
		}
	}
	if search == nil || search.Calls != 2 || search.Errors != 0 {
		t.Fatalf("search=%#v", search)
	}
	if slow == nil || slow.Calls != 1 || slow.Errors != 1 {
		t.Fatalf("slow=%#v", slow)
	}
}

type overlapConn struct {
	fakeConn
	delay    time.Duration
	inflight *atomic.Int32
	peak     *atomic.Int32
}

func (c *overlapConn) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	n := c.inflight.Add(1)
	for {
		old := c.peak.Load()
		if n <= old || c.peak.CompareAndSwap(old, n) {
			break
		}
	}
	defer c.inflight.Add(-1)
	time.Sleep(c.delay)
	return c.fakeConn.CallTool(ctx, name, args)
}

func TestHost_CallsDifferentServersInParallel(t *testing.T) {
	path := writeManifest(t, `
[[server]]
name = "search"
command = "unused"

[[server]]
name = "calendar"
command = "unused"
`)
	var inflight, peak atomic.Int32
	host, err := mcp.Start(context.Background(), mcp.Options{
		ManifestPath: path,
		Dial: func(context.Context, mcp.ServerSpec, io.Writer) (mcp.Conn, error) {
			return &overlapConn{
				fakeConn: fakeConn{tools: []mcp.Tool{{OriginalName: "go"}}},
				delay:    80 * time.Millisecond,
				inflight: &inflight,
				peak:     &peak,
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = host.Close() })

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := host.Call(context.Background(), "search__go", json.RawMessage(`{}`)); err != nil {
			t.Errorf("search: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		if _, err := host.Call(context.Background(), "calendar__go", json.RawMessage(`{}`)); err != nil {
			t.Errorf("calendar: %v", err)
		}
	}()
	wg.Wait()
	if peak.Load() < 2 {
		t.Fatalf("peak inflight = %d, want cross-server overlap", peak.Load())
	}
}

func TestHost_SerializesSameServerCalls(t *testing.T) {
	path := writeManifest(t, `
[[server]]
name = "search"
command = "unused"
`)
	var inflight, peak atomic.Int32
	host, err := mcp.Start(context.Background(), mcp.Options{
		ManifestPath: path,
		Dial: func(context.Context, mcp.ServerSpec, io.Writer) (mcp.Conn, error) {
			return &overlapConn{
				fakeConn: fakeConn{tools: []mcp.Tool{{OriginalName: "go"}}},
				delay:    40 * time.Millisecond,
				inflight: &inflight,
				peak:     &peak,
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = host.Close() })

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = host.Call(context.Background(), "search__go", json.RawMessage(`{"n":1}`))
	}()
	go func() {
		defer wg.Done()
		_, _ = host.Call(context.Background(), "search__go", json.RawMessage(`{"n":2}`))
	}()
	wg.Wait()
	if peak.Load() != 1 {
		t.Fatalf("peak inflight = %d, want 1 (stdio child is one pipe)", peak.Load())
	}
}

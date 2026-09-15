package agent_test

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/channel/pendant"
	"github.com/shotah/ai-gantry/internal/cron"
	"github.com/shotah/ai-gantry/internal/memory"
	"github.com/shotah/ai-gantry/internal/provider"
	"github.com/shotah/ai-gantry/internal/session"
)

// Frozen Pacific noon so testdata/pendant/*.txt is a readable Completer dump,
// not a moving NOW. Production still uses time.Now.
//
// These dumps omit memory hydration, MCP health, wait notes, and tool
// schemas unless the test wires Memory. completer_*.txt is the agent
// Request (trailing [harness] system). completer_*_gemini_wire.txt is
// what Gemini's OpenAI-compat body actually gets (prompt_wire_test.go
// proves it from the socket). Hours/aims/loops: completer_horizon_harness.txt.
// Everything at once (cron wakes, Cab surface, last contact):
// completer_fullboard_harness.txt.
func payloadClock() (loc *time.Location, now time.Time) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		panic(err)
	}
	return loc, time.Date(2026, time.September, 14, 12, 2, 0, 0, loc)
}

func TestPendantInbound_CompleterPayload(t *testing.T) {
	tests := []struct {
		name    string
		inbound string
		want    string
	}{
		{
			name:    "current PWA geo only — no phone clock",
			inbound: "inbound_geo.json",
			want:    "completer_geo.txt",
		},
		{
			name:    "GPS off — crane clock still present, no location",
			inbound: "inbound_nogeo.json",
			want:    "completer_nogeo.txt",
		},
		{
			name:    "old mouth at/tz ignored — crane clock wins",
			inbound: "inbound_stale_clock.json",
			want:    "completer_geo.txt",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", "pendant", tc.inbound))
			if err != nil {
				t.Fatal(err)
			}
			msg, ok, err := pendant.InboundTurn(raw)
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				t.Fatal("fixture must start a turn")
			}
			msg.SessionID = "payload-" + tc.inbound

			loc, now := payloadClock()
			var captured provider.Request
			fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
				captured = req
				return &provider.Result{Content: "ok"}, nil
			}}
			a, err := agent.New(agent.Options{
				Persona:   "You are Kit.",
				Completer: fc,
				Sessions:  newMemHistory(),
				Model:     "m",
				Location:  loc,
				TZName:    "America/Los_Angeles",
				Now:       func() time.Time { return now },
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := a.Handle(context.Background(), msg); err != nil {
				t.Fatal(err)
			}
			got := formatCompleterRequest(captured)
			assertGolden(t, filepath.Join("testdata", "pendant", tc.want), got)
			gemini := captured
			gemini.Messages = provider.WireMessages("gemini-3.6-flash", captured.Messages)
			assertGolden(t, filepath.Join("testdata", "pendant", strings.TrimSuffix(tc.want, ".txt")+"_gemini_wire.txt"), formatCompleterRequest(gemini))
		})
	}
}

func formatCompleterRequest(req provider.Request) string {
	var b strings.Builder
	fmt.Fprintf(&b, "tools: %d\n", len(req.Tools))
	for i, m := range req.Messages {
		fmt.Fprintf(&b, "\n## [%d] %s\n%s\n", i, m.Role, m.Content)
		for _, u := range m.ImageURLs {
			fmt.Fprintf(&b, "image: %s\n", u)
		}
	}
	return b.String()
}

func TestPendantInbound_CompleterPayloadHorizon(t *testing.T) {
	ctx := context.Background()
	mem, err := memory.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mem.Close() })
	if _, err := mem.Store(ctx, memory.KindPreference, memory.SubjectHours, "sleep: 22:00-06:00\nwork: 07:00-14:00\nquiet: (none)\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Store(ctx, memory.KindInsight, "aim/training", "3x gym this month"); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Store(ctx, memory.KindFact, "waiting/dentist", "book cleaning"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join("testdata", "pendant", "inbound_geo.json"))
	if err != nil {
		t.Fatal(err)
	}
	msg, ok, err := pendant.InboundTurn(raw)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	msg.SessionID = "payload-horizon"
	loc, now := payloadClock()
	var captured provider.Request
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		captured = req
		return &provider.Result{Content: "ok"}, nil
	}}
	a, err := agent.New(agent.Options{
		Persona:   "You are Kit.",
		Completer: fc,
		Sessions:  newMemHistory(),
		Memory:    mem,
		Model:     "m",
		Location:  loc,
		TZName:    "America/Los_Angeles",
		Now:       func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Handle(ctx, msg); err != nil {
		t.Fatal(err)
	}
	assertGolden(t, filepath.Join("testdata", "pendant", "completer_horizon_harness.txt"), promptHarnessClock(captured.Messages)+"\n")
}

// Full board: real session + cron stores, memory on, Cab head unit. Pins
// [hours] [aims] [loops] [wakes] [surface] [last contact] together, and that
// rows already on [aims] are not paid again in [memory] hydration.
func TestPendantInbound_CompleterPayloadFullBoard(t *testing.T) {
	ctx := context.Background()
	loc, now := payloadClock()
	dir := t.TempDir()
	mem, err := memory.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mem.Close() })
	for _, row := range []struct{ kind, subject, content string }{
		{memory.KindPreference, memory.SubjectHours, "sleep: 22:00-06:00\nwork: 07:00-14:00\nquiet: (none)\n"},
		{memory.KindPreference, "pref/food", "tacos, pho"},
		{memory.KindInsight, "aim/training", "3x gym this month"},
		{memory.KindFact, "waiting/dentist", "book cleaning"},
		{memory.KindFact, "follow/visa", "packet in"},
	} {
		if _, err := mem.Store(ctx, row.kind, row.subject, row.content); err != nil {
			t.Fatal(err)
		}
	}
	sessions, err := session.Open(dir, 50, 100000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sessions.Close() })
	jobs, err := cron.OpenDB(sessions.DB(), 50)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join("testdata", "pendant", "inbound_cab_auto.json"))
	if err != nil {
		t.Fatal(err)
	}
	msg, ok, err := pendant.InboundTurn(raw)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	msg.SessionID = "payload-fullboard"
	delivery := cron.Delivery{SessionID: msg.SessionID}
	gym := now.Add(5 * time.Hour)
	if _, err := jobs.Schedule(ctx, "Remind them to leave for the gym", cron.Parsed{
		Kind: cron.KindOnce, Expr: gym.Format(time.RFC3339), NextRun: gym, Timezone: "America/Los_Angeles",
	}, delivery); err != nil {
		t.Fatal(err)
	}
	if _, err := jobs.Schedule(ctx, cron.SparkPingPrefix, cron.SparkPingParsed(now.Add(time.Hour), "America/Los_Angeles"), delivery); err != nil {
		t.Fatal(err)
	}

	var captured provider.Request
	fc := &fakeCompleter{fn: func(req provider.Request) (*provider.Result, error) {
		captured = req
		return &provider.Result{Content: "ok"}, nil
	}}
	a, err := agent.New(agent.Options{
		Persona:   "You are Kit.",
		Completer: fc,
		Sessions:  sessions,
		Memory:    mem,
		Wakes:     jobs,
		Model:     "m",
		Location:  loc,
		TZName:    "America/Los_Angeles",
		Now:       func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Handle(ctx, msg); err != nil {
		t.Fatal(err)
	}
	assertGolden(t, filepath.Join("testdata", "pendant", "completer_fullboard_harness.txt"), promptHarnessClock(captured.Messages)+"\n")

	hydration := promptBlock(captured.Messages, "[memory]")
	if hydration == "" || !strings.Contains(hydration, "pref/food") {
		t.Fatalf("durable preference should hydrate:\n%s", formatCompleterRequest(captured))
	}
	for _, stamped := range []string{"aim/training", "waiting/dentist", "follow/visa"} {
		if strings.Contains(hydration, stamped) {
			t.Errorf("%s is on [aims]/[loops] and must not repeat in [memory]:\n%s", stamped, hydration)
		}
	}

	// Second turn: the first turn's user row is now the last human contact.
	// created_at is the store's wall clock, so only the shape is pinned.
	if _, err := a.Handle(ctx, msg); err != nil {
		t.Fatal(err)
	}
	clock := promptHarnessClock(captured.Messages)
	if !strings.Contains(clock, "\n[last contact] last human message ") || strings.Contains(clock, "first message") {
		t.Fatalf("second turn last contact:\n%s", clock)
	}
	if !strings.Contains(harnessHeader(clock), "last contact") {
		t.Fatalf("header must name last contact:\n%s", clock)
	}
}

func promptBlock(msgs []provider.Message, prefix string) string {
	for _, m := range msgs {
		if m.Role == provider.RoleSystem && strings.HasPrefix(m.Content, prefix) {
			return m.Content
		}
	}
	return ""
}

func harnessHeader(clock string) string {
	if i := strings.IndexByte(clock, '\n'); i >= 0 {
		return clock[:i]
	}
	return clock
}

// updateGoldens rewrites testdata/pendant/*.txt from the current run:
// go test ./internal/agent/ -run Payload -update. Read the diff before
// trusting it — the golden is the contract.
var updateGoldens = flag.Bool("update", false, "rewrite Completer payload goldens")

func assertGolden(t *testing.T, path, got string) {
	t.Helper()
	if *updateGoldens {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v\n--- got (Completer payload) ---\n%s", path, err, got)
	}
	if string(want) != got {
		t.Fatalf("completer payload mismatch\n--- want (%s) ---\n%s\n--- got ---\n%s", path, want, got)
	}
}

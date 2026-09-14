package agent_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/channel/pendant"
	"github.com/shotah/ai-gantry/internal/provider"
)

// Frozen Pacific noon so testdata/pendant/*.txt is a readable Completer dump,
// not a moving NOW. Production still uses time.Now.
//
// These dumps omit memory hydration, [hours], MCP health, wait notes, and
// tool schemas (nil Memory/Tools/Wait). The mouth contract is persona +
// RoleUser speech + [harness] after it. Open completer_geo.txt to read it.
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

func assertGolden(t *testing.T, path, got string) {
	t.Helper()
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v\n--- got (Completer payload) ---\n%s", path, err, got)
	}
	if string(want) != got {
		t.Fatalf("completer payload mismatch\n--- want (%s) ---\n%s\n--- got ---\n%s", path, want, got)
	}
}

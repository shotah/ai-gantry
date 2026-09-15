package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shotah/ai-gantry/internal/agent"
	"github.com/shotah/ai-gantry/internal/channel/pendant"
	"github.com/shotah/ai-gantry/internal/provider"
)

// End to end: PWA inbound JSON → Handle → real provider.Client → the
// OpenAI-compat HTTP body. completer_geo.txt and completer_geo_gemini_wire.txt
// were pinned from the agent Request and from WireMessages separately; this
// proves the bytes on the socket match those same goldens.
func TestPendantInbound_WireBody(t *testing.T) {
	tests := []struct {
		model string
		want  string
	}{
		{model: "gpt-5.4", want: "completer_geo.txt"},
		{model: "gemini-3.6-flash", want: "completer_geo_gemini_wire.txt"},
	}
	for _, tc := range tests {
		t.Run(tc.model, func(t *testing.T) {
			var body wireBody
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode body: %v", err)
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id": "x",
					"choices": []map[string]any{
						{"index": 0, "message": map[string]any{"role": "assistant", "content": "ok"}},
					},
				})
			}))
			t.Cleanup(srv.Close)

			raw, err := os.ReadFile(filepath.Join("testdata", "pendant", "inbound_geo.json"))
			if err != nil {
				t.Fatal(err)
			}
			msg, ok, err := pendant.InboundTurn(raw)
			if err != nil || !ok {
				t.Fatalf("ok=%v err=%v", ok, err)
			}
			msg.SessionID = "wire-" + tc.model
			loc, now := payloadClock()
			a, err := agent.New(agent.Options{
				Persona:   "You are Kit.",
				Completer: provider.New(srv.URL, "k", tc.model),
				Sessions:  newMemHistory(),
				Model:     tc.model,
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
			assertGolden(t, filepath.Join("testdata", "pendant", tc.want), body.format())
		})
	}
}

// wireBody is the slice of the chat.completions request the goldens pin.
type wireBody struct {
	Model    string            `json:"model"`
	Messages []wireMessage     `json:"messages"`
	Tools    []json.RawMessage `json:"tools"`
}

type wireMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// text is the message content whether the SDK sent a string or content parts.
func (m wireMessage) text() string {
	var s string
	if err := json.Unmarshal(m.Content, &s); err == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(m.Content, &parts); err != nil {
		return string(m.Content)
	}
	var b strings.Builder
	for _, p := range parts {
		if p.Type == "text" {
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

// format mirrors formatCompleterRequest so the wire can be diffed against
// the agent-side goldens byte for byte.
func (b wireBody) format() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "tools: %d\n", len(b.Tools))
	for i, m := range b.Messages {
		fmt.Fprintf(&sb, "\n## [%d] %s\n%s\n", i, m.Role, m.text())
	}
	return sb.String()
}

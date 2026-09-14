package provider_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/provider"
)

func TestWireMessages_OpenAIKeepsTrailingHarness(t *testing.T) {
	in := []provider.Message{
		{Role: provider.RoleSystem, Content: "You are Kit."},
		{Role: provider.RoleUser, Content: "what's near me"},
		{Role: provider.RoleSystem, Content: "[harness]\n[location] 47.6\n[current time] NOW"},
	}
	got := provider.WireMessages("gpt-5.4", in)
	if len(got) != 3 || got[2].Role != provider.RoleSystem || !strings.Contains(got[2].Content, "[harness]") {
		t.Fatalf("openai wire %+v", got)
	}
	if got[0].Content != "You are Kit." || got[1].Content != "what's near me" {
		t.Fatalf("openai mutated standing turns %+v", got)
	}
}

func TestWireMessages_GeminiFoldsHarnessIntoSystem(t *testing.T) {
	in := []provider.Message{
		{Role: provider.RoleSystem, Content: "You are Kit."},
		{Role: provider.RoleUser, Content: "old"},
		{Role: provider.RoleAssistant, Content: "prior"},
		{Role: provider.RoleSystem, Content: "[memory] a fact"},
		{Role: provider.RoleUser, Content: "what's near me"},
		{Role: provider.RoleSystem, Content: "[harness]\n[location] 47.6, -122.3\n[current time] NOW"},
		{Role: provider.RoleSystem, Content: "[system] wait note"},
	}
	got := provider.WireMessages("gemini-3.6-flash", in)
	if len(got) != 4 {
		t.Fatalf("len=%d %+v", len(got), got)
	}
	if got[0].Role != provider.RoleSystem {
		t.Fatalf("first role %s", got[0].Role)
	}
	sys := got[0].Content
	if !strings.HasPrefix(sys, "[harness]") {
		t.Fatalf("this-turn harness must lead the one Gemini system instruction:\n%s", sys)
	}
	if !strings.Contains(sys, "[location] 47.6") || !strings.Contains(sys, "[current time] NOW") {
		t.Fatalf("clock/GPS missing from system:\n%s", sys)
	}
	if !strings.Contains(sys, "You are Kit.") || !strings.Contains(sys, "[memory] a fact") || !strings.Contains(sys, "wait note") {
		t.Fatalf("standing system dropped:\n%s", sys)
	}
	if got[1].Role != provider.RoleUser || got[1].Content != "old" {
		t.Fatalf("history user %+v", got[1])
	}
	if got[2].Role != provider.RoleAssistant || got[3].Content != "what's near me" {
		t.Fatalf("later turns %+v %+v", got[2], got[3])
	}
	for i, m := range got[1:] {
		if m.Role == provider.RoleSystem {
			t.Fatalf("trailing system still on wire at %d: %+v", i+1, m)
		}
	}
}

func TestClient_Complete_GeminiWireHasOneSystem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		systems := 0
		for _, m := range body.Messages {
			if m.Role == "system" {
				systems++
				if !strings.Contains(m.Content, "[harness]") || !strings.Contains(m.Content, "You are Kit.") {
					t.Fatalf("gemini system = %q", m.Content)
				}
			}
		}
		if systems != 1 {
			t.Fatalf("gemini system count = %d want 1: %+v", systems, body.Messages)
		}
		if body.Messages[len(body.Messages)-1].Role != "user" || body.Messages[len(body.Messages)-1].Content != "what's near me" {
			t.Fatalf("last %+v", body.Messages[len(body.Messages)-1])
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

	c := provider.New(srv.URL, "k", "gemini-3.6-flash")
	if _, err := c.Complete(context.Background(), provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: "You are Kit."},
			{Role: provider.RoleUser, Content: "what's near me"},
			{Role: provider.RoleSystem, Content: "[harness]\n[current time] NOW"},
		},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestClient_Complete_OpenAIWireKeepsTrailingSystem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.Messages) != 3 || body.Messages[2].Role != "system" || !strings.Contains(body.Messages[2].Content, "[harness]") {
			t.Fatalf("openai wire %+v", body.Messages)
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

	c := provider.New(srv.URL, "k", "gpt-5.4")
	if _, err := c.Complete(context.Background(), provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: "You are Kit."},
			{Role: provider.RoleUser, Content: "what's near me"},
			{Role: provider.RoleSystem, Content: "[harness]\n[current time] NOW"},
		},
	}); err != nil {
		t.Fatal(err)
	}
}

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

func TestClient_Complete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" && !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); !strings.Contains(got, "test-key") {
			t.Errorf("Authorization = %q", got)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["model"] != "test-model" {
			t.Errorf("model = %v, want test-model", body["model"])
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-test",
			"choices": []map[string]any{
				{"index": 0, "message": map[string]any{"role": "assistant", "content": "  hello there  "}},
			},
		})
	}))
	t.Cleanup(srv.Close)

	c := provider.New(srv.URL, "test-key", "test-model")
	got, err := c.Complete(context.Background(), provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: "be brief"},
			{Role: provider.RoleUser, Content: "hi"},
			{Role: provider.RoleAssistant, Content: "prior"},
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got.Content != "hello there" {
		t.Errorf("Content = %q, want %q", got.Content, "hello there")
	}
}

func TestClient_Complete_ToolCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["tools"]; !ok {
			t.Error("expected tools in request")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-tools",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": nil,
						"tool_calls": []map[string]any{
							{
								"id":   "call_1",
								"type": "function",
								"function": map[string]any{
									"name":      "demo__echo",
									"arguments": `{"text":"hi"}`,
								},
							},
						},
					},
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	c := provider.New(srv.URL, "k", "m")
	got, err := c.Complete(context.Background(), provider.Request{
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "echo"}},
		Tools: []provider.ToolDef{{
			Name:        "demo__echo",
			Description: "echo",
			Parameters:  map[string]any{"type": "object"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ToolCalls) != 1 || got.ToolCalls[0].Name != "demo__echo" {
		t.Fatalf("%+v", got.ToolCalls)
	}
}

func TestClient_Complete_EmptyMessages(t *testing.T) {
	c := provider.New("http://example.invalid", "k", "m")
	_, err := c.Complete(context.Background(), provider.Request{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_Complete_UnknownRole(t *testing.T) {
	c := provider.New("http://example.invalid", "k", "m")
	_, err := c.Complete(context.Background(), provider.Request{
		Messages: []provider.Message{{Role: "nope", Content: "x"}},
	})
	if err == nil || !strings.Contains(err.Error(), "unknown role") {
		t.Fatalf("err = %v", err)
	}
}

func TestClient_Complete_EmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-test",
			"choices": []any{},
		})
	}))
	t.Cleanup(srv.Close)

	c := provider.New(srv.URL, "k", "m")
	_, err := c.Complete(context.Background(), provider.Request{
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
	})
	if err == nil || !strings.Contains(err.Error(), "empty choices") {
		t.Fatalf("err = %v", err)
	}
}

func TestClient_Complete_EmptyContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-test",
			"choices": []map[string]any{
				{"index": 0, "message": map[string]any{"role": "assistant", "content": "   "}},
			},
		})
	}))
	t.Cleanup(srv.Close)

	c := provider.New(srv.URL, "k", "m")
	_, err := c.Complete(context.Background(), provider.Request{
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
	})
	if err == nil || !strings.Contains(err.Error(), "empty assistant content") {
		t.Fatalf("err = %v", err)
	}
}

func TestClient_Complete_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)

	c := provider.New(srv.URL, "k", "m")
	_, err := c.Complete(context.Background(), provider.Request{
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_Complete_ToolMessageRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []map[string]any `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		foundTool := false
		var sawSkipSig bool
		for _, m := range body.Messages {
			if m["role"] == "tool" {
				foundTool = true
			}
			if m["role"] == "assistant" {
				tcs, _ := m["tool_calls"].([]any)
				for _, raw := range tcs {
					tc, _ := raw.(map[string]any)
					extra, _ := tc["extra_content"].(map[string]any)
					google, _ := extra["google"].(map[string]any)
					if google["thought_signature"] == "skip_thought_signature_validator" {
						sawSkipSig = true
					}
				}
			}
		}
		if !foundTool {
			t.Error("expected tool role message")
		}
		if !sawSkipSig {
			t.Error("expected synthesized thought_signature skip token on assistant tool_calls")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "x",
			"choices": []map[string]any{
				{"index": 0, "message": map[string]any{"role": "assistant", "content": "done"}},
			},
		})
	}))
	t.Cleanup(srv.Close)

	c := provider.New(srv.URL, "k", "m")
	got, err := c.Complete(context.Background(), provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "hi"},
			{
				Role: provider.RoleAssistant,
				ToolCalls: []provider.ToolCall{
					{ID: "c1", Name: "demo__echo", Arguments: `{}`},
				},
			},
			{Role: provider.RoleTool, Content: "ok", ToolCallID: "c1"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Content != "done" {
		t.Fatalf("%q", got.Content)
	}
}

func TestClient_Complete_PreservesThoughtSignatureRaw(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []map[string]any `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		for _, m := range body.Messages {
			if m["role"] != "assistant" {
				continue
			}
			tcs, _ := m["tool_calls"].([]any)
			if len(tcs) == 0 {
				t.Fatal("missing tool_calls")
			}
			tc, _ := tcs[0].(map[string]any)
			extra, _ := tc["extra_content"].(map[string]any)
			google, _ := extra["google"].(map[string]any)
			if google["thought_signature"] != "sig-from-model" {
				t.Fatalf("thought_signature=%v want sig-from-model", google["thought_signature"])
			}
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

	raw := json.RawMessage(`{"id":"c1","type":"function","function":{"name":"demo__echo","arguments":"{}"},"extra_content":{"google":{"thought_signature":"sig-from-model"}}}`)
	c := provider.New(srv.URL, "k", "m")
	_, err := c.Complete(context.Background(), provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "hi"},
			{
				Role: provider.RoleAssistant,
				ToolCalls: []provider.ToolCall{
					{ID: "c1", Name: "demo__echo", Arguments: `{}`, Raw: raw},
				},
			},
			{Role: provider.RoleTool, Content: "ok", ToolCallID: "c1"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

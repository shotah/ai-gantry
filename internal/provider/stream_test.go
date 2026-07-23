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

func TestClient_CompleteStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&map[string]any{})
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		chunks := []string{
			`{"id":"1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":"Hel"}}]}`,
			`{"id":"1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"lo"}}]}`,
			`{"id":"1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		}
		for _, c := range chunks {
			_, _ = w.Write([]byte("data: " + c + "\n\n"))
			if flusher != nil {
				flusher.Flush()
			}
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	t.Cleanup(srv.Close)

	c := provider.New(srv.URL, "k", "m")
	var seen []string
	got, err := c.CompleteStream(context.Background(), provider.Request{
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
	}, func(full string) error {
		seen = append(seen, full)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Content != "Hello" {
		t.Fatalf("Content=%q", got.Content)
	}
	if got.FinishReason != "stop" {
		t.Fatalf("FinishReason=%q, want stop", got.FinishReason)
	}
	if len(seen) < 2 || seen[len(seen)-1] != "Hello" {
		t.Fatalf("seen=%v", seen)
	}
}

func TestClient_CompleteStream_ToolCallsSkipText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		chunks := []string{
			`{"id":"1","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"demo__echo","arguments":""}}]}}]}`,
			`{"id":"1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"x\":1}"}}]}}]}`,
			`{"id":"1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		}
		for _, c := range chunks {
			_, _ = w.Write([]byte("data: " + c + "\n\n"))
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	t.Cleanup(srv.Close)

	c := provider.New(srv.URL, "k", "m")
	called := 0
	got, err := c.CompleteStream(context.Background(), provider.Request{
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "go"}},
		Tools:    []provider.ToolDef{{Name: "demo__echo", Parameters: map[string]any{"type": "object"}}},
	}, func(string) error {
		called++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if called != 0 {
		t.Fatalf("onText called %d times for tool-only stream", called)
	}
	if len(got.ToolCalls) != 1 || got.ToolCalls[0].Name != "demo__echo" {
		t.Fatalf("toolcalls=%+v", got.ToolCalls)
	}
	if !strings.Contains(got.ToolCalls[0].Arguments, "x") {
		t.Fatalf("args=%q", got.ToolCalls[0].Arguments)
	}
}

// Gemini often streams parallel tool calls all with index=0 but distinct ids.
func TestClient_CompleteStream_ParallelToolsSameIndex(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		chunks := []string{
			`{"id":"1","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"a1","type":"function","function":{"name":"ytmusic__search_tracks","arguments":""}}]}}]}`,
			`{"id":"1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"q\":\"x\"}"}}]}}]}`,
			`{"id":"1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"b2","type":"function","function":{"name":"cast__list_local_hardware","arguments":""}}]}}]}`,
			`{"id":"1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{}"}}]}}]}`,
			`{"id":"1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		}
		for _, c := range chunks {
			_, _ = w.Write([]byte("data: " + c + "\n\n"))
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	t.Cleanup(srv.Close)

	c := provider.New(srv.URL, "k", "m")
	got, err := c.CompleteStream(context.Background(), provider.Request{
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "play"}},
		Tools: []provider.ToolDef{
			{Name: "ytmusic__search_tracks", Parameters: map[string]any{"type": "object"}},
			{Name: "cast__list_local_hardware", Parameters: map[string]any{"type": "object"}},
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ToolCalls) != 2 {
		t.Fatalf("want 2 tool calls, got %+v", got.ToolCalls)
	}
	if got.ToolCalls[0].Name != "ytmusic__search_tracks" || got.ToolCalls[0].ID != "a1" {
		t.Fatalf("call0=%+v", got.ToolCalls[0])
	}
	if got.ToolCalls[1].Name != "cast__list_local_hardware" || got.ToolCalls[1].ID != "b2" {
		t.Fatalf("call1=%+v", got.ToolCalls[1])
	}
	if strings.Contains(got.ToolCalls[0].Name, "cast") || strings.Contains(got.ToolCalls[1].Name, "ytmusic") {
		t.Fatalf("names mashed: %+v", got.ToolCalls)
	}
}

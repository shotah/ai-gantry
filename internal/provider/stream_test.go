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

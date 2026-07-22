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
	got, err := c.Complete(context.Background(), []provider.Message{
		{Role: provider.RoleSystem, Content: "be brief"},
		{Role: provider.RoleUser, Content: "hi"},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got != "hello there" {
		t.Errorf("Complete = %q, want %q", got, "hello there")
	}
}

func TestClient_Complete_EmptyMessages(t *testing.T) {
	c := provider.New("http://example.invalid", "k", "m")
	_, err := c.Complete(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

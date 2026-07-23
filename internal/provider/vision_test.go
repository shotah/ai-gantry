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

func TestClient_Complete_UserImageParts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		msgs, _ := body["messages"].([]any)
		if len(msgs) != 1 {
			t.Fatalf("messages=%d", len(msgs))
		}
		m, _ := msgs[0].(map[string]any)
		content, ok := m["content"].([]any)
		if !ok || len(content) != 2 {
			t.Fatalf("content=%T %#v", m["content"], m["content"])
		}
		raw, _ := json.Marshal(content)
		if !strings.Contains(string(raw), "image_url") || !strings.Contains(string(raw), "data:image/png;base64,aa") {
			t.Fatalf("content=%s", raw)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "c",
			"choices": []map[string]any{
				{"index": 0, "message": map[string]any{"role": "assistant", "content": "a cat"}},
			},
		})
	}))
	t.Cleanup(srv.Close)

	c := provider.New(srv.URL, "k", "m")
	got, err := c.Complete(context.Background(), provider.Request{
		Messages: []provider.Message{{
			Role:      provider.RoleUser,
			Content:   "what is this?",
			ImageURLs: []string{"data:image/png;base64,aa"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Content != "a cat" {
		t.Fatalf("%q", got.Content)
	}
}

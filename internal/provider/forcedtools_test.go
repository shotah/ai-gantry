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

// nameEnumFrom digs the constrained name list out of a captured request body.
func nameEnumFrom(t *testing.T, body map[string]any) []any {
	t.Helper()
	rf, ok := body["response_format"].(map[string]any)
	if !ok {
		t.Fatalf("response_format missing from %v", body)
	}
	if rf["type"] != "json_schema" {
		t.Errorf("response_format type = %v, want json_schema", rf["type"])
	}
	js, _ := rf["json_schema"].(map[string]any)
	schema, _ := js["schema"].(map[string]any)
	props, _ := schema["properties"].(map[string]any)
	name, _ := props["name"].(map[string]any)
	enum, _ := name["enum"].([]any)
	return enum
}

// Ollama drops tool_calls whenever a response_format is set, so a constrained
// repair comes back as JSON in content. The client must send the name enum and
// convert that reply into an ordinary tool call, or the agent loop would treat a
// tool call as the user-facing answer.
func TestClient_Complete_ForcedToolNames(t *testing.T) {
	var (
		enum      []any
		sentTools bool
		streamed  any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		enum = nameEnumFrom(t, body)
		_, sentTools = body["tools"]
		streamed = body["stream"]

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-test",
			"choices": []map[string]any{{
				"index":         0,
				"finish_reason": "stop",
				"message": map[string]any{
					"role":    "assistant",
					"content": `{"name":"garmin__hrv_get","arguments":{"date":"2026-07-27"}}`,
				},
			}},
		})
	}))
	t.Cleanup(srv.Close)

	names := []string{"garmin__wellness_get_body_battery", "garmin__hrv_get"}
	c := provider.New(srv.URL, "test-key", "test-model")
	got, err := c.Complete(context.Background(), provider.Request{
		Messages:       []provider.Message{{Role: provider.RoleUser, Content: "hrv yesterday?"}},
		Tools:          []provider.ToolDef{{Name: "garmin__hrv_get", Parameters: map[string]any{"type": "object"}}},
		ForceToolNames: names,
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if len(enum) != len(names) || enum[0] != names[0] || enum[1] != names[1] {
		t.Errorf("name enum = %v, want %v", enum, names)
	}
	// Tools must still be sent: the model reads parameter schemas from there, and
	// without them it invents argument names.
	if !sentTools {
		t.Error("tools omitted alongside response_format; arguments become guesswork")
	}
	if streamed == true {
		t.Error("stream = true; a constrained reply is JSON, not prose")
	}

	if len(got.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want the content JSON converted to one call", len(got.ToolCalls))
	}
	call := got.ToolCalls[0]
	if call.Name != "garmin__hrv_get" {
		t.Errorf("name = %q", call.Name)
	}
	if !strings.Contains(call.Arguments, "2026-07-27") {
		t.Errorf("arguments = %q, want the date preserved", call.Arguments)
	}
	if call.ID == "" {
		t.Error("id empty; the tool result could not be matched to the call")
	}
	if got.Content != "" {
		t.Errorf("content = %q, want it consumed by the tool call", got.Content)
	}
	if got.FinishReason != "tool_calls" {
		t.Errorf("finish_reason = %q, want tool_calls", got.FinishReason)
	}
}

// Streaming a grammar-constrained reply would type raw JSON into the user's
// bubble, so the streaming path must degrade to a single blocking call.
func TestClient_CompleteStream_ForcedToolNamesDoesNotStream(t *testing.T) {
	var streamed any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		streamed = body["stream"]
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-test",
			"choices": []map[string]any{{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": `{"name":"garmin__hrv_get","arguments":{}}`,
				},
			}},
		})
	}))
	t.Cleanup(srv.Close)

	progress := 0
	c := provider.New(srv.URL, "test-key", "test-model")
	got, err := c.CompleteStream(context.Background(), provider.Request{
		Messages:       []provider.Message{{Role: provider.RoleUser, Content: "hrv?"}},
		ForceToolNames: []string{"garmin__hrv_get"},
	}, func(string, string) error {
		progress++
		return nil
	})
	if err != nil {
		t.Fatalf("CompleteStream: %v", err)
	}
	if streamed == true {
		t.Error("stream = true, want the constrained call to go non-streaming")
	}
	if progress != 0 {
		t.Errorf("onProgress called %d times, want none (JSON must not reach the reply)", progress)
	}
	if len(got.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(got.ToolCalls))
	}
}

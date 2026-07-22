package mcp

import (
	"bytes"
	"log/slog"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSchemaToMap(t *testing.T) {
	m, err := schemaToMap(nil)
	if err != nil || m["type"] != "object" {
		t.Fatalf("%v %#v", err, m)
	}
	in := map[string]any{"type": "object", "properties": map[string]any{}}
	m, err = schemaToMap(in)
	if err != nil || m["type"] != "object" {
		t.Fatal(err)
	}
	type sch struct {
		Type string `json:"type"`
	}
	m, err = schemaToMap(sch{Type: "string"})
	if err != nil || m["type"] != "string" {
		t.Fatalf("%v %#v", err, m)
	}
}

func TestContentToString(t *testing.T) {
	if contentToString(nil) != "" {
		t.Fatal("nil")
	}
	res := &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "hello"}},
	}
	if contentToString(res) != "hello" {
		t.Fatalf("got %q", contentToString(res))
	}
	res = &mcpsdk.CallToolResult{StructuredContent: map[string]any{"ok": true}}
	if contentToString(res) == "" {
		t.Fatal("expected structured json")
	}
}

func TestLineLoggerWrite(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	l := newLineLogger(log, "demo")
	n, err := l.Write([]byte("one\n\ntwo\n"))
	if err != nil || n == 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if buf.Len() == 0 {
		t.Fatal("expected log output")
	}
}

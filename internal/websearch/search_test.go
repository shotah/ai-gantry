package websearch

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpen_RequiresKeys(t *testing.T) {
	if _, err := Open(Options{EngineID: "cx"}); err == nil || !strings.Contains(err.Error(), "GOOGLE_PSE_API_KEY") {
		t.Fatalf("missing key: %v", err)
	}
	if _, err := Open(Options{APIKey: "k"}); err == nil || !strings.Contains(err.Error(), "GOOGLE_PSE_ENGINE_ID") {
		t.Fatalf("missing engine: %v", err)
	}
}

func TestIsReplacedMCP(t *testing.T) {
	if !IsReplacedMCP("google-search", "unused") || !IsReplacedMCP("x", "mcp-gemini-google-search") {
		t.Fatal("want replaced")
	}
	if IsReplacedMCP("google", "google-mcp") || IsReplacedMCP("math", "mcp-go-math") {
		t.Fatal("workspace/math are not search")
	}
}

func TestIsSearchTool(t *testing.T) {
	for _, name := range []string{ToolName, "google_search", "google-search__web_search", "google_search__web_search"} {
		if !IsSearchTool(name) {
			t.Fatalf("%q should be a search tool", name)
		}
	}
	if IsSearchTool("google__search_query") || IsSearchTool("memory_store") {
		t.Fatal("other tools must not match")
	}
}

func TestSearch_FormatsHits(t *testing.T) {
	var gotQ, gotKey, gotCx, gotNum string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQ = r.URL.Query().Get("q")
		gotKey = r.URL.Query().Get("key")
		gotCx = r.URL.Query().Get("cx")
		gotNum = r.URL.Query().Get("num")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"items": [
				{"title": "Go", "link": "https://go.dev", "snippet": "The Go programming language"},
				{"title": "Tour", "link": "https://go.dev/tour", "snippet": "A tour of Go"}
			]
		}`)
	}))
	t.Cleanup(srv.Close)

	tools, err := Open(Options{APIKey: "k", EngineID: "cx", Endpoint: srv.URL, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	out, err := tools.Call(context.Background(), ToolName, []byte(`{"query":"golang"}`))
	if err != nil {
		t.Fatal(err)
	}
	if gotQ != "golang" || gotKey != "k" || gotCx != "cx" || gotNum != "8" {
		t.Fatalf("query = q=%q key=%q cx=%q num=%q", gotQ, gotKey, gotCx, gotNum)
	}
	for _, want := range []string{
		`Google results for "golang"`,
		"1. Go",
		"https://go.dev",
		"The Go programming language",
		"2. Tour",
		"https://go.dev/tour",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestSearch_AliasAndEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[]}`)
	}))
	t.Cleanup(srv.Close)
	tools, err := Open(Options{APIKey: "k", EngineID: "cx", Endpoint: srv.URL, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	out, err := tools.Call(context.Background(), "google-search__web_search", []byte(`{"query":"nope"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out != `No results for "nope"` {
		t.Fatalf("out = %q", out)
	}
	if _, err := tools.Call(context.Background(), ToolName, []byte(`{"query":"  "}`)); err == nil || !strings.Contains(err.Error(), "query is required") {
		t.Fatalf("empty query: %v", err)
	}
}

func TestSearch_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":{"code":403,"message":"Daily Limit Exceeded"}}`)
	}))
	t.Cleanup(srv.Close)
	tools, err := Open(Options{APIKey: "k", EngineID: "cx", Endpoint: srv.URL, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = tools.Call(context.Background(), ToolName, []byte(`{"query":"x"}`))
	if err == nil || !strings.Contains(err.Error(), "Daily Limit Exceeded") {
		t.Fatalf("err = %v", err)
	}
}

func TestSearch_Unconfigured(t *testing.T) {
	var t0 Tools
	if _, err := t0.Call(context.Background(), ToolName, []byte(`{"query":"x"}`)); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("err = %v", err)
	}
}

func TestFormatResults(t *testing.T) {
	got := formatResults("q", []customSearchItem{{Title: "", Link: "", Snippet: "only snippet"}})
	if !strings.Contains(got, "(no title)") || !strings.Contains(got, "only snippet") {
		t.Fatalf("got %q", got)
	}
}

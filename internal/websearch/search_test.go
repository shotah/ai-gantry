package websearch

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpen_RequiresKey(t *testing.T) {
	if _, err := Open(Options{}); err == nil || !strings.Contains(err.Error(), "BRAVE_SEARCH_API_KEY") {
		t.Fatalf("missing key: %v", err)
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
	var gotQ, gotCount, gotToken, gotQueryKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQ = r.URL.Query().Get("q")
		gotCount = r.URL.Query().Get("count")
		gotToken = r.Header.Get(tokenHeader)
		gotQueryKey = r.URL.Query().Get("key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"web": {
				"results": [
					{"title": "Go", "url": "https://go.dev", "description": "The Go programming language"},
					{"title": "Tour", "url": "https://go.dev/tour", "description": "A tour of Go"}
				]
			}
		}`)
	}))
	t.Cleanup(srv.Close)

	tools, err := Open(Options{APIKey: "k", Endpoint: srv.URL, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	out, err := tools.Call(context.Background(), ToolName, []byte(`{"query":"golang"}`))
	if err != nil {
		t.Fatal(err)
	}
	if gotQ != "golang" || gotCount != "8" || gotToken != "k" || gotQueryKey != "" {
		t.Fatalf("query = q=%q count=%q token=%q key=%q", gotQ, gotCount, gotToken, gotQueryKey)
	}
	for _, want := range []string{
		`Search results for "golang"`,
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
		_, _ = io.WriteString(w, `{"web":{"results":[]}}`)
	}))
	t.Cleanup(srv.Close)
	tools, err := Open(Options{APIKey: "k", Endpoint: srv.URL, HTTPClient: srv.Client()})
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
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"status":401,"detail":"Invalid subscription token"}}`)
	}))
	t.Cleanup(srv.Close)
	tools, err := Open(Options{APIKey: "k", Endpoint: srv.URL, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = tools.Call(context.Background(), ToolName, []byte(`{"query":"x"}`))
	if err == nil || !strings.Contains(err.Error(), "Invalid subscription token") {
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
	got := formatResults("q", []braveWebResult{{Title: "", URL: "", Description: "only snippet"}})
	if !strings.Contains(got, "(no title)") || !strings.Contains(got, "only snippet") {
		t.Fatalf("got %q", got)
	}
}

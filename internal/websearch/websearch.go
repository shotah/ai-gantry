// Package websearch is the harness builtin for Google web search.
//
// Search is a first-class agent capability (like memory and cron), not an MCP
// grant. The model calls unprefixed web_search; gantry GETs Google Custom
// Search and returns titles, URLs, and snippets. No second model, no Gemini
// grounding.
package websearch

import (
	"errors"
	"net/http"
	"strings"
	"time"
)

// ToolName is the builtin exposed to the model (not MCP-prefixed).
const ToolName = "web_search"

const (
	defaultEndpoint = "https://www.googleapis.com/customsearch/v1"
	defaultNum      = 8
	httpTimeout     = 15 * time.Second
)

// Options is the Custom Search client (GOOGLE_PSE_API_KEY + GOOGLE_PSE_ENGINE_ID).
type Options struct {
	APIKey     string
	EngineID   string
	Endpoint   string       // tests; default Google Custom Search JSON API
	HTTPClient *http.Client // tests
}

type googleSearchService struct {
	apiKey     string
	engineID   string
	endpoint   string
	httpClient *http.Client
}

func errNeedAPIKey() error {
	return errors.New("GOOGLE_PSE_API_KEY is required for web_search")
}

func errNeedEngineID() error {
	return errors.New("GOOGLE_PSE_ENGINE_ID is required for web_search")
}

// Open builds an in-process Custom Search client. The HTTP call happens on Search.
func Open(opts Options) (Tools, error) {
	apiKey := strings.TrimSpace(opts.APIKey)
	engineID := strings.TrimSpace(opts.EngineID)
	if apiKey == "" {
		return Tools{}, errNeedAPIKey()
	}
	if engineID == "" {
		return Tools{}, errNeedEngineID()
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: httpTimeout}
	}
	endpoint := strings.TrimSpace(opts.Endpoint)
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	return Tools{
		svc: &googleSearchService{
			apiKey:     apiKey,
			engineID:   engineID,
			endpoint:   endpoint,
			httpClient: client,
		},
	}, nil
}

// IsReplacedMCP reports leftover google-search MCP grants that the builtin replaces.
func IsReplacedMCP(name, command string) bool {
	n := strings.TrimSpace(name)
	c := strings.TrimSpace(command)
	return n == "google-search" || c == "mcp-gemini-google-search"
}

// IsSearchTool reports the builtin name and leftover MCP / invented aliases.
func IsSearchTool(name string) bool {
	switch strings.TrimSpace(name) {
	case ToolName, "google_search", "google-search__web_search", "google_search__web_search":
		return true
	default:
		return false
	}
}

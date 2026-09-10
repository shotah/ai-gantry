package websearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type customSearchResponse struct {
	Items []customSearchItem `json:"items"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type customSearchItem struct {
	Title   string `json:"title"`
	Link    string `json:"link"`
	Snippet string `json:"snippet"`
}

func (s *googleSearchService) Search(ctx context.Context, query string) (string, error) {
	if s == nil || s.httpClient == nil {
		return "", errors.New("websearch: service is not configured")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return "", errors.New("query is required")
	}

	endpoint, err := url.Parse(s.endpoint)
	if err != nil {
		return "", fmt.Errorf("websearch: endpoint: %w", err)
	}
	q := endpoint.Query()
	q.Set("key", s.apiKey)
	q.Set("cx", s.engineID)
	q.Set("q", query)
	q.Set("num", strconv.Itoa(defaultNum))
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return "", fmt.Errorf("websearch: request: %w", err)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("websearch: google search failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("websearch: read body: %w", err)
	}

	var parsed customSearchResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("websearch: google search failed (%s): %s", resp.Status, clipBody(body))
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return "", fmt.Errorf("websearch: google search failed: %s", parsed.Error.Message)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("websearch: google search failed (%s): %s", resp.Status, clipBody(body))
	}
	return formatResults(query, parsed.Items), nil
}

func formatResults(query string, items []customSearchItem) string {
	if len(items) == 0 {
		return fmt.Sprintf("No results for %q", query)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Google results for %q:\n", query)
	for i, item := range items {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = "(no title)"
		}
		link := strings.TrimSpace(item.Link)
		snippet := strings.ReplaceAll(strings.TrimSpace(item.Snippet), "\n", " ")
		fmt.Fprintf(&b, "\n%d. %s\n", i+1, title)
		if link != "" {
			fmt.Fprintf(&b, "   %s\n", link)
		}
		if snippet != "" {
			fmt.Fprintf(&b, "   %s\n", snippet)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func clipBody(body []byte) string {
	s := strings.TrimSpace(string(body))
	if s == "" {
		return "empty body"
	}
	if len(s) > 240 {
		return s[:240]
	}
	return s
}

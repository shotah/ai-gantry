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

type braveSearchResponse struct {
	Web   *braveWebResults `json:"web"`
	Error *struct {
		Detail string `json:"detail"`
		Status int    `json:"status"`
	} `json:"error"`
	Message string `json:"message"`
}

type braveWebResults struct {
	Results []braveWebResult `json:"results"`
}

type braveWebResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

func (s *searchService) Search(ctx context.Context, query string) (string, error) {
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
	q.Set("q", query)
	q.Set("count", strconv.Itoa(defaultNum))
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return "", fmt.Errorf("websearch: request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set(tokenHeader, s.apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("websearch: brave search failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("websearch: read body: %w", err)
	}

	var parsed braveSearchResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("websearch: brave search failed (%s): %s", resp.Status, clipBody(body))
	}
	if msg := braveErrorMessage(parsed); msg != "" {
		return "", fmt.Errorf("websearch: brave search failed: %s", msg)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("websearch: brave search failed (%s): %s", resp.Status, clipBody(body))
	}
	var items []braveWebResult
	if parsed.Web != nil {
		items = parsed.Web.Results
	}
	return formatResults(query, items), nil
}

func braveErrorMessage(parsed braveSearchResponse) string {
	if parsed.Error != nil && strings.TrimSpace(parsed.Error.Detail) != "" {
		return strings.TrimSpace(parsed.Error.Detail)
	}
	return strings.TrimSpace(parsed.Message)
}

func formatResults(query string, items []braveWebResult) string {
	if len(items) == 0 {
		return fmt.Sprintf("No results for %q", query)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Search results for %q:\n", query)
	for i, item := range items {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = "(no title)"
		}
		link := strings.TrimSpace(item.URL)
		snippet := strings.ReplaceAll(strings.TrimSpace(item.Description), "\n", " ")
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

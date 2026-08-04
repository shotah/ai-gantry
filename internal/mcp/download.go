package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"time"
)

// DownloadConfigured reports whether this server declares a binary URL for
// `gantry tools-fetch`. Ignored by the runtime host.
func (s ServerSpec) DownloadConfigured() bool {
	return strings.TrimSpace(s.DownloadURL) != ""
}

// LatestTagFunc resolves a GitHub "latest" release tag for owner/repo.
type LatestTagFunc func(owner, repo string) (tag string, err error)

// ResolveDownload expands download_url placeholders and returns the final URL
// plus a suggested archive filename (last path segment).
//
// Placeholders: {os} {arch} (and {GOOS} {GOARCH} aliases), and when
// download_tag is set: {tag} {version} (version = tag without leading "v").
// download_tag may be "latest" (resolved via GitHub API at plan time).
//
// Source-agnostic URL host (GitHub, GitLab, S3, …). "latest" only works for
// github.com owner/repo URLs.
func (s ServerSpec) ResolveDownload(goos, goarch string) (downloadURL, filename string, err error) {
	u, file, _, err := s.ResolveDownloadLookup(goos, goarch, nil)
	return u, file, err
}

// ResolveDownloadLookup is ResolveDownload with an optional latest-tag lookup.
// resolvedTag is the concrete tag after expanding "latest" (empty if unused).
func (s ServerSpec) ResolveDownloadLookup(goos, goarch string, lookup LatestTagFunc) (downloadURL, filename, resolvedTag string, err error) {
	raw := strings.TrimSpace(s.DownloadURL)
	if raw == "" {
		return "", "", "", fmt.Errorf("mcp: server %q: download_url is empty", s.Name)
	}
	goos = strings.TrimSpace(goos)
	goarch = strings.TrimSpace(goarch)
	if goos == "" {
		goos = "linux"
	}
	if goarch == "" {
		goarch = "amd64"
	}

	tag := strings.TrimSpace(s.DownloadTag)
	needsTag := strings.Contains(raw, "{tag}") || strings.Contains(raw, "{version}")
	if needsTag && tag == "" {
		return "", "", "", fmt.Errorf("mcp: server %q: download_url uses {tag}/{version} but download_tag is empty", s.Name)
	}
	if strings.EqualFold(tag, "latest") {
		owner, repo, ok := githubOwnerRepo(raw)
		if !ok {
			return "", "", "", fmt.Errorf("mcp: server %q: download_tag=latest requires a github.com/<owner>/<repo>/… download_url", s.Name)
		}
		if lookup == nil {
			lookup = GitHubLatestTag
		}
		resolved, lerr := lookup(owner, repo)
		if lerr != nil {
			return "", "", "", fmt.Errorf("mcp: server %q: resolve latest: %w", s.Name, lerr)
		}
		tag = strings.TrimSpace(resolved)
		if tag == "" {
			return "", "", "", fmt.Errorf("mcp: server %q: latest tag empty for %s/%s", s.Name, owner, repo)
		}
	}

	expanded := expandDownloadURL(raw, goos, goarch, tag)
	u, err := url.Parse(expanded)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", "", "", fmt.Errorf("mcp: server %q: download_url is not a valid absolute URL: %q", s.Name, expanded)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return "", "", "", fmt.Errorf("mcp: server %q: download_url scheme must be http(s), got %q", s.Name, u.Scheme)
	}
	filename = path.Base(u.Path)
	if filename == "" || filename == "." || filename == "/" {
		filename = path.Base(strings.TrimSpace(s.Command)) + ".tar.gz"
	}
	return expanded, filename, tag, nil
}

func expandDownloadURL(raw, goos, goarch, tag string) string {
	version := strings.TrimPrefix(tag, "v")
	r := strings.NewReplacer(
		"{os}", goos,
		"{arch}", goarch,
		"{GOOS}", goos,
		"{GOARCH}", goarch,
		"{tag}", tag,
		"{version}", version,
	)
	return r.Replace(raw)
}

var githubRepoRE = regexp.MustCompile(`(?i)^https?://github\.com/([^/]+)/([^/]+)/`)

func githubOwnerRepo(rawURL string) (owner, repo string, ok bool) {
	m := githubRepoRE.FindStringSubmatch(strings.TrimSpace(rawURL))
	if len(m) != 3 {
		return "", "", false
	}
	owner, repo = m[1], m[2]
	repo = strings.TrimSuffix(repo, ".git")
	if owner == "" || repo == "" {
		return "", "", false
	}
	return owner, repo, true
}

// GitHubLatestTag fetches the latest release tag from the GitHub API.
// Uses GITHUB_TOKEN when set (higher rate limits).
func GitHubLatestTag(owner, repo string) (string, error) {
	owner = strings.TrimSpace(owner)
	repo = strings.TrimSpace(repo)
	if owner == "" || repo == "" {
		return "", fmt.Errorf("github: owner and repo required")
	}
	api := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)
	client := &http.Client{Timeout: 20 * time.Second}
	req, err := http.NewRequest(http.MethodGet, api, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ai-gantry")
	if tok := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github API %s: %s: %s", api, resp.Status, strings.TrimSpace(string(body)))
	}
	var parsed struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("github API decode: %w", err)
	}
	if strings.TrimSpace(parsed.TagName) == "" {
		return "", fmt.Errorf("github API: empty tag_name for %s/%s", owner, repo)
	}
	return parsed.TagName, nil
}

// DownloadPlan is one fetchable binary declared via download_url in mcp.toml.
type DownloadPlan struct {
	Name     string `json:"name"`
	Command  string `json:"command"`
	URL      string `json:"url"`
	Filename string `json:"filename"`
	Tag      string `json:"tag,omitempty"` // concrete download_tag after latest resolve
}

// DownloadPlans returns fetch plans for every server with download_url set.
// download_tag=latest is resolved via GitHubLatestTag.
func DownloadPlans(m *Manifest, goos, goarch string) ([]DownloadPlan, error) {
	return DownloadPlansLookup(m, goos, goarch, GitHubLatestTag)
}

// DownloadPlansLookup is DownloadPlans with a custom latest-tag lookup (tests).
func DownloadPlansLookup(m *Manifest, goos, goarch string, lookup LatestTagFunc) ([]DownloadPlan, error) {
	if m == nil {
		return nil, nil
	}
	out := make([]DownloadPlan, 0)
	for _, s := range m.Servers {
		if !s.DownloadConfigured() {
			continue
		}
		u, file, tag, err := s.ResolveDownloadLookup(goos, goarch, lookup)
		if err != nil {
			return nil, err
		}
		cmd := path.Base(strings.TrimSpace(s.Command))
		out = append(out, DownloadPlan{
			Name:     s.Name,
			Command:  cmd,
			URL:      u,
			Filename: file,
			Tag:      tag,
		})
	}
	return out, nil
}

// Commands returns every server command basename (for docker-cp inventories).
func Commands(m *Manifest) []string {
	if m == nil {
		return nil
	}
	out := make([]string, 0, len(m.Servers))
	seen := make(map[string]struct{}, len(m.Servers))
	for _, s := range m.Servers {
		cmd := path.Base(strings.TrimSpace(s.Command))
		if cmd == "" || cmd == "." {
			continue
		}
		if _, ok := seen[cmd]; ok {
			continue
		}
		seen[cmd] = struct{}{}
		out = append(out, cmd)
	}
	return out
}

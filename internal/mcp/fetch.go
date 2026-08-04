package mcp

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FetchOptions controls InstallDownloads / FetchDownloads.
type FetchOptions struct {
	// OutDir receives extracted command binaries (required).
	OutDir string
	// CacheDir stores versioned archives. Defaults to <OutDir>/../.cache when empty
	// is awkward; callers should set it. If empty, uses <OutDir>/.download-cache.
	CacheDir string
	// GOOS / GOARCH for URL placeholders (default linux/amd64).
	GOOS   string
	GOARCH string
	// Force re-downloads and re-extracts even when archive + binary exist.
	Force bool
	// Prune removes binaries in OutDir that are not declared in the manifest.
	Prune bool
	// Lookup resolves download_tag=latest (default GitHubLatestTag).
	Lookup LatestTagFunc
	// HTTPClient downloads archives (default 2m timeout client).
	HTTPClient *http.Client
	// Logf receives progress lines (optional).
	Logf func(format string, args ...any)
}

// FetchResult summarizes one tools-fetch run.
type FetchResult struct {
	Plans     []DownloadPlan
	Installed []string // command basenames newly written
	Skipped   []string // already up to date
}

func (o FetchOptions) logf(format string, args ...any) {
	if o.Logf != nil {
		o.Logf(format, args...)
	}
}

func (o *FetchOptions) normalize() error {
	o.OutDir = strings.TrimSpace(o.OutDir)
	if o.OutDir == "" {
		return fmt.Errorf("mcp: fetch: --outdir is required")
	}
	if strings.TrimSpace(o.CacheDir) == "" {
		o.CacheDir = filepath.Join(o.OutDir, ".download-cache")
	}
	if strings.TrimSpace(o.GOOS) == "" {
		o.GOOS = "linux"
	}
	if strings.TrimSpace(o.GOARCH) == "" {
		o.GOARCH = "amd64"
	}
	if o.Lookup == nil {
		o.Lookup = GitHubLatestTag
	}
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{Timeout: 2 * time.Minute}
	}
	return nil
}

// FetchDownloads resolves mcp.toml download_* entries and installs binaries into OutDir.
// Skips a tool when the versioned archive and extracted binary are already present
// (download_tag=latest refreshes when the resolved filename/tag changes).
func FetchDownloads(m *Manifest, opt FetchOptions) (*FetchResult, error) {
	if err := opt.normalize(); err != nil {
		return nil, err
	}
	plans, err := DownloadPlansLookup(m, opt.GOOS, opt.GOARCH, opt.Lookup)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(opt.OutDir, 0o755); err != nil {
		return nil, fmt.Errorf("mcp: fetch: mkdir outdir: %w", err)
	}
	if err := os.MkdirAll(opt.CacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("mcp: fetch: mkdir cache: %w", err)
	}

	res := &FetchResult{Plans: plans}
	for _, p := range plans {
		tarball := filepath.Join(opt.CacheDir, p.Filename)
		dest := filepath.Join(opt.OutDir, p.Command)
		if !opt.Force && fileExists(tarball) && fileExists(dest) {
			opt.logf("skip %s: already have %s -> %s", p.Name, p.Filename, dest)
			res.Skipped = append(res.Skipped, p.Command)
			continue
		}
		if opt.Force || !fileExists(tarball) {
			opt.logf("fetch %s (%s) from %s", p.Name, p.Command, p.URL)
			if err := downloadFile(opt.HTTPClient, p.URL, tarball); err != nil {
				return nil, fmt.Errorf("mcp: fetch %s: %w", p.Name, err)
			}
		} else {
			opt.logf("reuse cached archive %s for %s", p.Filename, p.Name)
		}
		if err := extractCommandFromTarGz(tarball, p.Command, dest); err != nil {
			return nil, fmt.Errorf("mcp: fetch %s: %w", p.Name, err)
		}
		if err := os.Chmod(dest, 0o755); err != nil {
			return nil, fmt.Errorf("mcp: fetch %s: chmod: %w", p.Name, err)
		}
		opt.logf("installed %s -> %s", p.Command, dest)
		res.Installed = append(res.Installed, p.Command)
	}

	if opt.Prune {
		wanted := Commands(m)
		if err := pruneOutDir(opt.OutDir, wanted, opt.Logf); err != nil {
			return nil, err
		}
	}
	return res, nil
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func downloadFile(client *http.Client, url, dest string) error {
	tmp := dest + ".partial"
	_ = os.Remove(tmp)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "ai-gantry")
	if tok := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("GET %s: %s: %s", url, resp.Status, strings.TrimSpace(string(body)))
	}
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// extractCommandFromTarGz finds command (basename) in a gzip tar and writes it to dest.
func extractCommandFromTarGz(archive, command, dest string) error {
	command = filepath.Base(strings.TrimSpace(command))
	if command == "" || command == "." {
		return fmt.Errorf("empty command name")
	}
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	var found bool
	tmp := dest + ".partial"
	_ = os.Remove(tmp)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// TypeReg ('0') or historic empty typeflag (0) for regular files.
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != 0 {
			continue
		}
		name := filepath.ToSlash(hdr.Name)
		name = strings.TrimPrefix(name, "./")
		if strings.Contains(name, "..") {
			return fmt.Errorf("refusing archive path %q", hdr.Name)
		}
		base := pathBaseSlash(name)
		if base != command {
			continue
		}
		out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		n, copyErr := io.Copy(out, io.LimitReader(tr, hdr.Size))
		closeErr := out.Close()
		if copyErr != nil {
			_ = os.Remove(tmp)
			return copyErr
		}
		if closeErr != nil {
			_ = os.Remove(tmp)
			return closeErr
		}
		if hdr.Size > 0 && n != hdr.Size {
			_ = os.Remove(tmp)
			return fmt.Errorf("short read for %s: %d/%d", command, n, hdr.Size)
		}
		if err := os.Rename(tmp, dest); err != nil {
			_ = os.Remove(tmp)
			return err
		}
		found = true
		break
	}
	if !found {
		return fmt.Errorf("archive missing binary %q", command)
	}
	return nil
}

func pathBaseSlash(name string) string {
	name = strings.TrimSuffix(name, "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		return name[i+1:]
	}
	return name
}

func pruneOutDir(outDir string, wanted []string, logf func(string, ...any)) error {
	keep := make(map[string]struct{}, len(wanted))
	for _, w := range wanted {
		keep[w] = struct{}{}
	}
	entries, err := os.ReadDir(outDir)
	if err != nil {
		return fmt.Errorf("mcp: prune: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if _, ok := keep[name]; ok {
			continue
		}
		path := filepath.Join(outDir, name)
		if logf != nil {
			logf("pruning stale %s (not in mcp.toml)", path)
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("mcp: prune %s: %w", path, err)
		}
	}
	return nil
}

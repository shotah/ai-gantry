package mcp_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/shotah/ai-gantry/internal/mcp"
)

func TestFetchDownloads_InstallAndSkip(t *testing.T) {
	archive := makeTarGz(t, "mcp-go-math", []byte("#!/bin/true\n"))
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(archive)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	manifest := filepath.Join(dir, "mcp.toml")
	content := `
[[server]]
name = "math"
command = "mcp-go-math"
download_url = "` + srv.URL + `/mcp-go-math_0.0.1_linux_amd64.tar.gz"
`
	if err := os.WriteFile(manifest, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := mcp.LoadManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(dir, "bin")
	cache := filepath.Join(dir, "cache")
	res, err := mcp.FetchDownloads(m, mcp.FetchOptions{
		OutDir:     out,
		CacheDir:   cache,
		GOOS:       "linux",
		GOARCH:     "amd64",
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Fatalf("hits=%d want 1", hits)
	}
	if len(res.Installed) != 1 || res.Installed[0] != "mcp-go-math" {
		t.Fatalf("installed=%v", res.Installed)
	}
	dest := filepath.Join(out, "mcp-go-math")
	body, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "#!/bin/true\n" {
		t.Fatalf("body=%q", body)
	}

	res2, err := mcp.FetchDownloads(m, mcp.FetchOptions{
		OutDir:     out,
		CacheDir:   cache,
		GOOS:       "linux",
		GOARCH:     "amd64",
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Fatalf("second fetch should skip download; hits=%d", hits)
	}
	if len(res2.Skipped) != 1 || len(res2.Installed) != 0 {
		t.Fatalf("res2 installed=%v skipped=%v", res2.Installed, res2.Skipped)
	}
}

func TestFetchDownloads_Prune(t *testing.T) {
	archive := makeTarGz(t, "keep-me", []byte("ok"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	out := filepath.Join(dir, "bin")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(out, "old-tool")
	if err := os.WriteFile(stale, []byte("stale"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, "mcp.toml")
	content := `
[[server]]
name = "keep"
command = "keep-me"
download_url = "` + srv.URL + `/keep-me.tar.gz"
`
	if err := os.WriteFile(manifest, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := mcp.LoadManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	_, err = mcp.FetchDownloads(m, mcp.FetchOptions{
		OutDir:     out,
		CacheDir:   filepath.Join(dir, "cache"),
		Prune:      true,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale tool should be pruned: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "keep-me")); err != nil {
		t.Fatal(err)
	}
}

func makeTarGz(t *testing.T, name string, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{
		Name: name,
		Mode: 0o755,
		Size: int64(len(payload)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

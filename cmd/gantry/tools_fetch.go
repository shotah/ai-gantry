package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/shotah/ai-gantry/internal/mcp"
)

// toolsFetchCmd resolves mcp.toml download_* and installs binaries.
//
//	gantry tools-fetch --outdir DIR [--manifest path] [--os linux] [--arch amd64]
//	                   [--cache DIR] [--force] [--prune]
//
// Skips tools whose versioned archive + binary are already present. When
// download_tag=latest, a newer GitHub release changes the archive name and
// triggers a re-download. Optional GITHUB_TOKEN raises API rate limits.
func toolsFetchCmd() int {
	manifest := strings.TrimSpace(os.Getenv("MCP_MANIFEST"))
	if manifest == "" {
		manifest = "mcp.toml"
	}
	goos, goarch := runtime.GOOS, runtime.GOARCH
	outdir := ""
	cache := ""
	force := false
	prune := false

	args := os.Args[2:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--manifest":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "tools-fetch: --manifest needs a path")
				return 2
			}
			manifest = args[i]
		case "--os":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "tools-fetch: --os needs a value")
				return 2
			}
			goos = args[i]
		case "--arch":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "tools-fetch: --arch needs a value")
				return 2
			}
			goarch = args[i]
		case "--outdir":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "tools-fetch: --outdir needs a path")
				return 2
			}
			outdir = args[i]
		case "--cache":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "tools-fetch: --cache needs a path")
				return 2
			}
			cache = args[i]
		case "--force":
			force = true
		case "--prune":
			prune = true
		case "-h", "--help":
			fmt.Fprint(os.Stderr, `gantry tools-fetch — download + install MCP binaries from mcp.toml

Usage:
  gantry tools-fetch --outdir DIR [flags]

Flags:
  --manifest path   MCP manifest (default: MCP_MANIFEST or mcp.toml)
  --outdir DIR      directory for extracted command binaries (required)
  --cache DIR       archive cache (default: <outdir>/.download-cache)
  --os name         GOOS for URL placeholders (default: runtime)
  --arch name       GOARCH for URL placeholders (default: runtime)
  --force           re-download / re-extract even when cached
  --prune           remove outdir binaries not listed in mcp.toml

Env:
  MCP_MANIFEST    default manifest path
  GITHUB_TOKEN    optional; higher GitHub API rate limits for download_tag=latest
`)
			return 0
		default:
			fmt.Fprintf(os.Stderr, "tools-fetch: unknown arg %q\n", args[i])
			return 2
		}
	}

	if strings.TrimSpace(outdir) == "" {
		fmt.Fprintln(os.Stderr, "tools-fetch: --outdir is required")
		return 2
	}

	m, err := mcp.LoadManifest(manifest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tools-fetch: %v\n", err)
		return 1
	}
	res, err := mcp.FetchDownloads(m, mcp.FetchOptions{
		OutDir:   outdir,
		CacheDir: cache,
		GOOS:     goos,
		GOARCH:   goarch,
		Force:    force,
		Prune:    prune,
		Logf: func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, "tools-fetch: "+format+"\n", args...)
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "tools-fetch: %v\n", err)
		return 1
	}
	if res == nil || len(res.Plans) == 0 {
		fmt.Fprintln(os.Stderr, "tools-fetch: no download_url servers in manifest")
		return 0
	}
	fmt.Fprintf(os.Stderr, "tools-fetch: done installed=%d skipped=%d total=%d\n",
		len(res.Installed), len(res.Skipped), len(res.Plans))
	return 0
}

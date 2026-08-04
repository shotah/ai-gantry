package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/shotah/ai-gantry/internal/mcp"
)

// toolsPlanCmd prints a JSON plan of MCP binaries from mcp.toml (inspect / CI).
// Prefer `gantry tools-fetch` to download and install.
//
//	gantry tools-plan [--manifest path] [--os linux] [--arch amd64]
//
// Output:
//
//	{"commands":["…"],"downloads":[{"name","command","url","filename"},…]}
//
// Servers with download_url are listed under downloads.
// download_tag=latest is resolved via the GitHub API (optional GITHUB_TOKEN).
func toolsPlanCmd() int {
	manifest := strings.TrimSpace(os.Getenv("MCP_MANIFEST"))
	if manifest == "" {
		manifest = "mcp.toml"
	}
	goos, goarch := "linux", "amd64"
	args := os.Args[2:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--manifest":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "tools-plan: --manifest needs a path")
				return 2
			}
			manifest = args[i]
		case "--os":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "tools-plan: --os needs a value")
				return 2
			}
			goos = args[i]
		case "--arch":
			i++
			if i >= len(args) {
				fmt.Fprintln(os.Stderr, "tools-plan: --arch needs a value")
				return 2
			}
			goarch = args[i]
		case "-h", "--help":
			fmt.Fprint(os.Stderr, `gantry tools-plan — JSON inventory from mcp.toml

Usage:
  gantry tools-plan [--manifest path] [--os linux] [--arch amd64]

Install binaries with: gantry tools-fetch --outdir DIR

Env:
  MCP_MANIFEST   default mcp.toml
`)
			return 0
		default:
			fmt.Fprintf(os.Stderr, "tools-plan: unknown arg %q\n", args[i])
			return 2
		}
	}

	m, err := mcp.LoadManifest(manifest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tools-plan: %v\n", err)
		return 1
	}
	downloads, err := mcp.DownloadPlans(m, goos, goarch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tools-plan: %v\n", err)
		return 1
	}
	out := struct {
		Commands  []string           `json:"commands"`
		Downloads []mcp.DownloadPlan `json:"downloads"`
	}{
		Commands:  mcp.Commands(m),
		Downloads: downloads,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintf(os.Stderr, "tools-plan: encode: %v\n", err)
		return 1
	}
	return 0
}

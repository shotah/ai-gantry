// Package examples embeds operator templates for `gantry init`.
// Keep files here as the source of truth; deploy/ is a thin local-dev mount.
package examples

import "embed"

// FS holds persona templates and the sample MCP manifest.
//
//go:embed persona/*.example.md mcp.toml.example env.example
var FS embed.FS

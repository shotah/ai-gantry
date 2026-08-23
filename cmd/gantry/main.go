// Command gantry is the ai-gantry AI harness binary.
//
//	gantry run         — start the daemon (default)
//	gantry init        — scaffold persona + mcp.toml from examples/
//	gantry auth        — run an MCP server's declared auth flow (mcp.toml)
//	gantry tools-plan  — JSON release/command inventory from mcp.toml
//	gantry tools-fetch — download + install MCP binaries from mcp.toml
//	gantry status      — exit-code healthcheck + JSON doctor (Docker healthcheck)
//	gantry doctor      — alias of status
//	gantry version     — build info
package main

import (
	"fmt"
	"os"
)

// Set via -ldflags at release build time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cmd := "run"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "run":
		os.Exit(run())
	case "init":
		os.Exit(initCmd())
	case "auth":
		os.Exit(authCmd())
	case "tools-plan":
		os.Exit(toolsPlanCmd())
	case "tools-fetch":
		os.Exit(toolsFetchCmd())
	case "status", "doctor":
		os.Exit(status())
	case "version":
		fmt.Printf("gantry %s (commit=%s date=%s)\n", version, commit, date)
	case "help", "-h", "--help":
		printHelp()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		printHelp()
		os.Exit(2)
	}
}

func printHelp() {
	fmt.Fprintf(os.Stderr, `gantry — stupid-simple AI harness (long-horizon personal agent)

Usage:
  gantry [run]        Start the daemon (default)
  gantry init         Scaffold persona + mcp.toml (+ .env.example) from embedded templates
  gantry auth         Run MCP auth flows declared in mcp.toml (gantry auth [server])
  gantry tools-plan   JSON MCP binary inventory from mcp.toml
  gantry tools-fetch  Download + install MCP binaries declared in mcp.toml
  gantry status       Exit 0 if alive (heartbeat). JSON doctor on stdout
  gantry doctor       Alias of status
  gantry version      Print build info
  gantry help         Show this help

init / auth / tools-* env (optional):
  PERSONA_DIR    default deploy/persona
  MCP_MANIFEST   default deploy/mcp.toml (auth/tools-* default: mcp.toml)
`)
}

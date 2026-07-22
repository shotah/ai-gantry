// Command gantry is the ai-gantry agent runtime binary.
//
//	gantry run      — start the daemon (default)
//	gantry init     — scaffold persona + mcp.toml from examples/
//	gantry status   — exit-code healthcheck (Docker healthcheck)
//	gantry version  — build info
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
	case "status":
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
	fmt.Fprintf(os.Stderr, `gantry — stupid-simple agent runtime

Usage:
  gantry [run]     Start the daemon (default)
  gantry init      Scaffold persona + mcp.toml (+ .env.example) from embedded templates
  gantry status    Exit 0 if healthy (Docker healthcheck)
  gantry version   Print build info
  gantry help      Show this help

init env (optional):
  PERSONA_DIR    default deploy/persona
  MCP_MANIFEST   default deploy/mcp.toml
`)
}

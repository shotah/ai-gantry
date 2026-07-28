package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/shotah/ai-gantry/internal/mcp"
)

// authCmd runs the auth subprocess declared in mcp.toml for a server.
//
//	gantry auth                 — list servers with auth_* configured
//	gantry auth <server>        — run that server's auth_command/auth_args
//	gantry auth <server> -- …   — append extra args after auth_args
//
// Env: MCP_MANIFEST (default mcp.toml). Process env is passed through to the
// child (HOME, OAuth client ids, token paths, etc.).
func authCmd() int {
	manifestPath := envOr("MCP_MANIFEST", "mcp.toml")
	m, err := mcp.LoadManifest(manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "auth: %v\n", err)
		return 1
	}

	args := os.Args[2:]
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		return printAuthHelp(m, manifestPath)
	}

	name := strings.TrimSpace(args[0])
	extra := args[1:]
	if len(extra) > 0 && extra[0] == "--" {
		extra = extra[1:]
	}

	spec, ok := mcp.FindServer(m, name)
	if !ok {
		fmt.Fprintf(os.Stderr, "auth: unknown server %q (manifest %s)\n", name, manifestPath)
		listAuthServers(m)
		return 2
	}
	if !spec.AuthConfigured() {
		fmt.Fprintf(os.Stderr, "auth: server %q has no auth_command/auth_args in %s\n", name, manifestPath)
		listAuthServers(m)
		return 2
	}

	cmd, authArgs, _ := spec.AuthCmd()
	fmt.Fprintf(os.Stderr, "auth: %s → %s %s\n", name, cmd, strings.Join(mcp.ExpandAuthArgs(authArgs), " "))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := mcp.RunAuth(ctx, spec, extra); err != nil {
		fmt.Fprintf(os.Stderr, "auth: %v\n", err)
		return 1
	}
	return 0
}

func printAuthHelp(m *mcp.Manifest, manifestPath string) int {
	fmt.Fprintf(os.Stderr, `gantry auth — run an MCP server's declared auth flow

Usage:
  gantry auth                 List servers with auth configured
  gantry auth <server>        Run auth for that server (from mcp.toml)
  gantry auth <server> -- …   Append extra args after auth_args

Manifest: %s  (override with MCP_MANIFEST)

`, manifestPath)
	listAuthServers(m)
	return 0
}

func listAuthServers(m *mcp.Manifest) {
	servers := mcp.AuthServers(m)
	if len(servers) == 0 {
		fmt.Fprintf(os.Stderr, "No servers declare auth_command/auth_args.\n")
		return
	}
	fmt.Fprintf(os.Stderr, "Auth-capable servers:\n")
	for _, s := range servers {
		cmd, args, _ := s.AuthCmd()
		fmt.Fprintf(os.Stderr, "  %-20s %s %s\n", s.Name, cmd, strings.Join(args, " "))
	}
}

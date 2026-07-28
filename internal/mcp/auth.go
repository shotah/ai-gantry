package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ExpandAuthArgs replaces $VAR and ${VAR} in auth args from the process env.
// Unset variables expand to empty strings.
func ExpandAuthArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = os.Expand(a, os.Getenv)
	}
	return out
}

// RunAuth executes the server's declared auth subprocess with stdin/stdout/stderr
// inherited (interactive OAuth / login). extraArgs are appended after auth_args.
// Returns a wrapping error if the process exits non-zero.
func RunAuth(ctx context.Context, spec ServerSpec, extraArgs []string) error {
	command, args, ok := spec.AuthCmd()
	if !ok {
		return fmt.Errorf("mcp: server %q has no auth_command/auth_args", spec.Name)
	}
	args = ExpandAuthArgs(args)
	if len(extraArgs) > 0 {
		args = append(args, extraArgs...)
	}

	cmd := exec.CommandContext(ctx, command, args...) //nolint:gosec // G204: from operator mcp.toml
	cmd.Env = append(os.Environ(), spec.Env...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mcp: auth %q (%s): %w", spec.Name, strings.Join(append([]string{command}, args...), " "), err)
	}
	return nil
}

// AuthServers returns servers that declare an auth flow, in manifest order.
func AuthServers(m *Manifest) []ServerSpec {
	if m == nil {
		return nil
	}
	var out []ServerSpec
	for _, s := range m.Servers {
		if s.AuthConfigured() {
			out = append(out, s)
		}
	}
	return out
}

// FindServer returns the named server spec.
func FindServer(m *Manifest, name string) (ServerSpec, bool) {
	if m == nil {
		return ServerSpec{}, false
	}
	name = strings.TrimSpace(name)
	for _, s := range m.Servers {
		if s.Name == name {
			return s, true
		}
	}
	return ServerSpec{}, false
}

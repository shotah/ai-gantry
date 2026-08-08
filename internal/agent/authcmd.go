package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shotah/ai-gantry/internal/mcp"
)

// AuthGuideURL is linked from /auth replies when the operator needs the full guide.
const AuthGuideURL = "https://github.com/shotah/ai-gantry/blob/main/docs/auth.md"

// parseAuthCommand recognizes /auth with optional server and code/wait args.
// Unlike parseCommand, this allows multi-field messages.
func parseAuthCommand(text string) (server, arg string, ok bool) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "", "", false
	}
	cmd := fields[0]
	if i := strings.Index(cmd, "@"); i >= 0 {
		cmd = cmd[:i]
	}
	if !strings.EqualFold(cmd, "/auth") {
		return "", "", false
	}
	if len(fields) >= 2 {
		server = fields[1]
	}
	if len(fields) >= 3 {
		// OAuth codes can contain characters that split awkwardly; join the rest.
		arg = strings.Join(fields[2:], " ")
	}
	return server, arg, true
}

func (a *Agent) handleAuth(ctx context.Context, server, arg string) (string, error) {
	if strings.TrimSpace(a.mcpManifest) == "" {
		return "auth: MCP manifest not configured", nil
	}
	m, err := mcp.LoadManifest(a.mcpManifest)
	if err != nil {
		return "", fmt.Errorf("auth: %w", err)
	}

	server = strings.TrimSpace(server)
	if server == "" {
		return formatAuthServerList(m), nil
	}

	spec, ok := mcp.FindServer(m, server)
	if !ok {
		return fmt.Sprintf("auth: unknown server %q\n\n%s", server, formatAuthServerList(m)), nil
	}
	if !spec.AuthConfigured() {
		return fmt.Sprintf("auth: server %q has no auth_command/auth_args\n\n%s", server, formatAuthServerList(m)), nil
	}

	extra, err := chatAuthExtra(spec, arg)
	if err != nil {
		return err.Error(), nil
	}

	// Bound chat auth so a stuck child cannot hold the session lock forever.
	authCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	a.log.Info("chat auth", "server", spec.Name, "step", chatAuthStep(extra))
	stdout, stderr, runErr := mcp.RunAuthOutput(authCtx, spec, extra)
	if runErr != nil {
		out := runErr.Error()
		if stdout != "" {
			out = stdout + "\n" + out
		}
		if stderr != "" && !strings.Contains(out, stderr) {
			out = out + "\n" + stderr
		}
		out = strings.TrimSpace(out) + "\nguide: " + AuthGuideURL
		return out, nil
	}
	if stdout == "" {
		stdout = fmt.Sprintf("%s: auth ok", spec.Name)
	}
	if !strings.Contains(stdout, "guide:") {
		stdout = strings.TrimSpace(stdout) + "\nguide: " + AuthGuideURL
	}
	return stdout, nil
}

func formatAuthServerList(m *mcp.Manifest) string {
	servers := mcp.AuthServers(m)
	var b strings.Builder
	b.WriteString("usage: /auth <server>  or  /auth <server> <code>\n")
	b.WriteString("auth-capable servers:\n")
	if len(servers) == 0 {
		b.WriteString("  (none)\n")
	}
	for _, s := range servers {
		note := ""
		switch {
		case isDeviceAuth(s):
			note = "  (device flow: /auth " + s.Name + " then /auth " + s.Name + " wait)"
		case isGarminAuth(s):
			note = "  (MFA: /auth garmin then /auth garmin <code>; needs GARMIN_EMAIL/PASSWORD)"
		}
		fmt.Fprintf(&b, "  %s%s\n", s.Name, note)
	}
	b.WriteString("guide: " + AuthGuideURL)
	return strings.TrimRight(b.String(), "\n")
}

func isGarminAuth(spec mcp.ServerSpec) bool {
	if strings.EqualFold(spec.Name, "garmin") {
		return true
	}
	_, args, ok := spec.AuthCmd()
	if !ok {
		return false
	}
	return len(args) == 1 && args[0] == "login"
}

func isDeviceAuth(spec mcp.ServerSpec) bool {
	if strings.EqualFold(spec.Name, "youtube") {
		return true
	}
	_, args, ok := spec.AuthCmd()
	if !ok {
		return false
	}
	for _, a := range args {
		if a == "oauth" {
			return true
		}
	}
	return false
}

func chatAuthExtra(spec mcp.ServerSpec, arg string) ([]string, error) {
	arg = strings.TrimSpace(arg)
	if isDeviceAuth(spec) {
		switch strings.ToLower(arg) {
		case "", "start":
			return []string{"--start"}, nil
		case "wait":
			return []string{"--wait"}, nil
		default:
			return nil, fmt.Errorf(
				"youtube uses device flow — open the URL, enter the code, then: /auth %s wait\n(not an OAuth paste code)\nguide: %s",
				spec.Name, AuthGuideURL,
			)
		}
	}
	if arg == "" {
		return []string{"url"}, nil
	}
	return []string{"exchange", arg}, nil
}

func chatAuthStep(extra []string) string {
	if len(extra) == 0 {
		return "unknown"
	}
	if extra[0] == "exchange" {
		return "exchange"
	}
	return extra[0]
}

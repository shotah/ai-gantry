package mcp

import (
	"fmt"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Manifest is the on-disk MCP server list (mcp.toml).
type Manifest struct {
	// DynamicTools, when false, publishes the full catalog every turn
	// (small models / rollback). Omitted defaults to true.
	DynamicTools *bool        `toml:"dynamic_tools"`
	Servers      []ServerSpec `toml:"server"`
}

// DynamicToolsOn is the prefix-enable filter. Default true when the key is omitted.
func (m *Manifest) DynamicToolsOn() bool {
	if m == nil || m.DynamicTools == nil {
		return true
	}
	return *m.DynamicTools
}

// ServerSpec describes one stdio MCP server process.
type ServerSpec struct {
	Name        string   `toml:"name"`
	Command     string   `toml:"command"`
	Args        []string `toml:"args"`
	Env         []string `toml:"env"`          // optional KEY=VALUE entries appended to process env
	Tools       []string `toml:"tools"`        // optional allowlist of original tool names
	Exclude     []string `toml:"exclude"`      // optional denylist (shell-style * ? patterns)
	ToolsPrefix string   `toml:"tools_prefix"` // optional prefix override (default: name)
	// Force publishes this server's prefix even when dynamic_tools is on
	// (no idle drop). Small-model furniture; prefer a tight tools allowlist.
	Force bool `toml:"force"`
	// AuthCommand / AuthArgs declare how to (re)authorize this server.
	// Used by `gantry auth <name>`. If AuthArgs is set and AuthCommand is
	// empty, Command is used. Omit both when the server has no auth flow.
	AuthCommand string   `toml:"auth_command"`
	AuthArgs    []string `toml:"auth_args"`

	// DownloadURL is an optional HTTP(S) URL of a binary archive for
	// `gantry tools-fetch` (native deploy + Docker bake). Ignored by the
	// runtime host. Source-agnostic (GitHub, GitLab, S3, …).
	// Placeholders: {os} {arch}; with DownloadTag: {tag} {version}.
	DownloadURL string `toml:"download_url"`
	// DownloadTag pins a release tag once (e.g. "v0.0.2") for {tag}/{version}
	// in DownloadURL. Use "latest" to resolve the current GitHub release at
	// `gantry tools-fetch` / `tools-plan` time (testing convenience).
	DownloadTag string `toml:"download_tag"`
}

// AuthConfigured reports whether this server declares an auth subprocess.
func (s ServerSpec) AuthConfigured() bool {
	return strings.TrimSpace(s.AuthCommand) != "" || len(s.AuthArgs) > 0
}

// AuthCmd returns the executable and args for `gantry auth`.
// AuthCommand defaults to Command when only AuthArgs is set.
func (s ServerSpec) AuthCmd() (command string, args []string, ok bool) {
	if !s.AuthConfigured() {
		return "", nil, false
	}
	command = strings.TrimSpace(s.AuthCommand)
	if command == "" {
		command = strings.TrimSpace(s.Command)
	}
	if command == "" {
		return "", nil, false
	}
	args = append([]string(nil), s.AuthArgs...)
	return command, args, true
}

// LoadManifest reads and validates a TOML MCP manifest.
// A missing file is an error (misconfigured mount). Zero servers is allowed.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("mcp: read manifest %s: %w", path, err)
	}
	var m Manifest
	if err := toml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("mcp: parse manifest %s: %w", path, err)
	}
	seen := make(map[string]struct{}, len(m.Servers))
	for i := range m.Servers {
		s := &m.Servers[i]
		s.Name = strings.TrimSpace(s.Name)
		s.Command = strings.TrimSpace(s.Command)
		if s.Name == "" {
			return nil, fmt.Errorf("mcp: server[%d]: name is required", i)
		}
		if s.Command == "" {
			return nil, fmt.Errorf("mcp: server %q: command is required", s.Name)
		}
		if _, ok := seen[s.Name]; ok {
			return nil, fmt.Errorf("mcp: duplicate server name %q", s.Name)
		}
		seen[s.Name] = struct{}{}
	}
	return &m, nil
}

// ForcePrefixes returns tools_prefix-or-name for servers with force = true.
func (m *Manifest) ForcePrefixes() []string {
	if m == nil {
		return nil
	}
	var out []string
	for _, s := range m.Servers {
		if !s.Force {
			continue
		}
		p := strings.TrimSpace(s.ToolsPrefix)
		if p == "" {
			p = s.Name
		}
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

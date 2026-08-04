package mcp

import (
	"fmt"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Manifest is the on-disk MCP server list (mcp.toml).
type Manifest struct {
	Servers []ServerSpec `toml:"server"`
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

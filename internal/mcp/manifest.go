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
	Name    string   `toml:"name"`
	Command string   `toml:"command"`
	Args    []string `toml:"args"`
	Env     []string `toml:"env"` // optional KEY=VALUE entries appended to process env
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

// Package doctor is the operator-facing report `gantry status` prints.
//
// It is pulled by Docker healthchecks and by gantree via `docker exec`.
// Collect is read-only: heartbeat row, persona files, mcp.toml, and a
// boot snapshot. It never dials MCP, never loads LLM config, and never
// writes. The snapshot is written once after MCP boot (not per turn).
package doctor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/shotah/ai-gantry/internal/heartbeat"
	"github.com/shotah/ai-gantry/internal/mcp"
	"github.com/shotah/ai-gantry/internal/persona"
)

// SnapshotFile is the boot MCP snapshot under DATA_DIR.
const SnapshotFile = "doctor.json"

// Report is the stable JSON stdout of `gantry status`.
// Exit 0 means Alive (heartbeat fresh). OK is operator health and may
// be false while Alive is true — chat-only and all-skipped must not
// restart the container.
//
// Version / Commit are the binary ldflags, filled by `gantry status`
// (Collect is files-only and leaves them empty).
type Report struct {
	Alive   bool         `json:"alive"`
	OK      bool         `json:"ok"`
	Reason  string       `json:"reason,omitempty"`
	Version string       `json:"version,omitempty"`
	Commit  string       `json:"commit,omitempty"`
	Channel string       `json:"channel"`
	Persona PersonaFiles `json:"persona"`
	MCP     MCPReport    `json:"mcp"`
}

// PersonaFiles is presence of the two standing-prompt files.
type PersonaFiles struct {
	Dir       string `json:"dir"`
	PersonaMD bool   `json:"persona_md"`
	SelfMD    bool   `json:"self_md"`
}

// MCPReport is connected vs skipped for the running process (snapshot)
// or file inference when the snapshot is missing.
type MCPReport struct {
	Listed    int         `json:"listed"`
	Connected int         `json:"connected"`
	Skipped   int         `json:"skipped"`
	Servers   []MCPServer `json:"servers"`
}

// MCPServer is one manifest server as the operator (and gantree) should
// see it. State is connected | skipped | unknown.
type MCPServer struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
	Note   string `json:"note,omitempty"`
	Auth   bool   `json:"auth"`
}

const (
	stateConnected = "connected"
	stateSkipped   = "skipped"
	stateUnknown   = "unknown"

	reasonNoHeartbeat = "no_heartbeat"
	reasonAllSkipped  = "mcp_all_skipped"
)

// Paths are the mounts/env `gantry status` reads. Defaults match config.
type Paths struct {
	DataDir    string
	PersonaDir string
	Manifest   string
	Channel    string
}

// PathsFromEnv reads CHANNEL / PERSONA_DIR / DATA_DIR / MCP_MANIFEST
// without config.Load (status must not require LLM keys).
func PathsFromEnv() Paths {
	p := Paths{
		DataDir:    strings.TrimSpace(os.Getenv("DATA_DIR")),
		PersonaDir: strings.TrimSpace(os.Getenv("PERSONA_DIR")),
		Manifest:   strings.TrimSpace(os.Getenv("MCP_MANIFEST")),
		Channel:    strings.ToLower(strings.TrimSpace(os.Getenv("CHANNEL"))),
	}
	if p.DataDir == "" {
		p.DataDir = "/data"
	}
	if p.PersonaDir == "" {
		p.PersonaDir = "/persona"
	}
	if p.Manifest == "" {
		p.Manifest = "/etc/gantry/mcp.toml"
	}
	if p.Channel == "" {
		p.Channel = "telegram"
	}
	return p
}

// Collect builds a report from files + heartbeat. No MCP dial, no writes.
func Collect(p Paths) Report {
	rep := Report{
		Channel: p.Channel,
		Persona: PersonaFiles{
			Dir:       p.PersonaDir,
			PersonaMD: filePresent(filepath.Join(p.PersonaDir, persona.FilePersona)),
			SelfMD:    filePresent(filepath.Join(p.PersonaDir, persona.FileSelf)),
		},
	}
	if err := heartbeat.CheckFile(p.DataDir, heartbeat.DefaultMaxAge); err != nil {
		rep.Alive = false
		rep.Reason = reasonNoHeartbeat
	} else {
		rep.Alive = true
	}

	if snap, ok := readSnapshot(filepath.Join(p.DataDir, SnapshotFile)); ok {
		rep.MCP = snap
	} else {
		rep.MCP = mcpFromManifest(p.Manifest)
	}

	rep.OK = rep.Alive
	if rep.MCP.Listed > 0 && rep.MCP.Connected == 0 {
		rep.OK = false
		if rep.Alive {
			rep.Reason = reasonAllSkipped
		}
	}
	return rep
}

// WriteSnapshot stores boot MCP health under DATA_DIR. Call once after
// mcp.Start, never from the turn path.
func WriteSnapshot(dataDir string, rows []mcp.ServerStatus) error {
	if strings.TrimSpace(dataDir) == "" {
		return nil
	}
	rep := snapshotFromHealth(rows)
	data, err := json.Marshal(rep)
	if err != nil {
		return err
	}
	path := filepath.Join(dataDir, SnapshotFile)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func snapshotFromHealth(rows []mcp.ServerStatus) MCPReport {
	out := MCPReport{Listed: len(rows), Servers: make([]MCPServer, 0, len(rows))}
	for _, r := range rows {
		s := MCPServer{
			Name: r.Name,
			Auth: r.Auth,
			Note: r.Note,
		}
		if r.Reason != "" {
			s.Reason = string(r.Reason)
		}
		if r.State == mcp.ServerSkipped {
			s.State = stateSkipped
			out.Skipped++
		} else {
			s.State = stateConnected
			out.Connected++
		}
		out.Servers = append(out.Servers, s)
	}
	return out
}

func readSnapshot(path string) (MCPReport, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return MCPReport{}, false
	}
	var m MCPReport
	if err := json.Unmarshal(b, &m); err != nil {
		return MCPReport{}, false
	}
	return m, true
}

func mcpFromManifest(path string) MCPReport {
	m, err := mcp.LoadManifest(path)
	if err != nil {
		return MCPReport{}
	}
	out := MCPReport{Listed: len(m.Servers), Servers: make([]MCPServer, 0, len(m.Servers))}
	for _, spec := range m.Servers {
		out.Servers = append(out.Servers, MCPServer{
			Name:  spec.Name,
			State: stateUnknown,
			Auth:  spec.AuthConfigured(),
		})
	}
	return out
}

func filePresent(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

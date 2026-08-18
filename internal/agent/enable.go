package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shotah/ai-gantry/internal/mcpenable"
	"github.com/shotah/ai-gantry/internal/provider"
)

func (a *Agent) publishedTools(ctx context.Context, sessionID string) []provider.ToolDef {
	if a.tools == nil {
		return nil
	}
	all := a.tools.Tools()
	if a.enable == nil {
		return all
	}
	now := time.Now()
	rows, err := a.enable.List(ctx, sessionID, now)
	if err != nil {
		a.log.Warn("mcp enable list failed; publishing builtins only", "err", err)
		return mcpenable.Publish(all, nil, a.enableForce)
	}
	return mcpenable.Publish(all, rows, a.enableForce)
}

func (a *Agent) enableIndexBlock(ctx context.Context, sessionID string) string {
	if a.enable == nil || a.tools == nil {
		return ""
	}
	now := time.Now()
	rows, err := a.enable.List(ctx, sessionID, now)
	if err != nil {
		return ""
	}
	return mcpenable.FormatIndex(rows, mcpenable.Index(a.tools.Tools()), a.enableForce)
}

func (a *Agent) guardEnable(ctx context.Context, name string) error {
	if a.enable == nil || mcpenable.AlwaysOn(name) || name == mcpenable.ToolName {
		return nil
	}
	sessionID := mcpenable.SessionID(ctx)
	if sessionID == "" {
		return nil
	}
	now := time.Now()
	rows, err := a.enable.List(ctx, sessionID, now)
	if err != nil {
		return err
	}
	var keys []string
	for _, r := range rows {
		keys = append(keys, r.Prefix)
	}
	if mcpenable.Allowed(name, keys, a.enableForce) {
		return nil
	}
	return fmt.Errorf("%s", mcpenable.EnableHint(name, a.catalogIndex()))
}

func (a *Agent) touchEnable(ctx context.Context, name string) {
	if a.enable == nil || mcpenable.AlwaysOn(name) {
		return
	}
	sessionID := mcpenable.SessionID(ctx)
	if sessionID == "" {
		return
	}
	if err := a.enable.Touch(ctx, sessionID, name, time.Now()); err != nil {
		a.log.Debug("mcp enable touch skipped", "err", err)
	}
}

func (a *Agent) catalogIndex() []string {
	if a.tools == nil {
		return nil
	}
	return mcpenable.Index(a.tools.Tools())
}

func parseEnableHoldCommand(text string) (cmd, prefix string, ok bool) {
	fields := strings.Fields(text)
	if len(fields) < 2 {
		return "", "", false
	}
	raw := fields[0]
	if i := strings.Index(raw, "@"); i >= 0 {
		raw = raw[:i]
	}
	switch strings.ToLower(raw) {
	case "/short", "/brief", "/off":
	default:
		return "", "", false
	}
	return strings.ToLower(raw), fields[1], true
}

func (a *Agent) handleEnableHold(ctx context.Context, sessionID, cmd, prefix string) (string, error) {
	if a.enable == nil {
		return "dynamic tools are off (mcp.toml dynamic_tools = false) — full catalog is published", nil
	}
	prefix = strings.TrimSpace(prefix)
	now := time.Now()
	index := a.catalogIndex()
	switch cmd {
	case "/off":
		if err := a.enable.Off(ctx, sessionID, prefix); err != nil {
			return "", err
		}
		return "off " + prefix, nil
	case "/brief", "/short":
		hold := mcpenable.HoldShort
		if cmd == "/brief" {
			hold = mcpenable.HoldBrief
		}
		landed, failed, err := a.enable.Enable(ctx, sessionID, []string{prefix}, hold, mcpenable.SourceHuman, now, index)
		if err != nil {
			return "", err
		}
		var b strings.Builder
		if len(landed) > 0 {
			fmt.Fprintf(&b, "%s %s", strings.TrimPrefix(cmd, "/"), strings.Join(landed, ", "))
		}
		if len(failed) > 0 {
			if b.Len() > 0 {
				b.WriteString(" — ")
			}
			b.WriteString(strings.Join(failed, "; "))
		}
		if b.Len() == 0 {
			return "nothing changed", nil
		}
		return b.String(), nil
	default:
		return "usage: /brief <prefix> | /short <prefix> | /off <prefix>", nil
	}
}

package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/shotah/ai-gantry/internal/cron"
)

const plannerUsage = "usage: /planner (on | off | 07:10)"

// PlannerControl is the optional /planner surface (one daily planning session).
type PlannerControl interface {
	ProactiveEnabled() bool
	DefaultAt() string
	SessionAt(ctx context.Context, sessionID string) (string, error)
	ResolvedAt(ctx context.Context, sessionID string) (string, error)
	SetAt(ctx context.Context, sessionID, at string) error
	EnsureFor(ctx context.Context, delivery cron.Delivery) (cron.Job, bool, error)
}

func parsePlannerCommand(text string) (arg string, ok bool) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "", false
	}
	cmd := fields[0]
	if i := strings.Index(cmd, "@"); i >= 0 {
		cmd = cmd[:i]
	}
	if !strings.EqualFold(cmd, "/planner") {
		return "", false
	}
	if len(fields) >= 2 {
		arg = strings.ToLower(strings.TrimSpace(fields[1]))
	}
	return arg, true
}

func (a *Agent) handlePlanner(ctx context.Context, msg channelDelivery, arg string) (string, error) {
	if a.planner == nil {
		return "planner: not configured (cron/store unavailable)", nil
	}
	switch arg {
	case "":
		return a.plannerStatus(ctx, msg.SessionID)
	case "on", "true":
		if err := a.planner.SetAt(ctx, msg.SessionID, ""); err != nil {
			return "", err
		}
		if _, _, err := a.planner.EnsureFor(ctx, cron.Delivery{
			SessionID: msg.SessionID,
		}); err != nil {
			return "", err
		}
		return fmt.Sprintf("planner on — daily planning session at %s (move with /planner 09:30, off with /planner off)",
			a.planner.DefaultAt()), nil
	case "off", "false":
		if err := a.planner.SetAt(ctx, msg.SessionID, cron.PlannerOff); err != nil {
			return "", err
		}
		if _, _, err := a.planner.EnsureFor(ctx, cron.Delivery{SessionID: msg.SessionID}); err != nil {
			return "", err
		}
		return "planner off — no daily planning session; reminders you scheduled still fire. /planner on to resume", nil
	default:
		at, err := normalizePlannerAtArg(arg)
		if err != nil {
			return plannerUsage, nil
		}
		if err := a.planner.SetAt(ctx, msg.SessionID, at); err != nil {
			return "", err
		}
		if _, _, err := a.planner.EnsureFor(ctx, cron.Delivery{
			SessionID: msg.SessionID,
		}); err != nil {
			return "", err
		}
		return fmt.Sprintf("planner at %s — /planner off to stop, /planner on for the default (%s)", at, a.planner.DefaultAt()), nil
	}
}

func (a *Agent) plannerStatus(ctx context.Context, sessionID string) (string, error) {
	resolved, err := a.planner.ResolvedAt(ctx, sessionID)
	if err != nil {
		return "", err
	}
	pref, err := a.planner.SessionAt(ctx, sessionID)
	if err != nil {
		return "", err
	}
	chat := "default (" + a.planner.DefaultAt() + ")"
	switch {
	case pref == cron.PlannerOff || resolved == cron.PlannerOff:
		chat = "off"
	case pref != "":
		chat = pref
	}
	return fmt.Sprintf("daily planner (/planner)\ndefault: %s\nthis agent: %s\n%s",
		a.planner.DefaultAt(), chat, plannerUsage), nil
}

func normalizePlannerAtArg(arg string) (string, error) {
	hour, minute, err := cron.ParsePlannerAt(arg)
	if err != nil {
		return "", err
	}
	return cron.FormatPlannerAt(hour, minute), nil
}

package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/shotah/ai-gantry/internal/cron"
)

const sparkUsage = "usage: /engagement | /spark  (on | off | 4 | 3-5)"

// SparkControl is the optional /spark surface (looking-after-you wakes).
type SparkControl interface {
	ProactiveEnabled() bool
	DefaultQty() string
	Window() (startHour, endHour int)
	SessionQty(ctx context.Context, sessionID string) (string, error)
	ResolvedQty(ctx context.Context, sessionID string) (string, error)
	SetQty(ctx context.Context, sessionID, qty string) error
	EnsureFor(ctx context.Context, delivery cron.Delivery) (cron.Job, bool, error)
}

func parseSparkCommand(text string) (arg string, ok bool) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "", false
	}
	cmd := fields[0]
	if i := strings.Index(cmd, "@"); i >= 0 {
		cmd = cmd[:i]
	}
	if !strings.EqualFold(cmd, "/spark") && !strings.EqualFold(cmd, "/engagement") {
		return "", false
	}
	if len(fields) >= 2 {
		arg = strings.ToLower(strings.TrimSpace(fields[1]))
	}
	return arg, true
}

func (a *Agent) handleSpark(ctx context.Context, msg channelDelivery, arg string) (string, error) {
	if a.spark == nil {
		return "spark: not configured (cron/store unavailable)", nil
	}
	switch arg {
	case "":
		return a.sparkStatus(ctx, msg.SessionID)
	case "on", "true":
		if err := a.spark.SetQty(ctx, msg.SessionID, ""); err != nil {
			return "", err
		}
		if _, _, err := a.spark.EnsureFor(ctx, cron.Delivery{
			SessionID: msg.SessionID,
			UserID:    msg.UserID,
			ChatID:    msg.ChatID,
			ThreadID:  msg.ThreadID,
		}); err != nil {
			return "", err
		}
		return fmt.Sprintf("spark on — about %s looking-after-you wakes / day (turn down with /engagement 2, off with /engagement off)",
			a.spark.DefaultQty()), nil
	case "off", "false":
		if err := a.spark.SetQty(ctx, msg.SessionID, "0"); err != nil {
			return "", err
		}
		if _, _, err := a.spark.EnsureFor(ctx, cron.Delivery{SessionID: msg.SessionID}); err != nil {
			return "", err
		}
		return "spark off — no looking-after-you wakes; dated reminders you scheduled still fire. /engagement on to resume", nil
	default:
		qty, err := normalizeSparkQtyArg(arg)
		if err != nil {
			return sparkUsage, nil
		}
		if err := a.spark.SetQty(ctx, msg.SessionID, qty); err != nil {
			return "", err
		}
		if _, _, err := a.spark.EnsureFor(ctx, cron.Delivery{
			SessionID: msg.SessionID,
			UserID:    msg.UserID,
			ChatID:    msg.ChatID,
			ThreadID:  msg.ThreadID,
		}); err != nil {
			return "", err
		}
		return fmt.Sprintf("spark %s / day — /engagement off to stop, /engagement on for the default", qty), nil
	}
}

func (a *Agent) sparkStatus(ctx context.Context, sessionID string) (string, error) {
	start, end := a.spark.Window()
	resolved, err := a.spark.ResolvedQty(ctx, sessionID)
	if err != nil {
		return "", err
	}
	pref, err := a.spark.SessionQty(ctx, sessionID)
	if err != nil {
		return "", err
	}
	chat := "default (" + a.spark.DefaultQty() + ")"
	switch {
	case pref == "0":
		chat = "off"
	case pref != "":
		chat = pref + " / day"
	}
	if resolved == "0" {
		chat = "off"
	}
	return fmt.Sprintf("engagement (/spark) — looking-after-you wakes\ndefault: %s / day · window %02d–%02d\nthis chat: %s\n%s",
		a.spark.DefaultQty(), start, end, chat, sparkUsage), nil
}

func normalizeSparkQtyArg(arg string) (string, error) {
	qtyMin, qtyMax, err := cron.ParseSparkQty(arg)
	if err != nil {
		return "", err
	}
	if qtyMin == qtyMax {
		return fmt.Sprintf("%d", qtyMin), nil
	}
	return fmt.Sprintf("%d-%d", qtyMin, qtyMax), nil
}

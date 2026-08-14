package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shotah/ai-gantry/internal/provider"
)

// Tool names exposed to the model.
const (
	ToolSchedule = "cron_schedule"
	ToolList     = "cron_list"
	ToolCancel   = "cron_cancel"
)

// Tools adapts Store into agent tool defs / calls.
type Tools struct {
	Store *Store
	TZ    string // IANA name from CRON_TZ
}

// ToolDefs returns the three builtin cron tool schemas.
func ToolDefs() []provider.ToolDef {
	return []provider.ToolDef{
		{
			Name: ToolSchedule,
			Description: "Schedule a proactive agent turn (reminder, digest, or spark-of-life). " +
				"Fires later, runs tools, and pushes the reply to this chat. " +
				`when: RFC3339, "15:04", "in 30m", or for spark "4-6@06-21". ` +
				`repeat: once|daily|every:1h|spark.`,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"prompt": map[string]any{
						"type": "string",
						"description": "what the agent should do when the job fires. " +
							"For live-data jobs (calendar, mail, fitness, search, sheets), list the tools to call first and say not to invent numbers.",
					},
					"when": map[string]any{
						"type":        "string",
						"description": `e.g. "17:00", "in 2h", RFC3339, or spark "4-6@06-21"`,
					},
					"repeat": map[string]any{
						"type":        "string",
						"description": "once (default), daily, every:30m, or spark",
					},
				},
				"required": []string{"prompt", "when"},
			},
		},
		{
			Name:        ToolList,
			Description: "List scheduled cron jobs for this agent.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        ToolCancel,
			Description: "Cancel (disable) a cron job by id from cron_list.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "integer"},
				},
				"required": []string{"id"},
			},
		},
	}
}

// IsCronTool reports whether name is a builtin cron tool.
func IsCronTool(name string) bool {
	switch name {
	case ToolSchedule, ToolList, ToolCancel:
		return true
	default:
		return false
	}
}

// Call executes a builtin cron tool.
func (t Tools) Call(ctx context.Context, name string, arguments json.RawMessage) (string, error) {
	if t.Store == nil {
		return "", fmt.Errorf("cron: store not configured")
	}
	var args map[string]any
	if len(arguments) > 0 && string(arguments) != "null" {
		if err := json.Unmarshal(arguments, &args); err != nil {
			return "", fmt.Errorf("cron: bad arguments: %w", err)
		}
	}
	if args == nil {
		args = map[string]any{}
	}

	switch name {
	case ToolSchedule:
		prompt, _ := args["prompt"].(string)
		when, _ := args["when"].(string)
		repeat, _ := args["repeat"].(string)
		delivery, ok := DeliveryFrom(ctx)
		if !ok || delivery.SessionID == "" {
			return "", fmt.Errorf("cron: missing delivery context (schedule from an interactive turn)")
		}
		loc, err := loadTZ(t.TZ)
		if err != nil {
			return "", err
		}
		parsed, err := ParseSchedule(when, repeat, loc, time.Now())
		if err != nil {
			return "", err
		}
		// Spark planners must go through EnsureSpark so reboots / re-schedules
		// cannot stack a second daily planner or compound ping jobs.
		var job Job
		if parsed.Kind == KindSpark {
			job, _, err = t.Store.EnsureSpark(ctx, prompt, parsed, delivery)
		} else {
			job, err = t.Store.Schedule(ctx, prompt, parsed, delivery)
		}
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("scheduled id=%d kind=%s next_run=%s tz=%s",
			job.ID, job.Kind, job.NextRunAt.UTC().Format(time.RFC3339), job.Timezone), nil

	case ToolList:
		delivery, ok := DeliveryFrom(ctx)
		sessionID := ""
		if ok {
			sessionID = delivery.SessionID
		}
		jobs, err := t.Store.ListSession(ctx, sessionID, false)
		if err != nil {
			return "", err
		}
		if len(jobs) == 0 {
			return "no active cron jobs", nil
		}
		var b strings.Builder
		for _, j := range jobs {
			_, _ = fmt.Fprintf(&b, "id=%d kind=%s next=%s prompt=%q\n",
				j.ID, j.Kind, j.NextRunAt.UTC().Format(time.RFC3339), truncate(j.Prompt, 80))
		}
		return strings.TrimRight(b.String(), "\n"), nil

	case ToolCancel:
		id, err := asInt64(args["id"])
		if err != nil {
			return "", err
		}
		delivery, ok := DeliveryFrom(ctx)
		if ok && delivery.SessionID != "" {
			job, err := t.Store.Get(ctx, id)
			if err != nil {
				return "", err
			}
			if job.SessionID != delivery.SessionID {
				return "", fmt.Errorf("cron: job %d not in this session", id)
			}
		}
		if err := t.Store.Cancel(ctx, id); err != nil {
			return "", err
		}
		return fmt.Sprintf("cancelled id=%d", id), nil

	default:
		return "", fmt.Errorf("cron: unknown tool %q", name)
	}
}

func asInt64(v any) (int64, error) {
	switch x := v.(type) {
	case float64:
		return int64(x), nil
	case int:
		return int64(x), nil
	case int64:
		return x, nil
	case json.Number:
		return x.Int64()
	case string:
		return strconv.ParseInt(x, 10, 64)
	default:
		return 0, fmt.Errorf("cron: invalid id %v", v)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

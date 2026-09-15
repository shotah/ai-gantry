package cron

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
)

const (
	wakesMax  = 3
	wakesClip = 40
)

// FormatWakes is the per-turn [wakes] line: the next few human-facing jobs
// (once/daily/every) for this session, soonest first, so the model does not
// cron_list just to avoid double-scheduling. Spark, examples, and wait pokes
// are harness-internal and skipped. Empty if none.
func FormatWakes(jobs []Job, now time.Time) string {
	due := make([]Job, 0, len(jobs))
	for _, j := range jobs {
		if !j.Enabled || j.NextRunAt.IsZero() {
			continue
		}
		switch j.Kind {
		case KindOnce, KindDaily, KindEvery:
			due = append(due, j)
		}
	}
	if len(due) == 0 {
		return ""
	}
	sort.SliceStable(due, func(i, k int) bool { return due[i].NextRunAt.Before(due[k].NextRunAt) })
	more := 0
	if len(due) > wakesMax {
		more = len(due) - wakesMax
		due = due[:wakesMax]
	}
	parts := make([]string, 0, len(due))
	for _, j := range due {
		part := channel.WhenShort(j.NextRunAt, now)
		if p := clipWake(j.Prompt); p != "" {
			part += " " + p
		}
		parts = append(parts, part)
	}
	line := "[wakes] " + strings.Join(parts, " · ")
	if more > 0 {
		line += fmt.Sprintf(" (+%d more — cron_list)", more)
	}
	return line
}

func clipWake(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= wakesClip {
		return s
	}
	return strings.TrimRight(string(r[:wakesClip-1]), " ") + "…"
}

package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/shotah/ai-gantry/internal/aims"
)

const aimsUsage = "usage: /aims [area | rubric | block <area> <from> <to>]"

func parseAimsCommand(text string) (args []string, ok bool) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return nil, false
	}
	cmd := fields[0]
	if i := strings.Index(cmd, "@"); i >= 0 {
		cmd = cmd[:i]
	}
	if !strings.EqualFold(cmd, "/aims") {
		return nil, false
	}
	return fields[1:], true
}

func (a *Agent) handleAims(ctx context.Context, args []string) (string, error) {
	if a.aims == nil {
		return "aims: not configured", nil
	}
	if len(args) == 0 {
		return a.aimsBoard(ctx)
	}
	switch strings.ToLower(args[0]) {
	case "rubric":
		return aims.RubricText, nil
	case "block":
		if len(args) != 4 {
			return aimsUsage, nil
		}
		if err := a.aims.SetBlock(ctx, args[1], args[2], args[3]); err != nil {
			return "", err
		}
		return fmt.Sprintf("block %s %s..%s", args[1], args[2], args[3]), nil
	default:
		if len(args) != 1 {
			return aimsUsage, nil
		}
		return a.aimsArea(ctx, args[0])
	}
}

func (a *Agent) aimsBoard(ctx context.Context) (string, error) {
	areas, err := a.aims.Areas(ctx)
	if err != nil {
		return "", err
	}
	if len(areas) == 0 {
		return "no aims — memory_store insight aim/<area> opens one", nil
	}
	now := a.clockNow()
	var b strings.Builder
	for _, area := range areas {
		st, err := a.aims.StatsAt(ctx, area, now)
		if err != nil {
			return "", err
		}
		line := area
		if s := aims.Suffix(st); s != "" {
			line += " — " + s
		} else {
			line += " — no ledger yet"
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if extra := a.aims.BoardTrend(ctx, areas, now); extra != "" {
		b.WriteString(extra)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

func (a *Agent) aimsArea(ctx context.Context, area string) (string, error) {
	areas, err := a.aims.Areas(ctx)
	if err != nil {
		return "", err
	}
	known := false
	for _, live := range areas {
		if live == area {
			known = true
			break
		}
	}
	if !known {
		if len(areas) == 0 {
			return "no aims — memory_store insight aim/<area> opens one", nil
		}
		return fmt.Sprintf("unknown aim %q (live: %s)", area, strings.Join(areas, ", ")), nil
	}
	now := a.clockNow().In(a.loc)
	today := now.Format("2006-01-02")
	from := now.AddDate(0, 0, -13).Format("2006-01-02")
	rows, err := a.aims.History(ctx, area, from, today, 50)
	if err != nil {
		return "", err
	}
	st, err := a.aims.StatsAt(ctx, area, now)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s", area)
	if s := aims.Suffix(st); s != "" {
		fmt.Fprintf(&b, " — %s", s)
	}
	if trend := a.aims.AreaTrend(ctx, area, now); trend != "" {
		b.WriteByte('\n')
		b.WriteString(trend)
	}
	if len(rows) == 0 {
		b.WriteString("\nno ledger rows")
		return b.String(), nil
	}
	for _, ev := range rows {
		b.WriteByte('\n')
		b.WriteString(aims.FormatLine(ev))
	}
	return b.String(), nil
}

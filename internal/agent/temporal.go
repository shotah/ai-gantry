package agent

import (
	"fmt"
	"time"
)

// temporalAnchor returns a fresh per-turn clock note for the model.
// Not persisted in session history — regenerated on every Handle.
// ISO dates for today/tomorrow are spelled out so small local models don't
// fumble date math when building calendar tool arguments.
func temporalAnchor(now time.Time, tzName string) string {
	if tzName == "" {
		tzName = now.Location().String()
	}
	yesterday := now.AddDate(0, 0, -1)
	tomorrow := now.AddDate(0, 0, 1)
	return fmt.Sprintf(
		"[current time] %s, %s %d, %d · %s (%s) · yesterday=%s (%s) · today=%s · tomorrow=%s (%s)",
		now.Weekday().String(),
		now.Month().String(),
		now.Day(),
		now.Year(),
		now.Format("3:04 PM"),
		tzName,
		yesterday.Format("2006-01-02"),
		yesterday.Weekday().String(),
		now.Format("2006-01-02"),
		tomorrow.Format("2006-01-02"),
		tomorrow.Weekday().String(),
	)
}

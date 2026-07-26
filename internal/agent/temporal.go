package agent

import (
	"fmt"
	"time"
)

// temporalAnchor returns a fresh per-turn clock note for the model.
// Not persisted in session history — regenerated on every Handle.
func temporalAnchor(now time.Time, tzName string) string {
	if tzName == "" {
		tzName = now.Location().String()
	}
	return fmt.Sprintf(
		"[current time] %s, %s %d, %d · %s (%s)",
		now.Weekday().String(),
		now.Month().String(),
		now.Day(),
		now.Year(),
		now.Format("3:04 PM"),
		tzName,
	)
}

package agent

import (
	"fmt"
	"time"
)

// temporalAnchor returns a fresh per-turn clock note for the model.
// Not persisted in session history — regenerated on every Handle.
// ISO dates for today/tomorrow and the numeric UTC offset are spelled out so
// small local models don't fumble date math or default tool timestamps to Zulu.
func temporalAnchor(now time.Time, tzName string) string {
	if tzName == "" {
		tzName = now.Location().String()
	}
	yesterday := now.AddDate(0, 0, -1)
	tomorrow := now.AddDate(0, 0, 1)
	// "-07:00" is Go's layout token (like "2006-01-02"), not a fixed Pacific
	// offset — it prints whatever zone `now` is in (from CRON_TZ / Location).
	offset := now.Format("-07:00")

	return fmt.Sprintf(
		"[current time] %s, %s %d, %d · %s (%s, UTC%s) · yesterday=%s (%s) · today=%s · tomorrow=%s (%s). "+
			"Tool date/time args use this timezone unless the tool schema says otherwise — prefer calendar dates or RFC3339 with offset %s; do not default to UTC/Z.",
		now.Weekday().String(),
		now.Month().String(),
		now.Day(),
		now.Year(),
		now.Format("3:04 PM"),
		tzName,
		offset,
		yesterday.Format("2006-01-02"),
		yesterday.Weekday().String(),
		now.Format("2006-01-02"),
		tomorrow.Format("2006-01-02"),
		tomorrow.Weekday().String(),
		offset,
	)
}

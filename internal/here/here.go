// Package here holds the last known location per session.
// In-memory: process restart clears it. Not a Completer wake.
package here

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/shotah/ai-gantry/internal/channel"
)

// Pin is the last known location for a chat session.
type Pin struct {
	Lat       float64
	Lon       float64
	Label     string  // venue title, if any
	AccuracyM float64 // meters; 0 means unknown
	At        time.Time
}

var (
	mu   sync.Mutex
	pins = map[string]Pin{}
)

// Remember stores this-send GPS as the session's last known location.
func Remember(sessionID string, g *channel.Geo, at time.Time) {
	if g == nil || at.IsZero() {
		return
	}
	Set(sessionID, Pin{
		Lat: g.Lat, Lon: g.Lon, Label: g.Label, AccuracyM: g.AccuracyM, At: at,
	})
}

// Set stores the latest pin for sessionID.
func Set(sessionID string, p Pin) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || p.At.IsZero() {
		return
	}
	mu.Lock()
	pins[sessionID] = p
	mu.Unlock()
}

// Get returns the last known pin, if any.
func Get(sessionID string) (Pin, bool) {
	mu.Lock()
	defer mu.Unlock()
	p, ok := pins[strings.TrimSpace(sessionID)]
	return p, ok
}

// Format is the prompt line for the user-turn time footer. Empty if no pin.
func Format(p Pin, now time.Time, tzName string) string {
	if p.At.IsZero() {
		return ""
	}
	at := p.At.In(now.Location())
	tag := "[location]"
	if p.AccuracyM > 0 {
		tag = fmt.Sprintf("[location ±%.0fm]", p.AccuracyM)
	}
	line := fmt.Sprintf("%s %.6f, %.6f at %s %s (%s)",
		tag, p.Lat, p.Lon,
		at.Format("Mon Jan 2, 2006 3:04 PM"),
		formatZone(at, tzName),
		age(now.Sub(p.At)),
	)
	if p.Label != "" {
		line += " — " + p.Label
	}
	return line
}

func formatZone(at time.Time, tzName string) string {
	if tzName == "" {
		tzName = at.Location().String()
	}
	abbr := at.Format("MST")
	if abbr != "" && abbr != tzName {
		return abbr
	}
	return tzName
}

func age(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

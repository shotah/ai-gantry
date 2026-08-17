// Package here holds the last Telegram location pin per session.
// In-memory: process restart clears it. Not a Completer wake.
package here

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Pin is the last shared location for a chat session.
type Pin struct {
	Lat   float64
	Lon   float64
	Label string // venue title, if any
	At    time.Time
}

var (
	mu   sync.Mutex
	pins = map[string]Pin{}
)

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

// Get returns the last pin, if any.
func Get(sessionID string) (Pin, bool) {
	mu.Lock()
	defer mu.Unlock()
	p, ok := pins[strings.TrimSpace(sessionID)]
	return p, ok
}

// Format is the prompt line for the temporal footer. Empty if no pin.
func Format(p Pin, now time.Time, tzName string) string {
	if p.At.IsZero() {
		return ""
	}
	at := p.At.In(now.Location())
	line := fmt.Sprintf("[last pin] %.6f, %.6f at %s %s (%s ago)",
		p.Lat, p.Lon,
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
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

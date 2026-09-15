// Package channel defines the Channel interface for inbound/outbound messaging.
package channel

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Message is an inbound user message from a channel.
type Message struct {
	SessionID string
	UserID    string
	Text      string
	// Images are vision inputs for this turn (data: URLs or https).
	// Not persisted in session history — only the Text (or "[photo]") is stored.
	Images []Image
	// Geo is this-send coordinates (pendant GPS, Telegram location/venue).
	// Not persisted in session history.
	Geo *Geo
	// Surface is the mouth's screen for this send (browser, android,
	// android_auto, ios, carplay). Prompt-only [surface] stamp; not persisted.
	Surface string
	// Input is how the human produced this turn: "spoken" for hold-to-talk,
	// where the mouth reads the reply aloud. Prompt-only [input] stamp; not
	// persisted. Empty means typed.
	Input    string
	ChatID   string
	ThreadID int
}

// Room is the pendant mouth's look as last announced on the crane socket:
// the mailbox tells every socket when the face, wallpaper, or theme changes.
// A zero time means no notice for that piece since boot.
type Room struct {
	Theme      string // catalog id; empty with ThemeAt set means cleared
	ThemeAt    time.Time
	Backdrop   bool // wallpaper set; false with BackdropAt set means cleared
	BackdropAt time.Time
	FaceAt     time.Time
}

// Geo is this-send coordinates from the mouth.
type Geo struct {
	Lat       float64
	Lon       float64
	Label     string  // venue title, if any
	AccuracyM float64 // meters; 0 means unknown
}

// Age is the prompt-side "how long ago" for a harness stamp: just now,
// 5m ago, 3h ago, 12d ago. Negative durations read as just now.
func Age(d time.Duration) string {
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

// WhenShort is a harness clock stamp relative to now: "5:00 PM" today,
// "Wed 8:00 AM" inside six days, else "Sep 22 8:00 AM". Rendered in now's zone.
func WhenShort(at, now time.Time) string {
	at = at.In(now.Location())
	switch {
	case at.Year() == now.Year() && at.YearDay() == now.YearDay():
		return at.Format("3:04 PM")
	case at.Sub(now) < 6*24*time.Hour && now.Sub(at) < 6*24*time.Hour:
		return at.Format("Mon 3:04 PM")
	default:
		return at.Format("Jan 2 3:04 PM")
	}
}

// Footer is the prompt line for this turn's time footer. Empty if g is nil.
func (g *Geo) Footer() string {
	if g == nil {
		return ""
	}
	tag := "[location]"
	if g.AccuracyM > 0 {
		tag = fmt.Sprintf("[location ±%.0fm]", g.AccuracyM)
	}
	line := fmt.Sprintf("%s %.6f, %.6f", tag, g.Lat, g.Lon)
	if l := strings.TrimSpace(g.Label); l != "" {
		line += " — " + l
	}
	return line
}

// Image is one picture attached to an inbound message.
type Image struct {
	URL string `json:"url"` // https://… or data:image/…;base64,…
}

// Outbound is a proactive push (cron/watch). Mouths deliver to the CHANNEL
// allowlist; SessionID/UserID/ChatID/ThreadID are unused for routing.
type Outbound struct {
	SessionID string
	UserID    string
	ChatID    string
	ThreadID  int
	Text      string
	// PhotoURL, when set, is sent via SendPhoto (Telegram) in addition to Text.
	PhotoURL string
	// Photos are extra SendPhoto URLs (https or data:image/…). MCP image
	// tools fill these via PhotoSink; they are not stored in session history.
	Photos []string
	// ID is an optional mailbox frame id so crane logs, the Worker queue, and
	// the phone bubble share one token. Pendant generates one when empty.
	ID string
}

// AgentSession is the one conversation for this process. CHANNEL picks the
// mouth; cron, memory, and history do not store a chat destination.
const AgentSession = "gantry"

// Handler processes one inbound message and returns reply text.
type Handler func(ctx context.Context, msg Message) (reply string, err error)

// Channel delivers messages until the context is cancelled or a fatal error.
type Channel interface {
	Run(ctx context.Context, handle Handler) error
}

// Pusher sends proactive messages (scheduled jobs). Optional on a Channel.
type Pusher interface {
	Push(ctx context.Context, msg Outbound) error
}

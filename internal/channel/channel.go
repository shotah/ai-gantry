// Package channel defines the Channel interface for inbound/outbound messaging.
package channel

import (
	"context"
	"fmt"
	"strings"
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
	Geo      *Geo
	ChatID   string
	ThreadID int
}

// Geo is this-send coordinates from the mouth.
type Geo struct {
	Lat       float64
	Lon       float64
	Label     string  // venue title, if any
	AccuracyM float64 // meters; 0 means unknown
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

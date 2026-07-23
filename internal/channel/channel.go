// Package channel defines the Channel interface for inbound/outbound messaging.
package channel

import "context"

// Message is an inbound user message from a channel.
type Message struct {
	SessionID string
	UserID    string
	Text      string
	// Images are vision inputs for this turn (data: URLs or https).
	// Not persisted in session history — only the Text (or "[photo]") is stored.
	Images []Image
	// Optional delivery hints (set by telegram; used when scheduling cron jobs).
	ChatID   string
	ThreadID int
}

// Image is one picture attached to an inbound message.
type Image struct {
	URL string // https://… or data:image/…;base64,…
}

// Outbound is a proactive push (cron) not tied to an inbound update.
type Outbound struct {
	SessionID string
	UserID    string
	ChatID    string
	ThreadID  int
	Text      string
	// PhotoURL, when set, is sent via SendPhoto (Telegram) in addition to Text.
	PhotoURL string
}

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

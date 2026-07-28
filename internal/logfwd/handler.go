// Package logfwd tees slog ERROR/WARN records to an HTML sender (Telegram).
package logfwd

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Sender delivers a pre-formatted HTML alert. Failures must not recurse into
// the teed logger (Handler suppresses forwards while SendHTML runs).
type Sender interface {
	SendHTML(ctx context.Context, html string) error
}

// SenderFunc adapts a function to Sender.
type SenderFunc func(ctx context.Context, html string) error

// SendHTML implements Sender.
func (f SenderFunc) SendHTML(ctx context.Context, html string) error { return f(ctx, html) }

// Options configures the tee handler.
type Options struct {
	// MinLevel is the lowest level forwarded (typically Error or Warn).
	MinLevel slog.Level
	// Window is the dedupe interval for the same message key. Zero → 5m.
	Window time.Duration
	// MaxRunes caps the HTML payload. Zero → defaultMaxRunes.
	MaxRunes int
	// SendTimeout bounds each SendHTML call. Zero → 10s.
	SendTimeout time.Duration
}

// Handler wraps a slog.Handler and asynchronously forwards qualifying records.
type Handler struct {
	next   slog.Handler
	opts   Options
	state  *shared
	attrs  []slog.Attr
	groups []string
}

type shared struct {
	mu         sync.Mutex
	sender     Sender
	forwarding atomic.Bool
	recent     map[string]dedupe
}

type dedupe struct {
	lastSent   time.Time
	suppressed int
}

// New wraps next. Forwarding is a no-op until SetSender is called.
func New(next slog.Handler, opts Options) *Handler {
	if opts.Window <= 0 {
		opts.Window = 5 * time.Minute
	}
	if opts.MaxRunes <= 0 {
		opts.MaxRunes = defaultMaxRunes
	}
	if opts.SendTimeout <= 0 {
		opts.SendTimeout = 10 * time.Second
	}
	return &Handler{
		next: next,
		opts: opts,
		state: &shared{
			recent: make(map[string]dedupe),
		},
	}
}

// SetSender attaches (or clears) the outbound sender. Safe to call after boot.
func (h *Handler) SetSender(s Sender) {
	h.state.mu.Lock()
	h.state.sender = s
	h.state.mu.Unlock()
}

// Enabled reports whether next would handle the level.
func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

// Handle writes to next, then may forward asynchronously.
func (h *Handler) Handle(ctx context.Context, rec slog.Record) error {
	err := h.next.Handle(ctx, rec)
	h.maybeForward(rec)
	return err
}

// WithAttrs returns a child that includes attrs in forwarded payloads.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	child := *h
	child.next = h.next.WithAttrs(attrs)
	child.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &child
}

// WithGroup returns a child that prefixes subsequent attrs with group.
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	child := *h
	child.next = h.next.WithGroup(name)
	child.groups = append(append([]string{}, h.groups...), name)
	return &child
}

func (h *Handler) maybeForward(rec slog.Record) {
	if rec.Level < h.opts.MinLevel {
		return
	}
	if h.state.forwarding.Load() {
		return
	}
	h.state.mu.Lock()
	sender := h.state.sender
	if sender == nil {
		h.state.mu.Unlock()
		return
	}
	key := dedupeKey(rec)
	now := time.Now()
	ent := h.state.recent[key]
	if !ent.lastSent.IsZero() && now.Sub(ent.lastSent) < h.opts.Window {
		ent.suppressed++
		h.state.recent[key] = ent
		h.state.mu.Unlock()
		return
	}
	suppressed := ent.suppressed
	ent.lastSent = now
	ent.suppressed = 0
	h.state.recent[key] = ent
	// prune stale keys occasionally
	if len(h.state.recent) > 256 {
		h.pruneLocked(now)
	}
	attrs := append([]slog.Attr{}, h.attrs...)
	h.state.mu.Unlock()

	html := FormatHTML(rec, attrs, suppressed, h.opts.MaxRunes)
	timeout := h.opts.SendTimeout
	go func() {
		if !h.state.forwarding.CompareAndSwap(false, true) {
			return
		}
		defer h.state.forwarding.Store(false)
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		_ = sender.SendHTML(ctx, html) // drop on failure — never log (loop guard)
	}()
}

func (h *Handler) pruneLocked(now time.Time) {
	for k, ent := range h.state.recent {
		if now.Sub(ent.lastSent) > 2*h.opts.Window {
			delete(h.state.recent, k)
		}
	}
}

func dedupeKey(rec slog.Record) string {
	return rec.Level.String() + "\x00" + strings.TrimSpace(rec.Message)
}

// ParseLevel maps TELEGRAM_ERROR_REPORTING values to a min level.
// enabled=false means reporting is off.
func ParseLevel(s string) (level slog.Level, enabled bool, err error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "off", "false", "0", "no":
		return 0, false, nil
	case "error":
		return slog.LevelError, true, nil
	case "warn", "warning":
		return slog.LevelWarn, true, nil
	default:
		return 0, false, errInvalidReporting(s)
	}
}

type reportingError string

func (e reportingError) Error() string { return string(e) }

func errInvalidReporting(s string) error {
	return reportingError("TELEGRAM_ERROR_REPORTING: must be off|error|warn, got \"" + s + "\"")
}

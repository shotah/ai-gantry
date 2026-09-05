// Package pendant is an outbound WebSocket mouth to the gantry-pendant Worker.
//
// The crane dials the Durable Object mailbox. No inbound port. Allowlist is
// Google sub. GPS on the frame updates here.Set — it is never stuffed into Text.
package pendant

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/shotah/ai-gantry/internal/channel"
)

// Config configures the pendant channel.
type Config struct {
	MailboxURL   string
	Bearer       string
	AllowedUsers []string
	Logger       *slog.Logger
}

type conn interface {
	ReadMessage() (int, []byte, error)
	WriteMessage(int, []byte) error
	Close() error
}

type dialFunc func(ctx context.Context, mailbox string, header http.Header) (conn, error)

// Channel dials the Worker mailbox and fans inbound frames into a Handler.
type Channel struct {
	mailbox string
	bearer  string
	slug    string
	allowed map[string]struct{}
	log     *slog.Logger
	dial    dialFunc

	mu      sync.Mutex
	writeMu sync.Mutex
	live    conn
}

// New requires mailbox URL, bearer, and a non-empty Google-sub allowlist.
func New(cfg Config) (*Channel, error) {
	mailbox := strings.TrimSpace(cfg.MailboxURL)
	bearer := strings.TrimSpace(cfg.Bearer)
	if mailbox == "" {
		return nil, fmt.Errorf("pendant: mailbox URL is required (PENDANT_MAILBOX_URL)")
	}
	if bearer == "" {
		return nil, fmt.Errorf("pendant: bearer is required (PENDANT_BEARER)")
	}
	ws, slug, err := normalizeMailbox(mailbox)
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]struct{})
	for _, id := range cfg.AllowedUsers {
		id = normalizeSub(id)
		if id == "" {
			continue
		}
		allowed[id] = struct{}{}
	}
	if len(allowed) == 0 {
		return nil, fmt.Errorf("pendant: allowlist is empty (set PENDANT_ALLOWED_USERS)")
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	ch := &Channel{
		mailbox: ws,
		bearer:  bearer,
		slug:    slug,
		allowed: allowed,
		log:     log,
		dial:    defaultDial,
	}
	return ch, nil
}

// MailboxSlug is the crane id in PENDANT_MAILBOX_URL (/ws/<slug>).
func MailboxSlug(raw string) string {
	_, slug, err := normalizeMailbox(raw)
	if err != nil || slug == "" {
		return "crane"
	}
	return slug
}

func normalizeMailbox(raw string) (string, string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", "", fmt.Errorf("pendant: PENDANT_MAILBOX_URL is not a valid URL")
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	case "wss", "ws":
	default:
		return "", "", fmt.Errorf("pendant: PENDANT_MAILBOX_URL must be ws(s) or http(s)")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	slug := ""
	if len(parts) >= 2 && parts[0] == "ws" {
		slug = parts[1]
	} else if len(parts) == 1 {
		slug = parts[0]
		u.Path = "/ws/" + slug
	}
	if slug == "" {
		return "", "", fmt.Errorf("pendant: PENDANT_MAILBOX_URL must include /ws/<slug>")
	}
	q := u.Query()
	if q.Get("role") == "" {
		q.Set("role", "crane")
	}
	u.RawQuery = q.Encode()
	return u.String(), slug, nil
}

func defaultDial(ctx context.Context, mailbox string, header http.Header) (conn, error) {
	d := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	c, _, err := d.DialContext(ctx, mailbox, header)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Channel) isAllowed(sub string) bool {
	_, ok := c.allowed[strings.TrimSpace(sub)]
	return ok
}

// Run dials the mailbox until ctx is cancelled.
func (c *Channel) Run(ctx context.Context, handle channel.Handler) error {
	backoff := time.Second
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		err := c.serve(ctx, handle)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil && err != io.EOF {
			c.log.Warn("pendant mailbox disconnected", "err", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (c *Channel) serve(ctx context.Context, handle channel.Handler) error {
	header := http.Header{}
	header.Set("Authorization", "Bearer "+c.bearer)
	cn, err := c.dial(ctx, c.mailbox, header)
	if err != nil {
		return err
	}
	c.setLive(cn)
	defer func() {
		c.setLive(nil)
		_ = cn.Close()
	}()
	if err := c.writeOn(cn, cmdsFrame()); err != nil {
		return err
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		_, raw, err := cn.ReadMessage()
		if err != nil {
			return err
		}
		if err := c.dispatch(ctx, cn, raw, handle); err != nil {
			return err
		}
	}
}

func (c *Channel) dispatch(ctx context.Context, cn conn, raw []byte, handle channel.Handler) error {
	var frame inboundFrame
	if err := json.Unmarshal(raw, &frame); err != nil {
		c.log.Warn("pendant bad frame")
		return nil
	}
	if frame.Kind == "ack" || frame.Kind == "error" || frame.Kind == "reply" || frame.Kind == "push" || frame.Kind == "cmds" {
		return nil
	}
	sub := strings.TrimSpace(frame.UserID)
	if !c.isAllowed(sub) {
		c.log.Info("pendant ignore (not allowlisted)")
		return nil
	}
	sid := sessionID(c.slug, sub)
	now := time.Now()
	applyGeo(sid, frame.Context, now)
	if silentPin(frame.Text, frame.Images, frame.Context) {
		c.log.Info("pendant last pin updated", "session_id", sid)
		return nil
	}
	text := strings.TrimSpace(frame.Text)
	if text == "" && len(frame.Images) > 0 {
		text = "[photo]"
	}
	if text == "" && len(frame.Images) == 0 {
		return nil
	}
	reply, err := handle(ctx, channel.Message{
		SessionID: sid,
		UserID:    sub,
		Text:      text,
		Images:    frame.Images,
		ChatID:    sub,
	})
	if err != nil {
		c.log.Error("pendant handle", "err", err)
		return nil
	}
	if strings.TrimSpace(reply) == "" {
		return nil
	}
	return c.writeOn(cn, outboundFrame{Text: reply, Kind: "reply"})
}

// Push sends a cron/spark outbound on the live socket, or opens a short dial.
func (c *Channel) Push(ctx context.Context, msg channel.Outbound) error {
	sub := strings.TrimSpace(msg.UserID)
	if sub == "" {
		sub = strings.TrimSpace(msg.ChatID)
	}
	if !c.isAllowed(sub) {
		return fmt.Errorf("pendant: push user is not allowlisted")
	}
	body := outboundFrame{Text: msg.Text, Kind: "push"}
	if live := c.getLive(); live != nil {
		return c.writeOn(live, body)
	}
	header := http.Header{}
	header.Set("Authorization", "Bearer "+c.bearer)
	cn, err := c.dial(ctx, c.mailbox, header)
	if err != nil {
		return err
	}
	defer func() { _ = cn.Close() }()
	return writeFrame(cn, body)
}

func (c *Channel) writeOn(cn conn, frame outboundFrame) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return writeFrame(cn, frame)
}

func writeFrame(cn conn, frame outboundFrame) error {
	raw, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	return cn.WriteMessage(websocket.TextMessage, raw)
}

func (c *Channel) setLive(cn conn) {
	c.mu.Lock()
	c.live = cn
	c.mu.Unlock()
}

func (c *Channel) getLive() conn {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.live
}

// Package pendant is an outbound WebSocket mouth to the gantry-pendant Worker.
//
// The crane dials the Durable Object mailbox. No inbound port. Allowlist is
// Google sub and/or verified email. GPS on the frame updates here.Set — it is
// never stuffed into Text.
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

const typingInterval = 4 * time.Second

// typingEvery is the chat-action refresh. Tests may shorten it.
var typingEvery = typingInterval

// Config configures the pendant channel.
type Config struct {
	MailboxURL    string
	Bearer        string
	AllowedUsers  []string
	Logger        *slog.Logger
	StreamReplies bool // draft bubble: spinup, tool trace, then reply
}

type conn interface {
	ReadMessage() (int, []byte, error)
	WriteMessage(int, []byte) error
	Close() error
}

type dialFunc func(ctx context.Context, mailbox string, header http.Header) (conn, error)

// Channel dials the Worker mailbox and fans inbound frames into a Handler.
type Channel struct {
	mailbox       string
	bearer        string
	slug          string
	entries       []Entry
	allowed       map[string]struct{}
	log           *slog.Logger
	dial          dialFunc
	streamReplies bool
	onAdmit       func(ctx context.Context, sessionID, userID string)

	mu      sync.Mutex
	writeMu sync.Mutex
	live    conn
}

// New requires mailbox URL, bearer, and a non-empty allowlist.
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
	entries, err := ParseAllowlist(cfg.AllowedUsers)
	if err != nil {
		return nil, err
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	ch := &Channel{
		mailbox:       ws,
		bearer:        bearer,
		slug:          slug,
		entries:       entries,
		allowed:       allowLookup(entries),
		log:           log,
		dial:          defaultDial,
		streamReplies: cfg.StreamReplies,
	}
	return ch, nil
}

// SetOnAdmit runs once per newly seen Google sub (email-only allowlist
// learns the push target so spark/cron can wake that phone).
func (c *Channel) SetOnAdmit(fn func(ctx context.Context, sessionID, userID string)) {
	c.mu.Lock()
	c.onAdmit = fn
	c.mu.Unlock()
}

// editStream carries spinup + tool-trace lines — the agent only emits those
// when the writer advertises the optional interfaces.
var (
	_ channel.ProgressWriter = (*editStream)(nil)
	_ channel.StatusWriter   = (*editStream)(nil)
	_ channel.Discarder      = (*editStream)(nil)
)

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

func (c *Channel) isAllowed(sub, email string) bool {
	sub = strings.TrimSpace(sub)
	email = strings.ToLower(strings.TrimSpace(email))
	c.mu.Lock()
	defer c.mu.Unlock()
	if sub != "" {
		if _, ok := c.allowed[sub]; ok {
			return true
		}
	}
	if email != "" {
		if _, ok := c.allowed[email]; ok {
			return true
		}
	}
	return false
}

// rememberSub records a Google sub as a push target. True when it was new.
func (c *Channel) rememberSub(sub string) bool {
	sub = strings.TrimSpace(sub)
	if sub == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.allowed[sub]; ok {
		return false
	}
	c.allowed[sub] = struct{}{}
	return true
}

// TrustSub records a Google sub as a push target without OnAdmit.
// Boot uses this for user_ids already on enabled cron jobs (email-only
// allowlist would otherwise deny Push after restart).
func (c *Channel) TrustSub(sub string) {
	_ = c.rememberSub(sub)
}

func (c *Channel) noteUser(ctx context.Context, sessionID, sub string) {
	if !c.rememberSub(sub) {
		return
	}
	c.log.Info("pendant learned sub", "session_id", sessionID)
	c.mu.Lock()
	fn := c.onAdmit
	c.mu.Unlock()
	if fn != nil {
		fn(ctx, sessionID, sub)
	}
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
	if err := c.writeOn(cn, allowFrame(c.entries)); err != nil {
		return err
	}
	c.log.Info("pendant allow sent", "n", len(c.entries))
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
	if frame.Kind == "ack" || frame.Kind == "error" || frame.Kind == "reply" || frame.Kind == "push" || frame.Kind == "cmds" || frame.Kind == "allow" || frame.Kind == "typing" || frame.Kind == "draft" {
		return nil
	}
	sub := strings.TrimSpace(frame.UserID)
	email := strings.ToLower(strings.TrimSpace(frame.Email))
	if !c.isAllowed(sub, email) {
		c.log.Info("pendant ignore (not allowlisted)")
		return nil
	}
	if sub == "" {
		c.log.Info("pendant ignore (missing user_id)")
		return nil
	}
	sid := sessionID(c.slug, sub)
	c.noteUser(ctx, sid, sub)
	now := time.Now()
	gotPin := applyGeo(sid, frame.Context, now)
	if silentPin(frame.Text, frame.Images, frame.Context) {
		c.log.Info("pendant last pin updated", "session_id", sid, "silent", true)
		return nil
	}
	if gotPin {
		c.log.Info("pendant last pin updated", "session_id", sid)
	} else {
		c.log.Info("pendant inbound without geo", "session_id", sid)
	}
	text := strings.TrimSpace(frame.Text)
	if text == "" && len(frame.Images) > 0 {
		text = "[photo]"
	}
	if text == "" && len(frame.Images) == 0 {
		return nil
	}
	stopTyping := c.startTyping(ctx, cn, sub)

	var stream *editStream
	handleCtx := ctx
	if c.streamReplies {
		stream = newEditStream(func(frame outboundFrame) error {
			return c.writeOn(cn, frame)
		}, sub)
		handleCtx = channel.WithReplyWriter(ctx, stream)
	}

	reply, err := handle(handleCtx, channel.Message{
		SessionID: sid,
		UserID:    sub,
		Text:      text,
		Images:    frame.Images,
		ChatID:    sub,
	})
	stopTyping()
	if err != nil {
		if stream != nil && stream.Started() {
			_ = stream.Discard(ctx)
		}
		c.log.Error("pendant handle", "err", err)
		return nil
	}
	if strings.TrimSpace(reply) == "" && stream != nil && stream.Started() {
		_ = stream.Discard(ctx)
		return nil
	}
	if stream != nil && stream.Started() {
		return stream.Finish(ctx, reply)
	}
	if strings.TrimSpace(reply) == "" {
		return nil
	}
	return c.writeOn(cn, outboundFrame{Text: reply, Kind: "reply", UserID: sub})
}

// Push sends a cron/spark outbound on the live socket, or opens a short dial.
func (c *Channel) Push(ctx context.Context, msg channel.Outbound) error {
	sub := strings.TrimSpace(msg.UserID)
	if sub == "" {
		sub = strings.TrimSpace(msg.ChatID)
	}
	if sub != "" && !c.isAllowed(sub, "") {
		return fmt.Errorf("pendant: push user is not allowlisted")
	}
	body := outboundFrame{Text: msg.Text, Kind: "push", UserID: sub}
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

func (c *Channel) startTyping(ctx context.Context, cn conn, userID string) func() {
	frame := outboundFrame{Kind: "typing", UserID: userID}
	_ = c.writeOn(cn, frame)
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		t := time.NewTicker(typingEvery)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				select {
				case <-done:
					return
				default:
				}
				_ = c.writeOn(cn, frame)
			}
		}
	}()
	return func() {
		close(done)
		<-finished
	}
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

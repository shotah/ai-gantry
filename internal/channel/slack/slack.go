// Package slack implements a Slack Socket Mode channel (outbound WebSocket).
//
// Auth model is allowlist-only (SLACK_ALLOWED_USERS) — no pairing flow.
// HTTP Events API (inbound Request URL) is intentionally unsupported.
package slack

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	slackapi "github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/shotah/ai-gantry/internal/channel"
)

const chunkPause = 100 * time.Millisecond

// Config configures the Slack channel.
type Config struct {
	BotToken      string // xoxb-…
	AppToken      string // xapp-… (connections:write)
	AllowedUsers  []string
	Logger        *slog.Logger
	StreamReplies bool // placeholder + chat.update while the model streams
}

// poster is the Slack API surface we use (narrow for tests).
type poster interface {
	PostMessageContext(ctx context.Context, channelID string, options ...slackapi.MsgOption) (string, string, error)
	UpdateMessageContext(ctx context.Context, channelID, timestamp string, options ...slackapi.MsgOption) (string, string, string, error)
	OpenConversation(params *slackapi.OpenConversationParameters) (*slackapi.Channel, bool, bool, error)
	AuthTest() (*slackapi.AuthTestResponse, error)
	GetFileContext(ctx context.Context, downloadURL string, writer io.Writer) error
	UploadFileContext(ctx context.Context, params slackapi.UploadFileParameters) (*slackapi.FileSummary, error)
}

// socketRunner runs Socket Mode until ctx cancels (overridable in tests).
type socketRunner func(ctx context.Context, c *Channel, handle channel.Handler) error

// Channel connects via Socket Mode and fans events into a Handler.
type Channel struct {
	botToken      string
	appToken      string
	allowed       map[string]struct{}
	log           *slog.Logger
	chunkMax      int
	streamReplies bool

	newPoster posterFactory
	runSocket socketRunner

	mu      sync.Mutex
	api     poster
	botUser string
}

type posterFactory func(botToken, appToken string) poster

func defaultPoster(botToken, appToken string) poster {
	return slackapi.New(botToken, slackapi.OptionAppLevelToken(appToken))
}

// New builds a Slack channel. Bot token, app token, and allowlist are required.
func New(cfg Config) (*Channel, error) {
	bot := strings.TrimSpace(cfg.BotToken)
	app := strings.TrimSpace(cfg.AppToken)
	if bot == "" {
		return nil, fmt.Errorf("slack: bot token is required (SLACK_BOT_TOKEN)")
	}
	if app == "" {
		return nil, fmt.Errorf("slack: app token is required (SLACK_APP_TOKEN)")
	}
	if !strings.HasPrefix(bot, "xoxb-") {
		return nil, fmt.Errorf("slack: SLACK_BOT_TOKEN must start with xoxb-")
	}
	if !strings.HasPrefix(app, "xapp-") {
		return nil, fmt.Errorf("slack: SLACK_APP_TOKEN must start with xapp-")
	}
	allowed := make(map[string]struct{})
	for _, id := range cfg.AllowedUsers {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		allowed[id] = struct{}{}
	}
	if len(allowed) == 0 {
		return nil, fmt.Errorf("slack: allowlist is empty (set SLACK_ALLOWED_USERS)")
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	ch := &Channel{
		botToken:      bot,
		appToken:      app,
		allowed:       allowed,
		log:           log,
		chunkMax:      slackMaxMessageRunes,
		streamReplies: cfg.StreamReplies,
		newPoster:     defaultPoster,
	}
	ch.runSocket = ch.runSocketMode
	return ch, nil
}

// Run starts Socket Mode until ctx is cancelled.
func (c *Channel) Run(ctx context.Context, handle channel.Handler) error {
	api := c.newPoster(c.botToken, c.appToken)
	auth, err := api.AuthTest()
	if err != nil {
		return fmt.Errorf("slack: auth.test: %w", err)
	}
	c.mu.Lock()
	c.api = api
	c.botUser = auth.UserID
	c.mu.Unlock()
	c.log.Info("slack connected",
		"bot_user", auth.UserID,
		"team", auth.Team,
		"allowlist_users", len(c.allowed),
	)
	return c.runSocket(ctx, c, handle)
}

func (c *Channel) runSocketMode(ctx context.Context, _ *Channel, handle channel.Handler) error {
	api, ok := c.api.(*slackapi.Client)
	if !ok {
		// Tests inject a mock poster — use a real client only for Socket Mode.
		api = slackapi.New(c.botToken, slackapi.OptionAppLevelToken(c.appToken))
	}
	client := socketmode.New(api)

	go c.consumeEvents(ctx, client, handle)

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.RunContext(ctx)
	}()

	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		if err != nil && ctx.Err() == nil {
			return fmt.Errorf("slack: socket mode: %w", err)
		}
		return nil
	}
}

func (c *Channel) consumeEvents(ctx context.Context, client *socketmode.Client, handle channel.Handler) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-client.Events:
			if !ok {
				return
			}
			switch evt.Type {
			case socketmode.EventTypeConnecting:
				c.log.Debug("slack socket connecting")
			case socketmode.EventTypeConnectionError:
				c.log.Warn("slack socket connection error", "data", fmt.Sprintf("%v", evt.Data))
			case socketmode.EventTypeConnected:
				c.log.Info("slack socket connected")
			case socketmode.EventTypeEventsAPI:
				eventsAPI, ok := evt.Data.(slackevents.EventsAPIEvent)
				if !ok {
					continue
				}
				if evt.Request != nil {
					if err := client.Ack(*evt.Request); err != nil {
						c.log.Warn("slack ack failed", "err", err)
					}
				}
				c.handleEventsAPI(ctx, eventsAPI, handle)
			default:
				// Ignore interactive / slash for v1 (text /new works in DMs).
			}
		}
	}
}

func (c *Channel) handleEventsAPI(ctx context.Context, eventsAPI slackevents.EventsAPIEvent, handle channel.Handler) {
	if eventsAPI.Type != slackevents.CallbackEvent {
		return
	}
	switch ev := eventsAPI.InnerEvent.Data.(type) {
	case *slackevents.MessageEvent:
		c.handleMessage(ctx, ev, handle)
	case *slackevents.AppMentionEvent:
		c.handleAppMention(ctx, ev, handle)
	}
}

func (c *Channel) handleMessage(ctx context.Context, ev *slackevents.MessageEvent, handle channel.Handler) {
	if ev == nil {
		return
	}
	// DMs only for plain message events (channel traffic needs @mention).
	if ev.ChannelType != "im" && ev.ChannelType != "mpim" {
		return
	}
	// Allow empty subtype and file shares; ignore bot/system subtypes.
	if ev.SubType != "" && ev.SubType != "file_share" {
		return
	}
	if ev.BotID != "" || ev.User == "" {
		return
	}
	c.mu.Lock()
	botUser := c.botUser
	c.mu.Unlock()
	if botUser != "" && ev.User == botUser {
		return
	}
	c.dispatch(ctx, ev.User, ev.Channel, ev.TimeStamp, ev.ThreadTimeStamp, ev.Text, filesFromMessageEvent(ev), handle)
}

func (c *Channel) handleAppMention(ctx context.Context, ev *slackevents.AppMentionEvent, handle channel.Handler) {
	if ev == nil || ev.User == "" {
		return
	}
	c.mu.Lock()
	botUser := c.botUser
	c.mu.Unlock()
	text := stripMention(ev.Text, botUser)
	c.dispatch(ctx, ev.User, ev.Channel, ev.TimeStamp, ev.ThreadTimeStamp, text, ev.Files, handle)
}

func (c *Channel) dispatch(ctx context.Context, userID, channelID, ts, threadTS, text string, files []slackapi.File, handle channel.Handler) {
	if !c.isAllowed(userID) {
		c.log.Info("slack ignore unauthorized user", "user_id", userID)
		return
	}
	text = strings.TrimSpace(text)

	c.mu.Lock()
	api := c.api
	c.mu.Unlock()
	if api == nil {
		return
	}

	images, err := inboundImages(ctx, api, files)
	if err != nil {
		c.log.Error("slack file download failed", "err", err)
		_ = c.sendReply(ctx, api, channelID, threadParent(channelID, ts, threadTS), "sorry — couldn't download that image", "")
		return
	}
	if text == "" && len(images) == 0 {
		return
	}

	parentTS := threadParent(channelID, ts, threadTS)
	sessionID := sessionKey(channelID, userID, parentTS)

	var stream *editStream
	handleCtx := ctx
	if c.streamReplies {
		stream = newEditStream(api, channelID, parentTS, c.chunkMax)
		handleCtx = channel.WithReplyWriter(ctx, stream)
	}

	reply, err := handle(handleCtx, channel.Message{
		SessionID: sessionID,
		UserID:    userID,
		ChatID:    channelID,
		Text:      text,
		Images:    images,
	})
	if err != nil {
		c.log.Error("slack handler error", "err", err, "session_id", sessionID)
		_ = c.sendReply(ctx, api, channelID, parentTS, "sorry — something went wrong handling that message", "")
		return
	}
	if stream != nil && stream.Started() {
		urls, rest := channel.ExtractImageURLs(reply)
		if err := stream.Finish(ctx, rest); err != nil {
			c.log.Warn("slack stream finish failed; falling back to send", "err", err)
			if reply != "" {
				if err := c.sendReply(ctx, api, channelID, parentTS, reply, ""); err != nil {
					c.log.Error("slack send failed", "err", err, "session_id", sessionID)
				}
			}
			return
		}
		for _, u := range urls {
			if err := sendImage(ctx, api, channelID, parentTS, u); err != nil {
				c.log.Error("slack send image failed", "err", err, "session_id", sessionID)
			}
		}
		return
	}
	if reply == "" {
		return
	}
	if err := c.sendReply(ctx, api, channelID, parentTS, reply, ""); err != nil {
		c.log.Error("slack send failed", "err", err, "session_id", sessionID)
	}
}

func threadParent(channelID, ts, threadTS string) string {
	if threadTS != "" {
		return threadTS
	}
	// Top-level message: replies stay in a new thread rooted at this message
	// for channel mentions; for DMs, leave unthreaded (parentTS empty → no thread).
	if !strings.HasPrefix(channelID, "D") {
		return ts
	}
	return ""
}

func (c *Channel) isAllowed(userID string) bool {
	_, ok := c.allowed[userID]
	return ok
}

func sessionKey(channelID, userID, threadTS string) string {
	if threadTS != "" {
		return fmt.Sprintf("slack:%s:%s:%s", channelID, userID, threadTS)
	}
	return fmt.Sprintf("slack:%s:%s", channelID, userID)
}

func stripMention(text, botUser string) string {
	if botUser == "" {
		return text
	}
	return strings.TrimSpace(strings.ReplaceAll(text, "<@"+botUser+">", ""))
}

// Push sends a proactive message (cron). Allowlist enforced.
func (c *Channel) Push(ctx context.Context, msg channel.Outbound) error {
	if msg.UserID != "" && !c.isAllowed(msg.UserID) {
		return fmt.Errorf("slack: push denied for user %s", msg.UserID)
	}
	c.mu.Lock()
	api := c.api
	c.mu.Unlock()
	if api == nil {
		api = c.newPoster(c.botToken, c.appToken)
	}
	channelID, threadTS, err := resolveDest(api, msg)
	if err != nil {
		return err
	}
	return c.sendReply(ctx, api, channelID, threadTS, msg.Text, msg.PhotoURL)
}

func resolveDest(api poster, msg channel.Outbound) (channelID, threadTS string, err error) {
	if msg.ChatID != "" {
		channelID = msg.ChatID
	}
	// session: slack:<channel>:<user>[:thread_ts]
	parts := strings.Split(msg.SessionID, ":")
	if len(parts) >= 3 && parts[0] == "slack" {
		if channelID == "" {
			channelID = parts[1]
		}
		if len(parts) >= 4 {
			threadTS = parts[3]
		}
	}
	if channelID == "" && msg.UserID != "" {
		ch, _, _, openErr := api.OpenConversation(&slackapi.OpenConversationParameters{
			Users: []string{msg.UserID},
		})
		if openErr != nil {
			return "", "", fmt.Errorf("slack: open dm: %w", openErr)
		}
		channelID = ch.ID
	}
	if channelID == "" {
		return "", "", fmt.Errorf("slack: missing channel/user for push")
	}
	return channelID, threadTS, nil
}
